export type GpuNode = {
  id: string;
  label?: string;
  healthy?: boolean;
  fill: number;
  queue?: number;
  warmBlocks: number;
  totalBlocks?: number;
  inflight?: number;
  selected?: boolean;
  home?: boolean;
  detail?: string;
  missBlocks?: number;
};

type Props = {
  nodes: GpuNode[];
  onSelect?: (id: string) => void;
  className?: string;
};

export default function GpuRack({ nodes, onSelect, className }: Props) {
  return (
    <div className={className ? `chassis ${className}` : "chassis"}>
      {nodes.map((n) => {
        const fill = Math.max(0, Math.min(1, n.fill));
        const queue = Math.max(0, Math.min(1, n.queue ?? Math.min(1, (n.inflight ?? 0) / 8)));
        const cells = n.totalBlocks ?? 16;
        const warm = Math.max(0, Math.min(cells, n.warmBlocks));
        const miss = Math.max(0, Math.min(cells - warm, n.missBlocks ?? 0));
        const down = n.healthy === false;
        const isHome = n.home === true;
        const wrongHome = n.selected && isHome === false && miss > 0;
        return (
          <article
            key={n.id}
            className={`sled${n.selected ? " is-hit" : ""}${isHome ? " is-home" : ""}${wrongHome ? " is-wrong" : ""}${down ? " is-down" : ""}`}
            onClick={onSelect ? () => onSelect(n.id) : undefined}
            onKeyDown={
              onSelect
                ? (e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      onSelect(n.id);
                    }
                  }
                : undefined
            }
            role={onSelect ? "button" : undefined}
            tabIndex={onSelect ? 0 : undefined}
            title={onSelect ? (down ? "Bring back online" : "Take offline") : undefined}
          >
            <i className={`sled-led${n.selected ? " is-on" : down ? " is-off" : ""}`} />
            <div className="sled-id">
              <strong>
                {n.label ?? n.id}
                {isHome ? <span className="sled-badge">HOME</span> : null}
              </strong>
              <em className={n.selected ? "tag-hit" : down ? "tag-miss" : ""}>
                {down ? "offline" : n.selected ? "reserved" : n.detail ?? `${n.inflight ?? 0} inflight`}
              </em>
            </div>
            <div className="meters">
              <div className="meter">
                <label>KV</label>
                <span className="track kv">
                  <span style={{ width: `${fill * 100}%` }} />
                </span>
                <b>{Math.round(fill * 100)}%</b>
              </div>
              <div className="meter">
                <label>Queue</label>
                <span className="track q">
                  <span style={{ width: `${queue * 100}%` }} />
                </span>
                <b>{n.inflight ?? 0}</b>
              </div>
            </div>
            <div className="heat" aria-hidden>
              {Array.from({ length: cells }).map((_, i) => (
                <i key={i} className={i < warm ? "on" : i < warm + miss ? "debt" : ""} />
              ))}
            </div>
          </article>
        );
      })}
    </div>
  );
}
