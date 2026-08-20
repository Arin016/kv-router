export default function BrandMark({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden>
      <rect width="32" height="32" rx="8" fill="#0d11ff" />
      <path
        d="M7.5 21a8.5 8.5 0 1 1 9.2-12"
        fill="none"
        stroke="#8fbfff"
        strokeWidth="1.7"
        strokeLinecap="round"
      />
      <path fill="#fff" d="M16.2 7.2 26 12.4 19.6 14.2 22.4 21.2 15.4 13.4Z" />
    </svg>
  );
}
