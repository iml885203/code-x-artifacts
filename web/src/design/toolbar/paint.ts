import type { Target } from "../controller";
import type { DesignEdits } from "../edits";
import type { ColorChange } from "../properties/controls";
import { paintColor } from "../properties/palette";
import { formatColor, parseColor } from "../properties/css";
import { preservePriority, writeFillNone, writeFillReset, type Declarations } from "../properties/model";
import { nodeKey } from "../ops/room";
import { readTokens } from "../tokens";
import { parseAttrs, readTree } from "../tree";

function vectorNodes(edits: DesignEdits, target: Target): string[] {
  const tree = readTree(edits.map, target.file);
  const result: string[] = [];
  const visit = (pid: string) => {
    const node = tree.nodes.get(pid);
    if (node?.kind !== "el") return;
    if (["path", "line", "rect", "circle", "ellipse", "polygon", "polyline"].includes(node.tag)) result.push(pid);
    for (const child of node.children) visit(child);
  };
  visit(target.pid);
  return result;
}

export function paintOf(edits: DesignEdits, target: Target, kind: "fill" | "stroke") {
  const vector = target.report.tag === "svg" || target.report.tag === "path";
  if (vector) {
    const pid = vectorNodes(edits, target)[0] ?? target.pid;
    const attrs = parseAttrs(edits.map.get(nodeKey(target.file, pid, "attrs")));
    return { vector: true, value: attrs.find(([, key]) => key === kind)?.[2] ?? "none" };
  }
  const prop = kind === "fill" ? "background-color" : "border-color";
  return { vector: false, value: edits.frame(target.file).styles.get(target.pid)?.get(prop) ?? target.report.computed[kind === "fill" ? prop : "border-top-color"] ?? "transparent" };
}

export function paintColorOf(edits: DesignEdits, target: Target, kind: "fill" | "stroke") {
  const source = paintOf(edits, target, kind).value.replace(/\s*!important\s*$/i, "");
  return paintColor(source, readTokens(edits)) ?? parseColor(target.report.computed[source === "currentColor" ? "color" : kind === "fill" ? "background-color" : "border-top-color"] ?? "") ?? { r: 0, g: 0, b: 0, a: 0 };
}

export function writePaint(edits: DesignEdits, targets: Target[], kind: "fill" | "stroke", change: ColorChange | string | null) {
  // CSS and SVG changes share a transaction, including mixed selections.
  edits.transact((map) => {
    for (const target of targets) {
      if (!edits.frame(target.file).alive.has(target.pid) || edits.frame(target.file).root === target.pid) continue;
      const current = paintOf(edits, target, kind);
      const value = typeof change === "function" ? formatColor(change(paintColorOf(edits, target, kind))) : change;
      if (current.vector) {
        for (const pid of vectorNodes(edits, target)) {
          const key = nodeKey(target.file, pid, "attrs");
          const attrs = parseAttrs(map.get(key)).filter(([, name]) => name !== kind);
          if (value !== null) attrs.push(["", kind, value]);
          const next = JSON.stringify(attrs);
          if (map.get(key) !== next) map.set(key, next);
        }
      } else {
        const prop = kind === "fill" ? "background-color" : "border-color";
        const style: Declarations = value === "none" ? kind === "fill" ? writeFillNone() : { [prop]: "transparent !important" } : value === null && kind === "fill" ? writeFillReset() : { [prop]: preservePriority(prop, value ?? "", target.report.inline) };
        edits.apply([{ file: target.file, pid: target.pid, style }]);
      }
    }
  });
}

export function strokeWidthOf(edits: DesignEdits, target: Target): string {
  if (target.report.tag !== "svg" && target.report.tag !== "path") return target.report.computed["border-top-width"]?.replace(/px$/, "") ?? "0";
  const pid = vectorNodes(edits, target)[0];
  return pid ? parseAttrs(edits.map.get(nodeKey(target.file, pid, "attrs"))).find(([, name]) => name === "stroke-width")?.[2] ?? "1" : "1";
}

export function writeStrokeWidth(edits: DesignEdits, targets: Target[], width: number): boolean {
  if (!Number.isFinite(width) || width < 0.5 || width > 64) return false;
  edits.transact((map) => {
    for (const target of targets) {
      if (target.report.tag === "svg" || target.report.tag === "path") {
        for (const pid of vectorNodes(edits, target)) {
          const key = nodeKey(target.file, pid, "attrs");
          const attrs = parseAttrs(map.get(key)).filter(([, name]) => name !== "stroke-width");
          attrs.push(["", "stroke-width", String(width)]);
          const next = JSON.stringify(attrs);
          if (map.get(key) !== next) map.set(key, next);
        }
      } else edits.apply([{ file: target.file, pid: target.pid, style: { "border-width": preservePriority("border-width", `${String(width)}px`, target.report.inline), ...(target.report.computed["border-top-style"] === "none" ? { "border-style": "solid" } : {}) } }]);
    }
  });
  return true;
}
