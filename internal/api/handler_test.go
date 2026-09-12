package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arinmallanna/kv-router/internal/backend"
	"github.com/arinmallanna/kv-router/internal/cacheindex"
	"github.com/arinmallanna/kv-router/internal/scorer"
	"github.com/arinmallanna/kv-router/internal/telemetry"
	"github.com/arinmallanna/kv-router/internal/tokenizer"
)

// fakeBackend serves /health and a canned chat completion.
func fakeBackend(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"t","choices":[]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testServer(t *testing.T, urls ...string) (*Server, []*backend.Backend) {
	t.Helper()
	cfgs := make([]backend.BackendConfig, len(urls))
	for i, u := range urls {
		cfgs[i] = backend.BackendConfig{
			ID:                  string(rune('a' + i)),
			URL:                 u,
			CacheCapacityBlocks: 64,
			HealthCheckInterval: 10 * time.Millisecond,
			MaxConcurrent:       1,
		}
	}
	pool := backend.NewPool(cfgs)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go pool.StartHealthChecks(ctx)
	cache := cacheindex.New()
	for _, c := range cfgs {
		cache.Register(c.ID, c.CacheCapacityBlocks)
	}
	s := NewServer(":0",
		&tokenizer.BlockHasher{BlockSize: 64},
		cache,
		scorer.New(scorer.DefaultWeights()),
		pool, telemetry.New(1000), nil)
	// Wait for first probe to mark healthy.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(pool.Healthy()) == len(urls) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(pool.Healthy()) != len(urls) {
		t.Fatalf("backends never became healthy: %d/%d", len(pool.Healthy()), len(urls))
	}
	bl := make([]*backend.Backend, len(urls))
	for i := range urls {
		bl[i] = pool.Get(string(rune('a' + i)))
	}
	return s, bl
}

const chatBody = `{"model":"test","messages":[{"role":"user","content":"hello"}],"stream":false}`

// TestReserveRetryFallsThrough is the regression test for issue #1: when the
// top-ranked backend is full, the request must route to the next candidate,
// not fail with 429 while capacity sits idle.
func TestReserveRetryFallsThrough(t *testing.T) {
	s1 := fakeBackend(t)
	s2 := fakeBackend(t)
	s, backends := testServer(t, s1.URL, s2.URL)

	// Occupy backend "a" directly (simulates a burst winner).
	if !backends[0].TryReserve() {
		t.Fatal("could not pre-reserve backend a")
	}
	defer backends[0].Release()

	// Give "a" full prefix affinity so every scoring snapshot ranks it
	// first: old code then reserves "a", fails, and 429s. Fixed code falls
	// through to "b".
	hashes := (&tokenizer.BlockHasher{BlockSize: 64}).HashPrefix(
		[]tokenizer.Message{{Role: "user", Content: "hello"}})
	s.cache.Commit("a", "chat:test", hashes)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(chatBody))
	rec := httptest.NewRecorder()
	s.handleChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 routed to b, got %d", rec.Code)
	}
	if got := rec.Header().Get(headerBackend); got != "b" {
		t.Fatalf("expected route to b, got %q", got)
	}
}

// TestAllFullReturns429: every backend at capacity must 429, not 503.
func TestAllFullReturns429(t *testing.T) {
	s1 := fakeBackend(t)
	s2 := fakeBackend(t)
	s, backends := testServer(t, s1.URL, s2.URL)

	for _, b := range backends {
		if !b.TryReserve() {
			t.Fatal("could not pre-reserve backend")
		}
		defer b.Release()
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(chatBody))
	rec := httptest.NewRecorder()
	s.handleChatCompletions(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}
