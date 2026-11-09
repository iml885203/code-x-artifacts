import { useEffect, useRef, useState, type MouseEvent } from "react";
import { createPortal } from "react-dom";
import { useT } from "../../i18n";

export function useSelectionMenu(enabled: boolean, remove: () => void) {
  const t = useT();
  const [point, setPoint] = useState<{x:number;y:number} | null>(null);
  const menu = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!point || !enabled) return;
    menu.current?.querySelector("button")?.focus();
    const dismiss = (event: PointerEvent) => { if (event.target instanceof Node && !menu.current?.contains(event.target)) setPoint(null); };
    document.addEventListener("pointerdown", dismiss);
    return () => { document.removeEventListener("pointerdown", dismiss); };
  }, [point, enabled]);
  const open = (event: MouseEvent) => { if (enabled) { event.preventDefault(); setPoint({ x: event.clientX, y: event.clientY }); } };
  const surface = enabled && point && createPortal(<div ref={menu} role="menu" aria-label={t("toolbar.selection")} className="menu selection-menu" style={{position:"fixed",left:Math.min(point.x,window.innerWidth-200),top:Math.min(point.y,window.innerHeight-60)}} onKeyDown={(event) => {
    if (["Escape", "Tab"].includes(event.key)) { event.preventDefault(); setPoint(null); }
  }}><button type="button" className="menu-item danger" role="menuitem" onClick={() => { remove(); setPoint(null); }}>{t("toolbar.delete")}</button></div>, document.body);
  return { open, surface };
}
