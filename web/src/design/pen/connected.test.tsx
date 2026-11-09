import { useRef, type ReactNode } from "react";
import userEvent from "@testing-library/user-event";
import { Tooltip } from "../../Tooltip";
import { PenTool } from "./PenTool";
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { renderIn } from "../../test-support";
import { DesignEdits } from "../edits";
import { Editor } from "../controller";
import { useShellKeys, type Held } from "../toolbar/useShellKeys";
import { buildsFor } from "../ops/built";
import { nodeKey } from "../ops/room";
import { FILE, seed } from "../test-room";
import { parseAttrs } from "../tree";
import { PathDraw } from "./PathDraw";
import { DEFAULT_PEN } from "./stroke";
const frames = [{frame:{id:"hero",file:FILE,w:800,h:600},x:100,y:50}];
const view = {x:20,y:30,z:0.5};
const labels = {draw:"Draw a connected path",path:"Path"};
function ShellShortcuts({ edits }: { edits: DesignEdits }) {
 const held=useRef<Held>({edits,editor:new Editor(edits,{apply:()=>Promise.resolve()}),targets:()=>[],selected:[],ops:null,structure:null,tool:"pen"});
 const noop=()=>undefined;
 useShellKeys(false,held,{armTool:noop,zoomStep:noop,zoomTo:noop,fit:noop,setTool:noop,clear:noop,clearNotice:noop,togglePanel:noop,holdSpace:noop});
 return null;
}
function setup(options: {active?:boolean;editable?:boolean;edits?:DesignEdits|null}={}, extras?:ReactNode) {
 const doc=new Y.Doc(); seed(doc);const edits=new DesignEdits(doc);
 const created: {file:string;pid:string}[]=[];
 const props={active:true,editable:true,edits,frames,view,style:DEFAULT_PEN,labels,onCreated:(ref:{file:string;pid:string})=>created.push(ref),...options};
 const rendered=renderIn(<><ShellShortcuts edits={edits}/><PathDraw {...props}/>{extras}</>,"en","/a/x");
 return {doc,edits,created,...rendered,props};
}
function anchor(surface:HTMLElement,x:number,y:number,drag?:{x:number;y:number}) {
 const clientX=70+x/2,clientY=55+y/2;
 fireEvent.pointerDown(surface,{button:0,pointerId:1,clientX,clientY});
 if(drag)fireEvent.pointerMove(surface,{pointerId:1,clientX:70+drag.x/2,clientY:55+drag.y/2});
 fireEvent.pointerUp(surface,{pointerId:1,clientX:70+(drag?.x??x)/2,clientY:55+(drag?.y??y)/2});
}
const paths=(edits:DesignEdits)=>[...edits.tree(FILE).nodes.values()].filter(n=>n.tag==="path");
const attrs=(edits:DesignEdits,pid:string)=>parseAttrs(edits.map.get(nodeKey(FILE,pid,"attrs")));
describe("connected Pen interaction",()=>{
 it("previews the next straight or curved segment under pan and zoom without saving its cursor endpoint",()=>{
  const {edits}=setup();const surface=screen.getByRole("group");
  anchor(surface,10,20);
  fireEvent.pointerMove(surface,{pointerId:1,clientX:100,clientY:85});
  expect(document.querySelector(".path-next")?.getAttribute("d")).toBe("M1.5 1.5L51.5 41.5");
  expect(document.querySelectorAll(".path-anchor")).toHaveLength(1);expect(paths(edits)).toHaveLength(0);expect(edits.steps().undo).toBe(0);
  anchor(surface,60,20,{x:80,y:40});
  fireEvent.pointerMove(surface,{pointerId:1,clientX:120,clientY:85});
  expect(document.querySelector(".path-next")?.getAttribute("d")).toBe("M1.5 1.5C21.5 21.5 41.5 41.5 41.5 41.5");
  expect(document.querySelectorAll(".path-anchor")).toHaveLength(2);expect(paths(edits)).toHaveLength(0);
  fireEvent.pointerLeave(surface);expect(document.querySelector(".path-next")).toBeNull();
  fireEvent.pointerMove(surface,{pointerId:1,clientX:120,clientY:85});
  fireEvent.keyDown(surface,{key:"Escape"});
  expect(attrs(edits,paths(edits)[0]?.pid??"")).toContainEqual(["","d","M1.5 21.5C1.5 21.5 31.5 1.5 51.5 21.5"]);
  expect(edits.steps().undo).toBe(1);
  fireEvent.keyDown(surface,{key:"z",ctrlKey:true});expect(paths(edits)).toHaveLength(0);
 });
 it("undoes draft anchors and an active curve before shared edits and keeps an emptied draft local until Escape",()=>{
  const {edits}=setup();const surface=screen.getByRole("group");
  edits.apply([{file:FILE,pid:"box",style:{left:"99px"}}]);
  anchor(surface,10,20);anchor(surface,60,20,{x:80,y:40});
  fireEvent.pointerMove(surface,{pointerId:1,clientX:120,clientY:85});
  expect(fireEvent.keyDown(surface,{key:"z",metaKey:true})).toBe(false);
  expect(document.querySelectorAll(".path-anchor")).toHaveLength(1);
  expect(document.querySelector(".path-next")?.getAttribute("d")).toBe("M1.5 1.5L91.5 41.5");
  fireEvent.pointerDown(surface,{button:0,pointerId:4,clientX:110,clientY:75});
  fireEvent.pointerMove(surface,{pointerId:4,clientX:120,clientY:90});
  fireEvent.keyDown(surface,{key:"z",ctrlKey:true});
  fireEvent.pointerUp(surface,{pointerId:4,clientX:120,clientY:90});
  fireEvent.lostPointerCapture(surface,{pointerId:4});
  expect(document.querySelectorAll(".path-anchor")).toHaveLength(1);expect(paths(edits)).toHaveLength(0);
  for(let i=0;i<3;i++)expect(fireEvent.keyDown(surface,{key:"z",ctrlKey:true})).toBe(false);
  expect(document.querySelector(".pen-preview")).toBeNull();
  expect(edits.map.get(nodeKey(FILE,"box","s|left"))).toBe(" 99px");expect(edits.steps().undo).toBe(1);
  fireEvent.keyDown(surface,{key:"Escape"});
  fireEvent.keyDown(surface,{key:"z",ctrlKey:true});
  expect(edits.map.get(nodeKey(FILE,"box","s|left"))).not.toBe(" 99px");
 });
 it("ends an emptied draft on tool exit so ordinary Undo can reach the document again",()=>{
  const {edits,props,rerender}=setup();const surface=screen.getByRole("group");
  edits.apply([{file:FILE,pid:"box",style:{left:"99px"}}]);anchor(surface,10,20);
  fireEvent.keyDown(surface,{key:"z",ctrlKey:true});
  rerender(<><ShellShortcuts edits={edits}/><PathDraw {...props} active={false}/></>);
  fireEvent.keyDown(window,{key:"z",ctrlKey:true});
  expect(edits.map.get(nodeKey(FILE,"box","s|left"))).not.toBe(" 99px");expect(paths(edits)).toHaveLength(0);
 });
 it("leaves form and menu Undo shortcuts alone while a Pen draft is open",()=>{
  const {edits}=setup({},<><input aria-label="Draft setting"/><div role="menu"><button>Option</button></div></>);
  const surface=screen.getByRole("group");edits.apply([{file:FILE,pid:"box",style:{left:"99px"}}]);anchor(surface,10,20);anchor(surface,60,20);
  expect(fireEvent.keyDown(screen.getByRole("textbox"),{key:"z",ctrlKey:true})).toBe(true);
  expect(fireEvent.keyDown(screen.getByRole("button",{name:"Option"}),{key:"z",metaKey:true})).toBe(true);
  expect(document.querySelectorAll(".path-anchor")).toHaveLength(2);expect(paths(edits)).toHaveLength(0);
  expect(edits.map.get(nodeKey(FILE,"box","s|left"))).toBe(" 99px");
 });

 it("continues between gestures, closes at its start under zoom, and preserves selection identity through room reload, move, delete and Undo",()=>{
  const {edits,doc,created}=setup(); const surface=screen.getByRole("group",{name:labels.draw});
  anchor(surface,10,20);anchor(surface,60,20);anchor(surface,100,60,{x:120,y:80});
  expect(paths(edits)).toHaveLength(0);expect(document.querySelectorAll(".path-anchor")).toHaveLength(3);
  expect(document.querySelector(".pen-preview path")?.getAttribute("d")).toContain("C");
  fireEvent.pointerMove(surface,{pointerId:1,clientX:75,clientY:65});
  expect(document.querySelector(".path-next")?.getAttribute("d")).toBe("M91.5 41.5C111.5 61.5 1.5 1.5 1.5 1.5");
  anchor(surface,10,20);
  expect(paths(edits)).toHaveLength(1);expect(created).toHaveLength(1);expect(edits.steps().undo).toBe(1);
  const ref=created[0];if(!ref)throw new Error("missing selection");
  const path=paths(edits)[0];if(!path)throw new Error("missing path");
  expect(attrs(edits,path.pid)).toContainEqual(["","d","M1.5 1.5L51.5 1.5C51.5 1.5 71.5 21.5 91.5 41.5C111.5 61.5 1.5 1.5 1.5 1.5Z"]);
  const reloaded=new Y.Doc();Y.applyUpdate(reloaded,Y.encodeStateAsUpdate(doc));const saved=new DesignEdits(reloaded);
  expect(saved.tree(FILE).nodes.get(ref.pid)?.tag).toBe("svg");expect(attrs(saved,path.pid)).toEqual(attrs(edits,path.pid));
  const built=buildsFor(saved,FILE,new Set(["b","box","para","h1"]));expect(built[0]?.html).toContain(`data-pid="${ref.pid}"`);
  edits.apply([{...ref,style:{left:"100px",top:"120px"}}]);expect(edits.map.get(nodeKey(FILE,ref.pid,"s|left"))).toBe("100px");
  edits.apply([{...ref,remove:true}]);expect(paths(edits)).toHaveLength(0);
  edits.undo();expect(paths(edits)).toHaveLength(1);edits.undo();expect(edits.map.get(nodeKey(FILE,ref.pid,"s|left"))).toBe("8.5px");
  edits.undo();expect(paths(edits)).toHaveLength(0);
 });
 it("Escape finalizes an open path and consumes the key before shell shortcuts",()=>{
  const {edits}=setup();const surface=screen.getByRole("group");anchor(surface,10,20);anchor(surface,60,20);
  expect(fireEvent.keyDown(window,{key:"Escape"})).toBe(false);
  expect(paths(edits)).toHaveLength(1);expect(attrs(edits,paths(edits)[0]?.pid??"")).toContainEqual(["","d","M1.5 1.5L51.5 1.5"]);
  expect(document.querySelector(".pen-preview")).toBeNull();
 });
 it("cancels before a usable path and on interrupted input without writing or losing earlier anchors",()=>{
  const {edits}=setup();const surface=screen.getByRole("group");anchor(surface,10,20);fireEvent.keyDown(window,{key:"Escape"});expect(edits.steps().undo).toBe(0);
  fireEvent.pointerDown(surface,{button:0,pointerId:2,clientX:80,clientY:80});fireEvent.pointerCancel(surface,{pointerId:2});
  fireEvent.keyDown(window,{key:"Escape"});expect(paths(edits)).toHaveLength(0);
  anchor(surface,10,20);anchor(surface,60,20);
  fireEvent.pointerDown(surface,{button:0,pointerId:3,clientX:150,clientY:150});fireEvent.pointerCancel(surface,{pointerId:3});
  fireEvent.keyDown(window,{key:"Escape"});expect(paths(edits)).toHaveLength(1);
  expect(attrs(edits,paths(edits)[0]?.pid??"")).toContainEqual(["","d","M1.5 1.5L51.5 1.5"]);
 });
 it.each([{active:false},{editable:false},{edits:null}])("refuses drawing when unavailable",options=>{
  const {edits}=setup(options);expect(screen.queryByRole("group")).toBeNull();fireEvent.keyDown(window,{key:"Escape"});expect(edits.steps().undo).toBe(0);
 });
 it("drops a draft when permission is revoked and ignores an outside start",()=>{
  const {edits,rerender,props}=setup();const surface=screen.getByRole("group");
  fireEvent.pointerDown(surface,{button:0,pointerId:1,clientX:0,clientY:0});fireEvent.pointerUp(surface,{pointerId:1,clientX:0,clientY:0});expect(document.querySelector(".pen-preview")).toBeNull();
  anchor(surface,10,20);anchor(surface,60,20);rerender(<PathDraw {...props} editable={false}/>);fireEvent.keyDown(window,{key:"Escape"});expect(paths(edits)).toHaveLength(0);
 });
 it("lets settings inputs and menus dismiss with Escape without committing the canvas draft",async()=>{
  const user=userEvent.setup();
  const {edits}=setup({},<PenTool tool="pen" onTool={vi.fn()} settings={{style:DEFAULT_PEN,onChange:vi.fn()}}/>);
  const surface=screen.getByRole("group");anchor(surface,10,20);anchor(surface,60,20);
  await user.click(screen.getByRole("button",{name:"Pen"}));
  await user.click(screen.getByRole("textbox",{name:"Stroke color"}));
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("menu")).toBeNull();expect(paths(edits)).toHaveLength(0);
  expect(document.querySelectorAll(".path-anchor")).toHaveLength(2);
  await user.click(screen.getByRole("button",{name:"Pen"}));
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("menu")).toBeNull();expect(paths(edits)).toHaveLength(0);
  anchor(surface,100,20);await user.keyboard("{Escape}");expect(paths(edits)).toHaveLength(1);
 });
 it.each(["focus","hover"])("dismisses a real tooltip opened by %s on Escape before the canvas draft can finalize",async mode=>{
  const user=userEvent.setup();const {edits}=setup({},<Tooltip content="Drawing options"><button>Options</button></Tooltip>);
  const surface=screen.getByRole("group");anchor(surface,10,20);anchor(surface,60,20);
  if(mode==="focus")await user.tab();else await user.hover(screen.getByRole("button",{name:"Options"}));
  expect(await screen.findByRole("tooltip")).toHaveTextContent("Drawing options");
  if(mode==="hover")expect(surface).toHaveFocus();
  await user.keyboard("{Escape}");expect(screen.queryByRole("tooltip")).toBeNull();expect(paths(edits)).toHaveLength(0);
  expect(document.querySelectorAll(".path-anchor")).toHaveLength(2);
  anchor(surface,100,20);await user.keyboard("{Escape}");expect(paths(edits)).toHaveLength(1);
 });

 it("finalizes a focused canvas draft when other dismissible controls are closed",async()=>{
  const user=userEvent.setup();const {edits}=setup({},<><div role="dialog" hidden>Closed dialog</div><div role="menu" data-state="closed">Closed menu</div></>);
  const surface=screen.getByRole("group");anchor(surface,10,20);anchor(surface,60,20);
  expect(surface).toHaveFocus();await user.keyboard("{Escape}");expect(paths(edits)).toHaveLength(1);
 });

});
