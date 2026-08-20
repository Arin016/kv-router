// Package cacheindex maintains a bounded, per-backend cache-residency directory.
// It is a prediction layer: an engine adapter may later provide stronger block
// identities, but this directory never keeps an unbounded history or separate
// eviction/index sources of truth.
package cacheindex

import (
	"encoding/binary"
	"hash/fnv"
	"sync"
	"time"
)

type entry struct {
	lastSeen time.Time
	sequence uint64
}
type backendCache struct {
	capacity int
	entries  map[string]map[uint64]*entry
}

type Index struct {
	mu       sync.RWMutex
	backends map[string]*backendCache
	sequence uint64
}

type Stats struct {
	BackendID string `json:"backend_id"`
	Capacity  int    `json:"capacity_blocks"`
	Used      int    `json:"used_blocks"`
}

func New() *Index { return &Index{backends: make(map[string]*backendCache)} }
func (i *Index) Register(backendID string, capacity int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.backends[backendID] = &backendCache{capacity: capacity, entries: make(map[string]map[uint64]*entry)}
}

// Lookup returns the longest known prefix for a namespace on a backend.
func (i *Index) Lookup(backendID, namespace string, blocks []uint64) int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	b := i.backends[backendID]
	if b == nil {
		return 0
	}
	entries := b.entries[namespace]
	matched := 0
	for n, key := range cumulativeKeys(blocks) {
		if _, ok := entries[key]; !ok {
			break
		}
		matched = n + 1
	}
	return matched
}

// Commit records every cumulative prefix once, then evicts the oldest observed
// blocks to maintain the configured bound. Oversize requests retain only their
// newest capacity-sized suffix of prefix keys.
func (i *Index) Commit(backendID, namespace string, blocks []uint64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	b := i.backends[backendID]
	if b == nil || b.capacity <= 0 {
		return
	}
	entries := b.entries[namespace]
	if entries == nil {
		entries = make(map[uint64]*entry)
		b.entries[namespace] = entries
	}
	now := time.Now()
	keys := cumulativeKeys(blocks)
	for _, key := range keys {
		i.sequence++
		if e := entries[key]; e != nil {
			e.lastSeen = now
			e.sequence = i.sequence
		} else {
			entries[key] = &entry{lastSeen: now, sequence: i.sequence}
		}
	}
	for totalEntries(b) > b.capacity {
		i.evictOldest(b)
	}
}

func (i *Index) Usage(backendID string) Stats {
	i.mu.RLock()
	defer i.mu.RUnlock()
	b := i.backends[backendID]
	if b == nil {
		return Stats{BackendID: backendID}
	}
	return Stats{BackendID: backendID, Capacity: b.capacity, Used: totalEntries(b)}
}

func (i *Index) AllUsage() []Stats {
	i.mu.RLock()
	defer i.mu.RUnlock()
	result := make([]Stats, 0, len(i.backends))
	for id, b := range i.backends {
		result = append(result, Stats{BackendID: id, Capacity: b.capacity, Used: totalEntries(b)})
	}
	return result
}

func totalEntries(b *backendCache) int {
	n := 0
	for _, entries := range b.entries {
		n += len(entries)
	}
	return n
}
func (i *Index) evictOldest(b *backendCache) {
	var oldNS string
	var oldKey uint64
	var oldest time.Time
	var sequence uint64
	for ns, entries := range b.entries {
		for key, e := range entries {
			if oldest.IsZero() || e.lastSeen.Before(oldest) || (e.lastSeen.Equal(oldest) && e.sequence < sequence) {
				oldNS, oldKey, oldest, sequence = ns, key, e.lastSeen, e.sequence
			}
		}
	}
	if oldest.IsZero() {
		return
	}
	delete(b.entries[oldNS], oldKey)
	if len(b.entries[oldNS]) == 0 {
		delete(b.entries, oldNS)
	}
}
func cumulativeKeys(blocks []uint64) []uint64 {
	keys := make([]uint64, len(blocks))
	h := fnv.New64a()
	var b [8]byte
	for n, block := range blocks {
		binary.LittleEndian.PutUint64(b[:], block)
		_, _ = h.Write(b[:])
		keys[n] = h.Sum64()
	}
	return keys
}
