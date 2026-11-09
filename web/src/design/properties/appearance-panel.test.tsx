import { act, fireEvent, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { renderIn } from "../../test-support";
import { DesignEdits } from "../edits";
import type { Target } from "../controller";
import { FILE, key, seed } from "../test-room";
import { parseColor } from "./css";
import { PropertiesPanel } from "./PropertiesPanel";

function setup(computed: Record<string,string>, authored: Record<string,string> = {}, tag="div") {
  const doc=new Y.Doc(); seed(doc);
  const edits=new DesignEdits(doc);
  Object.entries(authored).forEach(([p,v]) => { edits.map.set(key("box",`s|${p}`),v); });
  const target: Target={frameId:"hero",file:FILE,pid:"box",report:{pid:"box",tag,rect:{x:0,y:0,w:100,h:100},attrs:[],computed,inline:Object.entries(authored)}};
  let view=renderIn(<PropertiesPanel edits={edits} targets={[target]} />);
  const reselect = (freshComputed: Record<string,string>) => {
    view.unmount();
    const report = {...target.report, computed:freshComputed, inline:[...(edits.frame(FILE).styles.get("box") ?? [])]};
    view=renderIn(<PropertiesPanel edits={edits} targets={[{...target,report}]} />);
  };
  const entry=(p:string) => edits.frame(FILE).styles.get("box")?.get(p);
  return {edits,doc,target,view,entry,reselect};
}
function enter(label:string,value:string) {
  const field=screen.getByRole("textbox",{name:label});
  fireEvent.change(field,{target:{value}});fireEvent.keyDown(field,{key:"Enter"});
}

describe("appearance room outcomes", () => {
  it("changes wrapping once, preserves children and other declarations, and Undo restores the authored reverse wrap", () => {
    const {edits,doc,entry,reselect}=setup({display:"flex","flex-wrap":"wrap-reverse"},{"flex-wrap":"wrap-reverse","gap":"9px"});
    const children=[...doc.getMap("design")].filter(([k]) => k.includes("|para|") || k.includes("|~t1|"));
    const control=screen.getByRole("combobox",{name:"Wrap"});
    expect(control).toHaveValue("wrap-reverse");
    fireEvent.change(control,{target:{value:"wrap"}});
    expect(entry("flex-wrap")).toBe("wrap");expect(entry("gap")).toBe("9px");
    expect([...doc.getMap("design")].filter(([k]) => k.includes("|para|") || k.includes("|~t1|"))).toEqual(children);
    expect(edits.steps().undo).toBe(1);
    reselect({display:"flex","flex-wrap":"wrap",gap:"9px"});
    expect(screen.getByRole("combobox",{name:"Wrap"})).toHaveValue("wrap");
    expect(edits.steps().undo).toBe(1);
    act(()=>{edits.undo();});expect(entry("flex-wrap")).toBe("wrap-reverse");
    reselect({display:"flex","flex-wrap":"wrap-reverse",gap:"9px"});
    expect(screen.getByRole("combobox",{name:"Wrap"})).toHaveValue("wrap-reverse");
  });
  it("shows mixed borders and edits only the chosen side without duplicating important or escalating normal styles", () => {
    const computed=Object.fromEntries(["top","right","bottom","left"].flatMap((s,i)=>[[`border-${s}-width`,`${String(i+1)}px`],[`border-${s}-style`,"solid"],[`border-${s}-color`,i===0 ? "rgb(1,2,3)" : "rgb(4,5,6)"]]));
    const {entry,edits,reselect}=setup(computed,{"border-top-width":"1px !important","border-right-width":"2px","border-left-color":"var(--accent)","color":"#112233"});
    expect(screen.getByRole("textbox",{name:"Border width"})).toHaveValue("");
    expect(screen.getByRole("textbox",{name:"Border width"})).toHaveAttribute("placeholder","Mixed");
    fireEvent.change(screen.getByRole("combobox",{name:"Border sides"}),{target:{value:"top"}});
    expect(screen.getByRole("textbox",{name:"Border width"})).toHaveValue("1");
    enter("Border width","8");
    expect(entry("border-top-width")).toBe("8px !important");
    expect(entry("border-right-width")).toBe("2px");expect(entry("border-left-color")).toBe("var(--accent)");expect(entry("color")).toBe("#112233");
    reselect({...computed,"border-top-width":"8px"});
    expect(screen.getByRole("textbox",{name:"Border width"})).toHaveAttribute("placeholder","Mixed");
    fireEvent.change(screen.getByRole("combobox",{name:"Border sides"}),{target:{value:"top"}});
    expect(screen.getByRole("textbox",{name:"Border width"})).toHaveValue("8");
    expect(screen.getByRole("combobox",{name:"Border style"})).toHaveValue("solid");
    fireEvent.change(screen.getByRole("combobox",{name:"Border sides"}),{target:{value:"right"}});
    expect(screen.getByRole("textbox",{name:"Border width"})).toHaveValue("2");
    enter("Border width","9");
    expect(entry("border-right-width")).toBe("9px");expect(edits.steps().undo).toBe(2);
    act(()=>{edits.undo();});expect(entry("border-right-width")).toBe("2px");
  });
  it("keeps a room style chosen before a stale frame report when setting the same border width", () => {
    const {entry}=setup({"border-top-style":"none","border-top-width":"0px","border-top-color":"rgb(0,0,0)"});
    fireEvent.change(screen.getByRole("combobox",{name:"Border sides"}),{target:{value:"top"}});
    fireEvent.change(screen.getByRole("combobox",{name:"Border style"}),{target:{value:"dashed"}});
    expect(entry("border-top-style")).toBe("dashed");
    enter("Border width","4");
    expect(entry("border-top-width")).toBe("4px");
    expect(entry("border-top-style")).toBe("dashed");
  });
  it("keeps a room shorthand style changed before the stale frame reports it", () => {
    const {entry,edits}=setup({"border-top-style":"none","border-top-width":"0px"});
    act(()=>{edits.apply([{file:FILE,pid:"box",style:{"border-style":"dotted dashed solid double"}}]);});
    fireEvent.change(screen.getByRole("combobox",{name:"Border sides"}),{target:{value:"right"}});
    enter("Border width","4");
    expect(entry("border-right-width")).toBe("4px");
    expect(entry("border-right-style")).toBeUndefined();
    expect(entry("border-style")).toBe("dotted dashed solid double");
  });
  it("distinguishes transparent color, no fill and reset with authored images and token bindings in one-step history", () => {
    const {entry,edits,reselect}=setup({"background-color":"rgb(51,102,153)"},{"background-color":"var(--accent) !important","background-image":"url(/files/art/image.png)","background-size":"cover","color":"#112233"});
    act(()=>{edits.map.set("t|--accent","#336699");});
    const picker = () => screen.getByRole("dialog",{name:"Fill"});
    const binding = () => within(picker()).getByRole("button",{name:/--accent/});
    fireEvent.click(screen.getByRole("button",{name:"Fill picker"}));
    expect(binding()).toHaveAttribute("aria-pressed","true");
    fireEvent.keyDown(document,{key:"Escape"});
    enter("Fill, Alpha (%)","0");
    expect(entry("background-color")).toBe("rgba(51, 102, 153, 0) !important");
    expect(entry("background-image")).toBe("url(/files/art/image.png)");
    reselect({"background-color":"rgba(51, 102, 153, 0)"});
    expect(screen.getByRole("textbox",{name:"Fill, hex"})).toHaveValue("#336699");
    expect(screen.getByRole("textbox",{name:"Fill, Alpha (%)"})).toHaveValue("0");
    fireEvent.click(screen.getByRole("button",{name:"Fill picker"}));
    expect(binding()).toHaveAttribute("aria-pressed","false");
    fireEvent.click(screen.getByRole("button",{name:"No fill"}));
    expect(entry("background-color")).toBe("transparent !important");expect(entry("background-image")).toBe("none !important");
    expect(edits.steps().undo).toBe(2);
    expect(binding()).toHaveAttribute("aria-pressed","false");
    expect(within(picker()).queryByText("var(--accent)")).toBeNull();
    expect(within(picker()).getByText("#000000")).toBeVisible();
    reselect({"background-color":"rgba(0, 0, 0, 0)","background-image":"none"});
    expect(screen.getByRole("textbox",{name:"Fill, Alpha (%)"})).toHaveValue("0");
    fireEvent.click(screen.getByRole("button",{name:"Fill picker"}));
    expect(binding()).toHaveAttribute("aria-pressed","false");
    expect(within(picker()).queryByText("var(--accent)")).toBeNull();
    fireEvent.click(screen.getByRole("button",{name:"Reset"}));
    for(const p of ["background-color","background-image","background-size"]) expect(entry(p)).toBe("");
    expect(entry("color")).toBe("#112233");expect(edits.steps().undo).toBe(3);
    reselect({"background-color":"rgba(0, 0, 0, 0)"});
    fireEvent.click(screen.getByRole("button",{name:"Fill picker"}));
    expect(binding()).toHaveAttribute("aria-pressed","false");
    expect(within(picker()).queryByText("var(--accent)")).toBeNull();
    expect(within(picker()).getByText("#000000")).toBeVisible();
    act(()=>{edits.undo();});expect(entry("background-color")).toBe("transparent !important");
    act(()=>{edits.undo();});expect(entry("background-image")).toBe("url(/files/art/image.png)");
    act(()=>{edits.undo();});expect(entry("background-color")).toBe("var(--accent) !important");
    expect(binding()).toHaveAttribute("aria-pressed","true");
    expect(within(picker()).getByText("var(--accent)")).toBeVisible();
  });
  it("keeps a chosen hue while black has no visible saturation or brightness", () => {
    const {entry}=setup({"background-color":"rgb(0,0,0)"});
    fireEvent.click(screen.getByRole("button",{name:"Fill picker"}));
    fireEvent.click(screen.getByRole("tab",{name:"Custom"}));
    const hue=screen.getByRole("slider",{name:"Hue"});
    fireEvent.change(hue,{target:{value:"120"}});fireEvent.keyUp(hue,{key:"ArrowRight"});
    const area=screen.getByRole("slider",{name:"Saturation, Brightness"});
    fireEvent.keyDown(area,{key:"ArrowRight",shiftKey:true});fireEvent.keyDown(area,{key:"ArrowUp",shiftKey:true});
    const color=parseColor(entry("background-color") ?? "");
    expect(color).not.toBeNull();
    expect(color?.g).toBeGreaterThan(color?.r ?? 0);
  });
  it("restores a canceled hue before the next saturation and brightness gesture", () => {
    const {entry}=setup({"background-color":"rgb(128,64,64)"});
    fireEvent.click(screen.getByRole("button",{name:"Fill picker"}));
    fireEvent.click(screen.getByRole("tab",{name:"Custom"}));
    const hue=screen.getByRole("slider",{name:"Hue"});
    fireEvent.pointerDown(hue);fireEvent.change(hue,{target:{value:"120"}});fireEvent.pointerCancel(hue);
    const area=screen.getByRole("slider",{name:"Saturation, Brightness"});
    fireEvent.keyDown(area,{key:"ArrowRight",shiftKey:true});
    const color=parseColor(entry("background-color") ?? "");
    expect(color?.r).toBeGreaterThan(color?.g ?? 0);
    expect(hue).toHaveValue("0");
  });
  it("commits one alpha gesture once and does not repeat it on blur", () => {
    const {entry,edits}=setup({"background-color":"rgb(51,102,153)"});
    fireEvent.click(screen.getByRole("button",{name:"Fill picker"}));
    fireEvent.click(screen.getByRole("tab",{name:"Custom"}));
    const slider=screen.getByRole("slider",{name:"Alpha (%)"});
    for(const value of [20,30,40]) fireEvent.change(slider,{target:{value:String(value)}});
    expect(edits.steps().undo).toBe(0);
    fireEvent.pointerUp(slider);
    expect(entry("background-color")).toBe("rgba(51, 102, 153, 0.4)");
    expect(edits.steps().undo).toBe(1);
    fireEvent.blur(slider);
    expect(edits.steps().undo).toBe(1);
  });
  it("omits text and stack authoring for a selected image and lets sections collapse without changing the room", () => {
    const {doc}=setup({}, {},"img");
    expect(screen.queryByRole("textbox",{name:"Font"})).toBeNull();
    expect(screen.queryByRole("group",{name:"Direction"})).toBeNull();
    const before=Y.encodeStateAsUpdate(doc);
    const summary=screen.getByText("Size",{selector:"summary"});
    fireEvent.click(summary);
    expect(summary.parentElement).not.toHaveAttribute("open");
    expect(Y.encodeStateAsUpdate(doc)).toEqual(before);
  });
});
