import type { ReactNode } from "react";
import MarketingChrome from "./MarketingChrome";

const POSTS = [
  {
    type: "Architecture",
    title: "Cache affinity is not a cache guarantee",
    dek: "Why repeated text is only a routing clue—and what model, tokenizer, template, adapter, and engine state have to do with real KV reuse.",
    read: "3 min",
    href: "/research/cache-affinity",
  },
  {
    type: "Scheduling",
    title: "A queue depth is not a reservation",
    dek: "How the gap between reading load and claiming capacity creates a herd, especially for long-lived streaming requests.",
    read: "3 min",
    href: "/research/queue-reservations",
  },
  {
    type: "Operations",
    title: "Readiness must mean routable",
    dek: "A live process with zero usable upstreams is not ready. Health semantics are product semantics.",
    read: "2 min",
    href: "/engineering#eligibility",
  },
  {
    type: "Privacy",
    title: "Observability without prompts",
    dek: "A route can be explainable without recording messages, tool payloads, raw block keys, or unbounded metric labels.",
    read: "3 min",
    href: "/research/prompt-safe-observability",
  },
  {
    type: "Performance",
    title: "Measure saved work, not magic",
    dek: "The benchmark that matters is avoided prefill under representative shared-prefix workloads.",
    read: "2 min",
    href: "/engineering#evaluation",
  },
];

function Essay({
  kicker,
  title,
  lead,
  next,
  children,
}: {
  kicker: string;
  title: string;
  lead: string;
  next: { href: string; label: string };
  children: ReactNode;
}) {
  return (
    <MarketingChrome active="research">
      <main>
        <header className="mkt-essay-hero">
          <a className="mkt-back" href="/research">
            ← All research
          </a>
          <p className="mkt-eyebrow">{kicker}</p>
          <h1 className="mkt-display mkt-display-essay">{title}</h1>
          <p className="mkt-lead">{lead}</p>
        </header>
        <article className="mkt-essay-layout" style={{ gridTemplateColumns: "minmax(0,1fr)" }}>
          <div className="mkt-essay-body">
            {children}
            <div className="mkt-article-end">
              <span className="mkt-eyebrow" style={{ margin: 0 }}>
                Next
              </span>
              <a href={next.href}>{next.label}</a>
            </div>
          </div>
        </article>
      </main>
    </MarketingChrome>
  );
}

function ResearchIndex() {
  return (
    <MarketingChrome active="research">
      <main>
        <header className="mkt-essay-hero">
          <p className="mkt-eyebrow">Notes</p>
          <h1 className="mkt-display mkt-display-sm">
            Inference is a
            <br />
            systems discipline.
          </h1>
          <p className="mkt-lead">
            Technical notes on the parts of inference routing that become ambiguous under load:
            cache identity, concurrent admission, stream lifetime, telemetry boundaries, and the
            evidence required before a performance result becomes a product claim.
          </p>
        </header>
        <section className="mkt-section" style={{ paddingTop: 0 }}>
          <div className="mkt-post-grid">
            {POSTS.map((p, i) => (
              <a className={i === 0 ? "mkt-post featured" : "mkt-post"} href={p.href} key={p.title}>
                <header>
                  <span>{p.type}</span>
                  <span>{p.read}</span>
                </header>
                <h2>{p.title}</h2>
                <p>{p.dek}</p>
                <span className="read">Read note →</span>
              </a>
            ))}
          </div>
        </section>
      </main>
    </MarketingChrome>
  );
}

function AffinityArticle() {
  return (
    <Essay
      kicker="Architecture · Note 01"
      title="Cache affinity is not a cache guarantee."
      lead="Repeated text is visible to the router. Reusable KV state lives inside the engine. The distance between those facts is the design problem."
      next={{ href: "/engineering", label: "Read the system architecture →" }}
    >
      <section>
        <p>
          The simplest cache-aware router hashes a prompt, remembers the backend that served it, and
          treats that backend as warm on the next request. The map is easy to implement. The claim it
          appears to support is much stronger than the evidence it contains.
        </p>
        <p>
          A successful dispatch proves only that traffic was sent. It does not prove prefill
          completed, that the corresponding blocks survived later cache pressure, or that another
          request will produce the same token sequence. Model revision, tokenizer, chat template,
          adapter state, and isolation policy all participate in cache identity.
        </p>
      </section>
      <section>
        <h2>Affinity is a prediction</h2>
        <p>
          Nostos therefore treats affinity as a scheduling signal, not an engine guarantee. Its
          directory records cumulative prefixes only after a successful upstream outcome, scopes
          observations by backend and model namespace, and caps the number retained. A match can
          change the ranking of healthy backends; it cannot make an unhealthy backend routable.
        </p>
        <div className="mkt-callout">
          <b>Rule</b>
          <p>The router should be useful when its cache model is right—and safe when it is stale.</p>
        </div>
      </section>
      <section>
        <h2>Identity comes before matching</h2>
        <p>
          A production cache key is not “the prompt.” It is the ordered token prefix as interpreted by
          a particular serving stack. Nostos currently uses fixed-size byte chunks as a passive
          approximation. That is useful for detecting repeated leading material, but it must remain
          labeled modeled affinity until an engine adapter supplies native token blocks and residency
          events.
        </p>
        <div className="mkt-diagram">
          <span>Namespace</span>
          <b>model · revision · tokenizer · template · adapter · tenant</b>
          <span>Prefix</span>
          <b>block₀ → block₁ → block₂ → …</b>
        </div>
      </section>
      <section>
        <h2>Bounded memory is product behavior</h2>
        <p>
          An index that grows with every unfamiliar prompt is a memory-exhaustion path. Nostos caps
          observations per backend and removes the least recently observed cumulative key when the
          bound is exceeded. The number describes router metadata capacity; it must not be presented
          as the amount of KV memory available on the GPU.
        </p>
      </section>
      <section>
        <h2>What we still need to prove</h2>
        <p>
          Router latency is necessary but insufficient. The decisive experiment measures recomputed
          prefill and first-byte latency under controlled prefix overlap, concurrency, cache churn,
          and decode length. Random, round-robin, and least-inflight policies provide the baselines.
          Until that artifact is reproducible against a named engine version, the honest output is a
          mechanism and a hypothesis—not a multiplier.
        </p>
      </section>
    </Essay>
  );
}

