package backend

import (
	"strings"
	"testing"
)

// Regression test for issue #4: the scorer must see each backend's configured
// MaxConcurrent, not a hardcoded 64.
func TestNewPoolExposesConfiguredMaxConcurrent(t *testing.T) {
	pool, err := NewPool([]BackendConfig{
		{ID: "small", URL: "http://localhost:1", CacheCapacityBlocks: 64, MaxConcurrent: 8},
		{ID: "large", URL: "http://localhost:2", CacheCapacityBlocks: 64, MaxConcurrent: 256},
		{ID: "defaulted", URL: "http://localhost:3", CacheCapacityBlocks: 64, MaxConcurrent: 0},
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	if got := pool.Get("small").MaxConcurrent(); got != 8 {
		t.Fatalf("small MaxConcurrent = %d, want 8", got)
	}
	if got := pool.Get("large").MaxConcurrent(); got != 256 {
		t.Fatalf("large MaxConcurrent = %d, want 256", got)
	}
	if got := pool.Get("defaulted").MaxConcurrent(); got != DefaultMaxConcurrent {
		t.Fatalf("defaulted MaxConcurrent = %d, want %d", got, DefaultMaxConcurrent)
	}
}

// Negative limits are invalid config and must never produce a zero-limit
// backend: NewPool defaults them defensively for direct callers.
func TestNewPoolDefaultsNegativeMaxConcurrent(t *testing.T) {
	pool, err := NewPool([]BackendConfig{
		{ID: "neg", URL: "http://localhost:1", CacheCapacityBlocks: 64, MaxConcurrent: -5},
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	if got := pool.Get("neg").MaxConcurrent(); got != DefaultMaxConcurrent {
		t.Fatalf("negative MaxConcurrent = %d, want %d", got, DefaultMaxConcurrent)
	}
}

// Issue #6: the backends map is keyed by ID; a duplicate must fail fast
// instead of silently overwriting the first entry.
func TestNewPoolRejectsDuplicateID(t *testing.T) {
	_, err := NewPool([]BackendConfig{
		{ID: "dup", URL: "http://localhost:1", CacheCapacityBlocks: 64},
		{ID: "dup", URL: "http://localhost:2", CacheCapacityBlocks: 64},
	})
	if err == nil {
		t.Fatal("duplicate backend id should be rejected")
	}
	if want := `duplicate backend id "dup"`; !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), want)
	}
}

// Issue #6: invalid upstream URLs must fail at construction, not first
// request, using the ValidURL helper.
func TestNewPoolRejectsInvalidURL(t *testing.T) {
	for _, bad := range []string{"", "localhost:8001", "ftp://x", "not a url", "http://"} {
		_, err := NewPool([]BackendConfig{
			{ID: "bad", URL: bad, CacheCapacityBlocks: 64},
		})
		if err == nil {
			t.Fatalf("URL %q should be rejected", bad)
		}
	}
}
