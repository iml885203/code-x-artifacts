import { useEffect, useReducer, useRef, useState, type DragEvent, type KeyboardEvent, type ReactNode } from "react";
import { Eye, EyeOff } from "../icons/visibility";
import { format, useT } from "../i18n";
import type { DesignEdits } from "./edits";
import { elementOrder, placement, type FrameTree, type TreeNode } from "./tree";
import { keyOf, type Selected } from "./useSelection";

interface Props {
  edits: DesignEdits;
  frames: { id: string; name: string; file: string }[];
  selected: Selected[];
  /** Whether this person may rename, hide and reorder; everyone else only reads the tree and selects from it. */
  editable: boolean;
  onSelect: (hit: Selected, additive: boolean) => void;
  /** What manages the page's frames, above the elements (pages/FramesPanel). */
  frameList?: ReactNode;
}

type Zone = "before" | "inside" | "after";
interface Drag {
  file: string;
  pid: string;
}

/** What a row says about an element: its name, or its tag with the start of its text. */
const labelOf = (n: TreeNode) => n.name || `<${n.tag}>`;

/**
 * The elements of each frame on the page as a tree, from the room: click
 * selects, double-click (or F2) renames, the eye hides, and dragging (or
 * Alt+Up and Alt+Down) reorders and re-parents. Each of those is a step of the
 * person's own history, like every other edit.
 */
