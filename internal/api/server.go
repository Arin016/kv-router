package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/arinmallanna/kv-router/internal/backend"
	"github.com/arinmallanna/kv-router/internal/cacheindex"
	"github.com/arinmallanna/kv-router/internal/scorer"
	"github.com/arinmallanna/kv-router/internal/telemetry"
	"github.com/arinmallanna/kv-router/internal/tokenizer"
)

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 30 * time.Second
)

// Server is the HTTP front-door for kv-router, exposing an OpenAI-compatible
// chat completions endpoint that routes requests based on KV-cache affinity.
type Server struct {
	listenAddr     string
	tokenizer      *tokenizer.BlockHasher
	cache          *cacheindex.Index
	scorer         *scorer.Scorer
	pool           *backend.Pool
	telemetry      *telemetry.Recorder
	backendConfigs []backend.BackendConfig
}

// NewServer constructs a Server from the resolved config and pre-built components.
func NewServer(
	listenAddr string,
	tok *tokenizer.BlockHasher,
	cache *cacheindex.Index,
	sc *scorer.Scorer,
	pool *backend.Pool,
	recorder *telemetry.Recorder,
	backendConfigs []backend.BackendConfig,
) *Server {
	return &Server{
		listenAddr:     listenAddr,
		tokenizer:      tok,
		cache:          cache,
		scorer:         sc,
		pool:           pool,
		telemetry:      recorder,
		backendConfigs: backendConfigs,
	}
}

// Start binds the HTTP server and blocks until ctx is cancelled, then drains
// in-flight requests within shutdownTimeout.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	srv := &http.Server{
		Addr:              s.listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
	}

	// Start listener in background.
	errCh := make(chan error, 1)
	go func() {
		slog.Info("kv-router listening", "addr", s.listenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	// Return bind failures immediately instead of waiting for a signal.
	select {
	case err, ok := <-errCh:
		if ok && err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	slog.Info("shutting down HTTP server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	// Return any listen error that fired before shutdown.
	if err, ok := <-errCh; ok {
		return err
	}
	return nil
}

// registerRoutes wires endpoints to the mux.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// The React control plane is a static client of the versioned ops API and
	// remains outside the request-routing hot path. Extensionless paths receive
	// the SPA entrypoint so product, engineering, research, and dashboard routes
	// all work on direct navigation and refresh.
	static := http.FileServer(http.Dir("./site/web"))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/og.png" || r.URL.Path == "/favicon.png" {
			static.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, "./site/web/index.html")
	})
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("GET /livez", s.handleLive)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("GET /health", s.handleReady) // compatibility alias
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.HandleFunc("GET /api/v1/overview", s.handleOverview)
	mux.HandleFunc("GET /api/v1/backends", s.handleBackends)
	mux.HandleFunc("GET /api/v1/cache", s.handleCache)
	mux.HandleFunc("GET /api/v1/routing/recent", s.handleRecentRoutes)
}

// handleHealth returns 200 when the server is ready to serve traffic.
func (s *Server) handleLive(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"live"}`))
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if len(s.pool.Healthy()) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not_ready","reason":"no healthy backends"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

// handleMetrics exposes basic operational counters (placeholder for prometheus integration).
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(s.telemetry.Prometheus()))
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// handleOverview is the compact, stable contract for the future operations UI.
func (s *Server) handleOverview(w http.ResponseWriter, _ *http.Request) {
	backends := s.pool.Snapshots()
	healthy := 0
	var inflight int64
	for _, b := range backends {
		if b.Healthy {
			healthy++
		}
		inflight += b.Inflight
	}
	writeJSON(w, map[string]any{"status": "ok", "backends": map[string]int{"healthy": healthy, "total": len(backends)}, "inflight": inflight, "cache": s.cache.AllUsage(), "routing": s.telemetry.Summary()})
}

func (s *Server) handleBackends(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.pool.Snapshots())
}
func (s *Server) handleCache(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.cache.AllUsage())
}
func (s *Server) handleRecentRoutes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.telemetry.Recent(100))
}
