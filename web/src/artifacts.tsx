// The list of artifacts the overview shows, kept above the pages so going into
// a viewer and back does not start from an empty wall, and so a change made in
// the viewer (who can open it, deleted) is already true when the wall returns.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { listArtifacts, SignedOutError, type Artifact } from "./api";

export type Load = { status: "loading" } | { status: "failed" } | { status: "ready"; list: Artifact[] };

interface Store {
  load: Load;
  /** Fetch the list again; what is shown stays until the answer comes. */
  refresh: () => void;
  /** Put an artifact that was just made at the top of the list. */
  add: (a: Artifact) => void;
  /** Put a changed artifact into the list. */
  replace: (a: Artifact) => void;
  remove: (id: string) => void;
}

const Context = createContext<Store | null>(null);

export function ArtifactsProvider({ children }: { children: ReactNode }) {
  const [load, setLoad] = useState<Load>({ status: "loading" });
  const abort = useRef<AbortController | null>(null);

  const refresh = useCallback(() => {
    abort.current?.abort();
    const controller = new AbortController();
    abort.current = controller;
    listArtifacts(controller.signal).then(
      (list) => {
        setLoad({ status: "ready", list });
      },
      (err: unknown) => {
        if (controller.signal.aborted) return;
        if (err instanceof SignedOutError) {
          window.location.assign("/login?next=" + encodeURIComponent("/"));
          return;
        }
        console.error("load artifacts", err);
        setLoad((was) => (was.status === "ready" ? was : { status: "failed" }));
      },
    );
  }, []);

  useEffect(
    () => () => {
      abort.current?.abort();
    },
    [],
  );

  const add = useCallback((a: Artifact) => {
    setLoad((was) => (was.status === "ready" ? { status: "ready", list: [a, ...was.list.filter((x) => x.id !== a.id)] } : was));
  }, []);
  const replace = useCallback((a: Artifact) => {
    setLoad((was) => (was.status === "ready" ? { status: "ready", list: was.list.map((x) => (x.id === a.id ? a : x)) } : was));
  }, []);
  const remove = useCallback((id: string) => {
    setLoad((was) => (was.status === "ready" ? { status: "ready", list: was.list.filter((x) => x.id !== id) } : was));
  }, []);

  const store = useMemo(() => ({ load, refresh, add, replace, remove }), [load, refresh, add, replace, remove]);
  return <Context value={store}>{children}</Context>;
}

export function useArtifacts(): Store {
  const s = useContext(Context);
  if (!s) throw new Error("useArtifacts outside ArtifactsProvider");
  return s;
}
