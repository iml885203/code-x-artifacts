import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { I18nProvider } from "../../i18n";
import { DesignEdits } from "../edits";
import { readTokens } from "../tokens";
import { TokenSection } from "./TokenSection";
import { TokenColor } from "./TokenColor";

// The panel's real catalogue and room; no mocked edit service.
import en from "../../../../internal/i18n/locales/en/props.json";
const words = en;

describe("shared design tokens", () => {
  it("writes a variable reference when choosing a token and keeps a bound selection", () => {
    let binding = "";
    render(<I18nProvider lang="en" strings={words}><TokenColor label="Fill" color={{ r: 1, g: 2, b: 3, a: 1 }} source="var(--accent)" tokens={{ "--accent": "#336699", "--space": "16px" }} onColor={() => undefined} onToken={(value) => { binding=value; }} /></I18nProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Fill picker" }));
    const choice = screen.getByRole("button", { name: /--accent/ });
    expect(choice).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(choice);
    expect(binding).toBe("var(--accent)");
    expect(screen.queryByRole("button", {name:/--space/})).toBeNull();
  });
  it("commits a shared value once, undoes it, and refuses a declaration escape", () => {
    const doc=new Y.Doc(); const edits=new DesignEdits(doc);
    edits.map.set("t|--accent","#336699");
    render(<I18nProvider lang="en" strings={words}><TokenSection edits={edits} /></I18nProvider>);
    const input=screen.getByRole("textbox",{name:"--accent"});
    fireEvent.change(input,{target:{value:"#112233"}});fireEvent.keyDown(input,{key:"Enter"});
    expect(readTokens(edits)["--accent"]).toBe("#112233");
    expect(edits.steps().undo).toBe(1);
    act(()=>{edits.undo();});expect(input).toHaveValue("#336699");
    fireEvent.change(input,{target:{value:"red;}body{color:blue"}});fireEvent.keyDown(input,{key:"Enter"});
    expect(input).toHaveValue("#336699");expect(screen.getByRole("alert")).toBeVisible();
    edits.destroy();doc.destroy();
  });
});
