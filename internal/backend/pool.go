package backend

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// BackendConfig holds the static configuration for a single backend instance.
type BackendConfig struct {
	ID                  string
	URL                 string
	CacheCapacityBlocks int
	HealthCheckInterval time.Duration
	MaxConcurrent       int
}

// Backend represents a single downstream KV-cache inference backend.
type Backend struct {
	ID            string
	URL           string
	client        *http.Client
	healthClient  *http.Client
	healthy       atomic.Bool
	queueDepth    atomic.Int64
	maxConcurrent int64
}

// IsHealthy returns the current health status of this backend.
func (b *Backend) IsHealthy() bool {
	return b.healthy.Load()
}

// QueueDepth returns the number of in-flight requests to this backend.
func (b *Backend) QueueDepth() int64 {
	return b.queueDepth.Load()
}

// TryReserve atomically reserves capacity for one request. A non-positive
// limit means the backend is unconstrained.
func (b *Backend) TryReserve() bool {
	for {
		current := b.queueDepth.Load()
		if b.maxConcurrent > 0 && current >= b.maxConcurrent {
			return false
		}
		if b.queueDepth.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

// Release releases a reservation created by TryReserve.
func (b *Backend) Release() { b.queueDepth.Add(-1) }

// Pool manages a set of backends and their health state.
type Pool struct {
	backends  map[string]*Backend
	intervals map[string]time.Duration
}

// Snapshot is a safe, immutable view of backend state for routing telemetry.
type Snapshot struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Healthy  bool   `json:"healthy"`
	Inflight int64  `json:"inflight"`
}

// NewPool constructs a Pool from the provided backend configurations.
// Each backend starts as healthy; the caller should invoke StartHealthChecks
// to begin continuous liveness probing.
func NewPool(configs []BackendConfig) *Pool {
	backends := make(map[string]*Backend, len(configs))
	intervals := make(map[string]time.Duration, len(configs))
	for _, cfg := range configs {
		maxConcurrent := cfg.MaxConcurrent
		if maxConcurrent <= 0 {
			maxConcurrent = 64
		}
		b := &Backend{
			ID:  cfg.ID,
			URL: cfg.URL,
			client: &http.Client{
				// Inference requests, particularly streams, must be allowed to run
				// for their caller's context lifetime. Timeouts belong on dial and
				// response-header phases, not on the whole response body.
				Transport: &http.Transport{ResponseHeaderTimeout: 30 * time.Second},
			},
			healthClient:  &http.Client{Timeout: 5 * time.Second},
			maxConcurrent: int64(maxConcurrent),
		}
		// A backend is not routable until it has passed its first probe.
		b.healthy.Store(false)
		backends[cfg.ID] = b
		interval := cfg.HealthCheckInterval
		if interval <= 0 {
			interval = 10 * time.Second
		}
		intervals[cfg.ID] = interval
	}

	return &Pool{
		backends:  backends,
		intervals: intervals,
	}
}

// Get returns a backend by ID, or nil if not found.
func (p *Pool) Get(id string) *Backend {
	return p.backends[id]
}

// Healthy returns all backends currently marked as healthy.
func (p *Pool) Healthy() []*Backend {
	healthy := make([]*Backend, 0, len(p.backends))
	for _, b := range p.backends {
		if b.healthy.Load() {
			healthy = append(healthy, b)
		}
	}
	sort.Slice(healthy, func(i, j int) bool { return healthy[i].ID < healthy[j].ID })
	return healthy
}

// All returns every registered backend regardless of health.
func (p *Pool) All() []*Backend {
	all := make([]*Backend, 0, len(p.backends))
	for _, b := range p.backends {
		all = append(all, b)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all
}

func (p *Pool) Snapshots() []Snapshot {
	all := p.All()
	result := make([]Snapshot, 0, len(all))
	for _, b := range all {
		result = append(result, Snapshot{ID: b.ID, URL: b.URL, Healthy: b.IsHealthy(), Inflight: b.QueueDepth()})
	}
	return result
}

// StartHealthChecks launches a goroutine that periodically pings each
// backend's /health endpoint. It blocks until ctx is cancelled.
func (p *Pool) StartHealthChecks(ctx context.Context) {
	var wg sync.WaitGroup
	for _, b := range p.backends {
		backend := b
		wg.Add(1)
		go func() {
			defer wg.Done()
			interval := p.intervals[backend.ID]
			p.probe(backend)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					p.probe(backend)
				}
			}
		}()
	}
	<-ctx.Done()
	wg.Wait()
}

// checkAll probes every backend's health endpoint concurrently.
func (p *Pool) checkAll() {
	var wg sync.WaitGroup
	for _, b := range p.backends {
		wg.Add(1)
		go func(backend *Backend) {
			defer wg.Done()
			p.probe(backend)
		}(b)
	}
	wg.Wait()
}

// probe sends a GET to the backend's /health endpoint and updates its
// healthy flag based on the response status.
func (p *Pool) probe(b *Backend) {
	url := fmt.Sprintf("%s/health", b.URL)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		b.healthy.Store(false)
		slog.Warn("backend health request could not be built", "backend", b.ID, "error", err)
		return
	}

	resp, err := b.healthClient.Do(req)
	if err != nil {
		b.healthy.Store(false)
		slog.Warn("backend health check failed", "backend", b.ID, "error", err)
		return
	}
	defer resp.Body.Close()

	wasHealthy := b.healthy.Load()
	nowHealthy := resp.StatusCode >= 200 && resp.StatusCode < 300
	b.healthy.Store(nowHealthy)

	if wasHealthy && !nowHealthy {
		slog.Warn("backend marked unhealthy", "backend", b.ID, "status", resp.StatusCode)
	} else if !wasHealthy && nowHealthy {
		slog.Info("backend marked healthy", "backend", b.ID)
	}
}

// ValidURL reports whether the backend URL is safe to use as an HTTP upstream.
func ValidURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
