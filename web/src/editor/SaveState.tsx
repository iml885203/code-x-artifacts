// What the doc says about being saved, beside the top bar's meta: from the room's persistence state.
import { useT, type Key } from "../i18n";

/** Saving: sent, not yet stored. Saved: stored. Offline: no connection, nothing waiting. Unsaved: no connection, and edits that have not been stored. */
export type SaveState = "saving" | "saved" | "offline" | "unsaved";

const words: Record<SaveState, Key> = { saving: "editor.saving", saved: "editor.saved", offline: "editor.offline", unsaved: "editor.unsaved" };

/** The state of a connection and of the unsaved edits, as a person reads it. */
export function saveStateOf(connection: string, saved: boolean): SaveState {
  if (connection === "offline") return saved ? "offline" : "unsaved";
  return saved ? "saved" : "saving";
}

export function SaveStateLabel({ state }: { state: SaveState }) {
  const t = useT();
  return (
    <span className={`ed-saved ed-saved-${state}`} role="status">
      {t(words[state])}
    </span>
  );
}
