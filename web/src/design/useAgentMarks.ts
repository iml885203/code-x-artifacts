import { useEffect, useState } from "react";
import type { Live } from "./useRoom";
import type { Peer } from "./presence";
import { announcements, freshMarks, markMs } from "./presenceAgent";

/** The elements agents changed a moment ago, as peers whose selection is what they changed; each goes after markMs. */
export function useAgentMarks(live: Live | null): Peer[] {
  const [marks, setMarks] = useState<Peer[]>([]);
  const awareness = live?.awareness;
  useEffect(() => {
    if (!awareness) return;
    const seen = new Map<string, number>();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const read = () => {
      clearTimeout(timer);
      const fresh = freshMarks(announcements(awareness), seen, Date.now());
      setMarks((before) => (before.length === 0 && fresh.length === 0 ? before : fresh));
      // They go at their time without anyone doing anything.
      if (fresh.length > 0) timer = setTimeout(read, markMs + 50);
    };
    read();
    awareness.on("change", read);
    return () => {
      awareness.off("change", read);
      clearTimeout(timer);
    };
  }, [awareness]);
  return awareness ? marks : [];
}
