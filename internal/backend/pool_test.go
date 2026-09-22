package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

// togglingHealthServer serves /health with a switchable status.
func togglingHealthServer(t *testing.T, up *atomic.Bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if up.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Regression test for issue #5: health must use hysteresis — three
// consecutive failures to mark unhealthy, two successes to readmit — so a
// single 429/503 spike cannot flap a backend out of rotation.
func TestHealthFlapDamping(t *testing.T) {
	var up atomic.Bool
	up.Store(true)
	srv := togglingHealthServer(t, &up)

	pool, err := NewPool([]BackendConfig{
		{ID: "a", URL: srv.URL, CacheCapacityBlocks: 64, HealthCheckInterval: time.Hour},
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	b := pool.Get("a")

	// Bootstrap: two immediate successes admit the backend (mirrors
	// StartHealthChecks' cold-start burst).
	pool.probe(b)
	pool.probe(b)
	if !b.IsHealthy() {
		t.Fatal("healthy backend should be admitted by bootstrap probes")
	}
	if got := b.healthyTransitions.Load(); got != 1 {
		t.Fatalf("healthyTransitions = %d, want 1", got)
	}

	// One or two failures must not flip a healthy backend.
	up.Store(false)
	pool.probe(b)
	if !b.IsHealthy() {
		t.Fatal("single failed probe must not mark backend unhealthy")
	}
	pool.probe(b)
	if !b.IsHealthy() {
		t.Fatal("two failed probes must not mark backend unhealthy")
	}
	// Third consecutive failure flips it.
	pool.probe(b)
	if b.IsHealthy() {
		t.Fatal("three consecutive failures should mark backend unhealthy")
	}
	if got := b.unhealthyTransitions.Load(); got != 1 {
		t.Fatalf("unhealthyTransitions = %d, want 1", got)
	}

	// One success must not readmit; two must.
	up.Store(true)
	pool.probe(b)
	if b.IsHealthy() {
		t.Fatal("single successful probe must not readmit backend")
	}
	pool.probe(b)
	if !b.IsHealthy() {
		t.Fatal("two consecutive successes should readmit backend")
	}
	if got := b.healthyTransitions.Load(); got != 2 {
		t.Fatalf("healthyTransitions = %d, want 2", got)
	}

	// Transitions surface in Snapshot telemetry.
	snaps := pool.Snapshots()
	if len(snaps) != 1 || snaps[0].HealthyTransitions != 2 || snaps[0].UnhealthyTransitions != 1 {
		t.Fatalf("snapshot transitions = %+v, want healthy=2 unhealthy=1", snaps[0])
	}
}

// Cold start with a long health interval must still admit a healthy backend
// promptly: only the bootstrap probe burst can run before the first tick.
func TestColdStartAdmitsHealthyBackendImmediately(t *testing.T) {
	var up atomic.Bool
	up.Store(true)
	srv := togglingHealthServer(t, &up)

	pool, err := NewPool([]BackendConfig{
		{ID: "a", URL: srv.URL, CacheCapacityBlocks: 64, HealthCheckInterval: time.Hour},
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pool.StartHealthChecks(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pool.Get("a").IsHealthy() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("healthy backend was not admitted within 2s despite 1h interval (bootstrap burst missing?)")
}
