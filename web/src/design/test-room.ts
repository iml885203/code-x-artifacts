import * as Y from "yjs";
import { ROOM_MAP } from "./edits";

export const FILE = "frames/hero.html";
export const key = (pid: string, field: string) => `${FILE}|n|${pid}|${field}`;

/**
 * The entries the server writes for a frame (internal/designroom, docs/design-room.md):
 * a body "b", a box "box" with an owned left and top that holds a paragraph "para"
 * with a text, and a heading "h1" that has no position of its own.
 */
export function seed(doc: Y.Doc) {
  const map = doc.getMap(ROOM_MAP);
  doc.transact(() => {
    map.set(`${FILE}|shell|root`, "b");
    const node = (pid: string, parent: string, order: string, extra: Record<string, string> = {}) => {
      map.set(key(pid, "kind"), "el");
      map.set(key(pid, "tag"), "div");
      map.set(key(pid, "parent"), parent);
      map.set(key(pid, "order"), order);
      for (const [field, value] of Object.entries(extra)) map.set(key(pid, field), value);
    };
    node("b", "", "");
    node("box", "b", "V", { "s|left": " 10px", "s|top": "20px", "s|width": "30px" });
    node("para", "box", "V");
    map.set(key("~t1", "kind"), "text");
    map.set(key("~t1", "parent"), "para");
    map.set(key("~t1", "order"), "V");
    map.set(key("~t1", "text"), new Y.Text("Hello"));
    node("h1", "b", "g");
  });
}
