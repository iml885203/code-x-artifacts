// @vitest-environment node
import { describe, expect, it } from "vitest";
import { actionForKey, artifactActions, exportLabel, visibleActions, type ActionViewer } from "./artifactActionList";
import { translator } from "./i18n";
import { artifact } from "./test-support";

const ids = (a: ReturnType<typeof artifact>, viewer: ActionViewer) => visibleActions(a, viewer).map((x) => x.id);
const owner = artifact({ title: "Notes", author: "alice", canChange: true, canEdit: true, canComment: true });
const reader = artifact({ title: "Notes", author: "alice" });

describe("the artifact action list", () => {
  it("has one entry per id and every entry is a link or runs something", () => {
    expect(new Set(artifactActions.map((a) => a.id)).size).toBe(artifactActions.length);
    for (const a of artifactActions) expect(a.href !== undefined || a.run !== undefined, a.id).toBe(true);
  });

  it("lists what the viewer offers, in its order, to someone who may change the artifact", () => {
    expect(ids(owner, { signedIn: true, surface: "viewer" })).toEqual([
      "comments", "versions", "files", "pin", "copy", "openRaw", "delete",
    ]);
  });

  it("lists what a card offers, in its order, without the panels", () => {
    expect(ids(owner, { signedIn: true, surface: "card" })).toEqual(["pin", "rename", "copy", "copyLink", "delete"]);
  });

  it("never offers a card an agent prompt, and gives rename to whoever may edit it, delete to the author alone", () => {
    expect(ids(reader, { signedIn: true, surface: "card" })).toEqual(["pin", "copy", "copyLink"]);
    // An editor may rename, as in the viewer's title, but not delete.
    const editor = artifact({ title: "Notes", author: "alice", canEdit: true, role: "editor" });
    expect(ids(editor, { signedIn: true, surface: "card" })).toEqual(["pin", "rename", "copy", "copyLink"]);
    expect(ids(owner, { signedIn: true, surface: "viewer" })).not.toContain("rename");
  });

  it("finds the action a letter runs, whatever its case, and none for a letter nothing uses", () => {
    const card = { signedIn: true, surface: "card" } as const;
    expect(actionForKey(owner, card, "p")?.id).toBe("pin");
    expect(actionForKey(owner, card, "R")?.id).toBe("rename");
    expect(actionForKey(owner, card, "c")?.id).toBe("copyLink");
    expect(actionForKey(owner, card, "x")).toBeUndefined();
    expect(actionForKey(reader, card, "r")).toBeUndefined();
  });

  it("leaves out what needs a person or the right to change", () => {
    expect(ids(reader, { signedIn: true, surface: "viewer" })).toEqual(["pin", "copy", "openRaw"]);
    expect(ids(reader, { signedIn: false, surface: "viewer" })).toEqual(["openRaw"]);
  });

  it("sets the dangerous actions apart", () => {
    expect(artifactActions.filter((a) => a.danger === true).map((a) => a.id)).toEqual(["delete"]);
  });
});

describe("the Export group's labels", () => {
  const t = translator({ "export.md": "Markdown (.md)", "export.docx": "Word (.docx)", "export.pptx": "PowerPoint (.pptx)", "export.html": "HTML (.html)", "export.pdf": "PDF", "export.zip": "Zip (.zip)" });

  it.each([["md", "Markdown (.md)"], ["docx", "Word (.docx)"], ["pptx", "PowerPoint (.pptx)"], ["html", "HTML (.html)"], ["pdf", "PDF"], ["zip", "Zip (.zip)"]])("names %s from the catalogue", (format, label) => {
    expect(exportLabel(t, format)).toBe(label);
  });

  it("shows a format the catalogue has no entry for as its file type", () => {
    expect(exportLabel(t, "epub")).toBe(".epub");
  });
});
