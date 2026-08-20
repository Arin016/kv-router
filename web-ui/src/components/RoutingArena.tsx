import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import GpuRack, { type GpuNode } from "./GpuRack";

type AlgoId = "rr" | "least" | "p2c" | "hash" | "nostos";
type Family = "agent" | "sys" | "cold";
type Scene = "followup" | "tenants" | "herd";

type Workload = { name: string; family: Family; blocks: number; session: string };

type Gpu = {
  id: string;
  kv: number;
  inflight: number;
  heat: Record<Family, number>;
  healthy: boolean;
};

type Stats = {
  n: number;
  tax: number;
  saved: number;
  lastGpu: string;
  match: number;
  total: number;
};

const CAP = 8;
const CELLS = 16;
const MS_PER_BLOCK = 18;

const SCENES: { id: Scene; label: string; tag: string; setup: string }[] = [
  {
    id: "followup",
    label: "Repeating chat",
    tag: "Same thread, turn 31",
    setup: "The conversation already ran on one GPU. The next turn should go back there.",
  },
  {
    id: "herd",
    label: "Hashed VIP",
    tag: "Sticky session",
    setup: "Hash locks a user to one GPU even when another sled already holds the prefix.",
  },
  {
    id: "tenants",
    label: "New prompts",
    tag: "No shared history",
    setup: "Every request is cold. A honest router does not pretend there is a warm home.",
  },
];

const ALGOS: { id: AlgoId; name: string; tag: string }[] = [
  { id: "rr", name: "Round-robin", tag: "Fair rotation · blind to KV" },
  { id: "least", name: "Least connections", tag: "Shortest queue · often coldest" },
  { id: "p2c", name: "Power of two", tag: "Random pair · warmth is luck" },
  { id: "hash", name: "Consistent hash", tag: "Session pin · not prefix pin" },
  { id: "nostos", name: "Nostos", tag: "Longest observed prefix wins" },
];

function hashStr(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619);
  return Math.abs(h) >>> 0;
}

function sessionLandingOn(label: string, index: number, n = 3): string {
  for (let i = 0; i < 400; i++) {
    const s = `${label}-${i}`;
    if (hashStr(s) % n === index) return s;
  }
  return label;
}

const AGENT_SESSION = sessionLandingOn("conv", 0);
const SYS_SESSION = sessionLandingOn("sys", 0);
const HERD_SESSION = sessionLandingOn("vip", 2);

function seed(): Gpu[] {
  return [
    { id: "gpu-a", kv: 0.62, inflight: 2, heat: { agent: 13, sys: 8, cold: 0 }, healthy: true },
    { id: "gpu-b", kv: 0.12, inflight: 0, heat: { agent: 0, sys: 1, cold: 0 }, healthy: true },
    { id: "gpu-c", kv: 0.84, inflight: 5, heat: { agent: 4, sys: 3, cold: 1 }, healthy: true },
  ];
}

function emptyStats(): Stats {
  return { n: 0, tax: 0, saved: 0, lastGpu: "—", match: 0, total: 0 };
}

function cloneFleet(): Record<AlgoId, Gpu[]> {
  const s = seed();
  return {
    rr: s.map((g) => ({ ...g, heat: { ...g.heat } })),
    least: s.map((g) => ({ ...g, heat: { ...g.heat } })),
    p2c: s.map((g) => ({ ...g, heat: { ...g.heat } })),
    hash: s.map((g) => ({ ...g, heat: { ...g.heat } })),
    nostos: s.map((g) => ({ ...g, heat: { ...g.heat } })),
  };
}

function emptyAll(): Record<AlgoId, Stats> {
  return {
    rr: emptyStats(),
    least: emptyStats(),
    p2c: emptyStats(),
    hash: emptyStats(),
    nostos: emptyStats(),
  };
}

function nextWork(scene: Scene, seq: number): Workload {
  if (scene === "herd") {
    return { name: "VIP follow-up", family: "agent", blocks: 13, session: HERD_SESSION };
  }
  if (scene === "tenants") {
    return { name: "new tenant", family: "cold", blocks: 11, session: `t-${seq}` };
  }
  const pack: Workload[] = [
    { name: "31-turn chat", family: "agent", blocks: 13, session: AGENT_SESSION },
    { name: "tool-call turn", family: "agent", blocks: 10, session: AGENT_SESSION },
    { name: "follow-up", family: "agent", blocks: 13, session: AGENT_SESSION },
    { name: "system prompt", family: "sys", blocks: 8, session: SYS_SESSION },
    { name: "RAG prefix", family: "sys", blocks: 8, session: SYS_SESSION },
  ];
  return pack[seq % pack.length];
}

