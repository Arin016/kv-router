import MarketingChrome from "./MarketingChrome";

const TOC = [
  ["premise", "Workload model"],
  ["contracts", "Request lifecycle"],
  ["envelope", "HTTP boundary"],
  ["identity-deep", "Cache identity"],
  ["directory-deep", "Residency directory"],
  ["eligibility", "Health and admission"],
  ["policy", "Scheduling policy"],
  ["streaming", "Streaming proxy"],
  ["failure", "Outcome semantics"],
  ["privacy", "Telemetry boundary"],
  ["replicas", "Multiple routers"],
  ["evaluation", "Validation and roadmap"],
] as const;

export default function EngineeringPage() {
  return (
    <MarketingChrome active="engineering">
      <main>
        <header className="mkt-essay-hero">
          <p className="mkt-eyebrow">Engineering · architecture note 01</p>
          <h1 className="mkt-display mkt-display-essay">What the router knows—and what it only predicts.</h1>
          <p className="mkt-lead">
            A concrete description of the Nostos request path: how a chat request becomes a
            model-scoped prefix signal, how that signal competes with load, when the router records
            affinity, and where the present implementation deliberately stops short of claiming
            engine-level cache truth.
          </p>
        </header>

        <div className="mkt-essay-layout">
          <aside className="mkt-essay-toc">
            <p className="mkt-essay-toc-label">On this page</p>
            <ol>
              {TOC.map(([id, label]) => (
                <li key={id}>
                  <a href={`#${id}`}>{label}</a>
                </li>
              ))}
            </ol>
          </aside>

          <article className="mkt-essay-body">
            <section id="premise">
              <h2>The workload is stateful even when the API is not</h2>
              <p>
                An OpenAI-compatible chat request looks like an ordinary stateless POST. The work
                performed by an inference backend is not stateless. Before a model can decode the
                next token, it runs the prompt through the transformer and materializes key and value
                tensors for each attention layer. This <em>prefill</em> phase creates the KV cache used
                by subsequent decoding steps.
              </p>
              <p>
                Follow-up turns often repeat almost the entire conversation. If a backend still has
                the corresponding prefix blocks resident, it may be able to reuse part of that
                prefill. Another otherwise identical backend may have none of those blocks and must
                reconstruct the prefix from the beginning. The two workers therefore have different
                expected costs for the same HTTP request.
              </p>
              <p>
                Nostos adds locality to the routing decision. It does not assume that locality is
                authoritative. Health and active work remain first-order signals because a probable
                cache hit on an unavailable or saturated worker has no operational value.
              </p>
              <div className="mkt-callout">
                <b>Primary invariant</b>
                <p>
                  The directory is a prediction of reusable work. It can influence scheduling; it
                  cannot certify the contents of an inference engine’s KV allocator.
                </p>
              </div>
            </section>

            <section id="contracts">
              <h2>The request lifecycle</h2>
              <p>
                The hot path is intentionally small. The HTTP handler reads the body, derives a
                routing key, takes a snapshot of healthy backends, asks the directory for a prefix
                match on each backend, selects a destination, reserves one unit of concurrency, and
                forwards the original bytes. An outcome then updates telemetry and, when justified,
                the affinity directory.
              </p>
              <div className="mkt-diagram">
                <span>1 · Ingress</span>
                <b>bounded body → routing fields + untouched forwarding bytes</b>
                <span>2 · Evidence</span>
                <b>model namespace + ordered block hashes → per-backend prefix depth</b>
                <span>3 · Decision</span>
                <b>healthy snapshots + prefix depth + inflight work → backend</b>
                <span>4 · Ownership</span>
                <b>atomic reservation → proxy for the lifetime of the response body</b>
                <span>5 · Outcome</span>
                <b>status + timing + terminal result → telemetry and conditional commit</b>
              </div>
              <p>
                These stages are separate for a practical reason: block identity can later come from
                a native engine adapter without changing HTTP forwarding, and the score can move from
                static weights to measured token-work estimates without changing commit rules.
              </p>
            </section>

            <section id="envelope">
              <h2>The HTTP boundary: inspect narrowly, forward faithfully</h2>
              <p>
                Nostos accepts <code>POST /v1/chat/completions</code> and limits the request body to
                10 MiB before parsing it. The current routing envelope reads the model, messages, and
                stream flag. The exact request body—not a re-serialized struct—is sent upstream, so
                sampling fields and provider-specific top-level extensions survive the hop.
              </p>
              <p>
                The forwarding layer preserves the original path and query string. It copies request
                headers, including credentials and tracing headers, while removing Host,
                Content-Length, and hop-by-hop transport headers. This makes the router a policy layer
                in front of the serving API rather than an alternate inference protocol.
              </p>
              <div className="mkt-diagram">
                <span>Inspected for routing</span>
                <b>model · string message content · stream mode</b>
                <span>Forwarded upstream</span>
                <b>original JSON bytes · path · query · end-to-end headers</b>
              </div>
              <p>
                There is an important current limitation: message content is parsed as a string.
                Multimodal content arrays and richer tool-message shapes are not yet accepted by the
                routing envelope, even though unknown top-level fields are preserved in the raw body.
                Moving the envelope to <code>json.RawMessage</code> is required before claiming full
                compatibility with the wider chat-completions schema.
              </p>
            </section>

            <section id="identity-deep">
              <h2>Cache identity is larger than prompt text</h2>
              <p>
                Repeated text is only a candidate for reuse. The actual token sequence depends on the
                tokenizer and chat template. Reusable KV state also depends on the model revision,
                adapter or LoRA, position scheme, engine block format, and any tenant boundary that
                forbids cross-domain reuse. A production identity must include every dimension that
                can change the computed tensors.
              </p>
              <div className="mkt-chips">
                <span>isolation domain</span>
                <span>engine + block format</span>
                <span>model revision + adapter</span>
                <span>tokenizer + chat template</span>
                <b>ordered engine token blocks</b>
              </div>
              <p>
                Today, Nostos uses a deliberately weaker compatibility key. It flattens each message
                as <code>role:content</code>, divides the resulting byte string into fixed-size chunks
                (64 bytes by default), and hashes each chunk with xxHash64. The cache namespace is
                <code>chat:&lt;model&gt;</code>. That prevents cross-model matches, but it does not yet
                distinguish model revisions, templates, tokenizers, adapters, or tenants.
              </p>
              <p>
                Byte chunks are not engine KV blocks. Unicode length, message-boundary encoding, and
                a growing final partial chunk can all make the passive fingerprint diverge from the
                engine’s tokenization. The correct product term is therefore <strong>modeled
                affinity</strong>, not “cache hit.” Native engine token IDs and residency events are
                the path from a useful heuristic to stronger evidence.
              </p>
              <div className="mkt-callout">
                <b>Identity invariant</b>
                <p>
                  A match may be reported only inside the same namespace. Extending the namespace is
                  correctness work, not optional metadata enrichment.
                </p>
              </div>
            </section>

            <section id="directory-deep">
              <h2>The residency directory is bounded and backend-local</h2>
              <p>
                Each configured backend owns one in-memory directory with a fixed observation
                capacity. Within it, entries are grouped by namespace. A prompt containing blocks
                <code>[A, B, C, D]</code> is represented by cumulative keys for <code>A</code>,
                <code>AB</code>, <code>ABC</code>, and <code>ABCD</code>. This lets a later request
                beginning with <code>[A, B, C, X]</code> match three leading blocks rather than falling
                back to an all-or-nothing prompt hash.
              </p>
              <div className="mkt-diagram">
                <span>Block fingerprints</span>
                <b>A · B · C · D</b>
                <span>Cumulative directory keys</span>
                <b>H(A) · H(A,B) · H(A,B,C) · H(A,B,C,D)</b>
                <span>Lookup rule</span>
                <b>walk from the first key and stop at the first missing prefix</b>
              </div>
              <p>
                Lookup and eviction share one structure, so an evicted observation cannot remain
                visible in a second index. Committing a prompt refreshes existing keys and inserts
                new ones. When total observations across a backend’s namespaces exceed the configured
                capacity, the least recently observed key is removed immediately.
              </p>
              <p>
                This capacity is a metadata bound, not a mirror of GPU memory. Engines can evict KV
                pages independently, and Nostos can retain an observation after the underlying page
                has disappeared. Conversely, an engine may retain reusable blocks that Nostos never
                observed. A TTL or native reconciliation feed can narrow that gap; neither changes
                the fundamental rule that the directory is advisory.
              </p>
            </section>

            <section id="eligibility">
              <h2>Health and admission answer different questions</h2>
              <p>
                Backends start unhealthy and become routable only after a successful
                <code>GET /health</code> probe. Each backend has its own probe interval and a five-second
                health timeout. The router’s <code>/livez</code> endpoint reports process liveness;
                <code>/readyz</code> reports readiness only when at least one backend is currently
                healthy.
              </p>
              <p>
                Admission is enforced with an atomic compare-and-swap counter. A backend has a
                configurable maximum concurrency, defaulting to 64. Once selected, the request must
                acquire a reservation before any upstream work begins. The reservation is released
                with the request outcome, including error and cancellation paths.
              </p>
              <div className="mkt-chips">
                <span>probe passed</span>
                <span>included in routing snapshot</span>
                <span>selected by policy</span>
                <span>atomic capacity claim</span>
                <b>dispatch</b>
              </div>
              <p>
                The current scheduler selects from healthy backends and reserves immediately after
                selection. Another request can consume the last slot in that interval; in that case
                Nostos returns 429 rather than overcommitting. The next scheduler contract should
                reserve as part of selection and retry another eligible backend when the preferred
                candidate loses the race. Readiness should eventually include that same admission
                view, not health alone.
              </p>
            </section>

            <section id="policy">
              <h2>The current policy is simple enough to inspect</h2>
              <p>
                For every healthy backend, the handler collects the longest observed prefix, current
                inflight count, and directory usage. If no backend has a prefix match, routing falls
                back to the smallest inflight count. Otherwise it evaluates a weighted score:
              </p>
              <div className="mkt-diagram">
                <span>Locality</span>
                <b>matched blocks / total request blocks</b>
                <span>Queue pressure</span>
                <b>inflight requests / configured comparison ceiling</b>
                <span>Directory pressure</span>
                <b>observations used / observation capacity</b>
                <span>Score</span>
                <b>locality × w₁ − queue pressure × w₂ − directory pressure × w₃</b>
              </div>
              <p>
                This is a routing heuristic, not a latency model. Request count ignores the difference
                between a short completion and a long decode. Directory fullness is not the engine’s
                eviction probability; inference caches normally operate near capacity. Static weights
                are useful while the mechanism is being validated because each decision remains
                explainable, but they should not be mistaken for calibrated cost.
              </p>
              <div className="mkt-callout">
                <b>Policy direction</b>
                <p>
                  Minimize expected remaining work: unmatched prompt tokens × observed prefill cost,
                  plus queued prefill and decode work, expected response length, and a passive failure
                  penalty. Keep deterministic tie-breaking and reserve the winner in the same operation.
                </p>
              </div>
            </section>

            <section id="streaming">
              <h2>The proxy holds ownership through the response body</h2>
              <p>
                Health probes and inference requests use separate HTTP clients. Health has a short
                total deadline. Inference has a 30-second response-header timeout but no fixed total
                body timeout; the client request context controls the lifetime of a long generation.
                This avoids terminating healthy streams simply because generation takes more than a
                few seconds.
              </p>
              <div className="mkt-chips">
                <span>reserve</span>
                <span>connect</span>
                <span>upstream headers</span>
                <span>first response bytes</span>
                <span>stream and flush</span>
                <b>EOF / error / cancel → release</b>
              </div>
              <p>
                Streaming responses are copied incrementally and flushed to the caller. Upstream
                status is written before the body, and hop-by-hop response headers are removed on the
                streaming path. The recorded “TTFT” is currently time to the first body bytes observed
                by the router; it is not yet a token-aware measurement and should be labeled that way
                when comparing engines.
              </p>
              <p>
                Non-streaming responses preserve upstream status and body. Response-header filtering
                should be unified with the streaming path so both modes apply the same end-to-end
                header policy.
              </p>
            </section>

            <section id="failure">
              <h2>Outcomes determine what the router is allowed to learn</h2>
              <p>
                Recording affinity before upstream work succeeds creates phantom warmth. Nostos
                commits a non-streaming observation only for responses below 400. For streams, it
                commits only after the proxy finishes successfully and has observed response bytes.
                That rule is conservative: an interrupted stream may have created reusable KV state,
                but the router declines to claim it without a clean outcome.
              </p>
              <div className="mkt-fail-grid">
                <article><b>Connection failure</b><p>No observation is committed. Capacity is released and the caller receives 502.</p></article>
                <article><b>Upstream 4xx</b><p>Status and body pass through. Rejected work does not create modeled affinity.</p></article>
                <article><b>Upstream 5xx</b><p>The failure is recorded. The directory is not updated.</p></article>
                <article><b>Client cancellation</b><p>The request context cancels upstream I/O and the deferred reservation releases.</p></article>
                <article><b>Successful response</b><p>Cumulative prefix observations are refreshed within the backend namespace.</p></article>
                <article><b>No healthy backend</b><p>The router returns 503 and readiness reports unavailable.</p></article>
              </div>
              <p>
                Nostos does not currently retry or fail over inference requests. That is deliberate
                until retry safety is explicit: a request can be retried only before client-visible
                response bytes, and non-idempotent provider behavior must be considered. Passive
                circuit breaking and exact streaming-status telemetry remain roadmap work.
              </p>
            </section>

            <section id="privacy">
              <h2>The control plane explains routes without storing prompts</h2>
              <p>
                The bounded event recorder stores the sanitized model label, chosen backend, matched
                and total block counts, score, queue depth, status, stream flag, and first-byte timing.
                It does not receive message content, tool payloads, or the raw hashes used by the
                directory. Operators can answer “why did this backend win?” without creating a second
                store of user conversations.
              </p>
              <div className="mkt-fail-grid">
                <article><b>Decision data</b><p>backend · match depth · score · queue snapshot · status · timing</p></article>
                <article><b>Excluded data</b><p>messages · tool bodies · raw block keys · reversible prompt identifiers</p></article>
              </div>
              <p>
                The dashboard consumes a versioned read API for overview, backend snapshots, cache
                usage, and recent routes. It does not traverse the directory directly. Prometheus
                counters use bounded dimensions rather than request- or prompt-derived labels, which
                keeps observability from becoming a memory or privacy liability of its own.
              </p>
            </section>

            <section id="replicas">
              <h2>A local directory cannot become global truth by replication</h2>
              <p>
                Every Nostos process learns from the traffic it personally routes. Two replicas in
                front of the same backend fleet can therefore hold different affinity views. Sharing
                an HTTP load balancer does not reconcile those views, and persisting the directory
                does not make stale observations authoritative.
              </p>
              <p>
                The simplest scale-out model is deterministic ownership: assign backend groups to
                router shards with rendezvous hashing, then route a conversation to the shard that
                owns the relevant group. Observation and decision stay colocated, and the hot path
                does not depend on a remote metadata service.
              </p>
              <p>
                A shared event stream or directory service can support larger topologies, but its
                consistency model must be stated. Engine-reported residency is stronger than
                router-observed history; delayed cross-router observations should carry age and
                confidence rather than being presented as exact state.
              </p>
            </section>

            <section id="evaluation">
              <h2>Validation must measure saved prefill, not just routing speed</h2>
              <p>
                A microbenchmark can show that hashing and lookup are inexpensive. It cannot show
                that the chosen backend reused KV state or that end-to-end latency improved. The
                useful experiment controls prefix overlap, arrival concurrency, prompt length,
                decode length, backend heterogeneity, and cache churn.
              </p>
              <div className="mkt-diagram">
                <span>Baselines</span>
                <b>random · round-robin · least-inflight · modeled-affinity</b>
                <span>Primary measurements</span>
                <b>prefill tokens recomputed · first-byte p50/p95/p99 · throughput · error rate</b>
                <span>Correctness properties</span>
                <b>bounded memory · no cross-namespace match · no phantom commit · no leaked reservation</b>
                <span>Fault cases</span>
                <b>health transition · connection failure · 4xx/5xx · slow stream · client cancellation</b>
              </div>
              <p>
                The immediate architecture work is equally concrete: replace the string-only request
                envelope, expand cache namespaces, integrate native engine block metadata, reserve
                during selection, add passive circuit state, and build a reproducible shared-prefix
                benchmark. Until those results exist, Nostos should report mechanism, observed route
                outcomes, and known uncertainty—not an invented speedup multiplier.
              </p>
              <div className="mkt-callout">
                <b>Evidence invariant</b>
                <p>
                  A performance claim belongs in product copy only when the workload, engine version,
                  configuration, baselines, and result distributions are reproducible.
                </p>
              </div>
            </section>
          </article>
        </div>
      </main>
    </MarketingChrome>
  );
}
