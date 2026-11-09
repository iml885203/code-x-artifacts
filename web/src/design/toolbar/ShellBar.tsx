import { Tooltip } from "../../Tooltip";
import type { ReactNode } from "react";
import { useT } from "../../i18n";
import { FrameIcon, PanelIcon, Hand, Pointer, PlayIcon, TextIcon } from "./icons";
import { PageMenu, type PageMenuProps } from "./PageMenu";
import { ShapeMenu } from "./ShapeMenu";
import type { Tool } from "./tools";
import { ZoomMenu } from "./ZoomMenu";
import "./toolbar.css";
import { DrawingMenu } from "./DrawingMenu";

interface Props {
  tool: Tool;
  /** Arms a tool; the one that is armed already is put away by choosing "select". */
  onTool: (tool: Tool) => void;
  page: PageMenuProps;
  /** Uploads and inserts a picture; null for someone who cannot edit, who then has no text or shape tools. */
  onPicture: ((file: File) => void) | null;
  /** Whether the Frame tool is offered (someone who may change the structure). */
  frames: boolean;
  /** Mutation controls stay visible for an eligible editor while the initial room or reset is pending. */
  ready: boolean;
  /** Plays the first frame of the page, given the button to give focus back to; null when the page has none to show. */
  onPresent: ((button: HTMLElement) => void) | null;
  panel: { open: boolean; onToggle: () => void };
  zoom: { percent: number; onStep: (direction: 1 | -1) => void; onReset: () => void; onFit: () => void; onPreset: (zoom: number) => void };
  /** The connection light of the room, at the start of the right side. */
  live?: ReactNode;
}

function ToolButton({ label, keys, pressed, onClick, children, disabled = false }: { label: string; keys?: string; pressed?: boolean; onClick: () => void; children: ReactNode; disabled?: boolean }) {
  return (
    <Tooltip content={keys ? `${label} (${keys})` : label}><button type="button" className="tool" aria-label={label} aria-pressed={pressed} onClick={onClick} disabled={disabled}>
      {children}
    </button></Tooltip>
  );
}

export function ShellBar({ tool, onTool, page, onPicture, frames, ready, onPresent, panel, zoom, live }: Props) {
  const t = useT();
  const toggle = (next: Tool) => () => {
    onTool(tool === next ? "select" : next);
  };
  return (
    <div className="canvas-bars">
      <div className="canvas-bar canvas-bar-left" role="toolbar" aria-label={t("design.view")}>
        <PageMenu {...page} />
        <ToolButton label={t("design.select")} keys="V" pressed={tool === "select"} onClick={() => { onTool("select"); }}>
          <Pointer />
        </ToolButton>
        <ToolButton label={t("design.hand")} keys="H" pressed={tool === "hand"} onClick={() => { onTool("hand"); }}>
          <Hand />
        </ToolButton>
        {onPicture && (
          <ToolButton label={t("ops.text")} keys="T" disabled={!ready} pressed={tool === "text"} onClick={toggle("text")}>
            <TextIcon />
          </ToolButton>
        )}
        {frames && (
          <ToolButton label={t("toolbar.frame")} keys="F" disabled={!ready} pressed={tool === "frame"} onClick={toggle("frame")}>
            <FrameIcon />
          </ToolButton>
        )}
        {onPicture && <DrawingMenu tool={tool} onTool={onTool} disabled={!ready} />}
        {onPicture && <ShapeMenu tool={tool} onTool={onTool} onPicture={onPicture} disabled={!ready} />}
      </div>
      <div className="canvas-bar canvas-bar-right" role="toolbar" aria-label={t("toolbar.view")}>
        {live}
        <Tooltip content={t("toolbar.present")}><button
          type="button"
          className="tool"
          aria-label={t("toolbar.present")}
          disabled={!onPresent}
          onClick={(e) => {
            onPresent?.(e.currentTarget);
          }}
        >
          <PlayIcon />
        </button></Tooltip>
        <ToolButton label={t("toolbar.panel")} keys="Cmd+\\" pressed={panel.open} onClick={panel.onToggle}>
          <PanelIcon />
        </ToolButton>
        <ZoomMenu {...zoom} />
      </div>
    </div>
  );
}
