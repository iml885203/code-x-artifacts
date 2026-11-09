import { Tooltip } from "../../Tooltip";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useRef } from "react";
import { useT } from "../../i18n";
import { keepFocusWhereItWent, keepTriggerToggle } from "../../menuFocus";
import { ArrowIcon, LineIcon, Chevron, OvalIcon, RectangleIcon } from "./icons";
import type { Tool } from "./tools";

interface Props {
  tool: Tool;
  disabled?: boolean;
  onTool: (tool: Tool) => void;
  onPicture: (file: File) => void;
}

/** The shapes in one group, and the image, which the file chooser places. */
export function ShapeMenu({ tool, onTool, onPicture, disabled = false }: Props) {
  const t = useT();
  const chooser = useRef<HTMLInputElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const armed = tool === "rect" || tool === "oval" || tool === "line" || tool === "arrow";
  const label = t("toolbar.shape");
  return (
    <>
      <DropdownMenu.Root modal={false}>
        <Tooltip content={label}><DropdownMenu.Trigger ref={trigger} className="tool tool-wide" aria-label={label} aria-pressed={armed} disabled={disabled}>
          {tool === "oval" ? <OvalIcon /> : tool === "line" ? <LineIcon /> : tool === "arrow" ? <ArrowIcon /> : <RectangleIcon />}
          <Chevron />
        </DropdownMenu.Trigger></Tooltip>
        <DropdownMenu.Portal>
          <DropdownMenu.Content onInteractOutside={(e) => { keepTriggerToggle(e, trigger.current); }} className="menu bar-menu" onCloseAutoFocus={keepFocusWhereItWent} align="start" sideOffset={8}>
            <DropdownMenu.Item disabled={disabled} className="menu-item" onSelect={() => { onTool(tool === "rect" ? "select" : "rect"); }}>
              {t("ops.rectangle")}
              <span className="menu-key" aria-hidden="true">R</span>
            </DropdownMenu.Item>
            <DropdownMenu.Item disabled={disabled} className="menu-item" onSelect={() => { onTool(tool === "oval" ? "select" : "oval"); }}>
              {t("ops.oval")}
              <span className="menu-key" aria-hidden="true">O</span>
            </DropdownMenu.Item>
            <DropdownMenu.Separator className="menu-sep" />
            {(["arrow", "line"] as const).map((shape) => <DropdownMenu.Item disabled={disabled} key={shape} className="menu-item" onSelect={() => { onTool(tool === shape ? "select" : shape); }}>{t(shape === "arrow" ? "toolbar.arrow" : "toolbar.line")}<span className="menu-key" aria-hidden="true">{shape === "arrow" ? "Shift+L" : "L"}</span></DropdownMenu.Item>)}
            <DropdownMenu.Separator className="menu-sep" />
            <DropdownMenu.Item disabled={disabled} className="menu-item" onSelect={() => { chooser.current?.click(); }}>
              {t("ops.image")}
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
      <input
        ref={chooser}
        type="file"
        disabled={disabled}
        accept="image/png,image/jpeg,image/gif,image/webp,image/svg+xml"
        hidden
        aria-label={t("ops.placeImage")}
        onChange={(e) => {
          const file = e.target.files?.[0];
          e.target.value = "";
          if (file && !disabled) onPicture(file);
        }}
      />
    </>
  );
}
