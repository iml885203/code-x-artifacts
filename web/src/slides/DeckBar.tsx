import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useRef, type ReactNode } from "react";
import { exportLabel } from "../artifactActionList";
import { Chevron, DownloadIcon, PanelIcon, PlayIcon } from "../design/toolbar/icons";
import { format, useT } from "../i18n";
import { keepFocusWhereItWent, keepTriggerToggle } from "../menuFocus";
import { Tooltip } from "../Tooltip";

/** The zoom of the stage: the slide as large as the stage holds, or a percentage of the 1920 px canvas. */
export type Zoom = "fit" | number;
export const zoomSteps = [50, 75, 100, 150, 200];

interface Props {
  exports: string[];
  onExport: (format: string) => void;
  onPresent: () => void;
  panel: { open: boolean; onToggle: () => void };
  percent: number;
  onZoom: (zoom: Zoom) => void;
}

function BarMenu({ label, trigger, wide = false, children }: { label: string; trigger: ReactNode; wide?: boolean; children: ReactNode }) {
  const button = useRef<HTMLButtonElement>(null);
  return (
    <DropdownMenu.Root modal={false}>
      <Tooltip content={label}>
        <DropdownMenu.Trigger ref={button} className={wide ? "deck-tool deck-tool-wide" : "deck-tool"} aria-label={label}>
          {trigger}
        </DropdownMenu.Trigger>
      </Tooltip>
      <DropdownMenu.Portal>
        <DropdownMenu.Content onInteractOutside={(e) => { keepTriggerToggle(e, button.current); }} onCloseAutoFocus={keepFocusWhereItWent} className="menu bar-menu" align="end" sideOffset={8}>
          {children}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

/** The tools floating at the top right of the stage: download, present, the side panel and zoom; the same for everyone who opens the deck. */
export function DeckBar({ exports, onExport, onPresent, panel, percent, onZoom }: Props) {
  const t = useT();
  return (
    <div className="deck-bar" role="toolbar" aria-label={t("toolbar.view")}>
      {exports.length > 0 && (
        <BarMenu label={t("deck.download")} trigger={<DownloadIcon />}>
          {exports.map((f) => (
            <DropdownMenu.Item key={f} className="menu-item" onSelect={() => { onExport(f); }}>
              {exportLabel(t, f)}
            </DropdownMenu.Item>
          ))}
        </BarMenu>
      )}
      <Tooltip content={t("toolbar.present")}>
        <button type="button" className="deck-tool" aria-label={t("toolbar.present")} onClick={onPresent}>
          <PlayIcon />
        </button>
      </Tooltip>
      <Tooltip content={t("deck.notes")}>
        <button type="button" className="deck-tool" aria-label={t("deck.notes")} aria-pressed={panel.open} onClick={panel.onToggle}>
          <PanelIcon />
        </button>
      </Tooltip>
      <BarMenu label={format(t("toolbar.zoom"), { percent: String(percent) })} wide trigger={<><span className="deck-zoom">{percent}%</span><Chevron /></>}>
        <DropdownMenu.Item className="menu-item" onSelect={() => { onZoom("fit"); }}>
          {t("design.fit")}
        </DropdownMenu.Item>
        {zoomSteps.map((z) => (
          <DropdownMenu.Item key={z} className="menu-item" onSelect={() => { onZoom(z); }}>
            {z}%
          </DropdownMenu.Item>
        ))}
      </BarMenu>
    </div>
  );
}
