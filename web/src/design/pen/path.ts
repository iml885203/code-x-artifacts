import { validPen, type PenStyle, type Point, type Stroke } from "./stroke";

/** A drag's vector points to the outgoing control; the incoming control is its reflection. */
export interface Anchor extends Point { handle?: Point }
/** Leaves room for six full-precision control coordinates per segment within the SVG attribute limit. */
export const MAX_ANCHORS = 512;
const finite = (p: Point) => [p.x,p.y].every(n=>Number.isFinite(n)&&Math.abs(n)<=1_000_000);
const rounded = (n: number) => Math.round(n*1000)/1000;

/** Control-hull bounds include the full curve and stroke, including a closing curve's handles. */
export function pathOf(anchors: readonly Anchor[], closed: boolean, style: PenStyle): Stroke|null {
 if(anchors.length<2||anchors.length>MAX_ANCHORS||!validPen(style)||anchors.some(a=>!finite(a)||(a.handle&&!finite(a.handle))))return null;
 const first=anchors[0];if(!first||!anchors.some(a=>Math.hypot(a.x-first.x,a.y-first.y)>=0.001))return null;
 const points:Point[]=[];
 for(let i=0;i<anchors.length;i++){
  const a=anchors[i];if(!a)continue;
  points.push(a);
  if(a.handle){
   if(i>0||closed)points.push({x:a.x-a.handle.x,y:a.y-a.handle.y});
   if(i<anchors.length-1||closed)points.push({x:a.x+a.handle.x,y:a.y+a.handle.y});
  }
 }
 if(points.some(p=>!finite(p)))return null;
 const pad=style.width/2;
 const x=Math.min(...points.map(p=>p.x))-pad,y=Math.min(...points.map(p=>p.y))-pad;
 const w=Math.max(...points.map(p=>p.x))-x+pad,h=Math.max(...points.map(p=>p.y))-y+pad;
 const local=(p:Point):Point=>({x:rounded(p.x-x),y:rounded(p.y-y)});
 // The native validator reads the emitted, rounded local controls, not their world coordinates.
 if(points.some(p=>!finite(local(p))))return null;
 const xy=(p:Point)=>{const q=local(p);return `${String(q.x)} ${String(q.y)}`;};
 const segment=(a:Anchor,b:Anchor)=>a.handle||b.handle
  ?`C${xy({x:a.x+(a.handle?.x??0),y:a.y+(a.handle?.y??0)})} ${xy({x:b.x-(b.handle?.x??0),y:b.y-(b.handle?.y??0)})} ${xy(b)}`
  :`L${xy(b)}`;
 let d=`M${xy(first)}`;
 for(let i=1;i<anchors.length;i++){const a=anchors[i-1],b=anchors[i];if(a&&b)d+=segment(a,b);}
 const last=anchors.at(-1);if(closed&&last)d+=segment(last,first)+"Z";
 if(d.length>65536)return null;
 return {x,y,w,h,d,...style};
}
