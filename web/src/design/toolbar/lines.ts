import type { DesignEdits } from "../edits";
import type { Ref } from "../ops/nodes";
import { lastOrder, newPid, nodeKey, rootOf } from "../ops/room";
import { readTree } from "../tree";

export interface LinePoint { x: number; y: number }

/** A vector is ordinary room SVG, so the existing inspector, persistence and own Undo apply. */
export function addLine(edits: DesignEdits, file: string, from: LinePoint, to: LinePoint, arrow: boolean, name: string): Ref | null {
  if (![from.x, from.y, to.x, to.y].every((value) => Number.isFinite(value) && Math.abs(value) <= 8192) || Math.hypot(to.x - from.x, to.y - from.y) < 1) return null;
  const root = rootOf(edits.map, file);
  if (!root) return null;
  const points = [from, to];
  const tips: LinePoint[] = [];
  if (arrow) {
    const angle = Math.atan2(to.y - from.y, to.x - from.x);
    for (const offset of [-0.5, 0.5]) tips.push({ x: to.x - 10 * Math.cos(angle + offset), y: to.y - 10 * Math.sin(angle + offset) });
    points.push(...tips);
  }
  const x = Math.min(...points.map((p) => p.x)) - 2, y = Math.min(...points.map((p) => p.y)) - 2;
  const w = Math.max(...points.map((p) => p.x)) - x + 2, h = Math.max(...points.map((p) => p.y)) - y + 2;
  const round = (n: number) => String(Math.round(n * 1000) / 1000);
  const at = (p: LinePoint) => `${round(p.x - x)} ${round(p.y - y)}`;
  let d = `M${at(from)} L${at(to)}`;
  for (const tip of tips) d += ` M${at(tip)} L${at(to)}`;
  const pid = newPid(), path = newPid();
  const attrs = (pairs: [string, string][]) => JSON.stringify(pairs.map(([key, value]) => ["", key, value]));
  const order = lastOrder(readTree(edits.map, file), root);
  edits.transact((map) => {
    const put = (id: string, fields: Record<string, string>) => { for (const [key, value] of Object.entries(fields)) map.set(nodeKey(file, id, key), value); };
    put(pid, { kind: "el", ns: "svg", tag: "svg", parent: root, order, attrs: attrs([["data-vector", arrow ? "arrow" : "line"], ["data-name", name], ["viewBox", `0 0 ${round(w)} ${round(h)}`], ["preserveAspectRatio", "none"]]), "s|position": "absolute", "s|left": `${round(x)}px`, "s|top": `${round(y)}px`, "s|width": `${round(w)}px`, "s|height": `${round(h)}px` });
    put(path, { kind: "el", ns: "svg", tag: "path", parent: pid, order: "V", attrs: attrs([["d", d], ["stroke", "currentColor"], ["stroke-width", "2"], ["fill", "none"], ["stroke-linecap", "round"], ["stroke-linejoin", "round"]]) });
  });
  return { file, pid };
}
