import { useEffect } from "react";
import type { Awareness } from "y-protocols/awareness";
import type { Connection } from "../editor/provider";
import { useT } from "../i18n";
import { peopleOfAwareness } from "./awarenessPeople";
import { publishRoomPeople } from "./roomPeople";
import "./presence.css";

export const connectionLabel = { connecting: "editor.connecting", live: "editor.live", reconnecting: "editor.reconnecting", offline: "editor.offline" } as const;

/**
 * What a live room (the Docs editor, the Design canvas) adds to the header's stack of who is here:
 * the people its awareness holds, which the stack merges with the server's list. It draws only the
 * connection light; the avatars are the stack's, so there is one look and one list.
 */
export function RoomPresence({ awareness, self, state }: { awareness: Awareness; self: number; state: Connection }) {
  const t = useT();
  useEffect(() => {
    let leave = publishRoomPeople(peopleOfAwareness(awareness, self));
    const changed = () => {
      leave();
      leave = publishRoomPeople(peopleOfAwareness(awareness, self));
    };
    awareness.on("change", changed);
    return () => {
      awareness.off("change", changed);
      leave();
    };
  }, [awareness, self]);
  return <span className={`ed-conn ed-conn-${state}`} role="status" aria-label={t(connectionLabel[state])} aria-atomic="true">{t(connectionLabel[state])}</span>;
}
