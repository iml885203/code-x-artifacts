import { useEffect } from "react";

/** The tab's title: "name · CodeXArtifacts", or just "CodeXArtifacts" for the overview. */
export function useDocumentTitle(name: string) {
  useEffect(() => {
    document.title = name ? `${name} · CodeXArtifacts` : "CodeXArtifacts";
  }, [name]);
}
