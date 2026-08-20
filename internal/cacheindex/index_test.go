package cacheindex

import "testing"

func TestCommitTracksPartialPrefixAndBoundsMemory(t *testing.T) {
	idx := New()
	idx.Register("a", 3)
	idx.Commit("a", "chat:model-a", []uint64{1, 2, 3, 4})
	if got := idx.Usage("a").Used; got != 3 {
		t.Fatalf("used = %d, want 3", got)
	}
	// The newest three cumulative prefixes survive, so a request beginning at
	// block 1 cannot be claimed as warm after the capacity eviction.
	if got := idx.Lookup("a", "chat:model-a", []uint64{1, 2}); got != 0 {
		t.Fatalf("evicted prefix matched %d blocks", got)
	}
	idx.Commit("a", "chat:model-b", []uint64{9})
	if got := idx.Lookup("a", "chat:model-b", []uint64{9}); got != 1 {
		t.Fatalf("namespace lookup = %d, want 1", got)
	}
	if got := idx.Lookup("a", "chat:model-a", []uint64{9}); got != 0 {
		t.Fatalf("cross-model lookup = %d, want 0", got)
	}
}
