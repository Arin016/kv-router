package eviction

import (
	"testing"
	"time"
)

func TestBasicInsertAndPredict(t *testing.T) {
	m := NewModel()
	m.Register("backend-1", 10)

	hashes := []uint64{100, 200, 300}
	evicted := m.RecordInsert("backend-1", hashes)

	if len(evicted) != 0 {
		t.Fatalf("expected no evictions, got %d", len(evicted))
	}

	matched, confidence := m.PredictCached("backend-1", hashes)
	if matched != 3 {
		t.Errorf("expected 3 matched blocks, got %d", matched)
	}
	if confidence <= 0.0 || confidence > 1.0 {
		t.Errorf("confidence out of range: %f", confidence)
	}
}

func TestPredictPartialMatch(t *testing.T) {
	m := NewModel()
	m.Register("backend-1", 20)

	m.RecordInsert("backend-1", []uint64{1, 2, 3, 4, 5})

	// Query with partial overlap.
	matched, confidence := m.PredictCached("backend-1", []uint64{3, 4, 5, 6, 7})
	if matched != 3 {
		t.Errorf("expected 3 matched blocks for partial overlap, got %d", matched)
	}
	if confidence <= 0.0 {
		t.Errorf("confidence should be positive for cached content, got %f", confidence)
	}
}

func TestPredictMiss(t *testing.T) {
	m := NewModel()
	m.Register("backend-1", 10)

	m.RecordInsert("backend-1", []uint64{1, 2, 3})

	matched, confidence := m.PredictCached("backend-1", []uint64{99, 100})
	if matched != 0 {
		t.Errorf("expected 0 matched blocks for miss, got %d", matched)
	}
	if confidence != 0.0 {
		t.Errorf("expected 0.0 confidence for miss, got %f", confidence)
	}
}

func TestLRUEvictionWhenCapacityExceeded(t *testing.T) {
	m := NewModel()
	m.Register("backend-1", 5) // capacity: 5 blocks

	// Insert 3 blocks.
	m.RecordInsert("backend-1", []uint64{10, 20, 30})
	// Insert 3 more -- should evict the first entry (3 blocks) to make room.
	evicted := m.RecordInsert("backend-1", []uint64{40, 50, 60})

	if len(evicted) != 1 {
		t.Fatalf("expected 1 eviction batch, got %d", len(evicted))
	}
	if len(evicted[0]) != 3 {
		t.Errorf("expected 3 evicted hashes, got %d", len(evicted[0]))
	}
	if evicted[0][0] != 10 || evicted[0][1] != 20 || evicted[0][2] != 30 {
		t.Errorf("expected evicted hashes [10,20,30], got %v", evicted[0])
	}

	// First entry should no longer be predicted as cached.
	matched, _ := m.PredictCached("backend-1", []uint64{10, 20, 30})
	if matched != 0 {
		t.Errorf("expected evicted prefix to show 0 matched blocks, got %d", matched)
	}

	// Second entry should still be cached.
	matched, _ = m.PredictCached("backend-1", []uint64{40, 50, 60})
	if matched != 3 {
		t.Errorf("expected 3 matched blocks for recent entry, got %d", matched)
	}
}

func TestMultipleEvictions(t *testing.T) {
	m := NewModel()
	m.Register("backend-1", 4)

	m.RecordInsert("backend-1", []uint64{1, 2}) // used: 2
	m.RecordInsert("backend-1", []uint64{3, 4}) // used: 4

	// This needs 3 blocks, only 0 free. Must evict both existing (2+2=4) to fit 3.
	evicted := m.RecordInsert("backend-1", []uint64{5, 6, 7})

	if len(evicted) != 2 {
		t.Fatalf("expected 2 eviction batches, got %d", len(evicted))
	}

	ratio := m.UsageRatio("backend-1")
	if ratio != 0.75 { // 3/4
		t.Errorf("expected usage ratio 0.75, got %f", ratio)
	}
}

func TestAccessUpdatesOrdering(t *testing.T) {
	m := NewModel()
	m.Register("backend-1", 9)

	// Insert A, B, C in order. A is LRU.
	m.RecordInsert("backend-1", []uint64{1, 2, 3})
	m.RecordInsert("backend-1", []uint64{4, 5, 6})
	m.RecordInsert("backend-1", []uint64{7, 8, 9})

	// Access A -- promotes it to MRU.
	m.RecordAccess("backend-1", []uint64{1, 2, 3})

	// Now B is LRU. Insert D (3 blocks) should evict B, not A.
	evicted := m.RecordInsert("backend-1", []uint64{10, 11, 12})

	if len(evicted) != 1 {
		t.Fatalf("expected 1 eviction batch, got %d", len(evicted))
	}
	// B = [4,5,6] should be evicted (was LRU after A was accessed).
	if evicted[0][0] != 4 || evicted[0][1] != 5 || evicted[0][2] != 6 {
		t.Errorf("expected B [4,5,6] evicted, got %v", evicted[0])
	}

	// A should still be predicted as cached.
	matched, _ := m.PredictCached("backend-1", []uint64{1, 2, 3})
	if matched != 3 {
		t.Errorf("expected A still cached (3 blocks), got %d", matched)
	}
}

