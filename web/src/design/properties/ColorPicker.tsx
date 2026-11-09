import { useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Tooltip } from "../../Tooltip";
import { format, useT } from "../../i18n";
import type { Tokens } from "../tokens";
import { Field, type ColorChange } from "./controls";
import { formatColor, hexOf, parseColor, type Rgba } from "./css";
import { tokenColor } from "./palette";
import { SolidColor } from "./SolidColor";
import "./picker.css";

export interface ColorPickerProps {
  label: string;
  color: Rgba;
  source?: string | undefined;
  mixed?: boolean | undefined;
  tokens: Tokens;
  onColor: (change: ColorChange) => void;
  onToken: (binding: string) => void;
  onNoFill?: (() => void) | undefined;
  onReset?: (() => void) | undefined;
}

/** Source-specific writes remain with the caller: the same picker serves CSS backgrounds and SVG paint. */
export function ColorPicker(props: ColorPickerProps) {
  const t = useT();
  const [position, setPosition] = useState<{left:number;top:number} | null>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  const id = useId();
  const close = () => { setPosition(null); trigger.current?.focus(); };
  useEffect(() => {
    if (!position) return;
    popup.current?.querySelector<HTMLElement>("button, input")?.focus();
    const dismiss = (event: PointerEvent) => {
      if (event.target instanceof Node && !popup.current?.contains(event.target) && !trigger.current?.contains(event.target)) setPosition(null);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); setPosition(null); trigger.current?.focus(); }
    };
    const reposition = () => { setPosition(null); };
    document.addEventListener("pointerdown", dismiss);
    document.addEventListener("keydown", escape, true);
    window.addEventListener("resize", reposition);
    return () => { document.removeEventListener("pointerdown", dismiss); document.removeEventListener("keydown", escape, true); window.removeEventListener("resize", reposition); };
  }, [position]);
  return <div className="prop-color" role="group" aria-label={props.label}>
    <Tooltip content={format(t("props.picker"), {name:props.label})} disabled={position !== null}><button type="button" ref={trigger} className="prop-swatch color-trigger" aria-label={format(t("props.picker"), {name:props.label})} aria-haspopup="dialog" aria-expanded={position !== null} aria-controls={position ? id : undefined} onClick={() => {
      if (position) close();
      else { const rect=trigger.current?.getBoundingClientRect(); setPosition({left:Math.max(8, Math.min(rect?.left ?? 8, window.innerWidth-308)), top:Math.max(8, Math.min(rect?.bottom ?? 8, window.innerHeight-550))}); }
    }}><span className={props.mixed ? "mixed-color" : undefined} style={props.mixed ? undefined : {background:formatColor(props.color)}} /></button></Tooltip>
    <ColorValues label={props.label} color={props.color} mixed={props.mixed} onColor={props.onColor} />
    {position && createPortal(<div className="color-popup" id={id} ref={popup} role="dialog" aria-label={props.label} style={{...position,maxHeight:Math.max(0,window.innerHeight-position.top-8)}} onBlur={event => {
      if (event.relatedTarget instanceof Node && !event.currentTarget.contains(event.relatedTarget) && !trigger.current?.contains(event.relatedTarget)) setPosition(null);
    }}><ColorPickerContent {...props} /></div>, document.body)}
  </div>;
}

export function ColorValues({label,color,onColor,mixed}: Pick<ColorPickerProps,"label"|"color"|"onColor"|"mixed">) {
  const t = useT();
  return <>
    <Field label={format(t("props.colorValue"), {name:label})} value={mixed ? "" : hexOf(color)} placeholder={mixed ? t("props.mixed") : undefined} onCommit={text => {
      const parsed=parseColor(text);
      if (!parsed) return false;
      onColor(current => ({...parsed, a:parsed.a < 1 || /^#(?:[\da-f]{4}|[\da-f]{8})$/i.test(text.trim()) || /^rgba\(/i.test(text.trim()) ? parsed.a : current.a})); return true;
    }} />
    <Field label={`${label}, ${t("props.alpha")}`} value={mixed ? "" : String(Math.round(color.a*100))} placeholder={mixed ? t("props.mixed") : undefined} short="%" onCommit={text => {
      const n=Number(text); if (!text.trim() || !Number.isFinite(n)) return false;
      onColor(current => ({...current,a:Math.max(0,Math.min(100,n))/100})); return true;
    }} />
  </>;
}

/** For hosts that already supply their own accessible popover and trigger. */
export function ColorPickerContent(props: ColorPickerProps) {
  const t=useT();
  const [tab,setTab]=useState<"theme"|"custom">("theme");
  const [query,setQuery]=useState("");
  const id=useId();
  const bound=/^var\((--[\w-]+)\)$/.exec((props.source ?? "").replace(/\s*!important\s*$/i, "").trim())?.[1];
  const names=Object.keys(props.tokens).filter(name => tokenColor(name,props.tokens) !== null && `${name} ${props.tokens[name] ?? ""}`.toLowerCase().includes(query.toLowerCase())).sort();
  return <>
    <strong className="picker-title">{props.label}</strong>
    <div className="picker-tabs" role="tablist" aria-label={props.label}>
      {(["theme","custom"] as const).map(value => <button key={value} id={`${id}-${value}`} role="tab" type="button" aria-selected={tab===value} aria-controls={`${id}-panel`} tabIndex={tab===value ? 0 : -1} onClick={() => { setTab(value); }} onKeyDown={event => {
        if (["ArrowLeft","ArrowRight","Home","End"].includes(event.key)) { event.preventDefault(); const next=event.key === "Home" ? "theme" : event.key === "End" ? "custom" : tab==="theme" ? "custom" : "theme"; setTab(next); document.getElementById(`${id}-${next}`)?.focus(); }
      }}>{t(value === "theme" ? "props.theme" : "props.custom")}</button>)}
    </div>
    <div className="picker-panel" id={`${id}-panel`} role="tabpanel" aria-labelledby={`${id}-${tab}`}>
      {tab === "theme" ? <>
        <input type="search" aria-label={t("props.searchColors")} placeholder={t("props.searchColors")} value={query} onChange={event => { setQuery(event.target.value); }} />
        <div className="picker-current"><span className="color-chip" style={{background:formatColor(props.color)}} />{props.mixed ? t("props.mixed") : bound ? `var(${bound})` : hexOf(props.color)}</div>
        <div className="picker-tokens">{names.map(name => <button className="color-choice" type="button" key={name} aria-pressed={bound===name} onClick={() => { props.onToken(`var(${name})`); }}><span className="color-chip" style={{background:tokenColor(name,props.tokens) ?? undefined}} /><span>{name}</span><small>{props.tokens[name]}</small></button>)}{names.length===0 && <p className="code-note">{t("props.noColors")}</p>}</div>
      </> : <><span className="prop-title">{t("props.solid")}</span><SolidColor color={props.color} onColor={props.onColor} /><div className="picker-value-labels"><span>{t("props.hex")}</span><span>{t("props.opacity")}</span></div><div className="prop-color"><ColorValues label={props.label} color={props.color} mixed={props.mixed} onColor={props.onColor} /></div></>}
      <div className="picker-actions">{props.onNoFill && <button type="button" onClick={props.onNoFill}><span className="color-chip no-fill" />{t("props.noFill")}</button>}{props.onReset && <Tooltip content={t("props.resetHelp")}><button type="button" onClick={props.onReset}>{t("props.reset")}</button></Tooltip>}</div>
    </div>
  </>;
}
