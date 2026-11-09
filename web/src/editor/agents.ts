// An agent the server shows in the room (docs/doc-edit.md, "agents online"): it is
// not a connection and has no caret of its own, so the block it just changed is
// marked with its name for a few seconds, then the mark fades.
import { Plugin, PluginKey } from "prosemirror-state";
import { Decoration, DecorationSet } from "prosemirror-view";
import type { Awareness } from "y-protocols/awareness";

/** How long the mark of a block stays, in milliseconds; the stylesheet fades it over the same time. */
export const markMs = 4000;

interface Announcement {
  name: string;
  block: string;
  /** When this browser first saw the announcement: the server's clock and this one are not the same, so the mark is timed here. */
  seen: number;
}

const refresh = new PluginKey<number>("agent-marks");

function announcements(awareness: Awareness, known: Map<string, number>, now: number): Announcement[] {
  const out: Announcement[] = [];
  const live = new Set<string>();
  for (const [client, state] of awareness.getStates()) {
    const s = state as { user?: { name?: unknown; agent?: unknown }; block?: unknown; at?: unknown };
    if (s.user?.agent !== true || typeof s.user.name !== "string" || typeof s.block !== "string" || s.block === "") continue;
    const key = `${String(client)}:${s.block}:${String(s.at)}`;
    live.add(key);
    if (!known.has(key)) known.set(key, now);
    out.push({ name: s.user.name, block: s.block, seen: known.get(key) ?? now });
  }
  for (const key of known.keys()) if (!live.has(key)) known.delete(key);
  return out;
}

/** Marks the block each announced agent last changed, for markMs after this browser heard of it. */
export function agentMarks(awareness: Awareness): Plugin {
  const known = new Map<string, number>();
  return new Plugin({
    key: refresh,
    state: { init: () => 0, apply: (tr, n) => (tr.getMeta(refresh) === true ? n + 1 : n) },
    props: {
      decorations(state) {
        const now = Date.now();
        const fresh = announcements(awareness, known, now).filter((a) => now - a.seen < markMs);
        if (fresh.length === 0) return DecorationSet.empty;
        const marks: Decoration[] = [];
        state.doc.descendants((node, pos) => {
          const id: unknown = node.attrs.id;
          for (const a of fresh) {
            if (id === a.block) marks.push(Decoration.node(pos, pos + node.nodeSize, { class: "ed-agent-block", "data-agent": a.name }));
          }
          return true;
        });
        return DecorationSet.create(state.doc, marks);
      },
    },
    view(view) {
      let timer: ReturnType<typeof setTimeout> | undefined;
      const redraw = () => {
        view.dispatch(view.state.tr.setMeta(refresh, true));
      };
      const onChange = () => {
        redraw();
        // The mark goes at its time without anyone doing anything.
        clearTimeout(timer);
        timer = setTimeout(redraw, markMs + 50);
      };
      awareness.on("change", onChange);
      return {
        destroy: () => {
          awareness.off("change", onChange);
          clearTimeout(timer);
        },
      };
    },
  });
}
