import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useRef } from "react";
import { Check } from "../../icons/chrome";
import { format, useT } from "../../i18n";
import { keepFocusWhereItWent, keepTriggerToggle } from "../../menuFocus";
import { Tooltip } from "../../Tooltip";
import type { DesignFrame } from "../manifest";
import { Chevron, ExpandIcon } from "./icons";

interface Props {
  frames: DesignFrame[];
  active: string;
  onActive: (id: string) => void;
  onExpand: (frame: DesignFrame, button: HTMLElement) => void;
  /** Focus inside the strip stops pointer hover from changing which frame it acts on. */
  onFocusWithin: (inside: boolean) => void;
}

export function ArtboardStrip({ frames, active, onActive, onExpand, onFocusWithin }: Props) {
  const t = useT();
  const trigger = useRef<HTMLButtonElement>(null);
  const frame = frames.find((f) => f.id === active) ?? frames[0];
  if (!frame) return null;
  const pick = format(t("design.framePicker"), { name: frame.name });
  const play = format(t("toolbar.expandArtboard"), { name: frame.name });
  return (
    <div
      className="frame-strip"
      role="group"
      aria-label={t("design.frameStrip")}
      onFocus={() => { onFocusWithin(true); }}
      onBlur={(e) => { if (!e.currentTarget.contains(e.relatedTarget)) onFocusWithin(false); }}
    >
      <DropdownMenu.Root modal={false}>
        <Tooltip content={frame.name}>
          <DropdownMenu.Trigger ref={trigger} className="tool tool-wide frame-strip-pick" aria-label={pick}>
            <span className="tool-name">{frame.name}</span>
            <Chevron />
          </DropdownMenu.Trigger>
        </Tooltip>
        <DropdownMenu.Portal>
          <DropdownMenu.Content onInteractOutside={(e) => { keepTriggerToggle(e, trigger.current); }} onCloseAutoFocus={keepFocusWhereItWent} className="menu bar-menu" align="center" side="top" sideOffset={8}>
            <DropdownMenu.RadioGroup value={frame.id} onValueChange={onActive}>
              {frames.map((f) => (
                <DropdownMenu.RadioItem key={f.id} className="menu-item" value={f.id}>
                  <DropdownMenu.ItemIndicator className="menu-check">
                    <Check />
                  </DropdownMenu.ItemIndicator>
                  {f.name}
                </DropdownMenu.RadioItem>
              ))}
            </DropdownMenu.RadioGroup>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
      <Tooltip content={play}>
        <button type="button" className="tool" aria-label={play} onClick={(e) => { onExpand(frame, e.currentTarget); }}>
          <ExpandIcon />
        </button>
      </Tooltip>
    </div>
  );
}
