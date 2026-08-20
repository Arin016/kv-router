# Nostos · kv-router

**Route each conversation to the GPU that already holds its prefix.**

Nostos (νόστος — *homecoming*) is a KV-cache-aware router for LLM inference fleets. It sits in front of vLLM, TGI, or any OpenAI-compatible backend, estimates which worker has already processed the longest part of a request, and routes with both cache locality and current load in view.

| | |
|---|---|
| **Live site** | [kv-router.vercel.app](https://kv-router.vercel.app) |
| **GitHub** | [github.com/Arin016/kv-router](https://github.com/Arin016/kv-router) |
| **Binary** | `kv-router` (Go) |
| **UI** | React + Vite in `web-ui/` |

The [live demo](https://kv-router.vercel.app) is the marketing site and routing arena — interactive policy comparison, engineering notes, and research. The **console** (`/dashboard`) needs a running `kv-router` process for live API data.

---

## Why this exists

Follow-up chat turns repeat almost the entire conversation. If that prefix is still resident in a backend's KV cache, prefill work can be skipped or reduced. A plain load balancer ignores that — it rotates, hashes sessions, or picks the shortest queue.

Nostos asks a different question: **which healthy backend already observed this prefix?**

It then atomically reserves capacity, proxies the stream, and records bounded affinity evidence — without storing prompts.

---

## Architecture

```
  Client                    Nostos (kv-router)                 Backends
  ──────                    ──────────────────                 ────────
  POST /v1/chat/completions
        ──────────────────▶  hash prefix blocks
                             radix-tree lookup (per backend)
                             score: cache hit − queue − pressure
                             atomic reserve slot
        ◀──────────────────  proxy stream
                             observe outcome (no prompt body)
```

**Hot path:** inspect → match longest observed prefix → reserve → forward → release on EOF/error/cancel.

**Four rules:**
1. **Scope** — cache identity is model + tokenizer + template + adapter + tenant, not text alone
2. **Admit** — health and free capacity filter first; locality only ranks eligible backends
3. **Hold** — reservations outlive response headers until the stream ends
4. **Explain** — telemetry records backend, match depth, score, TTFT — never the prompt

---

## Quick start (self-host)

### 1. Build

```bash
git clone https://github.com/Arin016/kv-router.git
cd kv-router

cd web-ui && npm install && npm run build && cd ..
go build -o kv-router ./cmd/kv-router
```

The UI is embedded from `site/web/` and served by the router at `/`.

### 2. Configure

`config.yaml`:

```yaml
listen_addr: ":8080"
block_size: 64

backends:
  - id: gpu-a
    url: http://10.0.0.11:8000
    cache_capacity_blocks: 4096
    health_check_interval: 10s
  - id: gpu-b
    url: http://10.0.0.12:8000
    cache_capacity_blocks: 4096
    health_check_interval: 10s

scorer:
  cache_hit_weight: 1.0
  queue_depth_weight: 0.5
  eviction_risk_weight: 0.3
```

### 3. Run

```bash
./kv-router --config ./config.yaml
```

| Endpoint | Purpose |
|----------|---------|
| `POST /v1/chat/completions` | OpenAI-compatible routing |
| `GET /livez` | Process liveness |
| `GET /readyz` | Ready when at least one backend is healthy |
| `GET /api/v1/overview` | Fleet + cache summary (console) |
| `GET /api/v1/backends` | Per-backend health and inflight |
| `GET /api/v1/cache` | Bounded residency directory |
| `GET /metrics` | Prometheus (expanding) |

Open **http://localhost:8080** for the product site and **http://localhost:8080/dashboard** for the live console.

### Local UI dev (hot reload)

```bash
cd web-ui && npm run dev
# → http://localhost:5174  (proxies /api to :8080)
```

---

## How routing works

### Prefix fingerprint

Chat messages are concatenated (role-prefixed) and split into fixed-size blocks (default 64 chars). Each block is hashed (xxhash). The sequence is the request's prefix fingerprint.

### Radix tree

A radix tree keyed by block-hash sequences records which backends have served prefixes at each depth. Lookup returns `{backend_id: matched_blocks}`.

### Scoring

```
score = (matched / total) × cache_hit_weight
      − (queue_depth / max) × queue_depth_weight
      − eviction_risk × eviction_risk_weight
```

Highest eligible score wins. No match → least-loaded fallback.

### After dispatch

1. Record hash sequence for the chosen backend (bounded LRU eviction)
2. Proxy request; hold reservation until stream completes
3. Emit sanitized decision event (no prompt content)

---

## Project layout

```
cmd/kv-router/          # entrypoint
internal/
  api/                  # HTTP handlers + static UI
  backend/              # pool, proxy, health
  cacheindex/           # residency directory
  radixtree/            # prefix index
  scorer/               # scheduling policy
web-ui/                 # Nostos marketing + console (React)
site/web/               # Vite build output (also Vercel deploy root)
```

---

## Deploy UI to Vercel

The marketing site deploys as a static SPA from `site/web/`:

```bash
# from repo root — vercel.json is already configured
npx vercel --prod
```

`vercel.json` builds `web-ui/`, outputs to `site/web/`, and rewrites `/engineering`, `/research`, and `/dashboard` to `index.html`.

> **Note:** Vercel hosts the UI only. Run `kv-router` separately for the OpenAI endpoint and live console API.

---

## Benchmarks

```bash
go test -bench=. ./...
```

End-to-end prefill savings under shared-prefix workloads are the benchmark that matters — not router microseconds alone. See [Engineering → Validation](https://kv-router.vercel.app/engineering#evaluation).

---

## License

MIT
