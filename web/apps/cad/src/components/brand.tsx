export function Brand({ compact = false }: { compact?: boolean }) {
  return <div className={`brand-lockup ${compact ? "compact" : ""}`}>
    <span className="brand-symbol" aria-hidden="true"><img src={`${import.meta.env.BASE_URL}brand.svg`} alt="" /></span>
    {!compact && <span><strong>occccad</strong><small>Parametric Design</small></span>}
  </div>;
}
