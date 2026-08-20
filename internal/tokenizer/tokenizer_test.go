package tokenizer

import (
	"testing"
)

func TestHashPrefix_Deterministic(t *testing.T) {
	bh := &BlockHasher{BlockSize: 16}
	msgs := []Message{
		{Role: "user", Content: "hello world"},
		{Role: "assistant", Content: "hi there"},
	}

	h1 := bh.HashPrefix(msgs)
	h2 := bh.HashPrefix(msgs)

	if len(h1) != len(h2) {
		t.Fatalf("hash length mismatch: %d vs %d", len(h1), len(h2))
	}
	for i := range h1 {
		if h1[i] != h2[i] {
			t.Fatalf("hash mismatch at block %d: %x vs %x", i, h1[i], h2[i])
		}
	}
}

func TestHashPrefix_DifferentInputs(t *testing.T) {
	bh := &BlockHasher{BlockSize: 16}

	h1 := bh.HashPrefix([]Message{{Role: "user", Content: "hello world"}})
	h2 := bh.HashPrefix([]Message{{Role: "user", Content: "goodbye world"}})

	if len(h1) == 0 || len(h2) == 0 {
		t.Fatal("expected non-empty hashes")
	}

	// At least one block must differ (content diverges early).
	allSame := true
	minLen := len(h1)
	if len(h2) < minLen {
		minLen = len(h2)
	}
	for i := 0; i < minLen; i++ {
		if h1[i] != h2[i] {
			allSame = false
			break
		}
	}
	if allSame && len(h1) == len(h2) {
		t.Fatal("different inputs produced identical hashes")
	}
}

func TestPrefixMatch_SharedPrefix(t *testing.T) {
	bh := &BlockHasher{BlockSize: 8}

	// Shared prefix: "user:hello " (11 chars = block0 + partial block1 overlap)
	// Then diverge.
	msgs1 := []Message{
		{Role: "user", Content: "hello world, this is a test"},
	}
	msgs2 := []Message{
		{Role: "user", Content: "hello world, this is different"},
	}

	h1 := bh.HashPrefix(msgs1)
	h2 := bh.HashPrefix(msgs2)

	match := PrefixMatch(h1, h2)
	// "user:hello world, this is " = 26 chars → blocks 0,1,2 (24 chars) are identical.
	// Block 3 starts at offset 24: "a te" vs "diff" → diverges.
	if match < 1 {
		t.Fatalf("expected at least 1 matching prefix block, got %d", match)
	}
	if match >= len(h1) || match >= len(h2) {
		t.Fatalf("expected prefix match to be less than full length, got %d (len1=%d, len2=%d)", match, len(h1), len(h2))
	}
}

func TestPrefixMatch_NoMatch(t *testing.T) {
	a := []uint64{1, 2, 3}
	b := []uint64{4, 5, 6}
	if got := PrefixMatch(a, b); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestPrefixMatch_FullMatch(t *testing.T) {
	a := []uint64{10, 20, 30}
	b := []uint64{10, 20, 30}
	if got := PrefixMatch(a, b); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestPrefixMatch_DifferentLengths(t *testing.T) {
	a := []uint64{10, 20, 30, 40}
	b := []uint64{10, 20, 30}
	if got := PrefixMatch(a, b); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestHashPrefix_EmptyInput(t *testing.T) {
	bh := &BlockHasher{BlockSize: 16}
	h := bh.HashPrefix(nil)
	if h != nil {
		t.Fatalf("expected nil for empty input, got %v", h)
	}
}

func TestHashPrefix_BlockBoundaryAlignment(t *testing.T) {
	bh := &BlockHasher{BlockSize: 4}
	// "user:ab" = 7 chars → blocks: "user" (4), ":ab" (3 partial)
	h := bh.HashPrefix([]Message{{Role: "user", Content: "ab"}})
	if len(h) != 2 {
		t.Fatalf("expected 2 blocks for 7-char input with BlockSize=4, got %d", len(h))
	}

	// "user:abcd" = 9 chars → blocks: "user" (4), ":abc" (4), "d" (1 partial)
	h2 := bh.HashPrefix([]Message{{Role: "user", Content: "abcd"}})
	if len(h2) != 3 {
		t.Fatalf("expected 3 blocks for 9-char input with BlockSize=4, got %d", len(h2))
	}

	// Exact boundary: "user:abc" = 8 chars → blocks: "user" (4), ":abc" (4) → exactly 2
	h3 := bh.HashPrefix([]Message{{Role: "user", Content: "abc"}})
	if len(h3) != 2 {
		t.Fatalf("expected 2 blocks for 8-char input with BlockSize=4, got %d", len(h3))
	}
}

func BenchmarkHashPrefix(b *testing.B) {
	bh := &BlockHasher{BlockSize: 64}
	msgs := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Write me a function that computes fibonacci numbers efficiently using memoization in Go."},
		{Role: "assistant", Content: "Here is an efficient implementation using a map for memoization..."},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bh.HashPrefix(msgs)
	}
}
