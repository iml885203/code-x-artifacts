import { useCallback, useEffect, useRef, useState } from "react";
import { formatSize } from "../../format";
import { format, useT } from "../../i18n";
import { ConfirmDialog } from "../ConfirmDialog";
import type { Accepted } from "./rules";

/** Asks the person whether a file the page wants to save may be saved. Resolves true only after their click on "Save". */
export type Ask = (file: Accepted) => Promise<boolean>;

/** What a page that asks while another request waits for the person gets. */
export const BUSY = "another download is waiting for the viewer's answer";

/**
 * The confirmation for pictor.download. One request waits at a time: a page
 * that asks again before the person has answered is refused, so a loop in the
 * page cannot pile up prompts. Each request has its own prompt, so accepting
 * one never allows the next.
 */
export function useDownloadPrompt(pageTitle: string): { ask: Ask; prompt: React.ReactNode } {
  const t = useT();
  const [waiting, setWaiting] = useState<{ file: Accepted; answer: (ok: boolean) => void } | null>(null);
  const current = useRef(waiting);
  useEffect(() => {
    current.current = waiting;
  }, [waiting]);

  const ask = useCallback<Ask>(
    (file) =>
      new Promise((resolve, reject) => {
        if (current.current) {
          reject(new Error(BUSY));
          return;
        }
        const entry = {
          file,
          answer: (ok: boolean) => {
            current.current = null;
            setWaiting(null);
            resolve(ok);
          },
        };
        current.current = entry;
        setWaiting(entry);
      }),
    [],
  );

  // The viewer going away (another artifact, sign-out) is a refusal, not a request left hanging.
  useEffect(
    () => () => {
      current.current?.answer(false);
    },
    [],
  );

  const file = waiting?.file;
  const prompt = (
    <ConfirmDialog
      open={waiting !== null}
      onOpenChange={() => {
        waiting?.answer(false);
      }}
      title={file ? format(t("download.title"), { name: file.name }) : ""}
      hint={file ? format(t("download.hint"), { page: pageTitle, type: file.type, size: formatSize(file.size) }) : ""}
      confirm={t("download.save")}
      failure={t("download.failed")}
      onConfirm={() => {
        waiting?.answer(true);
        return Promise.resolve();
      }}
    />
  );
  return { ask, prompt };
}
