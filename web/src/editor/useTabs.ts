import { useSyncExternalStore } from "react";
import type * as Y from "yjs";
import { tabsOf, type TabInfo } from "./tabs";

const known = new WeakMap<Y.Doc, { key: string; tabs: TabInfo[] }>();

/** The tabs of the room, in order, drawn again when anyone changes one (a title, the order, a tab added or deleted). */
function snapshot(doc: Y.Doc): TabInfo[] {
  const tabs = tabsOf(doc);
  const key = JSON.stringify(tabs);
  const was = known.get(doc);
  if (was?.key === key) return was.tabs;
  known.set(doc, { key, tabs });
  return tabs;
}

export function useTabs(doc: Y.Doc): TabInfo[] {
  return useSyncExternalStore(
    (onChange) => {
      doc.on("update", onChange);
      return () => {
        doc.off("update", onChange);
      };
    },
    () => snapshot(doc),
  );
}
