/** The brand geometry frames empty states without implying missing content. */
export function Easel({ lit = true }: { lit?: boolean }) {
  return <svg className="easel" data-lit={lit} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M3 3H15V9L9 15H3Z" /><path d="M16 10H22V22H10V16Z" /></svg>;
}
