package scorer

import (
	"testing"
)

func TestPerfectCacheHitBeatsPartialOnLoadedBackend(t *testing.T) {
	s := New(DefaultWeights())

	req := &Request{
		BlockHashes: []uint64{1, 2, 3, 4},
		TotalBlocks: 4,
	}

	// Backend A: perfect cache hit, empty queue, plenty of capacity.
	backendA := BackendState{
		ID:            "A",
		MatchedBlocks: 4,
		QueueDepth:    0,
		MaxQueueDepth: 100,
		UsedBlocks:    10,
		TotalCapacity: 1000,
		Healthy:       true,
	}

	// Backend B: partial cache hit, heavily loaded queue.
	backendB := BackendState{
		ID:            "B",
		MatchedBlocks: 2,
		QueueDepth:    80,
		MaxQueueDepth: 100,
		UsedBlocks:    10,
		TotalCapacity: 1000,
		Healthy:       true,
	}

	scoreA := s.Score(req, &backendA)
	scoreB := s.Score(req, &backendB)

	if scoreA <= scoreB {
		t.Errorf("perfect cache hit (%.4f) should beat partial hit on loaded backend (%.4f)", scoreA, scoreB)
	}
}

func TestUnhealthyBackendsNeverSelected(t *testing.T) {
	s := New(DefaultWeights())

	req := &Request{
		BlockHashes: []uint64{1, 2, 3},
		TotalBlocks: 3,
	}

	backends := []BackendState{
		{
			ID:            "unhealthy-perfect",
			MatchedBlocks: 3,
			QueueDepth:    0,
			MaxQueueDepth: 100,
			UsedBlocks:    0,
			TotalCapacity: 1000,
			Healthy:       false, // down
		},
		{
			ID:            "healthy-partial",
			MatchedBlocks: 1,
			QueueDepth:    50,
			MaxQueueDepth: 100,
			UsedBlocks:    500,
			TotalCapacity: 1000,
			Healthy:       true,
		},
	}

	result := s.Route(req, backends)
	if result == "unhealthy-perfect" {
		t.Error("unhealthy backend should never be selected, even with perfect cache hit")
	}
	if result != "healthy-partial" {
		t.Errorf("expected healthy-partial, got %q", result)
	}
}

func TestLoadBalancingFallbackWhenNoCacheHits(t *testing.T) {
	s := New(DefaultWeights())

	req := &Request{
		BlockHashes: []uint64{1, 2, 3},
		TotalBlocks: 3,
	}

	backends := []BackendState{
		{
			ID:            "loaded",
			MatchedBlocks: 0,
			QueueDepth:    80,
			MaxQueueDepth: 100,
			UsedBlocks:    100,
			TotalCapacity: 1000,
			Healthy:       true,
		},
		{
			ID:            "idle",
			MatchedBlocks: 0,
			QueueDepth:    5,
			MaxQueueDepth: 100,
			UsedBlocks:    100,
			TotalCapacity: 1000,
			Healthy:       true,
		},
		{
			ID:            "medium",
			MatchedBlocks: 0,
			QueueDepth:    40,
			MaxQueueDepth: 100,
			UsedBlocks:    100,
			TotalCapacity: 1000,
			Healthy:       true,
		},
	}

	result := s.Route(req, backends)
	if result != "idle" {
		t.Errorf("expected idle backend for load-balancing fallback, got %q", result)
	}
}

func TestHighEvictionRiskReducesScore(t *testing.T) {
	s := New(DefaultWeights())

	req := &Request{
		BlockHashes: []uint64{1, 2, 3, 4},
		TotalBlocks: 4,
	}

	// Backend with plenty of room.
	spacious := BackendState{
		ID:            "spacious",
		MatchedBlocks: 3,
		QueueDepth:    10,
		MaxQueueDepth: 100,
		UsedBlocks:    100,
		TotalCapacity: 1000,
		Healthy:       true,
	}

	// Backend nearly full — high eviction risk.
	full := BackendState{
		ID:            "full",
		MatchedBlocks: 3,
		QueueDepth:    10,
		MaxQueueDepth: 100,
		UsedBlocks:    950,
		TotalCapacity: 1000,
		Healthy:       true,
	}

	scoreSpacious := s.Score(req, &spacious)
	scoreFull := s.Score(req, &full)

	if scoreFull >= scoreSpacious {
		t.Errorf("full backend (%.4f) should score lower than spacious backend (%.4f) due to eviction risk",
			scoreFull, scoreSpacious)
	}
}

func TestRouteEmptyBackends(t *testing.T) {
	s := New(DefaultWeights())
	req := &Request{BlockHashes: []uint64{1}, TotalBlocks: 1}

	result := s.Route(req, nil)
	if result != "" {
		t.Errorf("expected empty string for no backends, got %q", result)
	}
}

func TestRouteAllUnhealthy(t *testing.T) {
	s := New(DefaultWeights())
	req := &Request{BlockHashes: []uint64{1}, TotalBlocks: 1}

	backends := []BackendState{
		{ID: "a", MatchedBlocks: 1, Healthy: false},
		{ID: "b", MatchedBlocks: 0, Healthy: false},
	}

	result := s.Route(req, backends)
	if result != "" {
		t.Errorf("expected empty string when all backends unhealthy, got %q", result)
	}
}