function prefixHome(gpus: Gpu[], work: Workload): { id: string; blocks: number } {
  let best = gpus[0];
  let bestHeat = best.heat[work.family];
  for (const g of gpus.slice(1)) {
    if (g.heat[work.family] > bestHeat) {
      best = g;
      bestHeat = g.heat[work.family];
    }
  }
  return { id: best.id, blocks: Math.min(work.blocks, bestHeat) };
}

function prefixScore(g: Gpu, work: Workload): number {
  const match = Math.min(work.blocks, g.heat[work.family]);
  const hit = work.blocks ? match / work.blocks : 0;
  return hit * 1.15 - (g.inflight / CAP) * 0.7 - (g.kv > 0.88 ? 0.22 : 0);
}

function pick(algo: AlgoId, work: Workload, gpus: Gpu[], rr: { i: number }): string {
  const eligible = gpus.filter((g) => g.healthy && g.inflight < CAP);
  const pool = eligible.length ? eligible : gpus.filter((g) => g.healthy);
  if (!pool.length) return gpus[0].id;

  if (algo === "rr") {
    rr.i = (rr.i + 1) % pool.length;
    return pool[rr.i].id;
  }
  if (algo === "least") {
    return pool.reduce((a, b) => (a.inflight <= b.inflight ? a : b)).id;
  }
  if (algo === "p2c") {
    const a = pool[Math.floor(Math.random() * pool.length)];
    let b = pool[Math.floor(Math.random() * pool.length)];
    if (pool.length > 1) {
      let guard = 0;
      while (b.id === a.id && guard++ < 8) b = pool[Math.floor(Math.random() * pool.length)];
    }
    return (a.inflight <= b.inflight ? a : b).id;
  }
  if (algo === "hash") {
    const target = gpus[hashStr(work.session) % gpus.length];
    if (target.healthy && target.inflight < CAP) return target.id;
    return pool[0].id;
  }
  let best = pool[0];
  let bestScore = prefixScore(best, work);
  for (const g of pool.slice(1)) {
    const s = prefixScore(g, work);
    if (s > bestScore) {
      best = g;
      bestScore = s;
    }
  }
  return best.id;
}

function applyHit(gpus: Gpu[], id: string, work: Workload): Gpu[] {
  return gpus.map((g) => {
    if (g.id !== id) {
      return {
        ...g,
        inflight: Math.max(0, g.inflight - (Math.random() < 0.32 ? 1 : 0)),
        kv: Math.max(0.08, g.kv - 0.005),
      };
    }
    const match = Math.min(work.blocks, g.heat[work.family]);
    const nextHeat = Math.min(CELLS, Math.max(match, Math.ceil(match + (work.blocks - match) * 0.4)));
    return {
      ...g,
      inflight: Math.min(CAP, g.inflight + 1),
      kv: Math.min(0.97, g.kv + 0.04),
      heat: { ...g.heat, [work.family]: nextHeat },
    };
  });
}

function reusePct(s: Stats): number {
  const t = s.saved + s.tax;
  return t ? (s.saved / t) * 100 : 0;
}

function hopTax(st: Stats): number {
  return Math.max(0, st.total - st.match) * MS_PER_BLOCK;
}

