// @vitest-environment node
import { describe, expect, it } from "vitest";
import { MAX_SAMPLES, sample, strokeOf } from "./stroke";
const style = { color: "#336699", width: 4 };
describe("a bounded, smoothed vector stroke", () => {
 it("keeps a single click as a visible round dot with a nonzero box", () => {
  expect(strokeOf([{ x: 10, y: 20 }], style)).toEqual({ x: 8, y: 18, w: 4, h: 4, d: "M2 2L2 2", ...style });
 });
 it("simplifies a straight sampled line and keeps its endpoints and width", () => {
  expect(strokeOf([{x:0,y:0},{x:2,y:0.1},{x:5,y:-0.1},{x:10,y:0}], style)?.d).toBe("M2 2.1L12 2.1");
 });
 it("smooths a corner through quadratic control points without leaving its padded box", () => {
  expect(strokeOf([{x:0,y:0},{x:10,y:10},{x:20,y:0}], style)).toEqual({x:-2,y:-2,w:24,h:14,d:"M2 2Q12 12 17 7L22 2",...style});
 });
 it.each([[], [{x:NaN,y:1}], [{x:Infinity,y:0}], [{x:1000001,y:0}], Array.from({length:MAX_SAMPLES+1},()=>({x:0,y:0}))].map(points=>({points})))("refuses empty, nonfinite, extreme or oversized samples", ({points}) => {
  expect(strokeOf(points,style)).toBeNull();
 });
 it.each([{color:"url(https://x)",width:3},{color:"#000",width:0},{color:"#000",width:65},{color:"#000",width:NaN}])("refuses invalid pen settings before making a stroke", (settings) => {expect(strokeOf([{x:0,y:0}],settings)).toBeNull();});
 it("preserves token references instead of resolving them", () => {expect(strokeOf([{x:0,y:0}],{color:"var(--accent)",width:2})?.color).toBe("var(--accent)");});
 it("bounds a long gesture while retaining its first and latest sample", () => {
  const points: {x:number;y:number}[]=[];
  for(let i=0;i<12000;i++)sample(points,{x:i,y:i%2});
  expect(points.length).toBeLessThanOrEqual(MAX_SAMPLES);
  expect(points[0]).toEqual({x:0,y:0});expect(points.at(-1)).toEqual({x:11999,y:1});
  const result=strokeOf(points,style);expect(result).not.toBeNull();expect(result?.d.length).toBeLessThan(65536);
 });
 it("keeps a dense extreme gesture as bounded vector data instead of dropping the stroke",()=>{
  const points=Array.from({length:MAX_SAMPLES},(_,i)=>({x:i+0.123,y:i%2?8191.987:0.456}));
  const result=strokeOf(points,style);expect(result).not.toBeNull();expect(result?.d.length).toBeLessThanOrEqual(65536);
 });

});
