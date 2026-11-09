import { format, useT } from "../../i18n";
import { Tooltip } from "../../Tooltip";
import type { DesignFrame } from "../manifest";
import type { LabelLayout } from "../labelLayout";
import { ExpandIcon } from "./icons";

export function ArtboardLabel({ frame, layout, selected, onSelect, onExpand }: {
  frame: DesignFrame; layout: LabelLayout; selected: boolean; onSelect?: (() => void) | undefined; onExpand: (button: HTMLElement) => void;
}) {
  const t = useT();
  const expand = format(t("toolbar.expandArtboard"), { name: frame.name });
  return <div className="dframe-label" data-inside={layout.name === "inside" ? "" : undefined}>
    <Tooltip content={frame.name}>{onSelect ? <button type="button" className="dframe-name" aria-label={format(t("structure.selectFrame"), { name: frame.name })} aria-pressed={selected} onClick={onSelect}>{frame.name}</button> : <span className="dframe-name">{frame.name}</span>}</Tooltip>
    {layout.actions === "row" && <span className="dframe-actions"><Tooltip content={expand}><button type="button" className="dframe-play" aria-label={expand} onClick={(event) => { onExpand(event.currentTarget); }}><ExpandIcon /></button></Tooltip></span>}
  </div>;
}
