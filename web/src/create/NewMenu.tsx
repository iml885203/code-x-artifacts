// "New" in the overview's bar: start an empty document, design or board without an agent.
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useId, useRef, type ReactNode } from "react";
import { useArtifacts } from "../artifacts";
import { Board, Brush, FileText } from "../icons/kind";
import { Chevron } from "../icons/nav";
import { Plus } from "../icons/chrome";
import { useT, type Key } from "../i18n";
import { useRouter } from "../router";
import { useToast } from "../Toast";
import { startNew, type Startable } from "./api";
import "./create.css";
import { keepFocusWhereItWent } from "../menuFocus";

// Each kind says what it is for: a board and a design both open on a canvas, and
// the line is how a person tells which one they need.
const entries = [
  { kind: "doc", label: "create.document", purpose: "create.documentFor", title: "create.untitledDocument", icon: <FileText /> },
  { kind: "design", label: "create.design", purpose: "create.designFor", title: "create.untitledDesign", icon: <Brush /> },
  { kind: "board", label: "create.board", purpose: "create.boardFor", title: "create.untitledBoard", icon: <Board /> },
] as const satisfies readonly { kind: Startable; label: Key; purpose: Key; title: Key; icon: ReactNode }[];

export function NewMenu() {
  const t = useT();
  const router = useRouter();
  const store = useArtifacts();
  const toast = useToast();
  // One at a time: a second choice while the first is being made would leave an empty artifact behind.
  const busy = useRef(false);
  const ids = useId();

  async function start(kind: Startable, title: string) {
    if (busy.current) return;
    busy.current = true;
    try {
      const made = await startNew(kind, title);
      store.add(made);
      router.go(`/a/${made.id}`);
    } catch (err) {
      console.error("start something new", err);
      toast({ message: t("create.failed"), error: true });
    } finally {
      busy.current = false;
    }
  }

  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger className="primary new-button">
        <Plus />
        {t("create.new")}
        <Chevron />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className="menu new-menu" onCloseAutoFocus={keepFocusWhereItWent} align="end" sideOffset={8}>
          {entries.map((e) => (
            <DropdownMenu.Item
              key={e.kind}
              className="menu-item new-item"
              aria-labelledby={`${ids}-${e.kind}`}
              aria-describedby={`${ids}-${e.kind}-for`}
              onSelect={() => {
                void start(e.kind, t(e.title));
              }}
            >
              <span className={`new-kind new-kind-${e.kind}`}>{e.icon}</span>
              <span className="new-words">
                <span id={`${ids}-${e.kind}`}>{t(e.label)}</span>
                <span id={`${ids}-${e.kind}-for`} className="new-purpose">{t(e.purpose)}</span>
              </span>
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
