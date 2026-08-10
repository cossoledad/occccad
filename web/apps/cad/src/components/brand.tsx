export function Brand({ compact = false }: { compact?: boolean }) {
  return <div className={`brand-lockup ${compact ? "compact" : ""}`}>
    <span className="brand-symbol">O</span>
    {!compact && <span><strong>occccad</strong><small>Cloud CAD Platform</small></span>}
  </div>;
}
