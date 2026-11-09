import type { ReactNode } from "react";

function Icon({ children }: { children: ReactNode }) {
  return <svg className="icon canvas-icon" viewBox="0 0 24 24" aria-hidden="true">{children}</svg>;
}

export const BulletsIcon = () => <Icon><path d="M9 6h11M9 12h11M9 18h11M4.5 6h.01M4.5 12h.01M4.5 18h.01" /></Icon>;
export const TableIcon = () => <Icon><rect x="3" y="5" width="18" height="14" rx="1" /><path d="M3 10h18M3 14.5h18M9 5v14M15 5v14" /></Icon>;
export const ShapeIcon = () => <Icon><rect x="4" y="4" width="10" height="10" rx="1" /><circle cx="15" cy="15" r="5" /></Icon>;
export const UndoIcon = () => <Icon><path d="M9 14L4 9l5-5" /><path d="M4 9h10a6 6 0 010 12h-3" /></Icon>;
export const RedoIcon = () => <Icon><path d="M15 14l5-5-5-5" /><path d="M20 9H10a6 6 0 000 12h3" /></Icon>;
