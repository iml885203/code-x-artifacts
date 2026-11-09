import { act, fireEvent, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { renderIn } from "../../test-support";
import { DesignEdits } from "../edits";
import type { Target } from "../controller";
import { FILE, key, seed } from "../test-room";
import { PropertiesPanel } from "./PropertiesPanel";

function setup(named = false) {
  const doc = new Y.Doc(); seed(doc);
  const edits = new DesignEdits(doc);
  edits.map.set(key("box", "rest"), "background-color:background-color;background:rgb(255, 0, 0) url(/files/art/p.png) !important;border-top-color:border-top-color;border:2px solid rgb(255, 0, 0) !important;color:#112233");
  edits.map.set(key("box", "s|background-color"), "#0000ff !important");
  edits.map.set(key("box", "s|border-top-color"), "#0000ff !important");
  const computed: Record<string, string> = {"background-color":"rgb(255, 0, 0)","background-image":"url(/files/art/p.png)"};
  for (const side of ["top", "right", "bottom", "left"]) Object.assign(computed, {[`border-${side}-width`]:"2px", [`border-${side}-style`]:"solid", [`border-${side}-color`]:"rgb(255, 0, 0)"});
  if (named) {
    const source = edits.map.get(key("box", "rest"));
    if (typeof source !== "string") throw new Error("Missing authored cascade fixture");
    edits.map.set(key("box", "rest"), source.split("rgb(255, 0, 0)").join("red"));
    computed["background-color"] = "rgb(0, 0, 255)";
  }
  const target: Target = {file:FILE,frameId:"hero",pid:"box",report:{pid:"box",tag:"div",rect:{x:0,y:0,w:100,h:100},attrs:[],computed,inline:[["background","rgb(255, 0, 0) url(/files/art/p.png) !important"],["border","2px solid rgb(255, 0, 0) !important"]]}};
  renderIn(<PropertiesPanel edits={edits} targets={[target]} />);
  return {doc,edits};
}

function enter(label: string, value: string) {
  const field=screen.getByRole("textbox", {name:label});
  fireEvent.change(field,{target:{value}});fireEvent.keyDown(field,{key:"Enter"});
}

describe("Properties over the shared cascade API", () => {
  it("shows the winning authored cascade rather than a covered original slot", () => {
    setup();
    expect(screen.getByRole("textbox",{name:"Fill, hex"})).toHaveValue("#ff0000");
    // Border shorthand expansion is checked in the Chromium boundary fixture:
    // jsdom retains a covered border-top-color unlike the browser CSSOM.
  });
  it("uses current named red rather than stale computed blue for an alpha edit", () => {
    const {edits}=setup(true);
    fireEvent.click(screen.getByRole("button",{name:"Fill picker"}));
    fireEvent.click(screen.getByRole("tab",{name:"Custom"}));
    const alpha=screen.getByRole("slider",{name:"Alpha (%)"});
    fireEvent.change(alpha,{target:{value:"50"}});fireEvent.pointerUp(alpha);
    expect(edits.map.get(key("box","override|background-color"))).toBe("rgba(255, 0, 0, 0.5) !important");
  });
  it("writes explicit priority once to a covered override and reset reveals the retained cascade in one-step Undo", () => {
    const {edits}=setup();
    const original=edits.map.get(key("box","rest"));
    fireEvent.change(screen.getByRole("combobox",{name:"Border sides"}),{target:{value:"top"}});
    enter("Border colour, hex","#008000");
    expect(edits.map.get(key("box","override|border-top-color"))).toBe("#008000 !important");
    expect(edits.map.get(key("box","s|border-top-color"))).toBe("#0000ff !important");
    fireEvent.click(screen.getByRole("button",{name:"Fill picker"}));
    fireEvent.click(screen.getByRole("button",{name:"No fill"}));
    expect(edits.map.get(key("box","override|background-color"))).toBe("transparent !important");
    expect(edits.map.get(key("box","s|background-color"))).toBe("#0000ff !important");
    expect(edits.map.get(key("box","s|background-image"))).toBe("none !important");
    fireEvent.click(screen.getByRole("button",{name:"Reset"}));
    expect(edits.map.get(key("box","override|background-color"))).toBe("");
    expect(edits.map.get(key("box","s|background-color"))).toBe("#0000ff !important");
    const projected=document.createElement("div");
    projected.style.cssText=edits.frame(FILE).inline.get("box") ?? "";
    expect(projected.style.backgroundColor).toBe("rgb(255, 0, 0)");
    expect(projected.style.backgroundImage).toContain("/files/art/p.png");
    expect(edits.map.get(key("box","rest"))).toBe(original);
    expect(edits.steps().undo).toBe(3);
    act(()=>{edits.undo();});expect(edits.map.get(key("box","override|background-color"))).toBe("transparent !important");
  });
});
