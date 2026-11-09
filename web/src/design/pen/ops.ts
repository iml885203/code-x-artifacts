import type { DesignEdits } from "../edits";
import type { Ref } from "../ops/nodes";
import { lastOrder, newPid, nodeKey, rootOf } from "../ops/room";
import { readTree } from "../tree";
import { strokeOf, type PenStyle, type Point, type Stroke } from "./stroke";

/** Adds ordinary SVG nodes in one local transaction; geometry edits and deletion use the same entries as every other node. */
export function addStroke(edits: DesignEdits, file: string, points: readonly Point[], style: PenStyle, name: string): Ref|null {
 return addVector(edits,file,strokeOf(points,style),name);
}

/** Both drawing tools persist the same ordinary SVG model. */
export function addVector(edits: DesignEdits, file: string, stroke: Stroke|null, name: string): Ref|null {
 const root=rootOf(edits.map,file);
 if(!stroke||!root)return null;
 const px=(value:number)=>`${String(Math.round(value*1000)/1000)}px`;
 const pid=newPid(),path=newPid();const order=lastOrder(readTree(edits.map,file),root);
 const attrs=(rows:[string,string][])=>JSON.stringify(rows.map(([key,value])=>["",key,value]));
 edits.transact(map=>{
  const put=(id:string,fields:Record<string,string>)=>{for(const [field,value] of Object.entries(fields))map.set(nodeKey(file,id,field),value);};
  put(pid,{kind:"el",ns:"svg",tag:"svg",parent:root,order,attrs:attrs([["data-pen","stroke"],["data-name",name],["viewBox",`0 0 ${String(stroke.w)} ${String(stroke.h)}`],["preserveAspectRatio","none"]]),"s|position":"absolute","s|left":px(stroke.x),"s|top":px(stroke.y),"s|width":px(stroke.w),"s|height":px(stroke.h)});
  put(path,{kind:"el",ns:"svg",tag:"path",parent:pid,order:"V",attrs:attrs([["d",stroke.d],["stroke",stroke.color],["stroke-width",String(stroke.width)],["fill","none"],["stroke-linecap","round"],["stroke-linejoin","round"],["pointer-events","none"]])});
 });
 return {file,pid};
}