func TestConfidenceDecayWithPressure(t *testing.T) {
	m := NewModel()
	m.Register("backend-1", 10)

	hashes := []uint64{1, 2, 3}
	m.RecordInsert("backend-1", hashes)

	// At low pressure (3/10 used), confidence should be high.
	_, confLow := m.PredictCached("backend-1", hashes)

	// Fill the cache to near capacity.
	m.RecordInsert("backend-1", []uint64{10, 11, 12})
	m.RecordInsert("backend-1", []uint64{20, 21})

	// 8/10 used now. Confidence for the oldest entry should drop.
	_, confHigh := m.PredictCached("backend-1", hashes)

	if confHigh >= confLow {
		t.Errorf("expected confidence to decrease with pressure: low-pressure=%f, high-pressure=%f",
			confLow, confHigh)
	}
}

func TestMultipleBackendsIndependent(t *testing.T) {
	m := NewModel()
	m.Register("alpha", 5)
	m.Register("beta", 5)

	m.RecordInsert("alpha", []uint64{1, 2, 3})
	m.RecordInsert("beta", []uint64{4, 5, 6})

	// Alpha should not see beta's content.
	matched, _ := m.PredictCached("alpha", []uint64{4, 5, 6})
	if matched != 0 {
		t.Errorf("alpha should not predict beta's content, got %d matched", matched)
	}

	// Beta should not see alpha's content.
	matched, _ = m.PredictCached("beta", []uint64{1, 2, 3})
	if matched != 0 {
		t.Errorf("beta should not predict alpha's content, got %d matched", matched)
	}

	// Evicting on alpha should not affect beta.
	m.RecordInsert("alpha", []uint64{7, 8, 9}) // fills alpha to 6/5 → evicts [1,2,3]

	matched, _ = m.PredictCached("beta", []uint64{4, 5, 6})
	if matched != 3 {
		t.Errorf("beta should still have its content, got %d matched", matched)
	}
}

func TestUsageRatio(t *testing.T) {
	m := NewModel()
	m.Register("backend-1", 10)

	if r := m.UsageRatio("backend-1"); r != 0.0 {
		t.Errorf("expected 0.0 for empty backend, got %f", r)
	}

	m.RecordInsert("backend-1", []uint64{1, 2, 3, 4, 5})
	if r := m.UsageRatio("backend-1"); r != 0.5 {
		t.Errorf("expected 0.5, got %f", r)
	}

	m.RecordInsert("backend-1", []uint64{6, 7, 8, 9, 10})
	if r := m.UsageRatio("backend-1"); r != 1.0 {
		t.Errorf("expected 1.0, got %f", r)
	}
}

func TestUsageRatioUnknownBackend(t *testing.T) {
	m := NewModel()
	if r := m.UsageRatio("nonexistent"); r != 0.0 {
		t.Errorf("expected 0.0 for unknown backend, got %f", r)
	}
}

func TestEvictedSince(t *testing.T) {
	m := NewModel()
	m.Register("backend-1", 3)

	before := time.Now()
	time.Sleep(time.Millisecond) // ensure time separation

	m.RecordInsert("backend-1", []uint64{1, 2, 3}) // fills cache
	m.RecordInsert("backend-1", []uint64{4, 5, 6}) // evicts [1,2,3]

	evicted := m.EvictedSince("backend-1", before)
	if len(evicted) != 1 {
		t.Fatalf("expected 1 evicted prefix since marker, got %d", len(evicted))
	}
	if evicted[0][0] != 1 || evicted[0][1] != 2 || evicted[0][2] != 3 {
		t.Errorf("expected [1,2,3] evicted, got %v", evicted[0])
	}
}

func TestEvictedSinceFiltersOld(t *testing.T) {
	m := NewModel()
	m.Register("backend-1", 3)

	m.RecordInsert("backend-1", []uint64{1, 2, 3})
	m.RecordInsert("backend-1", []uint64{4, 5, 6}) // evicts [1,2,3]

	time.Sleep(time.Millisecond)
	after := time.Now()
	time.Sleep(time.Millisecond)

	m.RecordInsert("backend-1", []uint64{7, 8, 9}) // evicts [4,5,6]

	evicted := m.EvictedSince("backend-1", after)
	if len(evicted) != 1 {
		t.Fatalf("expected 1 evicted prefix after marker, got %d", len(evicted))
	}
	if evicted[0][0] != 4 {
		t.Errorf("expected [4,5,6] as the recent eviction, got %v", evicted[0])
	}
}

func TestRecordInsertUnknownBackend(t *testing.T) {
	m := NewModel()
	evicted := m.RecordInsert("ghost", []uint64{1, 2})
	if evicted != nil {
		t.Errorf("expected nil for unknown backend, got %v", evicted)
	}
}

func TestRecordAccessUnknownBackend(t *testing.T) {
	m := NewModel()
	// Should not panic.
	m.RecordAccess("ghost", []uint64{1, 2})
}

func TestPredictUnknownBackend(t *testing.T) {
	m := NewModel()
	matched, conf := m.PredictCached("ghost", []uint64{1, 2})
	if matched != 0 || conf != 0.0 {
		t.Errorf("expected 0/0.0 for unknown backend, got %d/%f", matched, conf)
	}
}
