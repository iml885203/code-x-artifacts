import { useEffect } from "react";
import { recordOpen } from "../api";
import { useArtifacts } from "../artifacts";

/** Tells the server the artifact was opened, once per opening: it counts in the deployment's usage, and for a signed-in person it leads their "Recently opened" list. */
export function useRecordOpen(id: string, signedIn: boolean) {
  const { replace } = useArtifacts();
  useEffect(() => {
    recordOpen(id).then(
      (a) => {
        if (signedIn) replace(a);
      },
      (err: unknown) => {
        console.error("record the open of the artifact", err);
      },
    );
  }, [id, signedIn, replace]);
}
