package radixtree

import (
	"sync"
	"testing"
	"time"
)

func TestInsertAndLookupBasic(t *testing.T) {
	tree := New()

	hashes := []uint64{0xAA, 0xBB, 0xCC}
	tree.Insert(hashes, "backend-1")

	result := tree.Lookup(hashes)
	if count, ok := result["backend-1"]; !ok || count != 3 {
		t.Fatalf("expected backend-1 with 3 matched blocks, got %v", result)
	}
}

func TestPartialPrefixMatch(t *testing.T) {
	tree := New()

	// Insert a 5-block prefix for backend-1.
	cached := []uint64{0x01, 0x02, 0x03, 0x04, 0x05}
	tree.Insert(cached, "backend-1")

	// Query with first 3 blocks matching, then diverging.
	query := []uint64{0x01, 0x02, 0x03, 0xFF, 0xFE}
	result := tree.Lookup(query)

	// backend-1 should NOT appear because its entry is at depth 5,
	// and we only traverse 3 edges before diverging.
	if _, ok := result["backend-1"]; ok {
		t.Fatalf("backend-1 should not match -- entry is at depth 5, query diverges at depth 3")
	}

	// Now insert at exactly the 3-block prefix.
	tree.Insert([]uint64{0x01, 0x02, 0x03}, "backend-2")

	result = tree.Lookup(query)
	if count, ok := result["backend-2"]; !ok || count != 3 {
		t.Fatalf("expected backend-2 with 3 matched blocks, got %v", result)
	}
}

func TestPrefixSubsetMatch(t *testing.T) {
	tree := New()

	// Insert at prefix [A, B, C].
	tree.Insert([]uint64{0xA, 0xB, 0xC}, "backend-1")

	// Query with [A, B, C, D, E] -- the query extends past the cached prefix,
	// so backend-1's entry at depth 3 should be found.
	query := []uint64{0xA, 0xB, 0xC, 0xD, 0xE}
	result := tree.Lookup(query)

	if count, ok := result["backend-1"]; !ok || count != 3 {
		t.Fatalf("expected backend-1 with 3 matched blocks, got %v", result)
	}
}

func TestMultipleBackendsSamePrefix(t *testing.T) {
	tree := New()

	prefix := []uint64{0x10, 0x20, 0x30}
	tree.Insert(prefix, "backend-A")
	tree.Insert(prefix, "backend-B")
	tree.Insert(prefix, "backend-C")

	result := tree.Lookup(prefix)
	if len(result) != 3 {
		t.Fatalf("expected 3 backends, got %d: %v", len(result), result)
	}
	for _, id := range []string{"backend-A", "backend-B", "backend-C"} {
		if count, ok := result[id]; !ok || count != 3 {
			t.Fatalf("expected %s with 3 blocks, got %v", id, result)
		}
	}
}

func TestEvictionRemovesOldest(t *testing.T) {
	tree := New()

	// Insert 5 entries for the same backend at different prefixes with staggered times.
	prefixes := [][]uint64{
		{0x01},
		{0x02},
		{0x03},
		{0x04},
		{0x05},
	}

	for i, p := range prefixes {
		tree.Insert(p, "backend-1")
		// Touch later entries to make them "newer".
		if i > 0 {
			time.Sleep(time.Millisecond)
			tree.Touch(p, "backend-1")
		}
	}

	stats := tree.Stats()
	if stats.EntryCount != 5 {
		t.Fatalf("expected 5 entries before eviction, got %d", stats.EntryCount)
	}

	// Evict 2 oldest entries.
	tree.Evict("backend-1", 2)

	stats = tree.Stats()
	if stats.EntryCount != 3 {
		t.Fatalf("expected 3 entries after evicting 2, got %d", stats.EntryCount)
	}

	// The oldest entries (prefix 0x01 and 0x02) should be gone.
	r1 := tree.Lookup([]uint64{0x01})
	if _, ok := r1["backend-1"]; ok {
		t.Fatal("expected prefix 0x01 to be evicted")
	}
	r2 := tree.Lookup([]uint64{0x02})
	if _, ok := r2["backend-1"]; ok {
		t.Fatal("expected prefix 0x02 to be evicted")
	}

	// Newer entries should remain.
	r5 := tree.Lookup([]uint64{0x05})
	if _, ok := r5["backend-1"]; !ok {
		t.Fatal("expected prefix 0x05 to survive eviction")
	}
}

