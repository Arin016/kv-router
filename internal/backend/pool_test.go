package backend

import (
	"testing"
)

// Regression test for issue #4: the scorer must see each backend's configured
// MaxConcurrent, not a hardcoded 64.
func TestNewPoolExposesConfiguredMaxConcurrent(t *testing.T) {
	pool := NewPool([]BackendConfig{
		{ID: "small", URL: "http://localhost:1", CacheCapacityBlocks: 64, MaxConcurrent: 8},
		{ID: "large", URL: "http://localhost:2", CacheCapacityBlocks: 64, MaxConcurrent: 256},
		{ID: "defaulted", URL: "http://localhost:3", CacheCapacityBlocks: 64, MaxConcurrent: 0},
	})

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
	pool := NewPool([]BackendConfig{
		{ID: "neg", URL: "http://localhost:1", CacheCapacityBlocks: 64, MaxConcurrent: -5},
	})
	if got := pool.Get("neg").MaxConcurrent(); got != DefaultMaxConcurrent {
		t.Fatalf("negative MaxConcurrent = %d, want %d", got, DefaultMaxConcurrent)
	}
}
