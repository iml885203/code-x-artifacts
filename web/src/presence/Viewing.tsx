import { AvatarStack } from "./AvatarStack";
import { mergePeople } from "./people";
import { useRoomPeople } from "./roomPeople";
import { useViewers } from "./useViewers";

/** The top right of a viewer: who has this artifact open, readers and agents included. A live room's own people come from its awareness and are merged in, so each person shows once. */
export function Viewing({ id, limit, signedIn = true }: { id: string; limit?: number; signedIn?: boolean }) {
  const people = useViewingPeople(id, signedIn);
  return <AvatarStack people={people} {...(limit === undefined ? {} : { limit })} />;
}

/** Keep the heartbeat alive even when the phone's viewer list is folded away. A signed-out page beats as its browser's guest. */
export function useViewingPeople(id: string, signedIn = true) {
  const viewers = useViewers(id, signedIn);
  const room = useRoomPeople();
  return mergePeople(room, viewers);
}
