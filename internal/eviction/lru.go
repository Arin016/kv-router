// Package eviction models per-backend LRU cache state to predict
// which content blocks are still resident on each backend. The router
// uses these predictions to prefer backends that already hold the
// requested prefix, reducing redundant transfers and cache churn.
package eviction

import (
	"sync"
	"time"
)

// Entry represents a cached prefix on a single backend.
type Entry struct {
	Hashes     []uint64
	InsertedAt time.Time
	LastAccess time.Time
	SizeBlocks int
}

// BackendCache tracks the modeled cache state for one backend.
type BackendCache struct {
	entries  []*Entry // ordered by LastAccess ascending (LRU at index 0)
	capacity int      // total blocks this backend can hold
	used     int      // sum of SizeBlocks across entries
	evicted  []*Entry // entries evicted (retained for EvictedSince queries)
}

// Model is the top-level eviction model tracking all backends.
type Model struct {
	mu       sync.RWMutex
	backends map[string]*BackendCache
}

// NewModel creates an empty eviction model.
func NewModel() *Model {
	return &Model{
		backends: make(map[string]*BackendCache),
	}
}

// Register adds a backend with the given cache capacity (in blocks).
// Calling Register again for the same backend resets its state.
func (m *Model) Register(backendID string, capacity int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backends[backendID] = &BackendCache{
		entries:  make([]*Entry, 0),
		capacity: capacity,
		evicted:  make([]*Entry, 0),
	}
}

// RecordInsert records that the given prefix (identified by content hashes)
// was sent to this backend. Returns the hashes of any entries evicted to
// make room. Each hash slice in the return represents one evicted prefix.
func (m *Model) RecordInsert(backendID string, hashes []uint64) [][]uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	bc, ok := m.backends[backendID]
	if !ok {
		return nil
	}

	now := time.Now()
	entry := &Entry{
		Hashes:     hashes,
		InsertedAt: now,
		LastAccess: now,
		SizeBlocks: len(hashes),
	}

	// Evict LRU entries until we have room.
	var evictedHashes [][]uint64
	for bc.used+entry.SizeBlocks > bc.capacity && len(bc.entries) > 0 {
		victim := bc.entries[0]
		bc.entries = bc.entries[1:]
		bc.used -= victim.SizeBlocks
		victim.LastAccess = now // mark eviction time for EvictedSince
		bc.evicted = append(bc.evicted, victim)
		evictedHashes = append(evictedHashes, victim.Hashes)
	}

	// Insert as MRU (append to end).
	bc.entries = append(bc.entries, entry)
	bc.used += entry.SizeBlocks

	return evictedHashes
}

// RecordAccess marks a cache hit for the given prefix on this backend,
// promoting it to MRU position.
func (m *Model) RecordAccess(backendID string, hashes []uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bc, ok := m.backends[backendID]
	if !ok {
		return
	}

	now := time.Now()
	hashSet := makeHashSet(hashes)

	for i, e := range bc.entries {
		if matchesHashes(e.Hashes, hashSet) {
			e.LastAccess = now
			// Move to MRU position (end of slice).
			bc.entries = append(bc.entries[:i], bc.entries[i+1:]...)
			bc.entries = append(bc.entries, e)
			return
		}
	}
}

// PredictCached estimates how many blocks of the given prefix are still
// cached on this backend, along with a confidence score (0.0–1.0).
// Confidence decreases with cache pressure and entry age.
func (m *Model) PredictCached(backendID string, hashes []uint64) (matchedBlocks int, confidence float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bc, ok := m.backends[backendID]
	if !ok {
		return 0, 0.0
	}

	hashSet := makeHashSet(hashes)
	now := time.Now()

	for _, e := range bc.entries {
		matched := countMatchingHashes(e.Hashes, hashSet)
		if matched > 0 {
			matchedBlocks += matched

			// Confidence factors:
			// 1. Position pressure: entries near LRU end have lower confidence.
			// 2. Age decay: older entries are less likely to still be cached.
			positionConfidence := positionScore(e, bc)
			ageConfidence := ageScore(e.LastAccess, now)
			pressureConfidence := pressureScore(bc)

			// Geometric mean of factors gives a balanced confidence.
			entryConfidence := positionConfidence * ageConfidence * pressureConfidence
			if entryConfidence > confidence {
				confidence = entryConfidence
			}
		}
	}

	// Clamp confidence.
	if confidence > 1.0 {
		confidence = 1.0
	}
	return matchedBlocks, confidence
}

