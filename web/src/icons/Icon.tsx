// Drawn in one 16px grid with a 1.5px square-capped stroke, like the mark's
// pixels: no icon font, no emoji. One module per icon family under icons/,
// imported directly (no barrel): a new icon is a new file or one line in its family.
import type { ReactNode } from "react";

export function Icon({ children }: { children: ReactNode }) {
  return (
    <svg className="icon" viewBox="0 0 16 16" aria-hidden="true">
      {children}
    </svg>
  );
}
