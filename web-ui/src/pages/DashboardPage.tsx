import { useEffect, useMemo, useState } from "react";
import BrandMark from "../components/BrandMark";
import GpuRack, { type GpuNode } from "../components/GpuRack";

type Overview = {
  backends: { healthy: number; total: number };
  inflight: number;
  routing: { requests: number; errors: number; streams: number; cache_hit_requests: number };
  cache: Array<{ backend_id: string; capacity_blocks: number; used_blocks: number }>;
};
type Backend = { id: string; url: string; healthy: boolean; inflight: number };
type Event = {
  id: number;
  timestamp: string;
  model: string;
  backend_id: string;
  matched_blocks: number;
  total_blocks: number;
  score: number;
  status_code: number;
  ttft_ms?: number;
};

export default function DashboardPage() {
  const [overview, setOverview] = useState<Overview | null>(null);
  const [backends, setBackends] = useState<Backend[]>([]);
  const [events, setEvents] = useState<Event[]>([]);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    let live = true;
    let prev = "";
    const load = async () => {
      try {
        const [o, b, e] = await Promise.all([
          fetch("/api/v1/overview"),
          fetch("/api/v1/backends"),
          fetch("/api/v1/routing/recent"),
        ]);
        if (!o.ok || !b.ok || !e.ok) throw Error();
        const [oj, bj, ej] = await Promise.all([o.json(), b.json(), e.json()]);
        if (!live) return;
        const snap = JSON.stringify([oj, bj, ej]);
        if (snap === prev) return;
        prev = snap;
        setOverview(oj);
        setBackends(bj);
        setEvents(ej);
        setConnected(true);
      } catch {
        if (live) setConnected(false);
      }
    };
    void load();
    const timer = setInterval(load, 3000);
    return () => {
      live = false;
      clearInterval(timer);
    };
  }, []);

  const routes = overview?.routing.requests ?? 0;
  const affinity = routes ? Math.round((overview!.routing.cache_hit_requests / routes) * 100) : 0;
  const latest = events[0];

  const nodes: GpuNode[] = useMemo(() => {
    const usage = new Map((overview?.cache ?? []).map((c) => [c.backend_id, c]));
    if (!backends.length) {
      return [{ id: "—", fill: 0, warmBlocks: 0, healthy: false, detail: "no backends" }];
    }
    return backends.map((b) => {
      const c = usage.get(b.id);
      const cap = c?.capacity_blocks ?? 0;
      const used = c?.used_blocks ?? 0;
      const fill = cap > 0 ? used / cap : 0;
      const match = latest?.backend_id === b.id ? latest.matched_blocks : Math.round(fill * 16);
      return {
        id: b.id,
        fill,
        queue: Math.min(1, b.inflight / 8),
        inflight: b.inflight,
        warmBlocks: b.healthy ? Math.min(16, match) : 0,
        missBlocks: latest?.backend_id === b.id ? Math.max(0, 16 - match) : 0,
        totalBlocks: 16,
        healthy: b.healthy,
        selected: latest?.backend_id === b.id,
        detail: b.healthy ? `${used}/${cap || "—"} blk` : "unhealthy",
      };
    });
  }, [backends, overview, latest]);

  const spark = events.slice(0, 24).reverse().map((e) => e.ttft_ms ?? 0);
  const maxTtft = Math.max(1, ...spark);

  return (
    <div className="console">
      <div className="console-bg" aria-hidden />
      <div className="console-grain" aria-hidden />
      <div className="console-shell">
        <header className="console-top">
          <a className="mkt-brand" href="/">
            <span className="mkt-brand-mark">
              <BrandMark />
            </span>
            <span className="mkt-brand-name">Nostos</span>
          </a>
          <nav>
            <a href="#chassis">Chassis</a>
            <a href="#feed">Feed</a>
            <a href="/">Product</a>
          </nav>
          <span className={`console-live ${connected ? "is-up" : "is-down"}`}>
            <i />
            {connected ? "live · 3s" : "api offline"}
          </span>
        </header>

        <header className="console-hero">
          <div>
            <p className="console-eyebrow">Operations</p>
            <h1 className="console-display">What’s resident, what’s reserved.</h1>
            <p className="console-lead">
              Blue is modeled KV occupancy. Ice is admission pressure. The heatmap is prefix density
              on the last winner. Nothing here is the prompt.
            </p>
          </div>
          <aside className="console-aside">
            <div>
              <span className="console-stat-n">
                {overview ? `${overview.backends.healthy}/${overview.backends.total}` : "—"}
              </span>
              <span className="console-stat-l">Routable</span>
            </div>
            <div>
              <span className="console-stat-n">{routes ? `${affinity}%` : "—"}</span>
              <span className="console-stat-l">Warm routes</span>
            </div>
            <p className="console-aside-note">
              {connected
                ? `${overview?.inflight ?? 0} held · ${overview?.routing.streams ?? 0} streams`
                : "Bring the router up on :8080 to light the chassis."}
            </p>
          </aside>
        </header>

        <section className="console-section" id="chassis">
          <div className="console-section-head">
            <div>
              <p className="console-eyebrow">Chassis</p>
              <h2>GPU sleds</h2>
            </div>
            <p>Fill moves when the directory commits after a successful route — not on dispatch.</p>
          </div>
          <div className="studio">
            <div className="studio-bar">
              <span className="studio-meta">modeled residency · prompt-safe</span>
              <span className="studio-meta">
                {latest
                  ? `last → ${latest.backend_id}  ${latest.matched_blocks}/${latest.total_blocks}`
                  : "awaiting traffic"}
              </span>
            </div>
            <div className="studio-body">
              <GpuRack nodes={nodes} />
            </div>
          </div>
        </section>

        <section className="console-section">
          <div className="console-metrics">
            <article className="console-metric">
              <span>Requests</span>
              <b>{routes || "—"}</b>
              <small>this process</small>
            </article>
            <article className="console-metric is-accent">
              <span>Warm hits</span>
              <b>{overview ? overview.routing.cache_hit_requests : "—"}</b>
              <small>matched prefix &gt; 0</small>
            </article>
            <article className="console-metric">
              <span>Errors</span>
              <b>{overview?.routing.errors ?? "—"}</b>
              <small>non-success</small>
            </article>
            <article className="console-metric">
              <span>TTFT tape</span>
              <div className="spark" aria-hidden>
                {spark.length
                  ? spark.map((v, i) => (
                      <i key={i} style={{ height: `${Math.max(8, (v / maxTtft) * 100)}%` }} />
                    ))
                  : Array.from({ length: 12 }).map((_, i) => <i key={i} style={{ height: "8%" }} />)}
              </div>
              <small>recent first-token</small>
            </article>
          </div>
        </section>

        <section className="console-section" id="feed">
          <div className="console-section-head">
            <div>
              <p className="console-eyebrow">Ring</p>
              <h2>Decision feed</h2>
            </div>
            <p>Newest first. Match bar is prefix reuse, not tokens of user text.</p>
          </div>
          <div className="feed">
            {events.length ? (
              events.slice(0, 18).map((e) => {
                const pct = e.total_blocks ? e.matched_blocks / e.total_blocks : 0;
                const warm = e.matched_blocks > 0;
                return (
                  <div className={`feed-row${warm ? " is-warm" : ""}`} key={e.id}>
                    <time>{new Date(e.timestamp).toLocaleTimeString()}</time>
                    <code>{e.model || "default"}</code>
                    <div className="match-bar">
                      <span className="match-track">
                        <i style={{ width: `${Math.round(pct * 100)}%` }} />
                      </span>
                      <b>
                        {e.backend_id} · {e.matched_blocks}/{e.total_blocks}
                      </b>
                    </div>
                    <span className="score">{e.score.toFixed(3)}</span>
                    <span>{e.ttft_ms ? `${e.ttft_ms}ms` : e.status_code}</span>
                  </div>
                );
              })
            ) : (
              <div className="console-empty">
                No routes yet. POST /v1/chat/completions and watch occupancy move.
              </div>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
