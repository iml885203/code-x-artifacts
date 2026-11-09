import * as Menu from "@radix-ui/react-dropdown-menu";
import { Tooltip } from "../../Tooltip";
import { useT } from "../../i18n";
import { Chevron, NibIcon, PenIcon } from "./icons";
import type { Tool } from "./tools";

export function DrawingMenu({ tool, onTool, disabled = false }: { tool: Tool; onTool: (tool: Tool) => void; disabled?: boolean }) {
  const t = useT();
  return <Menu.Root modal={false}>
    <Tooltip content={t("toolbar.drawingTools")}><Menu.Trigger className="tool tool-wide" aria-label={t("toolbar.drawingTools")} aria-pressed={tool === "pencil" || tool === "pen"} disabled={disabled}>
      {tool === "pen" ? <NibIcon /> : <PenIcon />}<Chevron />
    </Menu.Trigger></Tooltip>
    <Menu.Portal><Menu.Content className="menu bar-menu" sideOffset={8}>
      <Menu.Item disabled={disabled} className="menu-item" onSelect={() => { onTool(tool === "pencil" ? "select" : "pencil"); }}><PenIcon />{t("toolbar.pencil")}<span className="menu-key">P</span></Menu.Item>
      <Menu.Item disabled={disabled} className="menu-item" onSelect={() => { onTool(tool === "pen" ? "select" : "pen"); }}><NibIcon />{t("pen.tool")}<span className="menu-key">Shift+P</span></Menu.Item>
    </Menu.Content></Menu.Portal>
  </Menu.Root>;
}
