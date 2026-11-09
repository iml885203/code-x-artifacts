import { Tooltip } from "../../Tooltip";
import { useRef } from "react";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { format, useT } from "../../i18n";
import { keepFocusWhereItWent, keepTriggerToggle } from "../../menuFocus";
import { Chevron } from "./icons";

interface Props {
  percent: number;
  onReset: () => void;
  onFit: () => void;
  onStep?: (direction: 1 | -1) => void;
  onPreset?: (zoom: number) => void;
}

/** The zoom level, which opens the two ways back: 100% and the whole page in view. */
export function ZoomMenu({ percent, onReset, onFit, onStep, onPreset }: Props) {
  const t = useT();
  const trigger = useRef<HTMLButtonElement>(null);
  const label = format(t("toolbar.zoom"), { percent: String(percent) });
  const choose = (action: () => void) => {
    trigger.current?.focus();
    action();
  };
  return (
    <DropdownMenu.Root modal={false}>
      <Tooltip content={label}><DropdownMenu.Trigger ref={trigger} className="tool tool-wide zoom-level" aria-label={label}>
        {percent}%
        <Chevron />
      </DropdownMenu.Trigger></Tooltip>
      <DropdownMenu.Portal>
        <DropdownMenu.Content onInteractOutside={(e) => { keepTriggerToggle(e, trigger.current); }} className="menu bar-menu" onCloseAutoFocus={keepFocusWhereItWent} align="end" sideOffset={8}>
          <DropdownMenu.Label className="menu-label">{label}</DropdownMenu.Label>
          <DropdownMenu.Item className="menu-item" onSelect={() => { choose(() => { onStep?.(1); }); }}>{t("design.zoomIn")}</DropdownMenu.Item>
          <DropdownMenu.Item className="menu-item" onSelect={() => { choose(() => { onStep?.(-1); }); }}>{t("design.zoomOut")}</DropdownMenu.Item>
          <DropdownMenu.Item className="menu-item" onSelect={() => { choose(onFit); }}>{t("design.fit")}</DropdownMenu.Item>
          <DropdownMenu.Separator className="menu-sep" />
          {[25, 50, 75, 100, 150, 200].map((value) => <DropdownMenu.Item key={value} className="menu-item" onSelect={() => { choose(() => { if (onPreset) onPreset(value / 100); else if (value === 100) onReset(); }); }}>{value}%</DropdownMenu.Item>)}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