export function Outline({ edits, frames, selected, editable, onSelect, frameList }: Props) {
  const t = useT();
  const [, redraw] = useReducer((n: number) => n + 1, 0);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [drag, setDrag] = useState<Drag | null>(null);
  const [over, setOver] = useState<{ key: string; zone: Zone } | null>(null);
  const list = useRef<HTMLDivElement>(null);
  const cancelled = useRef(false);
  useEffect(() => edits.onChange(redraw), [edits]);

  const last = selected[selected.length - 1];
  const lastKey = last ? keyOf(last) : "";
  useEffect(() => {
    if (!lastKey) return;
    const row = list.current?.querySelector(`[data-row="${CSS.escape(lastKey)}"]`);
    if (row && "scrollIntoView" in row) row.scrollIntoView({ block: "nearest" });
  }, [lastKey]);

  const chosen = new Set(selected.map(keyOf));
  const trees = new Map<string, FrameTree>(frames.map((f) => [f.id, edits.tree(f.file)]));

  /** Where a drop on a row puts the dragged element, or null when that is refused. */
  const target = (frame: { id: string; file: string }, row: TreeNode, zone: Zone) => {
    const tree = trees.get(frame.id);
    if (!tree || drag?.file !== frame.file) return null;
    if (zone === "inside") return placement(tree, drag.pid, row.pid, null) && { parent: row.pid, before: null };
    const siblings = elementOrder(tree).get(row.parent) ?? [];
    const before = zone === "before" ? row.pid : (siblings[siblings.indexOf(row.pid) + 1] ?? null);
    return placement(tree, drag.pid, row.parent, before) && { parent: row.parent, before };
  };

  const zoneOf = (e: DragEvent<HTMLElement>, root: boolean): Zone => {
    const r = e.currentTarget.getBoundingClientRect();
    const at = r.height > 0 ? (e.clientY - r.top) / r.height : 0.5;
    if (root || (at >= 0.25 && at <= 0.75)) return "inside";
    return at < 0.25 ? "before" : "after";
  };

  const renderRow = (frame: { id: string; name: string; file: string }, tree: FrameTree, row: TreeNode, level: number): React.ReactNode[] => {
    const key = keyOf({ frameId: frame.id, pid: row.pid });
    const isRoot = row.pid === tree.root;
    const can = editable && !isRoot;
    const label = labelOf(row);
    const snippet = row.name ? "" : row.children.map((c) => tree.nodes.get(c)).find((c) => c?.kind === "text" && c.text.trim() !== "")?.text.trim().slice(0, 24) ?? "";
    const siblings = elementOrder(tree).get(row.parent) ?? [];
    const at = siblings.indexOf(row.pid);
    const onKey = (e: KeyboardEvent<HTMLButtonElement>) => {
      if (!can) return;
      if (e.key === "F2") {
        e.preventDefault();
        setRenaming(key);
      } else if (e.altKey && e.key === "ArrowUp" && at > 0) {
        e.preventDefault();
        edits.move(frame.file, row.pid, row.parent, siblings[at - 1] ?? null);
      } else if (e.altKey && e.key === "ArrowDown" && at >= 0 && at < siblings.length - 1) {
        e.preventDefault();
        edits.move(frame.file, row.pid, row.parent, siblings[at + 2] ?? null);
      }
    };
    const dropping = over?.key === key ? over.zone : null;
    const me = (
      <li
        key={key}
        role="treeitem"
        aria-level={level + 1}
        aria-selected={chosen.has(key)}
        data-row={key}
        className={`orow${row.hidden ? " is-hidden" : ""}${chosen.has(key) ? " is-selected" : ""}${dropping ? ` drop-${dropping}` : ""}`}
        style={{ paddingLeft: 8 + level * 14 }}
        draggable={can && renaming !== key}
        onDragStart={(e) => {
          e.dataTransfer.effectAllowed = "move";
          e.dataTransfer.setData("text/plain", row.pid);
          setDrag({ file: frame.file, pid: row.pid });
        }}
        onDragEnd={() => {
          setDrag(null);
          setOver(null);
        }}
        onDragOver={(e) => {
          if (!drag) return;
          const zone = zoneOf(e, isRoot);
          if (!target(frame, row, zone)) {
            e.dataTransfer.dropEffect = "none";
            if (over) setOver(null);
            return;
          }
          e.preventDefault();
          e.dataTransfer.dropEffect = "move";
          if (over?.key !== key || over.zone !== zone) setOver({ key, zone });
        }}
        onDrop={(e) => {
          const to = drag && target(frame, row, zoneOf(e, isRoot));
          setOver(null);
          setDrag(null);
          if (!drag || !to) return;
          e.preventDefault();
          edits.move(frame.file, drag.pid, to.parent, to.before);
        }}
      >
        {renaming === key ? (
          <input
            className="orow-name"
            aria-label={format(t("design.rename"), { name: label })}
            defaultValue={row.name}
            placeholder={row.tag}
            maxLength={120}
            ref={(el) => el?.focus()}
            onKeyDown={(e) => {
              if (e.key === "Enter") e.currentTarget.blur();
              else if (e.key === "Escape") {
                cancelled.current = true;
                e.currentTarget.blur();
              }
            }}
            onBlur={(e) => {
              if (!cancelled.current) edits.rename(frame.file, row.pid, e.currentTarget.value);
              cancelled.current = false;
              setRenaming(null);
            }}
          />
        ) : (
          <button
            type="button"
            className="orow-main"
            onClick={(e) => {
              onSelect({ frameId: frame.id, pid: row.pid }, e.shiftKey);
            }}
            onDoubleClick={() => {
              if (can) setRenaming(key);
            }}
            onKeyDown={onKey}
          >
            <span className={row.name ? "orow-label named" : "orow-label mono"}>{label}</span>
            {snippet && <span className="orow-text">{snippet}</span>}
          </button>
        )}
        {can && (
          <button
            type="button"
            className="orow-eye"
            aria-pressed={row.hidden}
            aria-label={format(t(row.hidden ? "design.show" : "design.hide"), { name: label })}
            title={format(t(row.hidden ? "design.show" : "design.hide"), { name: label })}
            onClick={() => {
              edits.setHidden(frame.file, row.pid, !row.hidden);
            }}
          >
            {row.hidden ? <EyeOff /> : <Eye />}
          </button>
        )}
      </li>
    );
    const inside = row.children.flatMap((c) => {
      const n = tree.nodes.get(c);
      return n?.kind === "el" ? renderRow(frame, tree, n, level + 1) : [];
    });
    return [me, ...inside];
  };

  return (
    <nav className="outline" aria-label={t("design.outline")}>
      <h2 className="outline-title">{t("design.outline")}</h2>
      <div className="outline-body" ref={list}>
        {frameList}
        {frames.map((f) => {
          const tree = trees.get(f.id);
          const root = tree?.nodes.get(tree.root);
          return (
            <section key={f.id} aria-label={f.name}>
              {frames.length > 1 && <h3 className="outline-frame">{f.name}</h3>}
              <ul role="tree" className="outline-tree">
                {tree && root ? renderRow(f, tree, root, 0) : null}
              </ul>
            </section>
          );
        })}
      </div>
    </nav>
  );
}
