import { useEffect } from "react";
import { useArtifacts } from "./artifacts";
import type { Boot } from "./boot";
import { Connect } from "./connect/Connect";
import { ErrorPage } from "./ErrorPage";
import { Gallery } from "./gallery/Gallery";
import { Kit } from "./kit/Kit";
import { Login } from "./login/Login";
import { artifactIdOf, useRouter } from "./router";
import { Viewer } from "./viewer/Viewer";
import { fullPageView } from "./viewer/pageView";

/** One entry per page the shell serves; the server names the page in the boot block. */
export function App({ boot }: { boot: Boot }) {
  const router = useRouter();
  // The overview and the viewer are one app and the address is whichever of them is showing.
  const inApp = boot.page === "gallery" || boot.page === "viewer";
  const here = inApp ? router.path : boot.here;
  const page = { ...boot, here };
  if (inApp) return artifactIdOf(router.path) === "" ? <Gallery boot={page} /> : <ViewerPage boot={page} id={artifactIdOf(router.path)} />;
  switch (boot.page) {
    case "login":
      return <Login boot={page} />;
    case "connect":
      return <Connect boot={page} />;
    case "kit":
      return <Kit boot={page} />;
    case "error":
      return <ErrorPage boot={page} />;
    default:
      throw new Error(`unknown page ${boot.page}`);
  }
}

/** The artifact comes from the list when the overview has it (so a change made in the viewer is in it), else from the page the server sent. */
function ViewerPage({ boot, id }: { boot: Boot; id: string }) {
  const { load } = useArtifacts();
  const router = useRouter();
  const fromList = load.status === "ready" ? load.list.find((a) => a.id === id) : undefined;
  const artifact = fromList ?? (boot.artifact?.id === id ? boot.artifact : undefined);
  const full = fullPageView(router.path);
  useEffect(() => {
    // An address this page was not given the artifact for (forward to one deleted since): let the server answer it.
    if (!artifact) window.location.assign(router.path);
  }, [artifact, router.path]);
  return artifact ? <Viewer key={`${artifact.id}/${full ? `full/${String(full.version)}` : "viewer"}`} boot={boot} artifact={artifact} fullPage={full !== null} initialVersion={full?.version ?? null} /> : null;
}
