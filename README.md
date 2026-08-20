# kv-router

A KV-cache-aware request router for LLM inference backends. Routes incoming chat completion requests to the backend most likely to have relevant prefix blocks already cached, minimizing redundant KV-cache computation and improving throughput.

## Architecture

```
                         ┌─────────────────────────────────────┐
                         │            kv-router                │
                         │                                     │
  OpenAI-compat  ──────▶ │  ┌───────────┐   ┌──────────────┐  │
  /v1/chat/completions   │  │  Block     │──▶│  Radix Tree  │  │
                         │  │  Hasher    │   │  (prefix     │  │
                         │  │  (xxhash)  │   │   index)     │  │
                         │  └───────────┘   └──────┬───────┘  │
                         │                         │           │
                         │                         ▼           │
                         │  ┌──────────────────────────────┐   │
                         │  │         Scorer               │   │
                         │  │  cache_hit * w1              │   │
                         │  │  - queue_depth * w2          │   │
                         │  │  - eviction_risk * w3        │   │
                         │  └──────────────┬───────────────┘   │
                         │                 │                    │
                         └─────────────────┼────────────────────┘
                                           │
                         ┌─────────────────┼────────────────────┐
                         │                 ▼                     │
                         │  ┌──────────┐ ┌──────────┐ ┌──────┐ │
                         │  │Backend A │ │Backend B │ │ ...  │ │
                         │  │(vLLM/TGI)│ │(vLLM/TGI)│ │      │ │
                         │  └──────────┘ └──────────┘ └──────┘ │
                         │         Backend Pool                 │
                         └──────────────────────────────────────┘
```

## Quick Start

### Build

```bash
cd /path/to/kv-router
cd web-ui && npm install && npm run build && cd ..
go build -o kv-router ./cmd/kv-router
```

The compiled React application is served by the router at `/`, with product,
engineering, research, and command-center routes at `/`, `/engineering`,
`/research`, and `/dashboard`.

### Configure

Create `config.yaml`:

```yaml
listen_addr: ":8080"
block_size: 64

backends:
  - id: vllm-0
    url: http://localhost:8000
    cache_capacity_blocks: 4096
    health_check_interval: 10s
  - id: vllm-1
    url: http://localhost:8001
    cache_capacity_blocks: 4096
    health_check_interval: 10s

scorer:
  cache_hit_weight: 1.0
  queue_depth_weight: 0.5
  eviction_risk_weight: 0.3
```

### Run

```bash
./kv-router --config ./config.yaml

# Override listen address:
./kv-router --config ./config.yaml --listen :9090
```

The router exposes:
- `POST /v1/chat/completions` — OpenAI-compatible routing endpoint
- `GET /livez` — process liveness
- `GET /readyz` (or `/health`) — readiness; succeeds only when a backend is healthy
- `GET /api/v1/overview` — UI-facing fleet and cache summary
- `GET /api/v1/backends` — backend health and inflight requests
- `GET /api/v1/cache` — bounded cache-directory usage
- `GET /metrics` — Prometheus endpoint (expanded metrics are in progress)

## How It Works

### Block Hashing

Incoming chat messages are concatenated (role-prefixed) and split into fixed-size blocks (default 64 chars). Each block is hashed with xxhash to produce a sequence of `uint64` digests. This sequence is the "prefix fingerprint" of the request.

### Radix Tree Lookup

The router maintains a radix tree keyed by block hash sequences. When a request arrives, its hash sequence is traversed down the tree. Nodes at each depth record which backends have previously served prefixes at that length. The lookup returns `{backend_id: matched_blocks}` — how deep each backend's cache alignment goes.

### Scoring

Each healthy backend is scored:

```
score = (matched_blocks / total_blocks) × cache_hit_weight
      − (queue_depth / max_queue_depth) × queue_depth_weight
      − eviction_risk × eviction_risk_weight
```

Where `eviction_risk = 1 − (blocks_remaining / total_capacity)`.

The highest-scoring backend wins. If no backend has any cache match, the router falls back to least-loaded routing (lowest queue depth).

### After Routing

Once a backend is selected, the router:
1. Records the request's hash sequence in the radix tree for that backend (for future affinity)
2. Proxies the request to the backend
3. Tracks queue depth changes for scoring

## Benchmarks

_TODO: Add benchmarks for routing latency, cache-directory operations, and end-to-end throughput. Do not treat cache-affinity estimates as measured backend KV residency until an engine adapter supplies native cache metadata._

```bash
go test -bench=. ./...
```

## Design Decisions

1. **xxhash over SHA/crypto** — We need speed, not collision resistance. xxhash gives ~10 GB/s throughput on modern CPUs. Prefix matching only needs "probably equal" semantics.

2. **Fixed block size** — Simpler than variable-length chunking. The 64-char default balances granularity (catches most system prompt reuse) against tree depth explosion.

3. **Radix tree over hash map** — Captures partial prefix matches. A backend that has 80% of your prefix cached is still better than one with 0%, even if it doesn't have the full prefix. Flat hash maps can't express "longest common prefix."

4. **Separate eviction tracking from tree structure** — The tree records what was sent where. Eviction (LRU) happens per-backend based on `LastAccess` timestamps. This decouples the index from backend memory management.

5. **RWMutex on tree** — Read-heavy workload (many lookups per insert). Reader starvation is unlikely given the short critical sections. Sharding or lock-free approaches are future optimizations if contention appears.

6. **No external dependencies for CLI** — `flag` package keeps the binary small and startup instant. A router should have sub-millisecond cold start.

7. **Scorer weights are configurable** — Different deployments have different bottlenecks. A cluster with homogeneous backends cares more about cache hits; a heterogeneous cluster may weight queue depth higher.

## License

MIT
