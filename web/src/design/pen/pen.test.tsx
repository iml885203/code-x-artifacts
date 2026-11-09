import { act, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { renderIn } from "../../test-support";
import { DesignEdits } from "../edits";
import { buildsFor } from "../ops/built";
import { nodeKey } from "../ops/room";
import { FILE, seed } from "../test-room";
import { parseAttrs } from "../tree";
import { addStroke } from "./ops";
import { PenDraw } from "./PenDraw";
import { PenTool } from "./PenTool";
import { usePenSettings } from "./settings";
import { DEFAULT_PEN } from "./stroke";
const view={x:20,y:30,z:0.5};
const frames=[{frame:{id:"hero",file:FILE,w:800,h:600},x:100,y:50}];
function room(){const doc=new Y.Doc();seed(doc);return {doc,edits:new DesignEdits(doc)};}
const strokes=(edits:DesignEdits)=>[...edits.tree(FILE).nodes.values()].filter(n=>n.tag==="svg");
const pathAttrs=(edits:DesignEdits)=>parseAttrs(edits.map.get(nodeKey(FILE,[...edits.tree(FILE).nodes.values()].find(n=>n.tag==="path")?.pid??"","attrs")));
function renderDrawing(options:{active?:boolean;editable?:boolean}={}){
 const {doc,edits}=room();
 function Drawing(){
  const [,update]=useState(0);useEffect(()=>edits.onChange(()=>{update(n=>n+1);}),[]);
  return <><PenDraw active editable edits={edits} frames={frames} view={view} style={DEFAULT_PEN} {...options}/><output>{strokes(edits).length} strokes</output><button onClick={()=>edits.undo()}>Undo</button></>;
 }
 renderIn(<Drawing/>,"en","/a/x");return {doc,edits};
}
describe("drawing ordinary SVG in the shared room",()=>{
 it("shows a scaled preview, creates the frame-local vector on release, and one Undo removes the whole stroke",async()=>{
  const {edits}=renderDrawing(),surface=screen.getByRole("group",{name:"Draw a pencil stroke"});
  fireEvent.pointerDown(surface,{button:0,pointerId:1,clientX:75,clientY:60});
  fireEvent.pointerMove(surface,{pointerId:1,clientX:95,clientY:70});
  expect(document.querySelector(".pen-preview path")).toHaveAttribute("stroke",DEFAULT_PEN.color);
  expect(strokes(edits)).toHaveLength(0);
  fireEvent.pointerUp(surface,{pointerId:1,clientX:95,clientY:70});
  expect(screen.getByText("1 strokes")).toBeInTheDocument();expect(document.querySelector(".pen-preview")).toBeNull();
  const svg=strokes(edits)[0];expect(svg).toBeDefined();
  expect(edits.map.get(nodeKey(FILE,svg?.pid??"","s|left"))).toBe("8.5px");
  expect(edits.map.get(nodeKey(FILE,svg?.pid??"","s|width"))).toBe("43px");
  expect(pathAttrs(edits)).toContainEqual(["","stroke-width","3"]);
  const built=buildsFor(edits,FILE,new Set(["b","box","para","h1"]));
  expect(built[0]?.ns).toBe("svg");expect(built[0]?.html).toContain('d="M1.5 1.5L41.5 21.5"');
  expect(built[0]?.html).toContain('pointer-events="none"');
  await userEvent.click(screen.getByRole("button",{name:"Undo"}));expect(screen.getByText("0 strokes")).toBeInTheDocument();
 });
 it("keeps a tiny click, cancels an interrupted gesture, and ignores a gesture outside frames",()=>{
  const {edits}=renderDrawing(),surface=screen.getByRole("group");
  fireEvent.pointerDown(surface,{button:0,pointerId:1,clientX:0,clientY:0});fireEvent.pointerUp(surface,{pointerId:1,clientX:0,clientY:0});
  expect(strokes(edits)).toHaveLength(0);
  fireEvent.pointerDown(surface,{button:0,pointerId:2,clientX:75,clientY:60});fireEvent.pointerCancel(surface,{pointerId:2});
  expect(document.querySelector(".pen-preview")).toBeNull();expect(strokes(edits)).toHaveLength(0);
  fireEvent.pointerDown(surface,{button:0,pointerId:3,clientX:75,clientY:60});fireEvent.pointerUp(surface,{pointerId:3,clientX:75,clientY:60});
  expect(strokes(edits)).toHaveLength(1);expect(pathAttrs(edits)).toContainEqual(["","d","M1.5 1.5L1.5 1.5"]);
 });
 it.each([{active:false},{editable:false}])("has no drawing surface unless the tool is armed for a current editor",options=>{
  renderDrawing(options);expect(screen.queryByRole("group",{name:"Draw a pencil stroke"})).toBeNull();
 });
 it("moves, resizes and deletes the SVG through ordinary edits and a person's Undo leaves a peer's stroke",()=>{
  const a=room(),bDoc=new Y.Doc();Y.applyUpdate(bDoc,Y.encodeStateAsUpdate(a.doc));const b=new DesignEdits(bDoc);
  const mine=addStroke(a.edits,FILE,[{x:10,y:20},{x:30,y:40}],DEFAULT_PEN,"Stroke");
  const theirs=addStroke(b,FILE,[{x:50,y:60}],{color:"var(--accent)",width:4},"Peer stroke");
  Y.applyUpdate(a.doc,Y.encodeStateAsUpdate(bDoc));Y.applyUpdate(bDoc,Y.encodeStateAsUpdate(a.doc));
  expect(strokes(a.edits)).toHaveLength(2);expect(mine).not.toBeNull();expect(theirs).not.toBeNull();
  if(!mine||!theirs)return;
  a.edits.apply([{...mine,style:{left:"100px",top:"120px",width:"80px",height:"60px"}}]);
  expect(a.edits.map.get(nodeKey(FILE,mine.pid,"s|width"))).toBe("80px");
  a.edits.apply([{...mine,remove:true}]);expect(strokes(a.edits)).toHaveLength(1);
  a.edits.undo();a.edits.undo();a.edits.undo();
  expect(strokes(a.edits).map(n=>n.pid)).toEqual([theirs.pid]);
 });
 it("writes tiny offsets as decimal CSS lengths that the room accepts",()=>{
  const {edits}=room();const made=addStroke(edits,FILE,[{x:0.25000000001,y:0.25}],{color:"#000",width:0.5},"Tiny stroke");
  expect(made).not.toBeNull();if(!made)return;
  expect(edits.map.get(nodeKey(FILE,made.pid,"s|left"))).toBe("0px");
  expect(edits.map.get(nodeKey(FILE,made.pid,"s|width"))).toBe("0.5px");
 });
 it("does not write invalid samples or a stroke in a missing frame",()=>{
  const {edits}=room();expect(addStroke(edits,FILE,[],DEFAULT_PEN,"Stroke")).toBeNull();
  expect(addStroke(edits,"frames/missing.html",[{x:1,y:1}],DEFAULT_PEN,"Stroke")).toBeNull();expect(edits.steps().undo).toBe(0);
 });
});
describe("the pen's settings",()=>{
 it("arms the pen, preserves variable colors and width, and refuses invalid settings",async()=>{
  function Settings(){const settings=usePenSettings();const [tool,onTool]=useState<"select"|"pen">("select");return <><PenTool tool={tool} onTool={t=>{onTool(t==="pen"?"pen":"select");}} settings={settings}/><output>{JSON.stringify(settings.style)}</output></>;}
  renderIn(<Settings/>,"en","/a/x");await userEvent.click(screen.getByRole("button",{name:"Pen"}));
  expect(screen.getByRole("button",{name:"Pen"})).toHaveAttribute("aria-pressed","true");
  const color=screen.getByRole("textbox",{name:"Stroke color"});await userEvent.clear(color);await userEvent.type(color,"var(--accent)");
  const width=screen.getByRole("spinbutton",{name:"Stroke width"});fireEvent.change(width,{target:{value:"5"}});
  expect(screen.getByRole("status")).toHaveTextContent('"color":"var(--accent)","width":5');
  fireEvent.change(width,{target:{value:"100"}});expect(screen.getByRole("alert")).toHaveTextContent("0.5 to 64");expect(screen.getByRole("status")).toHaveTextContent('"width":5');
  await act(async()=>{await userEvent.keyboard("{Escape}");});
 });
});

describe("the pen's hint",()=>{
 it("names the pen and its shortcut through the shared hint on keyboard focus, and opening the settings leaves no hint behind",async()=>{
  function Hint(){const settings=usePenSettings();return <PenTool tool="select" onTool={vi.fn()} settings={settings}/>;}
  renderIn(<Hint/>,"en","/a/x");
  const pen=screen.getByRole("button",{name:"Pen"});
  expect(pen).not.toHaveAttribute("title");
  await userEvent.tab();
  const tip=await screen.findByRole("tooltip");
  expect(tip).toHaveTextContent("Pen (P)");expect(pen).toHaveAttribute("aria-describedby",tip.id);
  await userEvent.keyboard("{Escape}");expect(screen.queryByRole("tooltip")).toBeNull();
  await userEvent.tab({shift:true});await userEvent.tab();await userEvent.keyboard("{Enter}");
  expect(await screen.findByRole("textbox",{name:"Stroke color"})).toBeVisible();
  expect(screen.queryByRole("tooltip")).toBeNull();
 });
});
