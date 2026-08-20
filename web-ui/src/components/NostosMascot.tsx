type Props = { size?: number };

export default function NostosMascot({ size = 220 }: Props) {
  return (
    <div className="nostos-mascot" style={{ width: size, height: size }} aria-hidden>
      <svg viewBox="0 0 220 220" width={size} height={size}>
        <circle cx="110" cy="118" r="78" fill="none" stroke="rgba(143,191,255,0.28)" strokeWidth="1.5" />
        <circle cx="110" cy="118" r="78" fill="none" stroke="#8fbfff" strokeWidth="1.5" strokeDasharray="6 10" className="nostos-orbit" />

        <g className="nostos-home">
          <rect x="86" y="148" width="48" height="28" rx="6" fill="#0d0d0d" stroke="rgba(255,255,255,0.2)" />
          <rect x="92" y="154" width="10" height="16" rx="2" fill="#0d11ff" />
          <rect x="105" y="154" width="10" height="16" rx="2" fill="rgba(255,255,255,0.12)" />
          <rect x="118" y="154" width="10" height="16" rx="2" fill="rgba(143,191,255,0.45)" />
        </g>

        <g className="nostos-bird">
          <path fill="#0d11ff" d="M110 48 L148 72 L122 78 L132 104 L104 74Z" />
          <path fill="#8fbfff" d="M122 78 L148 72 L138 84Z" />
          <circle cx="128" cy="70" r="2.2" fill="#fff" />
        </g>
      </svg>
      <p className="nostos-mascot-cap">νόστος</p>
    </div>
  );
}
