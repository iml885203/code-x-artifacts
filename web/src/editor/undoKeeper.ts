// A tab's undo history outlives its view: the editor opens one view per tab, and going to another tab and back must not
// throw away what Undo would take back. y-prosemirror makes an UndoManager for each view and destroys it with the view, so the
// manager is made here, kept for as long as the doc is open, and handed to each view of the tab (a view destroys it,
// which a kept one only takes as "forget what listened to me").
import { ySyncPluginKey } from "y-prosemirror";
import * as Y from "yjs";
import { tabFragmentName } from "./tabs";

class KeptUndoManager extends Y.UndoManager {
  /** Called by the view that is going away: the manager stays, and what that view listened for goes. */
  override destroy(): void {
    (Object.getPrototypeOf(Y.UndoManager.prototype) as { destroy: (this: unknown) => void }).destroy.call(this);
  }

  /** The doc is closed, or the tab is gone. */
  release(): void {
    super.destroy();
  }
}

export class UndoKeeper {
  private readonly managers = new Map<string, KeptUndoManager>();
  private doc: Y.Doc | null = null;

  /** The undo history of a tab: the same manager every time it is asked for. Tracks what the tab's own editor writes, as y-prosemirror's does. */
  of(doc: Y.Doc, tab: string): Y.UndoManager {
    if (this.doc !== doc) {
      // The room was rebuilt: a history of the old document has nothing to undo in the new one.
      this.release();
      this.doc = doc;
    }
    let manager = this.managers.get(tab);
    if (!manager) {
      manager = new KeptUndoManager(doc.getXmlFragment(tabFragmentName(tab)), {
        trackedOrigins: new Set([ySyncPluginKey]),
        captureTransaction: (tr) => tr.meta.get("addToHistory") !== false,
      });
      this.managers.set(tab, manager);
    }
    return manager;
  }

  /** Lets go of the histories of tabs that are not in the doc any more. */
  keepOnly(tabs: string[]): void {
    for (const [tab, manager] of this.managers) {
      if (!tabs.includes(tab)) {
        manager.release();
        this.managers.delete(tab);
      }
    }
  }

  release(): void {
    this.keepOnly([]);
  }
}
