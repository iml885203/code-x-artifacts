import { useEffect, useId, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { createPortal } from "react-dom";
import type { EditorView } from "prosemirror-view";
import type { EditorState } from "prosemirror-state";
import { useT } from "../i18n";
import type { ToolContext } from "./feature";
import { insertionCommand, matchingInsertions } from "./insertions";

interface Props {
  state: EditorState;
  ctx: ToolContext;
  onClose: () => void;
  onCancel?: (query: string) => void;
  anchor?: RefObject<EditorView | null>;
  trigger?: RefObject<HTMLButtonElement | null>;
}

/** Queries belong to discovery UI; only a chosen command or a cancelled literal slash changes the document. */
export function InsertionMenu({ state, ctx, onClose, onCancel, anchor, trigger }: Props) {
  const t = useT();
  const [at, setAt] = useState<{ left: number; top: number } | null>(null);
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  const root = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const id = useId();
  const items = matchingInsertions(state, query, t);
  const active = Math.min(index, Math.max(0, items.length - 1));
  useEffect(() => {
    root.current?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: "nearest" });
  }, [active, query]);
  const cancel = () => { onCancel?.(query); onClose(); };
  const ready = !anchor || at !== null;
  useEffect(() => { if (ready) input.current?.focus(); }, [ready]);
  useLayoutEffect(() => {
    const view = anchor?.current;
    if (!view) return;
    const place = () => {
      const menu = root.current;
      if (!menu || view.isDestroyed) return;
      const caret = view.coordsAtPos(view.state.selection.from);
      const margin = 8;
      const left = Math.max(margin, Math.min(caret.left, window.innerWidth - menu.offsetWidth - margin));
      const below = caret.bottom + margin;
      const top = below + menu.offsetHeight <= window.innerHeight - margin ? below : Math.max(margin, caret.top - menu.offsetHeight - margin);
      setAt({ left, top });
    };
    place();
    window.addEventListener("resize", place);
    document.addEventListener("scroll", place, true);
    return () => { window.removeEventListener("resize", place); document.removeEventListener("scroll", place, true); };
  }, [anchor, state, query]);
  useEffect(() => {
    const outside = (event: PointerEvent) => {
      if (event.target instanceof Node && !root.current?.contains(event.target) && !trigger?.current?.contains(event.target)) cancel();
    };
    document.addEventListener("pointerdown", outside);
    return () => { document.removeEventListener("pointerdown", outside); };
  });
  const choose = (at: number) => {
    const item = items[at];
    if (!item) return;
    onClose();
    ctx.run(insertionCommand(item.command));
  };
  const menu = (
    <div className={anchor ? "ed-insertion-menu ed-slash-palette" : "ed-insertion-menu"} ref={root}
      style={anchor ? (at ?? { visibility: "hidden" }) : undefined}>
      <input ref={input} role="combobox" aria-label={t("editor.findBlock")} aria-expanded="true" aria-controls={id} aria-autocomplete="list" aria-activedescendant={items[active] ? `${id}-${items[active].id}` : undefined}
        value={query} placeholder={t("editor.findBlock")} onChange={(event) => { setQuery(event.target.value); setIndex(0); }}
        onKeyDown={(event) => {
          if (event.nativeEvent.isComposing) return;
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            setIndex(items.length === 0 ? 0 : (active + (event.key === "ArrowDown" ? 1 : -1) + items.length) % items.length);
          } else if (event.key === "Enter") { event.preventDefault(); choose(active); }
          else if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); cancel(); }
          else if (event.key === "Tab") { cancel(); }
        }} />
      <div id={id} role="listbox" aria-label={t("editor.insertBlock")}>
        {items.map((item, at) => (
          <button key={item.id} id={`${id}-${item.id}`} type="button" role="option" aria-selected={active === at} className="quiet ed-insertion-option" tabIndex={-1}
            onMouseDown={(event) => { event.preventDefault(); }} onPointerMove={() => { setIndex(at); }} onClick={() => { choose(at); }}>
            <span className="ed-insertion-face" aria-hidden="true">{item.face}</span><span>{t(item.label)}</span>
          </button>
        ))}
        {items.length === 0 && <p className="ed-insertion-empty" role="status">{t("editor.noBlocks")}</p>}
      </div>
      <p className="ed-insertion-hint">{t("editor.slashHint")}</p>
    </div>
  );
  return anchor ? createPortal(menu, document.body) : menu;
}