export default function RoutingArena() {
  const [hintScene, setHintScene] = useState<Scene | null>(null);
  const [scene, setScene] = useState<Scene>("followup");
  const [focus, setFocus] = useState<AlgoId>("rr");
  const [fleet, setFleet] = useState(cloneFleet);
  const [stats, setStats] = useState(emptyAll);
  const [work, setWork] = useState<Workload>(() => nextWork("followup", 0));
  const seq = useRef(0);
  const rr = useRef<Record<AlgoId, { i: number }>>({
    rr: { i: -1 },
    least: { i: -1 },
    p2c: { i: -1 },
    hash: { i: -1 },
    nostos: { i: -1 },
  });
  const sceneRef = useRef(scene);
  const fleetRef = useRef(fleet);
  const statsRef = useRef(stats);
  sceneRef.current = scene;
  fleetRef.current = fleet;
  statsRef.current = stats;

  const reset = useCallback((nextScene: Scene) => {
    sceneRef.current = nextScene;
    seq.current = 0;
    rr.current = { rr: { i: -1 }, least: { i: -1 }, p2c: { i: -1 }, hash: { i: -1 }, nostos: { i: -1 } };
    const f = cloneFleet();
    const s = emptyAll();
    fleetRef.current = f;
    statsRef.current = s;
    setScene(nextScene);
    setFleet(f);
    setStats(s);
    setWork(nextWork(nextScene, 0));
    setFocus("rr");
  }, []);

  const fire = useCallback(() => {
    const w = nextWork(sceneRef.current, seq.current++);
    const prev = fleetRef.current;
    const nextFleet = { ...prev };
    const nextStats = { ...statsRef.current };
    for (const a of ALGOS) {
      const gpus = prev[a.id];
      const id = pick(a.id, w, gpus, rr.current[a.id]);
      const g = gpus.find((x) => x.id === id);
      const match = Math.min(w.blocks, g?.heat[w.family] ?? 0);
      const miss = Math.max(0, w.blocks - match);
      const st = nextStats[a.id];
      nextFleet[a.id] = applyHit(gpus, id, w);
      nextStats[a.id] = {
        n: st.n + 1,
        tax: st.tax + miss * MS_PER_BLOCK,
        saved: st.saved + match * MS_PER_BLOCK,
        lastGpu: id,
        match,
        total: w.blocks,
      };
    }
    fleetRef.current = nextFleet;
    statsRef.current = nextStats;
    setWork(w);
    setFleet(nextFleet);
    setStats(nextStats);
  }, []);

  useEffect(() => {
    const id = window.setInterval(() => fire(), 1400);
    return () => window.clearInterval(id);
  }, [fire]);

  useEffect(() => {
    fire();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const sceneMeta = SCENES.find((s) => s.id === (hintScene ?? scene))!;
  const meta = ALGOS.find((a) => a.id === focus)!;
  const st = stats[focus];
  const nostosSt = stats.nostos;
  const gpus = fleet[focus];
  const home = prefixHome(gpus, work);
  const taxMs = hopTax(st);
  const nostosTax = hopTax(nostosSt);
  const wastedVsNostos = Math.max(0, taxMs - nostosTax);
  const wentHome = st.lastGpu === home.id && home.blocks >= work.blocks * 0.5;
  const hasWarmHome = home.blocks >= work.blocks * 0.4;
  const missBlocks = Math.max(0, st.total - st.match);

  const nodes: GpuNode[] = useMemo(() => {
    return gpus.map((g) => {
      const match = Math.min(work.blocks, g.heat[work.family]);
      const debt = Math.max(0, work.blocks - match);
      const hit = g.id === st.lastGpu;
      const isHome = g.id === home.id && hasWarmHome;
      return {
        id: g.id,
        fill: g.kv,
        inflight: g.inflight,
        queue: g.inflight / CAP,
        warmBlocks: Math.round((match / Math.max(work.blocks, 1)) * CELLS),
        missBlocks: Math.round((debt / Math.max(work.blocks, 1)) * CELLS),
        totalBlocks: CELLS,
        healthy: g.healthy,
        selected: hit,
        home: isHome,
        detail: isHome
          ? `home · ${home.blocks} blk`
          : hit
            ? `sent here · ${match}/${work.blocks}`
            : `${match} blk here`,
      };
    });
  }, [gpus, work, st.lastGpu, home.id, home.blocks, hasWarmHome]);

  const punch =
    !hasWarmHome ? (
      <>Cold request — every policy pays full prefill.</>
    ) : wentHome ? (
      <>
        <strong>{meta.name}</strong> sent turn 31 to <strong>{st.lastGpu}</strong> — where{" "}
        <strong>{home.blocks}</strong> blocks already live. <span className="arena-punch-good">~{taxMs}ms tax.</span>
      </>
    ) : (
      <>
        Prefix lives on <strong>{home.id}</strong>, but <strong>{meta.name}</strong> sent it to{" "}
        <strong>{st.lastGpu}</strong>.{" "}
        <span className="arena-punch-bad">
          Recomputing {missBlocks} blocks · ~{taxMs}ms
        </span>
      </>
    );

  return (
    <div className="arena">
      <div className="arena-guide" aria-hidden>
        <span className="on">① See HOME</span>
        <span className="on">② Hover a policy</span>
        <span>③ Read the tax</span>
      </div>

      <div className="arena-scenes" role="radiogroup" aria-label="Traffic shape">
        {SCENES.map((s) => (
          <button
            key={s.id}
            type="button"
            role="radio"
            aria-checked={scene === s.id}
            className={scene === s.id ? "on" : hintScene === s.id ? "is-hint" : ""}
            onMouseEnter={() => setHintScene(s.id)}
            onMouseLeave={() => setHintScene(null)}
            onFocus={() => setHintScene(s.id)}
            onBlur={() => setHintScene(null)}
            onClick={() => reset(s.id)}
          >
            <strong>{s.label}</strong>
            <em>{s.tag}</em>
          </button>
        ))}
      </div>

      <p className="arena-punch" role="status">
        {punch}
      </p>

      <div className="arena-setup">
        <div className="arena-setup-card">
          <span className="arena-setup-label">This turn</span>
          <strong>{work.name}</strong>
          <em>{work.blocks} KV blocks</em>
        </div>
        <div className="arena-setup-card is-home-card">
          <span className="arena-setup-label">Prefix home</span>
          {hasWarmHome ? (
            <>
              <strong className="is-home">{home.id}</strong>
              <em>{home.blocks}/{work.blocks} blocks resident</em>
            </>
          ) : (
            <>
              <strong>none</strong>
              <em>no shared prefix yet</em>
            </>
          )}
        </div>
        <p className="arena-setup-note">{sceneMeta.setup}</p>
      </div>

      {hasWarmHome && focus !== "nostos" ? (
        <div className="arena-vs">
          <div className="arena-vs-col is-bad">
            <span>{meta.name}</span>
            <strong>{st.lastGpu}</strong>
            <em>~{taxMs}ms · {st.match}/{st.total} reused</em>
          </div>
          <div className="arena-vs-mid">vs</div>
          <div className="arena-vs-col is-good">
            <span>Nostos</span>
            <strong>{nostosSt.lastGpu}</strong>
            <em>~{nostosTax}ms · {nostosSt.match}/{nostosSt.total} reused</em>
          </div>
          {wastedVsNostos > 0 ? (
            <p className="arena-vs-save">
              Nostos saves <strong>~{wastedVsNostos}ms</strong> on this hop
            </p>
          ) : null}
        </div>
      ) : null}

      <div className="arena-split">
        <div className="arena-plot" role="list" aria-label="Prefix reuse by algorithm">
          <p className="arena-metric">
            Hover a row. <strong>Higher bar = more prefix kept in KV</strong> across live traffic.
          </p>
          {ALGOS.map((a) => {
            const s = stats[a.id];
            const reuse = reusePct(s);
            const ours = a.id === "nostos";
            const hop = hopTax(s);
            return (
              <button
                key={a.id}
                type="button"
                role="listitem"
                className={`arena-row${ours ? " is-ours" : ""}${focus === a.id ? " is-focus" : ""}`}
                onMouseEnter={() => setFocus(a.id)}
                onFocus={() => setFocus(a.id)}
              >
                <span className="arena-lab">
                  <strong>{a.name}</strong>
                  <em>{a.tag}</em>
                </span>
                <span className="arena-track">
                  <span style={{ width: `${reuse}%` }} />
                </span>
                <span className="arena-val">
                  <b>{Math.round(reuse)}%</b>
                  <small>~{hop}ms now</small>
                </span>
              </button>
            );
          })}
          {focus !== "nostos" ? (
            <button type="button" className="arena-try" onClick={() => setFocus("nostos")}>
              See what Nostos does →
            </button>
          ) : null}
        </div>

        <aside className="arena-verdict">
          <p className="arena-verdict-kicker">{focus === "nostos" ? "Nostos" : "Classic load balancer"}</p>
          <h3>
            {wentHome
              ? "Routed to the GPU that already computed this."
              : hasWarmHome
                ? "Sent the request somewhere else."
                : "Nothing warm to route to."}
          </h3>

          <div className="arena-route">
            <div className={wentHome ? "arena-route-step is-good" : "arena-route-step is-bad"}>
              <span>Policy sent to</span>
              <strong>{st.lastGpu}</strong>
              <em>
                {st.match}/{st.total} blocks reused · ~{taxMs}ms prefill
              </em>
            </div>
            {hasWarmHome ? (
              <div className={`arena-route-step${home.id === st.lastGpu ? " is-good" : " is-home-ref"}`}>
                <span>Prefix lived on</span>
                <strong>{home.id}</strong>
                <em>{home.id === st.lastGpu ? "correct sled" : `${home.blocks} blocks ignored`}</em>
              </div>
            ) : null}
          </div>

          {focus !== "nostos" && wastedVsNostos > 0 ? (
            <p className="arena-delta">
              You paid <strong>~{wastedVsNostos}ms</strong> extra because {meta.name} did not ask where
              the prefix already was.
            </p>
          ) : focus === "nostos" && wentHome ? (
            <p className="arena-delta is-win">
              Nostos matched observed prefix depth, reserved capacity, and forwarded to{" "}
              <strong>{home.id}</strong>.
            </p>
          ) : null}
        </aside>
      </div>

      <div className="arena-rack">
        <div className="arena-rack-bar">
          <span className="arena-rack-led" />
          <strong>RACK-01</strong>
          <em>{meta.name}</em>
          <code>
            {hasWarmHome ? `HOME ${home.id}` : "no home"} · SENT {st.lastGpu}
          </code>
        </div>
        <GpuRack nodes={nodes} className="is-live" />
        <p className="arena-legend">
          <b className="leg-home">HOME</b> = observed prefix · <b className="leg-on">electric</b> = reused ·{" "}
          <b className="leg-debt">ice</b> = paid again
        </p>
      </div>
    </div>
  );
}
