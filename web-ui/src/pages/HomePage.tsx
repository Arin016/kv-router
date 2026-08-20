import { useState } from "react";
import NostosMascot from "../components/NostosMascot";
import RoutingArena from "../components/RoutingArena";
import MarketingChrome from "./MarketingChrome";

const STAGES = [
  { n: "01", label: "Inspect", hint: "Read only what routing needs" },
  { n: "02", label: "Match", hint: "Find the longest observed prefix" },
  { n: "03", label: "Reserve", hint: "Claim backend capacity atomically" },
  { n: "04", label: "Observe", hint: "Update affinity after success" },
];

const LAWS = [
  {
    id: "name",
    verb: "Scope",
    title: "Define what ‘the same’ means",
    stake: "Text alone is not cache identity",
    body: "KV reuse depends on the serving context: model, tokenizer, template, adapter, engine format, and isolation boundary. Nostos keeps affinity inside an explicit namespace instead of treating repeated text as proof of reuse.",
  },
  {
    id: "admit",
    verb: "Admit",
    title: "A route still needs capacity",
    stake: "Selection is not ownership",
    body: "A cache match can make a backend cheaper, not infinitely available. Nostos atomically claims a concurrency slot before it forwards the request; if the slot is gone, the request is not silently overcommitted.",
  },
  {
    id: "hold",
    verb: "Hold",
    title: "Streaming work ends at the body",
    stake: "Headers are only the beginning",
    body: "The backend remains busy while it decodes. The reservation is held until the response body completes, fails, or the client cancels—not merely until upstream headers arrive.",
  },
  {
    id: "see",
    verb: "Explain",
    title: "Observe decisions, not conversations",
    stake: "Operational detail without prompt storage",
    body: "The event ring records the chosen backend, match depth, score, status, stream mode, and time to first bytes. It does not accept message content or raw prefix identifiers.",
  },
] as const;

