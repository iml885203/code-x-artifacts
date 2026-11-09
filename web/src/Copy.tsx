import { useEffect, useRef, useState, type ReactNode } from "react";
import { Check } from "./icons/chrome";
import { CopyIcon } from "./icons/copy";
import { useT } from "./i18n";

type State = "idle" | "done" | "failed";

/** How long the button says it copied before it goes back to its own label. */
const FEEDBACK_MS = 1600;

/** A button that copies text and says so in its own place: the label turns into "Copied" with a check, then back. */
export function CopyButton({ text, children, className }: { text: () => string; children?: ReactNode; className?: string }) {
  const t = useT();
  const [state, setState] = useState<State>("idle");
  const timer = useRef<number | undefined>(undefined);
  useEffect(
    () => () => {
      window.clearTimeout(timer.current);
    },
    [],
  );

  async function copy() {
    try {
      await navigator.clipboard.writeText(text());
      setState("done");
    } catch {
      setState("failed");
    }
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      setState("idle");
    }, FEEDBACK_MS);
  }

  return (
    <button
      type="button"
      className={className ?? ""}
      data-copy={state}
      onClick={() => {
        void copy();
      }}
    >
      {state === "idle" ? <CopyIcon /> : <Check />}
      <span className="label" aria-live="polite">
        {state === "done" ? t("copy.done") : state === "failed" ? t("copy.failed") : children}
      </span>
    </button>
  );
}
