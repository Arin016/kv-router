package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Issue #3: 0 must mean the pool default, not unlimited and not an error.
func TestMaxConcurrentZeroMeansDefault(t *testing.T) {
	path := writeConfig(t, `
listen_addr: ":8080"
backends:
  - id: "a"
    url: "http://localhost:8001"
    cache_capacity_blocks: 64
    max_concurrent: 0
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("max_concurrent 0 should load, got error: %v", err)
	}
	if cfg.Backends[0].MaxConcurrent != 0 {
		t.Fatalf("max_concurrent 0 should be preserved for pool defaulting, got %d", cfg.Backends[0].MaxConcurrent)
	}
}

// Issue #3: negative values must fail fast at startup.
func TestMaxConcurrentNegativeRejected(t *testing.T) {
	path := writeConfig(t, `
listen_addr: ":8080"
backends:
  - id: "a"
    url: "http://localhost:8001"
    cache_capacity_blocks: 64
    max_concurrent: -1
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("negative max_concurrent should be rejected at startup")
	}
}