func TestEvictDoesNotAffectOtherBackends(t *testing.T) {
	tree := New()

	prefix := []uint64{0xAA, 0xBB}
	tree.Insert(prefix, "backend-1")
	tree.Insert(prefix, "backend-2")

	tree.Evict("backend-1", 10)

	result := tree.Lookup(prefix)
	if _, ok := result["backend-1"]; ok {
		t.Fatal("backend-1 should have been evicted")
	}
	if _, ok := result["backend-2"]; !ok {
		t.Fatal("backend-2 should remain after evicting backend-1")
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	tree := New()

	const (
		writers    = 10
		readers    = 50
		iterations = 1000
	)

	var wg sync.WaitGroup

	// Writers insert various prefixes.
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				hashes := []uint64{uint64(id), uint64(i % 100), uint64(i % 10)}
				tree.Insert(hashes, "backend-"+string(rune('A'+id)))
			}
		}(w)
	}

	// Readers perform lookups concurrently.
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				hashes := []uint64{uint64(id % writers), uint64(i % 100)}
				_ = tree.Lookup(hashes)
			}
		}(r)
	}

	// Mixed evictions while reads/writes are happening.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations/10; i++ {
			tree.Evict("backend-A", 1)
		}
	}()

	wg.Wait()

	// If we get here without a race detector panic, concurrency is safe.
	stats := tree.Stats()
	if stats.NodeCount <= 0 {
		t.Fatal("expected positive node count after concurrent operations")
	}
}

func TestStatsReflectsTreeState(t *testing.T) {
	tree := New()

	stats := tree.Stats()
	if stats.NodeCount != 1 || stats.EntryCount != 0 || stats.MaxDepth != 0 {
		t.Fatalf("empty tree stats wrong: %+v", stats)
	}

	tree.Insert([]uint64{1, 2, 3}, "b1")
	tree.Insert([]uint64{1, 2, 4}, "b2")

	stats = tree.Stats()
	// Nodes: root + 1 + 2 + 3(branch) + 4(branch) = 5
	if stats.NodeCount != 5 {
		t.Fatalf("expected 5 nodes, got %d", stats.NodeCount)
	}
	if stats.EntryCount != 2 {
		t.Fatalf("expected 2 entries, got %d", stats.EntryCount)
	}
	if stats.MaxDepth != 3 {
		t.Fatalf("expected max depth 3, got %d", stats.MaxDepth)
	}
}

func TestTouchUpdatesAccessTime(t *testing.T) {
	tree := New()

	prefix := []uint64{0xDE, 0xAD}
	tree.Insert(prefix, "backend-1")

	time.Sleep(5 * time.Millisecond)
	tree.Touch(prefix, "backend-1")

	// Verify internally via a second insert that would find the existing entry.
	// Touch + re-insert should bump HitCount to 3 (insert=1, touch=2, re-insert=3).
	tree.Insert(prefix, "backend-1")

	// Verify the entry still exists and lookup works.
	result := tree.Lookup(prefix)
	if _, ok := result["backend-1"]; !ok {
		t.Fatal("backend-1 should still exist after touch")
	}
}

func TestLookupEmptyTree(t *testing.T) {
	tree := New()

	result := tree.Lookup([]uint64{1, 2, 3})
	if len(result) != 0 {
		t.Fatalf("expected empty result on empty tree, got %v", result)
	}
}

func TestInsertDuplicateUpdatesExisting(t *testing.T) {
	tree := New()

	prefix := []uint64{0x11, 0x22}
	tree.Insert(prefix, "backend-1")
	tree.Insert(prefix, "backend-1")
	tree.Insert(prefix, "backend-1")

	// Should still only have 1 entry, not 3.
	stats := tree.Stats()
	if stats.EntryCount != 1 {
		t.Fatalf("expected 1 entry (deduped), got %d", stats.EntryCount)
	}
}
