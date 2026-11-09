// What a person does with an artifact from a card in the overview or from the
// viewer's "⋯" menu: the same actions with the same messages in both places,
// so they cannot drift. Whether an action is offered is the server's answer
// (canChange, signed in), decided by the caller.
import { useRef } from "react";
import { copyArtifact, deleteArtifact, setPin, type Artifact } from "./api";
import { useArtifacts } from "./artifacts";
import { useT } from "./i18n";
import { useRouter } from "./router";
import { useToast } from "./Toast";

async function toClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch (err) {
    console.error("copy to the clipboard", err);
    return false;
  }
}

export function useArtifactActions() {
  const t = useT();
  const router = useRouter();
  const store = useArtifacts();
  const toast = useToast();
  // The newest toggle of an artifact wins: the answer to an earlier one must not undo it.
  const latestPin = useRef(new Map<string, number>());

  return {
    async copyLink(a: Pick<Artifact, "id">) {
      toast(
        (await toClipboard(location.origin + "/a/" + a.id)) ? { message: t("copy.done") } : { message: t("copy.failed"), error: true },
      );
    },

    /** The copy is the person's own artifact, private: they land on it. */
    async copy(a: Pick<Artifact, "id">, version?: number) {
      try {
        const made = await copyArtifact(a.id, version);
        store.add(made);
        toast({ message: t("viewer.copied") });
        router.go(`/a/${made.id}`);
      } catch (err) {
        console.error("copy artifact", err);
        toast({ message: t("viewer.copyFailed"), error: true });
      }
    },

    /** Deletes it and takes it off the overview; what to show next is the caller's. */
    async remove(a: Pick<Artifact, "id">) {
      await deleteArtifact(a.id);
      store.remove(a.id);
    },

    /**
     * Pins or unpins at once and takes it back, with a message, when the server
     * refuses. `onChange` is told each state of the artifact for callers that
     * hold their own copy; the overview's list is updated here.
     */
    togglePin(a: Artifact, pinned: boolean, onChange?: (next: Artifact) => void) {
      const mine = (latestPin.current.get(a.id) ?? 0) + 1;
      latestPin.current.set(a.id, mine);
      const show = (next: Artifact) => {
        store.replace(next);
        onChange?.(next);
      };
      show({ ...a, pinned });
      setPin(a.id, pinned).then(
        (saved) => {
          if (latestPin.current.get(a.id) === mine) show(saved);
        },
        (err: unknown) => {
          console.error("change the pin", err);
          if (latestPin.current.get(a.id) !== mine) return;
          show(a);
          toast({ message: t("pin.failed"), error: true });
        },
      );
    },
  };
}
