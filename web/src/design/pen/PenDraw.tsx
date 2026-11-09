import { useRef, useState, type PointerEvent } from "react";
import { useT } from "../../i18n";
import type { DesignEdits } from "../edits";
import type { Ref } from "../ops/nodes";
import type { View } from "../view";
import { addStroke } from "./ops";
import { sample, strokeOf, type PenStyle, type Point, type Stroke } from "./stroke";
import "./pen.css";

export interface PenFrame { frame: {id:string;file:string;w:number;h:number}; x:number; y:number }
export interface PenDrawProps {
 active: boolean;
 editable: boolean;
 edits: DesignEdits|null;
 frames: readonly PenFrame[];
 view: View;
 style: PenStyle;
 onCreated?: (ref:Ref)=>void;
}
interface Gesture { frame:PenFrame; points:Point[]; style:PenStyle; pointer:number }

/** Mount in canvas-main beside FrameDraw, outside the transformed world; only a current editor may draw. */
export function PenDraw(props:PenDrawProps){
 if(!props.active||!props.editable||!props.edits)return null;
 return <Drawing {...props} edits={props.edits}/>;
}

function Drawing({edits,frames,view,style,onCreated}:PenDrawProps & {edits:DesignEdits}){
 const t=useT();const held=useRef<Gesture|null>(null);const [preview,setPreview]=useState<{stroke:Stroke;frame:PenFrame}|null>(null);
 const point=(event:{clientX:number;clientY:number},surface:HTMLElement)=>{
  const rect=surface.getBoundingClientRect();return {x:(event.clientX-rect.left-view.x)/view.z,y:(event.clientY-rect.top-view.y)/view.z};
 };
 const append=(event:{clientX:number;clientY:number},surface:HTMLElement,g:Gesture)=>{
  const p=point(event,surface);sample(g.points,{x:Math.max(0,Math.min(g.frame.frame.w,p.x-g.frame.x)),y:Math.max(0,Math.min(g.frame.frame.h,p.y-g.frame.y))});
 };
 const cancel=()=>{held.current=null;setPreview(null);};
 const move=(event:PointerEvent<HTMLDivElement>)=>{
  const g=held.current;if(g?.pointer!==event.pointerId)return;
  const coalesced=(event.nativeEvent as Partial<globalThis.PointerEvent>).getCoalescedEvents;
  const samples=coalesced?.call(event.nativeEvent)??[];
  for(const e of samples)append(e,event.currentTarget,g);
  append(event,event.currentTarget,g);
  const stroke=strokeOf(g.points,g.style);if(stroke)setPreview({stroke,frame:g.frame});
 };
 return <div className="pen-draw" role="group" aria-label={t("pen.draw")}
  onPointerDown={e=>{
   if(e.button!==0||held.current||!Number.isFinite(view.z)||view.z<=0)return;
   const p=point(e,e.currentTarget);
   const frame=[...frames].reverse().find(f=>p.x>=f.x&&p.y>=f.y&&p.x<=f.x+f.frame.w&&p.y<=f.y+f.frame.h);
   if(!frame)return;
   const g:Gesture={frame,points:[],style:{...style},pointer:e.pointerId};held.current=g;
   append(e,e.currentTarget,g);e.currentTarget.setPointerCapture(e.pointerId);
   const stroke=strokeOf(g.points,g.style);if(stroke)setPreview({stroke,frame});
  }}
  onPointerMove={move}
  onPointerUp={e=>{
   const g=held.current;if(g?.pointer!==e.pointerId)return;
   append(e,e.currentTarget,g);cancel();
   if(e.currentTarget.hasPointerCapture(e.pointerId))e.currentTarget.releasePointerCapture(e.pointerId);
   if(!frames.some(f=>f.frame.id===g.frame.frame.id&&f.frame.file===g.frame.frame.file))return;
   const result=addStroke(edits,g.frame.frame.file,g.points,g.style,t("pen.stroke"));if(result)onCreated?.(result);
  }}
  onPointerCancel={cancel} onLostPointerCapture={cancel}
 >{preview&&<svg className="pen-preview" aria-hidden="true" style={{left:view.x+(preview.frame.x+preview.stroke.x)*view.z,top:view.y+(preview.frame.y+preview.stroke.y)*view.z,width:preview.stroke.w*view.z,height:preview.stroke.h*view.z}} viewBox={`0 0 ${String(preview.stroke.w)} ${String(preview.stroke.h)}`} preserveAspectRatio="none"><path d={preview.stroke.d} stroke={preview.stroke.color} strokeWidth={preview.stroke.width} fill="none" strokeLinecap="round" strokeLinejoin="round"/></svg>}</div>;
}
