export function Brand({ compact = false }: { compact?: boolean }) {
  return <div className={`brand-lockup ${compact ? "compact" : ""}`}>
    <span className="brand-symbol" aria-hidden="true">
      <svg viewBox="0 0 32 32" role="img">
        <path className="brand-frame" d="M5 6.5h16.5L27 12v13.5H10.5L5 20z" />
        <path className="brand-profile" d="M10 11h9l3 3v7h-9l-3-3z" />
        <path className="brand-axis" d="M7 25V8m-2 2 2-2 2 2M4 22h22m-2-2 2 2-2 2" />
      </svg>
    </span>
    {!compact && <span><strong>occccad</strong><small>Cloud CAD Platform</small></span>}
  </div>;
}
