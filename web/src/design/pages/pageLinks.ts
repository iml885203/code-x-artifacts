import { useCallback, useEffect, useState } from "react";

export function pageLink(address: string, pageId: string): string {
  const url = new URL(address);
  url.searchParams.set("page", pageId);
  return url.href;
}

/** Page identity stays in the URL through version changes and can be copied independently. */
export function usePageLink() {
  const read = () => new URL(window.location.href).searchParams.get("page");
  const [pageId, setPageId] = useState(read);
  useEffect(() => {
    const update = () => { setPageId(read()); };
    window.addEventListener("popstate", update);
    return () => { window.removeEventListener("popstate", update); };
  }, []);
  const select = useCallback((id: string | null) => {
    setPageId(id);
    if (id) window.history.replaceState(window.history.state, "", pageLink(window.location.href, id));
  }, []);
  return [pageId, select] as const;
}
