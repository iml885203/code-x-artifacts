import { useEffect, useRef, useState } from "react";
import { useT } from "../../i18n";
import { type ColorChange } from "./controls";
import { fromHsv, toHsv, type Hsv } from "./color-space";
import { formatColor, hexOf, type Rgba } from "./css";

export function SolidColor({color,onColor}: {color:Rgba;onColor:(change:ColorChange)=>void}) {
  const t=useT();
  const [draft,setDraft]=useState(toHsv(color));
  const identity=formatColor(color);
  const [seen,setSeen]=useState(identity);
  const held=useRef(draft);
  const gestureStart=useRef<Hsv | null>(null);
  if (seen !== identity) { setSeen(identity); const next=toHsv(color); setDraft({...next,h:next.s === 0 ? draft.h : next.h}); }
  useEffect(() => { held.current=draft; }, [draft]);
  const update=(next:Hsv) => { held.current=next; setDraft(next); };
  const start=() => { gestureStart.current={...held.current}; };
  const cancel=() => { if (gestureStart.current) update(gestureStart.current); gestureStart.current=null; };
  const commit=() => { const next=held.current; gestureStart.current=null; onColor(current => fromHsv(next,current.a)); };
  const point=(event:React.PointerEvent<HTMLDivElement>) => {
    const rect=event.currentTarget.getBoundingClientRect();
    update({...held.current,s:Math.max(0,Math.min(1,(event.clientX-rect.left)/rect.width)),v:1-Math.max(0,Math.min(1,(event.clientY-rect.top)/rect.height))});
  };
  return <>
    <div className="color-area" role="slider" tabIndex={0} aria-label={`${t("props.saturation")}, ${t("props.brightness")}`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(draft.s*100)} aria-valuetext={`${t("props.saturation")} ${String(Math.round(draft.s*100))}%, ${t("props.brightness")} ${String(Math.round(draft.v*100))}%`} style={{backgroundColor:`hsl(${String(draft.h)} 100% 50%)`}} onPointerDown={event => { start(); event.currentTarget.setPointerCapture(event.pointerId); point(event); }} onPointerMove={event => { if (event.currentTarget.hasPointerCapture(event.pointerId)) point(event); }} onPointerUp={event => { point(event); event.currentTarget.releasePointerCapture(event.pointerId); commit(); }} onPointerCancel={cancel} onKeyDown={event => {
      const step=event.shiftKey ? 0.1 : 0.01;
      if (!["ArrowLeft","ArrowRight","ArrowUp","ArrowDown","Home","End"].includes(event.key)) return;
      event.preventDefault(); update({...draft,s:Math.max(0,Math.min(1,event.key==="Home" ? 0 : event.key==="End" ? 1 : draft.s+(event.key==="ArrowLeft" ? -step : event.key==="ArrowRight" ? step : 0))),v:Math.max(0,Math.min(1,draft.v+(event.key==="ArrowDown" ? -step : event.key==="ArrowUp" ? step : 0)))}); commit();
    }}><span style={{left:`${String(draft.s*100)}%`,top:`${String((1-draft.v)*100)}%`}} /></div>
    <ColorRange label={t("props.hue")} value={draft.h} max={360} className="hue-range" onStart={start} onCancel={cancel} onDraft={h => { update({...held.current,h}); }} onCommit={commit} />
    <ColorRange label={t("props.alpha")} value={Math.round(color.a*100)} max={100} className="alpha-range" style={{backgroundImage:`linear-gradient(to right, transparent, ${hexOf(color)}), repeating-conic-gradient(var(--line) 0% 25%, var(--field) 0% 50%)`,backgroundSize:"auto, 8px 8px"}} onCommit={a => { onColor(current => ({...current,a:a/100})); }} />
  </>;
}

function ColorRange({label,value,max,className,onDraft,onCommit,style,onStart,onCancel}:{label:string;value:number;max:number;className:string;onDraft?:(value:number)=>void;onCommit:(value:number)=>void;style?:React.CSSProperties;onStart?:()=>void;onCancel?:()=>void}) {
  const [draft,setDraft]=useState(value);
  const [seen,setSeen]=useState(value);
  const pending=useRef<number | null>(null);
  if (value !== seen) { setSeen(value); setDraft(value); }
  const commit=() => { if (pending.current !== null) { const next=pending.current; pending.current=null; onCommit(next); } };
  return <input type="range" className={className} style={style} aria-label={label} min={0} max={max} value={draft} onChange={event => { const n=Number(event.target.value); pending.current=n; setDraft(n); onDraft?.(n); }} onPointerDown={onStart} onPointerCancel={() => { pending.current=null; setDraft(value); onCancel?.(); }} onPointerUp={commit} onKeyUp={commit} onBlur={commit} />;
}
