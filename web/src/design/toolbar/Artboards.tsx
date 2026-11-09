import { format, useT } from "../../i18n";
import type { FrameLinks } from "../inspect";
import type { LabelLayout } from "../labelLayout";
import type { DesignFrame, Placed } from "../manifest";
import type { useLiveFrames } from "../useLiveFrames";
import { ArtboardLabel } from "./ArtboardLabel";

interface Props {
  frames: Placed[];
  labels: LabelLayout;
  selected: string | null;
  active: string;
  editing: string | undefined;
  hasFile: (file: string) => boolean;
  liveFrames: ReturnType<typeof useLiveFrames>;
  base: string;
  links: FrameLinks;
  onSelect: ((id: string) => void) | undefined;
  onExpand: (frame: DesignFrame, button: HTMLElement) => void;
  onHover: (id: string) => void;
}

export function Artboards({ frames, labels, selected, active, editing, hasFile, liveFrames, base, links, onSelect, onExpand, onHover }: Props) {
  const t = useT();
  return frames.map(({ frame, x, y }) => <div key={frame.id} className="dframe"
    style={{ left: x, top: y, width: frame.w, height: frame.h }} data-frame={frame.id}
    data-frame-selected={selected === frame.id ? "" : undefined} data-editing={editing === frame.id ? "" : undefined}
    data-active={labels.actions === "dock" && active === frame.id ? "" : undefined}
    onPointerEnter={(event) => { if (event.pointerType === "mouse") onHover(frame.id); }} onPointerDown={() => { onHover(frame.id); }}>
    <ArtboardLabel frame={frame} layout={labels} selected={selected === frame.id} onSelect={onSelect ? () => { onSelect(frame.id); } : undefined} onExpand={(button) => { onExpand(frame, button); }} />
    {hasFile(frame.file) ? <iframe key={liveFrames?.(frame.file).key ?? `${base}${frame.file}`} ref={links.refFor(frame.id)} src={liveFrames?.(frame.file).src ?? base + frame.file}
      title={format(t("design.frameSize"), { name: frame.name, w: String(frame.w), h: String(frame.h) })} sandbox="allow-scripts" loading="lazy" tabIndex={-1} /> : <div className="dframe-blank" aria-hidden="true" />}
  </div>);
}
