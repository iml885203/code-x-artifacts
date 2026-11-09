import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { renderIn } from "../../test-support";
import { DesignEdits } from "../edits";
import type { ElementReport } from "../inspect";
import { FILE, seed } from "../test-room";
import { SidePanel } from "./SidePanel";

const report: ElementReport = { pid: "box", tag: "div", rect: { x: 0, y: 0, w: 30, h: 40 }, attrs: [], inline: [["left", "10px"]], computed: { width: "30px" } };

function editorProps() {
  const doc = new Y.Doc();
  seed(doc);
  const edits = new DesignEdits(doc);
  return { edits, artifactId: "x", targets: [{ frameId: "hero", file: FILE, pid: "box", report }] };
}

describe("the side panel", () => {
  it("gives someone who can edit the properties first and the code on the other tab", async () => {
    const user = userEvent.setup();
    renderIn(<SidePanel report={report} count={1} onClose={vi.fn()} edit={editorProps()} />, "en", "/a/x");
    expect(screen.getByRole("tab", { name: "Design" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("textbox", { name: "Width" })).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Code" }));
    expect(screen.queryByRole("textbox", { name: "Width" })).not.toBeInTheDocument();
    expect(screen.getByText("Inline style")).toBeInTheDocument();
  });

  it("gives everyone else the code alone, with no control that changes the design", () => {
    renderIn(<SidePanel report={report} count={1} onClose={vi.fn()} />, "en", "/a/x");
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Code" })).toBeInTheDocument();
  });
});
