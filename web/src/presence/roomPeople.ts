import { useSyncExternalStore } from "react";
import type { Person } from "./people";

// The people a live room (the Docs editor, the Design canvas) knows are here. The room's own
// awareness is the source for them, and the header's stack reads this to show them with everyone
// else, so a room never needs a second list of its own people.
let current: Person[] = [];
const listeners = new Set<() => void>();

/** Says who the room shows now; returns what to call when the room goes away. */
export function publishRoomPeople(people: Person[]): () => void {
  current = people;
  for (const l of listeners) l();
  return () => {
    if (current !== people) return;
    current = [];
    for (const l of listeners) l();
  };
}

export function useRoomPeople(): Person[] {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => {
        listeners.delete(l);
      };
    },
    () => current,
  );
}