export default function HomePage() {
  const [law, setLaw] = useState<(typeof LAWS)[number]["id"]>("name");
  const active = LAWS.find((l) => l.id === law) ?? LAWS[0];

  return (
    <MarketingChrome active="product">
      <main>
        <section className="mkt-hero">
          <div>
            <p className="mkt-eyebrow">νόστος · homecoming</p>
            <h1 className="mkt-display">Route each conversation to the GPU that already knows its context.</h1>
            <p className="mkt-lead">
              Long prompts are expensive to process more than once. Nostos sits in front of an
              OpenAI-compatible inference fleet, estimates which backend has already processed the
              longest part of a request, and routes with both cache locality and current load in
              view.
            </p>
            <div className="mkt-cta-row">
              <a href="/dashboard" className="mkt-btn mkt-btn-primary">
                Open console
              </a>
              <a href="#arena" className="mkt-btn mkt-btn-ghost">
                Compare routing policies
              </a>
            </div>
            <ul className="mkt-usp-row">
              <li>One chat endpoint for the fleet</li>
              <li>Atomic concurrency reservations</li>
              <li>Prompt-safe route telemetry</li>
            </ul>
            <div className="mkt-hero-proof">
              <div>
                <span>Load balancer</span>
                <strong>Spins the wheel</strong>
                <em>Ignores where KV already lives</em>
              </div>
              <div className="mkt-hero-proof-arrow" aria-hidden>
                →
              </div>
              <div className="is-nostos">
                <span>Nostos</span>
                <strong>Sends it home</strong>
                <em>Routes to observed prefix depth</em>
              </div>
            </div>
          </div>
          <div className="mkt-hero-visual">
            <div className="mkt-hero-plane">
              <div className="mkt-hero-mascot">
                <NostosMascot size={220} />
              </div>
              <div className="mkt-hero-ribbon">
                {STAGES.map((s, i) => (
                  <span
                    key={s.label}
                    className={`mkt-ribbon-chip${i === 3 ? " is-live" : ""}${i < 3 ? " is-done" : ""}`}
                  >
                    <em>{s.n}</em>
                    {s.label}
                  </span>
                ))}
              </div>
            </div>
          </div>
        </section>

        <section className="mkt-strip" aria-label="Hot path">
          <div className="mkt-strip-inner">
            {STAGES.map((s) => (
              <div key={s.label} className="mkt-strip-item">
                <span className="mkt-strip-n">{s.n}</span>
                <strong>{s.label}</strong>
                <span>{s.hint}</span>
              </div>
            ))}
          </div>
        </section>

        <section id="arena" className="mkt-section">
          <div className="mkt-section-head">
            <p className="mkt-eyebrow">One request, five policies</p>
            <h2 className="mkt-h2">The useful work already exists. The routing policy decides whether you keep it.</h2>
            <p className="mkt-section-lead">
              A follow-up request shares a long prefix with an earlier turn. The backend marked HOME
              has observed that prefix before. Compare policies to see which ones preserve locality
              and which ones send the request to a cold worker.
            </p>
          </div>
          <RoutingArena />
        </section>

        <section id="vs" className="mkt-section">
          <div className="mkt-section-head">
            <p className="mkt-eyebrow">Four routing rules</p>
            <h2 className="mkt-h2">Locality is useful only when its limits are explicit.</h2>
            <p className="mkt-section-lead">
              Nostos treats cache affinity as evidence, not certainty. It scopes the match, accounts
              for load, reserves capacity, and records enough of the outcome to explain the decision.
            </p>
          </div>
          <div className="laws" role="tablist" aria-label="Routing laws">
            <div className="laws-rail" aria-hidden>
              {LAWS.map((l) => (
                <i key={l.id} className={l.id === law ? "on" : ""} />
              ))}
            </div>
            <div className="laws-tabs">
              {LAWS.map((l) => (
                <button
                  key={l.id}
                  type="button"
                  role="tab"
                  aria-selected={l.id === law}
                  className={l.id === law ? "on" : ""}
                  onClick={() => setLaw(l.id)}
                >
                  <span>{l.verb}</span>
                  <strong>{l.title}</strong>
                </button>
              ))}
            </div>
            <article className="laws-panel" role="tabpanel">
              <p className="laws-stake">{active.stake}</p>
              <h3>{active.title}</h3>
              <p>{active.body}</p>
            </article>
          </div>
          <p className="mkt-continue">
            <a href="/engineering">Mechanism, invariants, known limits →</a>
          </p>
        </section>

        <section id="host" className="mkt-section mkt-host">
          <p className="mkt-eyebrow">Process</p>
          <h2 className="mkt-h2">Add a routing layer without replacing your inference servers.</h2>
          <p className="mkt-section-lead">
            Run one binary in front of the fleet, register each backend, and keep clients on the
            familiar chat-completions contract.
          </p>
          <div className="mkt-host-steps">
            <div className="mkt-host-step">
              <span>1</span>
              <div>
                <strong>Build</strong>
                <pre className="mkt-cli">
                  <code>{`cd web-ui && npm i && npm run build
go build -o kv-router ./cmd/kv-router`}</code>
                </pre>
              </div>
            </div>
            <div className="mkt-host-step">
              <span>2</span>
              <div>
                <strong>Point at GPUs</strong>
                <pre className="mkt-cli">
                  <code>{`backends:
  - id: gpu-a
    url: http://10.0.0.11:8000
    cache_capacity_blocks: 4096`}</code>
                </pre>
              </div>
            </div>
            <div className="mkt-host-step">
              <span>3</span>
              <div>
                <strong>Serve</strong>
                <pre className="mkt-cli">
                  <code>{`./kv-router --config config.yaml
# console → /dashboard`}</code>
                </pre>
              </div>
            </div>
          </div>
        </section>

        <section className="mkt-section mkt-close">
          <h2 className="mkt-h2">Preserve the work your inference fleet has already done.</h2>
          <div className="mkt-cta-row">
            <a href="/dashboard" className="mkt-btn mkt-btn-primary">
              Open console
            </a>
            <a href="/research" className="mkt-btn mkt-btn-ghost">
              Field notes
            </a>
          </div>
        </section>
      </main>
    </MarketingChrome>
  );
}