function QueueArticle() {
  return (
    <Essay
      kicker="Scheduling · Note 02"
      title="A queue depth is not a reservation."
      lead="A load snapshot can be accurate when it is read and obsolete when the request arrives. Capacity has to be claimed, not merely observed."
      next={{ href: "/research/prompt-safe-observability", label: "Observability without prompts →" }}
    >
      <section>
        <p>
          At low traffic, a scheduler can read every backend’s inflight count, choose the smallest
          value, and appear correct. Under a burst, twenty goroutines can all observe the same zero
          before any one of them records ownership.
        </p>
        <p>
          Each request made a locally rational choice from a globally obsolete snapshot. The result
          is a concentrated queue on the worker that looked idle a few microseconds earlier.
        </p>
      </section>
      <section>
        <h2>The herd window</h2>
        <p>
          The bug lives between observation and ownership. The complete scheduler contract is a
          transaction-shaped operation: filter eligible workers, compare immutable snapshots,
          reserve the winner with compare-and-swap, then return a decision that owns that
          reservation. If the claim loses a race, selection must continue against the remaining
          eligible set.
        </p>
        <div className="mkt-chips">
          <span>snapshot</span>
          <span>filter</span>
          <span>score</span>
          <span>atomic reserve</span>
          <span>dispatch</span>
          <b>release</b>
        </div>
        <div className="mkt-callout">
          <b>Rule</b>
          <p>A decision without a reservation is only a recommendation.</p>
        </div>
      </section>
      <section>
        <h2>Locality changes cost, not capacity</h2>
        <p>
          A warm prefix can reduce prefill work, but it does not create decode capacity. Health, model
          compatibility, circuit state, and admission define the eligible set. Locality ranks within
          that set. The current implementation reserves immediately after scoring and returns 429 if
          the selected backend loses the capacity race; integrating reservation into selection is the
          next step.
        </p>
      </section>
      <section>
        <h2>The reservation must outlive headers</h2>
        <p>
          For streaming inference, work continues long after the first response bytes. Releasing
          capacity when headers arrive makes a busy decoder appear idle during the expensive part of
          the request. Nostos holds the reservation until EOF, proxy failure, or cancellation.
        </p>
        <div className="mkt-fail-grid">
          <article>
            <b>Connect fails</b>
            <p>Release before any retry.</p>
          </article>
          <article>
            <b>4xx or 5xx body</b>
            <p>Release after the body closes.</p>
          </article>
          <article>
            <b>Client disconnects</b>
            <p>Cancel upstream and release.</p>
          </article>
          <article>
            <b>Stream succeeds</b>
            <p>Release only at terminal EOF.</p>
          </article>
        </div>
      </section>
    </Essay>
  );
}

function PrivacyArticle() {
  return (
    <Essay
      kicker="Privacy · Note 03"
      title="Observability without prompts."
      lead="Operational telemetry should reconstruct the decision, not reconstruct the conversation."
      next={{ href: "/engineering#privacy", label: "Privacy boundary in context →" }}
    >
      <section>
        <p>
          An operator debugging inference needs answers to a bounded set of questions: which backend
          won, what prefix depth the router modeled, how much work was already inflight, when response
          bytes began, and how the request ended.
        </p>
        <p>None of those questions requires storing the conversation.</p>
      </section>
      <section>
        <h2>Begin with operator questions</h2>
        <p>
          A useful decision event records backend, match counts, score, queue snapshot, a sanitized
          model label, status, stream mode, first-byte timing, and outcome. Together these fields
          explain the path through the policy without retaining request content.
        </p>
        <div className="mkt-fail-grid">
          <article>
            <b>Keep</b>
            <p>backend · bounded counts · score · timings · outcome</p>
          </article>
          <article>
            <b>Discard</b>
            <p>messages · tool bodies · images · raw or reversible prefix keys</p>
          </article>
        </div>
      </section>
      <section>
        <h2>Hashes are not automatically anonymous</h2>
        <p>
          A deterministic hash is not automatically anonymous. Low-entropy prefixes can be guessed,
          and stable hashes enable correlation across events. The dashboard needs match lengths and
          confidence classes—not the internal identities used to find those matches.
        </p>
        <div className="mkt-callout">
          <b>Rule</b>
          <p>The safest prompt field is the one the telemetry schema never accepts.</p>
        </div>
      </section>
      <section>
        <h2>Keep observability off the hot lock</h2>
        <p>
          The dashboard reads a sanitized projection. It does not traverse the live residency
          directory or hold scheduler locks. A slow operator query must not become data-plane
          backpressure.
        </p>
        <div className="mkt-diagram">
          <span>Hot path</span>
          <b>decision + reservation + outcome</b>
          <span>Control plane</span>
          <b>overview · fleet · bounded recent events · metrics</b>
        </div>
      </section>
    </Essay>
  );
}

export default function ResearchPage() {
  const path = window.location.pathname;
  if (path.startsWith("/research/cache-affinity")) return <AffinityArticle />;
  if (path.startsWith("/research/queue-reservations")) return <QueueArticle />;
  if (path.startsWith("/research/prompt-safe-observability")) return <PrivacyArticle />;
  return <ResearchIndex />;
}
