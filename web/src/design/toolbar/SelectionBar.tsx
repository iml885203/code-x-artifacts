import * as Menu from "@radix-ui/react-dropdown-menu";
import { useEffect, useReducer, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useT } from "../../i18n";
import { Tooltip } from "../../Tooltip";
import { Close } from "../../icons/chrome";
import type { Target } from "../controller";
import type { DesignEdits } from "../edits";
import { ColorPickerContent } from "../properties/ColorPicker";
import { formatColor } from "../properties/css";
import type { ColorChange } from "../properties/controls";
import type { Operations } from "../ops/useDesignOps";
import { useTokens } from "../tokens";
import { PanelIcon } from "./icons";
import { paintColorOf, paintOf, strokeWidthOf, writePaint, writeStrokeWidth } from "./paint";

export function SelectionBar({ edits, targets, ops, onProperties, onComment, onDelete }: {
  edits: DesignEdits; targets: Target[]; ops: Operations; onProperties: () => void; onComment?: (() => void) | undefined; onDelete: () => void;
}) {
  const t = useT();
  const tokens = useTokens(edits);
  const [paint, setPaint] = useState<"fill" | "stroke" | null>(null);
  const [, refresh] = useReducer((value: number) => value + 1, 0);
  const [position, setPosition] = useState({ left: 8, top: 8 });
  const [problem, setProblem] = useState("");
  const popup = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement | null>(null);
  useEffect(() => edits.onChange(() => { refresh(); }), [edits]);
  useEffect(() => {
    if (!paint) return;
    popup.current?.querySelector<HTMLElement>("button, input")?.focus();
    const dismiss = (event: PointerEvent) => { if (event.target instanceof Node && !popup.current?.contains(event.target) && !trigger.current?.contains(event.target)) setPaint(null); };
    const escape = (event: KeyboardEvent) => { if (event.key === "Escape") { event.preventDefault(); event.stopImmediatePropagation(); setPaint(null); trigger.current?.focus(); } };
    document.addEventListener("pointerdown", dismiss);
    document.addEventListener("keydown", escape, true);
    return () => { document.removeEventListener("pointerdown", dismiss); document.removeEventListener("keydown", escape, true); };
  }, [paint]);
  const last = targets.at(-1);
  if (!last) return null;
  const vector = last.report.tag === "svg" || last.report.tag === "path";
  const chosen = paintOf(edits, last, paint ?? (vector ? "stroke" : "fill"));
  const color = paintColorOf(edits, last, paint ?? (vector ? "stroke" : "fill"));
  const apply = (change: ColorChange | string | null) => { writePaint(edits, targets, paint ?? "fill", change); };
  const button = (kind: "fill" | "stroke") => {
    const label = t(kind === "fill" ? "props.fillColor" : "props.borderColor");
    const display = paintColorOf(edits, last, kind);
    return <Tooltip content={label} key={kind}><button type="button" className="tool" aria-label={label} aria-haspopup="dialog" aria-expanded={paint === kind} onClick={(event) => { trigger.current = event.currentTarget;
      const box = event.currentTarget.getBoundingClientRect();
      setPosition({ left: Math.max(8, Math.min(box.left, window.innerWidth - 308)), top: Math.max(8, Math.min(box.top - 470, window.innerHeight - 470)) });
      setPaint(paint === kind ? null : kind); }}>
      <svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true" data-toolbar-sample={kind}>
        {kind === "fill" ? <rect x="3" y="3" width="18" height="18" rx="3" fill={formatColor(display)} stroke="var(--text-2)" strokeWidth="1.5" /> : <><path d="M3 12h18" stroke="var(--muted)" strokeWidth="5" strokeLinecap="round" /><path d="M3 12h18" stroke={formatColor(display)} strokeWidth="2" strokeLinecap="round" /></>}
      </svg>
    </button></Tooltip>;
  };
  return <div className="selection-bar" role="toolbar" aria-label={t("toolbar.selection")}>
    {!vector && button("fill")}{button("stroke")}
    {onComment && <Tooltip content={t("comments.onSelection")}><button type="button" className="tool" aria-label={t("comments.onSelection")} onClick={onComment}><svg className="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 4h18v13H8l-5 4z" /></svg></button></Tooltip>}
    <Tooltip content={`${t("toolbar.panel")} (Cmd+\\)`}><button type="button" className="tool" aria-label={t("toolbar.panel")} onClick={onProperties}><PanelIcon /></button></Tooltip>
    <Menu.Root modal={false}><Tooltip content={t("ops.arrange")}><Menu.Trigger className="tool" aria-label={t("ops.arrange")}><svg className="icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="5" cy="12" r="1" /><circle cx="12" cy="12" r="1" /><circle cx="19" cy="12" r="1" /></svg></Menu.Trigger></Tooltip>
      <Menu.Portal><Menu.Content className="menu bar-menu" side="top" sideOffset={8}>
        <Menu.Item className="menu-item" onSelect={ops.duplicate}>{t("ops.duplicate")}</Menu.Item>
        <Menu.Item className="menu-item" onSelect={ops.group}>{t("ops.group")}</Menu.Item>
        <Menu.Item className="menu-item" onSelect={ops.ungroup}>{t("ops.ungroup")}</Menu.Item>
        {(["forward", "backward", "front", "back"] as const).map((how) => <Menu.Item className="menu-item" key={how} onSelect={() => { ops.stack(how); }}>{t(`ops.${how}`)}</Menu.Item>)}
        <Menu.Separator className="menu-sep" /><Menu.Item className="menu-item danger" onSelect={onDelete}><Close />{t("toolbar.delete")}</Menu.Item>
      </Menu.Content></Menu.Portal>
    </Menu.Root>
    {paint && createPortal(<div ref={popup} className="color-popup" role="dialog" aria-label={t(paint === "fill" ? "props.fillColor" : "props.borderColor")} style={position}>{paint === "stroke" && <label className="stroke-width-control">{t("pen.width")}<input key={strokeWidthOf(edits, last)} type="number" min="0.5" max="64" step="0.5" aria-label={t("pen.width")} defaultValue={strokeWidthOf(edits, last)} onKeyDown={(event) => { if (event.key === "Enter") event.currentTarget.blur(); }} onBlur={(event) => { setProblem(writeStrokeWidth(edits, targets, Number(event.currentTarget.value)) ? "" : t("pen.invalid")); }} /></label>}{problem && <p role="alert">{problem}</p>}<ColorPickerContent label={t(paint === "fill" ? "props.fillColor" : "props.borderColor")} color={color} source={chosen.value} tokens={tokens} onColor={apply} onToken={apply} onNoFill={() => { apply("none"); }} onReset={() => { apply(null); }} /></div>, document.body)}
  </div>;
}
