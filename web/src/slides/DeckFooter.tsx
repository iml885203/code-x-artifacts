import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useRef, type ReactNode } from "react";
import { GridIcon, PlusIcon, SingleIcon } from "../design/toolbar/icons";
import { format, useT } from "../i18n";
import { More } from "../icons/menu";
import { keepFocusWhereItWent, keepTriggerToggle } from "../menuFocus";
import { Tooltip } from "../Tooltip";

/** What an editor can do to the selected slide from the bar. */
export interface FooterEdits {
  busy: boolean;
  onAdd: () => void;
  onDuplicate: () => void;
  onMove: (step: -1 | 1) => void;
  onDelete: () => void;
}

interface Props {
  selected: number;
  count: number;
  view: "single" | "grid";
  onView: (view: "single" | "grid") => void;
  edits: FooterEdits | null;
  /** The thumbnails, in the single view; the grid shows them on the stage instead. */
  children?: ReactNode;
}

function SlideMenu({ selected, count, edits }: { selected: number; count: number; edits: FooterEdits }) {
  const t = useT();
  const trigger = useRef<HTMLButtonElement>(null);
  const label = format(t("deck.actions"), { n: String(selected) });
  return (
    <DropdownMenu.Root modal={false}>
      <Tooltip content={label}>
        <DropdownMenu.Trigger ref={trigger} className="deck-tool" aria-label={label} disabled={edits.busy}>
          <More />
        </DropdownMenu.Trigger>
      </Tooltip>
      <DropdownMenu.Portal>
        <DropdownMenu.Content onInteractOutside={(e) => { keepTriggerToggle(e, trigger.current); }} onCloseAutoFocus={keepFocusWhereItWent} className="menu bar-menu" side="top" align="end" sideOffset={8}>
          <DropdownMenu.Item className="menu-item" onSelect={edits.onDuplicate}>{t("deck.duplicate")}</DropdownMenu.Item>
          <DropdownMenu.Item className="menu-item" disabled={selected <= 1} onSelect={() => { edits.onMove(-1); }}>
            {t("deck.moveEarlier")}
            <span className="menu-key" aria-hidden="true">Alt ←</span>
          </DropdownMenu.Item>
          <DropdownMenu.Item className="menu-item" disabled={selected >= count} onSelect={() => { edits.onMove(1); }}>
            {t("deck.moveLater")}
            <span className="menu-key" aria-hidden="true">Alt →</span>
          </DropdownMenu.Item>
          <DropdownMenu.Separator className="menu-sep" />
          <DropdownMenu.Item className="menu-item danger" disabled={count <= 1} onSelect={edits.onDelete}>
            {t("deck.delete")}
            <span className="menu-key" aria-hidden="true">Del</span>
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

/** The bar under the stage: a new slide, the thumbnails, what can be done to the selected slide, and single or grid view. */
export function DeckFooter({ selected, count, view, onView, edits, children }: Props) {
  const t = useT();
  return (
    <div className="deck-footer">
      {edits && (
        <Tooltip content={t("deck.add")}>
          <button type="button" className="deck-tool" aria-label={t("deck.add")} disabled={edits.busy} onClick={edits.onAdd}>
            <PlusIcon />
          </button>
        </Tooltip>
      )}
      <div className="deck-strip">{children}</div>
      {edits && <SlideMenu selected={selected} count={count} edits={edits} />}
      <div className="deck-views" role="group" aria-label={t("toolbar.view")}>
        <Tooltip content={t("deck.single")}>
          <button type="button" className="deck-tool" aria-label={t("deck.single")} aria-pressed={view === "single"} onClick={() => { onView("single"); }}>
            <SingleIcon />
          </button>
        </Tooltip>
        <Tooltip content={t("deck.grid")}>
          <button type="button" className="deck-tool" aria-label={t("deck.grid")} aria-pressed={view === "grid"} onClick={() => { onView("grid"); }}>
            <GridIcon />
          </button>
        </Tooltip>
      </div>
    </div>
  );
}
