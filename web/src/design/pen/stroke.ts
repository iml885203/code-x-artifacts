export interface Point { x: number; y: number }
export interface PenStyle { color: string; width: number }
export interface Stroke extends PenStyle { x: number; y: number; w: number; h: number; d: string }
export const MAX_SAMPLES = 4096;
export const DEFAULT_PEN: PenStyle = { color: "#336699", width: 3 };
const MAX_COORDINATE = 1_000_000;
const finite = (n: number) => Number.isFinite(n) && Math.abs(n) <= MAX_COORDINATE;
export const validPen = ({color,width}: PenStyle): boolean => Number.isFinite(width) && width >= 0.5 && width <= 64 && /^(?:#(?:[\da-f]{3}|[\da-f]{4}|[\da-f]{6}|[\da-f]{8})|var\(--[a-z_][\w-]{0,79}\)|currentColor)$/i.test(color);
const number = (n: number) => String(Math.round(n * 1000) / 1000);

/** Compacts a long gesture in place; its first point and its newest endpoint survive. */
export function sample(points: Point[], point: Point): void {
 if (!finite(point.x) || !finite(point.y)) return;
 const last=points.at(-1);
 if(last && Math.hypot(last.x-point.x,last.y-point.y)<0.25)return;
 if(points.length>=MAX_SAMPLES) points.splice(0,points.length,...points.filter((_,i)=>i%2===0));
 points.push(point);
}

const distance = (p: Point, a: Point, b: Point) => {
 const dx=b.x-a.x,dy=b.y-a.y;
 const t=dx===0&&dy===0?0:Math.max(0,Math.min(1,((p.x-a.x)*dx+(p.y-a.y)*dy)/(dx*dx+dy*dy)));
 return Math.hypot(p.x-a.x-t*dx,p.y-a.y-t*dy);
};
function simplify(points: readonly Point[], tolerance: number): Point[] {
 if(points.length<3)return [...points];
 const kept=new Set([0,points.length-1]);const pending:[number,number][]=[[0,points.length-1]];
 for(let pair=pending.pop();pair;pair=pending.pop()) {
  const [a,b]=pair;const start=points[a],end=points[b];if(!start||!end)continue;
  let far=tolerance,index=-1;
  for(let i=a+1;i<b;i++){const p=points[i];if(!p)continue;const d=distance(p,start,end);if(d>far){far=d;index=i;}}
  if(index>=0){kept.add(index);pending.push([a,index],[index,b]);}
 }
 return [...kept].sort((a,b)=>a-b).flatMap((i)=>points[i]?[points[i]]:[]);
}

/** Native SVG geometry, padded by half the round stroke so dots and narrow strokes have a real selection box. */
export function strokeOf(points: readonly Point[], style: PenStyle, tolerance=0.75): Stroke|null {
 if(!points.length||points.length>MAX_SAMPLES||!validPen(style)||!Number.isFinite(tolerance)||tolerance<0||points.some((p)=>!finite(p.x)||!finite(p.y)))return null;
 const xs=points.map(p=>p.x),ys=points.map(p=>p.y),pad=style.width/2;
 const x=Math.min(...xs)-pad,y=Math.min(...ys)-pad,w=Math.max(...xs)-x+pad,h=Math.max(...ys)-y+pad;
 if(![x,y,w,h].every(finite))return null;
 let local=simplify(points,tolerance).map(p=>({x:p.x-x,y:p.y-y}));
 while(local.length>1024)local=local.filter((_,i)=>i%2===0||i===local.length-1);
 const first=local[0],last=local.at(-1);if(!first||!last)return null;
 const xy=(p:Point)=>`${number(p.x)} ${number(p.y)}`;
 let d=`M${xy(first)}`;
 for(let i=1;i<local.length-1;i++){
  const p=local[i],next=local[i+1];if(!p||!next)continue;
  d+=`Q${xy(p)} ${xy({x:(p.x+next.x)/2,y:(p.y+next.y)/2})}`;
 }
 d+=`L${xy(last)}`;
 if(d.length>65536)return null;
 return {x,y,w,h,d,...style};
}
