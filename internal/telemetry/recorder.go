// Package telemetry provides bounded, prompt-safe route observability.
package telemetry

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RouteEvent intentionally excludes prompt content and reversible fingerprints.
type RouteEvent struct {
	ID            uint64    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	Model         string    `json:"model"`
	BackendID     string    `json:"backend_id"`
	MatchedBlocks int       `json:"matched_blocks"`
	TotalBlocks   int       `json:"total_blocks"`
	Score         float64   `json:"score"`
	QueueDepth    int       `json:"queue_depth"`
	StatusCode    int       `json:"status_code"`
	Stream        bool      `json:"stream"`
	TTFTMillis    int64     `json:"ttft_ms,omitempty"`
}

type Summary struct {
	Requests         uint64 `json:"requests"`
	Errors           uint64 `json:"errors"`
	Streams          uint64 `json:"streams"`
	CacheHitRequests uint64 `json:"cache_hit_requests"`
}

type Recorder struct {
	mu        sync.RWMutex
	capacity  int
	events    []RouteEvent
	next      int
	count     int
	sequence  atomic.Uint64
	requests  atomic.Uint64
	errors    atomic.Uint64
	streams   atomic.Uint64
	cacheHits atomic.Uint64
}

func New(capacity int) *Recorder {
	if capacity <= 0 {
		capacity = 1000
	}
	return &Recorder{capacity: capacity, events: make([]RouteEvent, capacity)}
}

func (r *Recorder) Record(event RouteEvent) {
	event.ID = r.sequence.Add(1)
	event.Timestamp = time.Now()
	r.requests.Add(1)
	if event.StatusCode >= 400 {
		r.errors.Add(1)
	}
	if event.Stream {
		r.streams.Add(1)
	}
	if event.MatchedBlocks > 0 {
		r.cacheHits.Add(1)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[r.next] = event
	r.next = (r.next + 1) % r.capacity
	if r.count < r.capacity {
		r.count++
	}
}

func (r *Recorder) Recent(limit int) []RouteEvent {
	if limit <= 0 || limit > r.capacity {
		limit = 100
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit > r.count {
		limit = r.count
	}
	result := make([]RouteEvent, 0, limit)
	for n := 0; n < limit; n++ {
		index := (r.next - 1 - n + r.capacity) % r.capacity
		result = append(result, r.events[index])
	}
	return result
}

func (r *Recorder) Summary() Summary {
	return Summary{Requests: r.requests.Load(), Errors: r.errors.Load(), Streams: r.streams.Load(), CacheHitRequests: r.cacheHits.Load()}
}

func (r *Recorder) Prometheus() string {
	s := r.Summary()
	return fmt.Sprintf("# TYPE kv_router_requests_total counter\nkv_router_requests_total %d\n# TYPE kv_router_errors_total counter\nkv_router_errors_total %d\n# TYPE kv_router_streams_total counter\nkv_router_streams_total %d\n# TYPE kv_router_cache_hit_requests_total counter\nkv_router_cache_hit_requests_total %d\n", s.Requests, s.Errors, s.Streams, s.CacheHitRequests)
}

func SanitizeModel(model string) string { return strings.TrimSpace(model) }
