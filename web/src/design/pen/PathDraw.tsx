import { useEffect, useRef, useState, type PointerEvent } from "react";
import type { DesignEdits } from "../edits";
import type { PenDrawProps, PenFrame } from "./PenDraw";
import { addVector } from "./ops";
import { MAX_ANCHORS, pathOf, type Anchor } from "./path";
import { type PenStyle, type Point } from "./stroke";
import "./path.css";

export interface PathDrawProps extends PenDrawProps { labels: {draw:string;path:string} }
interface Draft { frame:PenFrame; anchors:Anchor[]; style:PenStyle }
interface Held { pointer:number; point:Point; close:boolean; before:Anchor[] }

/** Mount outside the transformed world, beside the freehand surface. */
export function PathDraw(props:PathDrawProps){
 if(!props.active||!props.editable||!props.edits)return null;
 return <Drawing {...props} edits={props.edits}/>;
}

function Drawing({edits,frames,view,style,onCreated,labels}:PathDrawProps & {edits:DesignEdits}){
 const surface=useRef<HTMLDivElement>(null);
 const draft=useRef<Draft|null>(null),held=useRef<Held|null>(null);
 const [preview,setPreview]=useState<Draft|null>(null);
 const [cursor,setCursor]=useState<Point|null>(null);
 const show=()=>{const d=draft.current;setPreview(d?.anchors.length?{...d,anchors:[...d.anchors]}:null);};
 const finish=(closed:boolean)=>{
  const d=draft.current;draft.current=null;held.current=null;setPreview(null);setCursor(null);
  if(!d||!frames.some(f=>f.frame.id===d.frame.frame.id&&f.frame.file===d.frame.frame.file))return;
  const result=addVector(edits,d.frame.frame.file,pathOf(d.anchors,closed,d.style),labels.path);
  if(result)onCreated?.(result);
 };
 useEffect(()=>{
  const key=(event:KeyboardEvent)=>{
   const undo=(event.metaKey||event.ctrlKey)&&!event.altKey&&!event.shiftKey&&event.key.toLowerCase()==="z";
   if((event.key!=="Escape"&&!undo)||event.defaultPrevented||!draft.current)return;
   const target=event.target;
   const onCanvas=target===event.currentTarget||target===document||target===document.body||(target instanceof Node&&surface.current?.contains(target));
   // Portaled dismissible controls own shortcuts even when focus remains on the canvas.
   const dismissible=[...document.querySelectorAll('[role="menu"], [role="dialog"], [role="listbox"], [role="tooltip"]')].some(control=>!control.closest('[hidden], [aria-hidden="true"], [data-state="closed"]'));
   if(!onCanvas||dismissible)return;
   event.preventDefault();event.stopImmediatePropagation();
   const h=held.current;
   if(undo){
    // Keep an empty local session until Escape or tool exit, so repeated Undo cannot reach room history.
    held.current=null;
    draft.current.anchors=h?h.before:draft.current.anchors.slice(0,-1);
    if(h&&surface.current?.hasPointerCapture(h.pointer))surface.current.releasePointerCapture(h.pointer);
    show();return;
   }
   if(h)draft.current.anchors=h.before;
   finish(false);
  };
  window.addEventListener("keydown",key,true);
  return ()=>{window.removeEventListener("keydown",key,true);};
 });
 const point=(event:{clientX:number;clientY:number},surface:HTMLElement,frame?:PenFrame):Point|null=>{
  if(!Number.isFinite(view.z)||view.z<=0)return null;
  const rect=surface.getBoundingClientRect();
  const p={x:(event.clientX-rect.left-view.x)/view.z-(frame?.x??0),y:(event.clientY-rect.top-view.y)/view.z-(frame?.y??0)};
  return Number.isFinite(p.x)&&Number.isFinite(p.y)?p:null;
 };
 const move=(event:PointerEvent<HTMLDivElement>)=>{
  const d=draft.current,h=held.current;if(!d)return;
  const p=point(event,event.currentTarget,d.frame);if(!p)return;
  if(!h){
   setCursor(p.x>=0&&p.y>=0&&p.x<=d.frame.frame.w&&p.y<=d.frame.frame.h?p:null);return;
  }
  if(h.pointer!==event.pointerId||h.close)return;
  const last=d.anchors.at(-1);if(!last)return;
  const dx=p.x-h.point.x,dy=p.y-h.point.y;
  if(Math.hypot(dx,dy)*view.z>=3)last.handle={x:dx,y:dy};else delete last.handle;
  show();
 };
 const cancel=()=>{
  const d=draft.current,h=held.current;held.current=null;setCursor(null);
  if(d&&h){d.anchors=h.before;if(!d.anchors.length)draft.current=null;show();}
 };
 const release=(event:PointerEvent<HTMLDivElement>)=>{
  if(event.currentTarget.hasPointerCapture(event.pointerId))event.currentTarget.releasePointerCapture(event.pointerId);
 };
 const stroke=preview?pathOf(preview.anchors,false,preview.style):null;
 const first=preview?.anchors[0],last=preview?.anchors.at(-1);
 const close=cursor&&first&&preview.anchors.length>=2&&Math.hypot(cursor.x-first.x,cursor.y-first.y)*view.z<=8;
 const next=preview&&last&&cursor&&(preview.anchors.length<MAX_ANCHORS||close)?pathOf([last,close?first:cursor],false,preview.style):null;
 return <div ref={surface} tabIndex={-1} className="pen-draw path-draw" role="group" aria-label={labels.draw}
  onPointerDown={event=>{
   if(event.button!==0||held.current)return;
   let d=draft.current;
   const world=point(event,event.currentTarget);if(!world)return;
   if(!d?.anchors.length){
    const frame=[...frames].reverse().find(f=>world.x>=f.x&&world.y>=f.y&&world.x<=f.x+f.frame.w&&world.y<=f.y+f.frame.h);
    if(!frame)return;
    d={frame,anchors:[],style:{...style}};draft.current=d;
   }
   const p=point(event,event.currentTarget,d.frame);if(!p)return;
   if(p.x<0||p.y<0||p.x>d.frame.frame.w||p.y>d.frame.frame.h)return;
   const first=d.anchors[0],close=!!first&&d.anchors.length>=2&&Math.hypot(p.x-first.x,p.y-first.y)*view.z<=8;
   if(!close&&d.anchors.length>=MAX_ANCHORS)return;
   setCursor(null);
   held.current={pointer:event.pointerId,point:p,close,before:d.anchors.map(a=>({...a}))};
   if(!close)d.anchors.push(p);
   event.currentTarget.focus({preventScroll:true});event.currentTarget.setPointerCapture(event.pointerId);show();
  }}
  onPointerMove={move}
  onPointerLeave={()=>{setCursor(null);}}
  onPointerUp={event=>{
   const h=held.current;if(h?.pointer!==event.pointerId)return;
   move(event);held.current=null;release(event);
   if(h.close)finish(true);else show();
  }}
  onPointerCancel={event=>{if(held.current?.pointer===event.pointerId)cancel();}}
  onLostPointerCapture={event=>{if(held.current?.pointer===event.pointerId)cancel();}}
 >{preview&&<svg className="pen-preview path-preview" aria-hidden="true" style={{left:view.x+preview.frame.x*view.z,top:view.y+preview.frame.y*view.z,width:preview.frame.frame.w*view.z,height:preview.frame.frame.h*view.z}} viewBox={`0 0 ${String(preview.frame.frame.w)} ${String(preview.frame.frame.h)}`}>
  {stroke&&<path d={stroke.d} transform={`translate(${String(stroke.x)} ${String(stroke.y)})`} stroke={stroke.color} strokeWidth={stroke.width} fill="none" strokeLinecap="round" strokeLinejoin="round"/>}
  {next&&<path className="path-next" d={next.d} transform={`translate(${String(next.x)} ${String(next.y)})`} stroke={next.color} strokeWidth={next.width} fill="none" strokeLinecap="round" strokeLinejoin="round"/>}
  {preview.anchors.map((a,i)=><g key={i}>
   {a.handle&&<><line className="path-handle" x1={a.x-a.handle.x} y1={a.y-a.handle.y} x2={a.x+a.handle.x} y2={a.y+a.handle.y} vectorEffect="non-scaling-stroke"/>{[-1,1].map(sign=><circle key={sign} className="path-control" cx={a.x+sign*(a.handle?.x??0)} cy={a.y+sign*(a.handle?.y??0)} r={3/view.z}/>)}</>}
   <circle className={`path-anchor${i===0?" path-start":""}`} cx={a.x} cy={a.y} r={(i===0?5:3)/view.z}/>
  </g>)}
 </svg>}</div>;
}
