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

// DefaultMaxConcurrent is the fallback active-request limit applied when a
// backend is configured with MaxConcurrent <= 0.
const DefaultMaxConcurrent = 64

const (
	// unhealthyAfter consecutive failed probes mark a healthy backend
	// unhealthy, so a single 429/503 spike cannot flap it out (issue #5).
	unhealthyAfter = 3
	// healthyAfter consecutive successful probes readmit an unhealthy
	// backend. StartHealthChecks runs a bootstrap probe burst at startup so
	// a healthy backend is routable immediately instead of waiting a full
	// health-check interval (issue #5).
	healthyAfter = 2
)

// BackendConfig holds the static configuration for a single backend instance.
type BackendConfig struct {
	ID                  string
	URL                 string
	CacheCapacityBlocks int
	HealthCheckInterval time.Duration
	// MaxConcurrent is the maximum active requests accepted by this backend.
	// 0 means DefaultMaxConcurrent. Negative values are rejected by config
	// validation and defaulted defensively by NewPool for direct callers.
	MaxConcurrent int
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

	// Health hysteresis state and recorded transitions (issue #5).
	consecutiveFails     atomic.Int32
	consecutiveSuccesses atomic.Int32
	healthyTransitions   atomic.Uint64 // unhealthy -> healthy admissions
	unhealthyTransitions atomic.Uint64 // healthy -> unhealthy removals
}

// IsHealthy returns the current health status of this backend.
func (b *Backend) IsHealthy() bool {
	return b.healthy.Load()
}

// QueueDepth returns the number of in-flight requests to this backend.
func (b *Backend) QueueDepth() int64 {
	return b.queueDepth.Load()
}

// MaxConcurrent returns the active-request limit enforced by TryReserve.
// It is always positive for backends built by NewPool.
func (b *Backend) MaxConcurrent() int {
	return int(b.maxConcurrent)
}

// TryReserve atomically reserves capacity for one request. Backends built by
// NewPool always carry a positive limit (0 input means DefaultMaxConcurrent),
// so TryReserve enforces that limit. A zero-value Backend constructed without
// NewPool has a non-positive limit and is treated as unconstrained.
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
	ID                   string `json:"id"`
	URL                  string `json:"url"`
	Healthy              bool   `json:"healthy"`
	Inflight             int64  `json:"inflight"`
	HealthyTransitions   uint64 `json:"healthy_transitions"`
	UnhealthyTransitions uint64 `json:"unhealthy_transitions"`
}

// NewPool constructs a Pool from the provided backend configurations and
// fails fast on configs that would otherwise only surface at first request:
// empty or duplicate backend IDs (the backends map is keyed by ID) and
// non-http(s) URLs (validated with ValidURL).
// A MaxConcurrent of 0 means DefaultMaxConcurrent; negative values are
// invalid (rejected by config validation) and defaulted defensively here so
// direct callers cannot create a zero-limit backend.
// Each backend starts unhealthy; it becomes routable after its probe
// thresholds admit it. The caller should invoke StartHealthChecks to begin
// continuous liveness probing.
func NewPool(configs []BackendConfig) (*Pool, error) {
	backends := make(map[string]*Backend, len(configs))
	intervals := make(map[string]time.Duration, len(configs))
	for i, cfg := range configs {
		if cfg.ID == "" {
			return nil, fmt.Errorf("backend[%d]: id is required", i)
		}
		if _, exists := backends[cfg.ID]; exists {
			return nil, fmt.Errorf("duplicate backend id %q", cfg.ID)
		}
		if !ValidURL(cfg.URL) {
			return nil, fmt.Errorf("backend %q: url %q must be an absolute http(s) URL", cfg.ID, cfg.URL)
		}
		maxConcurrent := cfg.MaxConcurrent
		if maxConcurrent <= 0 {
			maxConcurrent = DefaultMaxConcurrent
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
	}, nil
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
		result = append(result, Snapshot{
			ID:                   b.ID,
			URL:                  b.URL,
			Healthy:              b.IsHealthy(),
			Inflight:             b.QueueDepth(),
			HealthyTransitions:   b.healthyTransitions.Load(),
			UnhealthyTransitions: b.unhealthyTransitions.Load(),
		})
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
			// Bootstrap burst: admission needs healthyAfter successes, so
			// give a healthy backend that many immediate probes to become
			// routable at cold start without waiting a full interval.
			for i := 0; i < healthyAfter && !backend.IsHealthy(); i++ {
				p.probe(backend)
			}
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

// probe pings the backend's /health endpoint once and applies health
// hysteresis (issue #5): unhealthyAfter consecutive failures mark a healthy
// backend unhealthy, healthyAfter consecutive successes readmit it. Every
// state transition is logged and counted for Snapshot telemetry.
func (p *Pool) probe(b *Backend) {
	if err := p.pingOnce(b); err != nil {
		b.consecutiveSuccesses.Store(0)
		n := b.consecutiveFails.Add(1)
		if n >= unhealthyAfter && b.healthy.CompareAndSwap(true, false) {
			b.unhealthyTransitions.Add(1)
			slog.Warn("backend marked unhealthy",
				"backend", b.ID,
				"consecutive_failures", n,
				"unhealthy_transitions", b.unhealthyTransitions.Load(),
				"error", err,
			)
			return
		}
		slog.Warn("backend health check failed",
			"backend", b.ID,
			"consecutive_failures", n,
			"error", err,
		)
		return
	}
	b.consecutiveFails.Store(0)
	n := b.consecutiveSuccesses.Add(1)
	if n >= healthyAfter && b.healthy.CompareAndSwap(false, true) {
		b.healthyTransitions.Add(1)
		slog.Info("backend marked healthy",
			"backend", b.ID,
			"consecutive_successes", n,
			"healthy_transitions", b.healthyTransitions.Load(),
		)
	}
}

// pingOnce probes /health once; nil means the backend answered 2xx.
func (p *Pool) pingOnce(b *Backend) error {
	url := fmt.Sprintf("%s/health", b.URL)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build health request: %w", err)
	}

	resp, err := b.healthClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ValidURL reports whether the backend URL is safe to use as an HTTP upstream.
func ValidURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
