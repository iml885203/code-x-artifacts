import { Tooltip } from "../Tooltip";
import { useT } from "../i18n";
import { displayOf, kindsTable } from "./kinds";
import "./kinds.css";

export type KindMarkSize = "chip" | "square" | "bar";

/**
 * What kind an artifact is, as an icon on its colour: the only way a screen shows a kind. The name is the tooltip
 * and the accessible name, never text on the page (DESIGN.md). A kind this bundle does not know looks like a web
 * page. `chip` sits in a card footer, `square` in a list row, `bar` in the viewer's top bar.
 */
export function KindMark({ kind, size = "chip" }: { kind: string; size?: KindMarkSize }) {
  const t = useT();
  const look = kindsTable[displayOf(kind)];
  const Glyph = look.icon;
  const name = t(look.label);
  return (
    <Tooltip content={name}><span
      className={`kind-mark kind-${size}${look.reserved ? " reserved" : ""}`}
      role="img"
      aria-label={name}
      tabIndex={0}
      style={{ "--kt": `var(--kind-${look.token}-bg)`, "--ki": `var(--kind-${look.token}-fg)` } as React.CSSProperties}
    >
      <Glyph />
    </span></Tooltip>
  );
}
