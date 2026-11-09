import { describe, expect, it } from "vitest";
import { pathOf, type Anchor } from "./path";
import nativeCases from "./path-cases.json";
const style = { color: "var(--accent)", width: 2 };
describe("connected path geometry", () => {
 it("keeps clicked segments straight and dragged anchors cubic, including the closing segment", () => {
  const anchors: Anchor[] = [{x:10,y:20},{x:50,y:20,handle:{x:10,y:20}},{x:90,y:60}];
  const open = pathOf(anchors, false, style);
  expect(open).toMatchObject({x:9,y:-1,w:82,h:62,...style});
  expect(open?.d).toBe("M1 21C1 21 31 1 41 21C51 41 81 61 81 61");
  expect(pathOf(anchors, true, style)?.d).toBe(`${open?.d??""}L1 21Z`);
  expect(pathOf([{x:0,y:0},{x:10,y:0},{x:20,y:10}], false, style)?.d).toBe("M1 1L11 1L21 11");
 });
 it("pads horizontal and vertical paths and rejects unusable or unbounded geometry", () => {
  expect(pathOf([{x:0,y:0},{x:10,y:0}],false,style)).toMatchObject({w:12,h:2});
  expect(pathOf([{x:0,y:0},{x:0,y:10}],false,style)).toMatchObject({w:2,h:12});
  for(const anchors of [[],[{x:0,y:0}],[{x:1,y:1},{x:1,y:1}],[{x:0,y:0},{x:NaN,y:1}],[{x:0,y:0},{x:1,y:1,handle:{x:Infinity,y:0}}],Array.from({length:4097},(_,x)=>({x,y:x}))]){
   expect(pathOf(anchors,false,style)).toBeNull();
  }
  expect(pathOf([{x:0,y:0},{x:10,y:10}],false,{...style,width:0})).toBeNull();
 });
 it("keeps the full supported anchor budget representable as one bounded SVG path",()=>{
  const anchors=Array.from({length:512},(_,i)=>({x:i%2?300000:-300000,y:i,handle:{x:100000,y:100000}}));
  const path=pathOf(anchors,true,style);expect(path).not.toBeNull();expect(path?.d.length).toBeLessThanOrEqual(65536);
  expect(pathOf(anchors.map(a=>({...a,x:a.x>0?500000:-500000})),true,style)).toBeNull();
  expect(pathOf([...anchors,{x:0,y:0}],false,style)).toBeNull();
 });
 it("uses endpoint handles only on participating segments and reflects them when closing",()=>{
  const anchors=[{x:10,y:20,handle:{x:0,y:10}},{x:50,y:20,handle:{x:10,y:0}}];
  expect(pathOf(anchors,false,style)).toMatchObject({x:9,y:19,w:42,h:12,d:"M1 1C1 11 31 1 41 1"});
  expect(pathOf(anchors,true,style)).toMatchObject({x:9,y:9,w:52,h:22,d:"M1 11C1 21 31 11 41 11C51 11 1 1 1 11Z"});
 });

 it.each(nativeCases.cases)("serializes the native validator contract: $name", c=>{
  const anchors=Array.from({length:c.count??c.anchors.length},(_,i)=>c.anchors[i%c.anchors.length]).filter((a):a is NonNullable<typeof a>=>a!==undefined);
  expect(pathOf(anchors,c.closed,nativeCases.style)?.d??null).toBe(c.expected);
 });

});