// UsageRatio returns current usage / capacity for the backend.
// Returns 0 if the backend is not registered.
func (m *Model) UsageRatio(backendID string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bc, ok := m.backends[backendID]
	if !ok {
		return 0.0
	}
	if bc.capacity == 0 {
		return 0.0
	}
	return float64(bc.used) / float64(bc.capacity)
}

// EvictedSince returns the hash slices of all prefixes evicted from
// this backend since the given time.
func (m *Model) EvictedSince(backendID string, since time.Time) [][]uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bc, ok := m.backends[backendID]
	if !ok {
		return nil
	}

	var result [][]uint64
	for _, e := range bc.evicted {
		if e.LastAccess.After(since) || e.LastAccess.Equal(since) {
			result = append(result, e.Hashes)
		}
	}
	return result
}

// --- Internal scoring functions ---

// positionScore returns a confidence factor based on where the entry sits
// relative to the LRU eviction frontier. MRU entries score ~1.0.
func positionScore(e *Entry, bc *BackendCache) float64 {
	n := len(bc.entries)
	if n <= 1 {
		return 1.0
	}
	for i, candidate := range bc.entries {
		if candidate == e {
			// i=0 is LRU (lowest confidence), i=n-1 is MRU (highest).
			return 0.5 + 0.5*(float64(i)/float64(n-1))
		}
	}
	return 0.5
}

// ageScore decays confidence as the entry ages. Half-life of 5 minutes.
func ageScore(lastAccess, now time.Time) float64 {
	age := now.Sub(lastAccess)
	if age <= 0 {
		return 1.0
	}
	const halfLife = 5 * time.Minute
	// Exponential decay: 0.5^(age/halfLife)
	decay := 1.0
	ageMinutes := age.Minutes()
	halfLifeMinutes := halfLife.Minutes()
	// Use iterative halving to avoid importing math.
	ratio := ageMinutes / halfLifeMinutes
	decay = expDecay(ratio)
	if decay < 0.1 {
		decay = 0.1 // floor so old entries aren't completely dismissed
	}
	return decay
}

// pressureScore reduces confidence when the cache is nearly full,
// since evictions become more likely.
func pressureScore(bc *BackendCache) float64 {
	if bc.capacity == 0 {
		return 0.0
	}
	ratio := float64(bc.used) / float64(bc.capacity)
	// Linear ramp: at 0% usage confidence=1.0, at 100% usage confidence=0.5.
	return 1.0 - 0.5*ratio
}

// expDecay approximates 0.5^x without importing math.
func expDecay(x float64) float64 {
	if x <= 0 {
		return 1.0
	}
	// ln(0.5) ≈ -0.693. Use Taylor-style approximation via repeated squaring.
	// For our use case, precision beyond 2 decimal places is unnecessary.
	result := 1.0
	base := 0.5
	// Decompose x into integer + fractional.
	intPart := int(x)
	fracPart := x - float64(intPart)
	for i := 0; i < intPart; i++ {
		result *= base
	}
	// Linear interpolation for fractional part: 0.5^frac ≈ 1 - 0.693*frac (first-order).
	result *= (1.0 - 0.693*fracPart)
	if result < 0 {
		result = 0.01
	}
	return result
}

// --- Hash utilities ---

func makeHashSet(hashes []uint64) map[uint64]struct{} {
	s := make(map[uint64]struct{}, len(hashes))
	for _, h := range hashes {
		s[h] = struct{}{}
	}
	return s
}

func matchesHashes(entryHashes []uint64, querySet map[uint64]struct{}) bool {
	for _, h := range entryHashes {
		if _, ok := querySet[h]; ok {
			return true
		}
	}
	return false
}

func countMatchingHashes(entryHashes []uint64, querySet map[uint64]struct{}) int {
	count := 0
	for _, h := range entryHashes {
		if _, ok := querySet[h]; ok {
			count++
		}
	}
	return count
}
