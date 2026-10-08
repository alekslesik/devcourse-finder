// Decorative flat illustration: a route between learning cards.
export default function RouteIllustration() {
  return <svg className="routeIllustration" viewBox="0 0 260 170" aria-hidden="true" focusable="false">
    <rect x="3" y="94" width="104" height="65" rx="14" fill="var(--surface)" stroke="var(--line)"/>
    <rect x="157" y="11" width="100" height="65" rx="14" fill="var(--surface)" stroke="var(--line)"/>
    <path d="M107 123h20c23 0-14-80 30-80" fill="none" stroke="var(--accent)" strokeWidth="3" strokeLinecap="round" strokeDasharray="5 8"/>
    <rect x="18" y="108" width="28" height="28" rx="8" fill="var(--accent-soft)"/>
    <path d="m28 116-5 6 5 6m7-12 5 6-5 6" fill="none" stroke="var(--accent)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
    <path d="M56 116h34M56 128h23M18 147h53" stroke="var(--line)" strokeWidth="4" strokeLinecap="round"/>
    <rect x="171" y="24" width="28" height="28" rx="8" fill="var(--success-soft)"/>
    <path d="m179 38 4 4 9-9" fill="none" stroke="var(--success)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
    <path d="M207 33h35M207 45h24M172 63h50" stroke="var(--line)" strokeWidth="4" strokeLinecap="round"/>
    <circle cx="129" cy="73" r="7" fill="#37e6b0"/>
  </svg>;
}
