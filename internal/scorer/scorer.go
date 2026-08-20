package scorer

import "math"

// Request represents an incoming routing request with the block hashes it needs.
type Request struct {
	BlockHashes []uint64
	TotalBlocks int
}

// BackendState represents the current state of a backend node.
type BackendState struct {
	ID            string
	MatchedBlocks int
	QueueDepth    int
	MaxQueueDepth int
	UsedBlocks    int
	TotalCapacity int
	Healthy       bool
}

// Weights holds the tunable scoring weights.
type Weights struct {
	CacheHit      float64
	QueueDepth    float64
	EvictionRisk  float64
}

// DefaultWeights returns sensible defaults.
func DefaultWeights() Weights {
	return Weights{
		CacheHit:     1.0,
		QueueDepth:   0.5,
		EvictionRisk: 0.3,
	}
}

// Scorer computes routing scores for backends based on cache affinity,
// queue pressure, and eviction risk.
type Scorer struct {
	Weights Weights
}

// New creates a Scorer with the given weights.
func New(w Weights) *Scorer {
	return &Scorer{Weights: w}
}

// Score computes a single backend's score for a request.
//
//	score = (matchedBlocks / totalBlocks) * cacheHitWeight
//	      - (queueDepth / maxQueueDepth) * queueDepthWeight
//	      - evictionRisk * evictionRiskWeight
//
// evictionRisk = 1.0 - (blocksRemaining / totalCapacity)
func (s *Scorer) Score(req *Request, backend *BackendState) float64 {
	if !backend.Healthy {
		return math.Inf(-1)
	}

	totalBlocks := float64(req.TotalBlocks)
	if totalBlocks == 0 {
		totalBlocks = float64(len(req.BlockHashes))
	}
	if totalBlocks == 0 {
		totalBlocks = 1 // avoid division by zero
	}

	cacheHitRatio := float64(backend.MatchedBlocks) / totalBlocks

	var queuePressure float64
	if backend.MaxQueueDepth > 0 {
		queuePressure = float64(backend.QueueDepth) / float64(backend.MaxQueueDepth)
	}

	var evictionRisk float64
	if backend.TotalCapacity > 0 {
		blocksRemaining := float64(backend.TotalCapacity - backend.UsedBlocks)
		evictionRisk = 1.0 - (blocksRemaining / float64(backend.TotalCapacity))
	}

	score := cacheHitRatio*s.Weights.CacheHit -
		queuePressure*s.Weights.QueueDepth -
		evictionRisk*s.Weights.EvictionRisk

	return score
}

// Route picks the best backend for a request. Returns the backend ID with the
// highest score. If no backend has any cache match, falls back to the backend
// with the lowest queue depth (pure load balancing).
func (s *Scorer) Route(req *Request, backends []BackendState) string {
	if len(backends) == 0 {
		return ""
	}

	// Check if any healthy backend has a cache match.
	anyCacheHit := false
	for i := range backends {
		if backends[i].Healthy && backends[i].MatchedBlocks > 0 {
			anyCacheHit = true
			break
		}
	}

	// Fallback: no cache hits anywhere — pick lowest queue depth.
	if !anyCacheHit {
		return leastLoaded(backends)
	}

	// Score all healthy backends, pick highest.
	bestID := ""
	bestScore := math.Inf(-1)

	for i := range backends {
		if !backends[i].Healthy {
			continue
		}
		score := s.Score(req, &backends[i])
		if score > bestScore {
			bestScore = score
			bestID = backends[i].ID
		}
	}

	return bestID
}

// leastLoaded returns the ID of the healthy backend with the lowest queue depth.
func leastLoaded(backends []BackendState) string {
	bestID := ""
	bestDepth := math.MaxInt

	for i := range backends {
		if !backends[i].Healthy {
			continue
		}
		if backends[i].QueueDepth < bestDepth {
			bestDepth = backends[i].QueueDepth
			bestID = backends[i].ID
		}
	}

	return bestID
}
