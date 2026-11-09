import { NodeSelection } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import { redo, undo } from "y-prosemirror";
import { describe, expect, it, vi } from "vitest";
import { addPictures, deleteImage, imageFilesOf, imageInfo, imageInput, insertImage, setImage } from "./image";
import { uploadName } from "../uploadName";
import { blocks, outline, para, person, reload, room, select } from "./testRoom";

const image = (id: string, src: string, extra: Record<string, unknown> = {}) => ({ type: "image", attrs: { id, src, alt: null, size: null, ...extra } });
const run = (view: EditorView, command: (s: EditorView["state"], d: EditorView["dispatch"]) => boolean) => command(view.state, view.dispatch);
const kinds = (view: EditorView) => blocks(view).map((b) => b.type as string);

describe("inserting a picture", () => {
  it("goes after the caret's block, is selected so its tools show, gets its own id, and survives Undo, Redo and reload", () => {
    const doc = room(para("b_1", "before"), para("b_2", "after"));
    const view = person(doc);
    select(view, "before", 6);
    expect(run(view, insertImage("/files/abc/shot.png"))).toBe(true);
    expect(kinds(view)).toEqual(["paragraph", "image", "paragraph"]);
    expect(imageInfo(view.state)).toMatchObject({ alt: "", size: null });
    expect(view.state.selection).toBeInstanceOf(NodeSelection);
    expect(blocks(view)[1]?.attrs).toMatchObject({ src: "/files/abc/shot.png", alt: null });
    expect((blocks(view)[1]?.attrs as { id: string }).id).toMatch(/^b_[a-z0-9]{8}$/);

    undo(view.state);
    expect(kinds(view)).toEqual(["paragraph", "paragraph"]);
    redo(view.state);
    expect(kinds(view)).toEqual(["paragraph", "image", "paragraph"]);
    expect(kinds(person(reload(doc)))).toEqual(["paragraph", "image", "paragraph"]);
  });

  it("takes an empty paragraph's place, and leaves a paragraph to go on typing in when the picture ends the doc", () => {
    const view = person(room(para("b_1", "")));
    run(view, insertImage("/files/abc/shot.png"));
    expect(kinds(view)).toEqual(["image", "paragraph"]);
  });
});

describe("the picture's tools", () => {
  const withPicture = () => {
    const view = person(room(para("b_1", "x"), image("b_i", "/files/abc/a.png"), para("b_2", "y")));
    view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, 4)));
    return view;
  };

  it("sets the alt text and each size, and takes a size off, with Undo and Redo", () => {
    const doc = room(para("b_1", "x"), image("b_i", "/files/abc/a.png"), para("b_2", "y"));
    const view = person(doc);
    view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, 4)));
    run(view, setImage({ alt: "A chart" }));
    for (const size of ["small", "medium", "large", "xlarge"] as const) {
      run(view, setImage({ size }));
      expect(imageInfo(view.state)).toMatchObject({ alt: "A chart", size });
      expect(view.dom.querySelector("img")?.getAttribute("width")).toBe({ small: "224", medium: "336", large: "504", xlarge: "672" }[size]);
    }
    run(view, setImage({ size: null }));
    expect(view.dom.querySelector("img")?.hasAttribute("width")).toBe(false);
    expect(imageInfo(view.state)).toMatchObject({ alt: "A chart", size: null });
    run(view, setImage({ alt: "" }));
    expect(blocks(view)[1]?.attrs).toMatchObject({ alt: null });
  });

  it("keeps the alt text through Undo, Redo and reload", () => {
    const doc = room(para("b_1", "x"), image("b_i", "/files/abc/a.png"));
    const view = person(doc);
    view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, 4)));
    run(view, setImage({ alt: "A chart", size: "large" }));
    expect(blocks(person(reload(doc)))[1]?.attrs).toMatchObject({ alt: "A chart", size: "large" });
    undo(view.state);
    expect(blocks(view)[1]?.attrs).toMatchObject({ alt: null, size: null });
    redo(view.state);
    expect(blocks(view)[1]?.attrs).toMatchObject({ alt: "A chart", size: "large" });
  });

  it("deletes the picture, and Undo brings it back where it was", () => {
    const view = withPicture();
    run(view, deleteImage);
    expect(kinds(view)).toEqual(["paragraph", "paragraph"]);
    undo(view.state);
    expect(kinds(view)).toEqual(["paragraph", "image", "paragraph"]);
    expect(blocks(view)[1]?.attrs).toMatchObject({ src: "/files/abc/a.png" });
  });

  it("are not available when no picture is selected", () => {
    const view = person(room(para("b_1", "x")));
    expect(run(view, setImage({ alt: "a" }))).toBe(false);
    expect(run(view, deleteImage)).toBe(false);
    expect(imageInfo(view.state)).toBeNull();
  });
});

describe("a picture whose file cannot be loaded", () => {
  it("shows the placeholder with the alt text in place of a broken picture, in the editor and after the doc is loaded again", () => {
    const doc = room(para("b_1", "x"), image("b_i", "/files/abc/gone.png", { alt: "Revenue" }));
    for (const view of [person(doc), person(reload(doc))]) {
      const img = view.dom.querySelector("img");
      expect(img?.getAttribute("src")).toBe("/files/abc/gone.png");
      img?.dispatchEvent(new Event("error"));
      expect(view.dom.querySelector("img")).toBeNull();
      expect(view.dom.querySelector(".ed-missing")?.textContent).toBe("Picture deleted: Revenue");
    }
  });

  it("says the picture is not available when it was never one of the doc's files", () => {
    const view = person(room(para("b_1", "x"), image("b_i", "https://example.com/far.png")));
    view.dom.querySelector("img")?.dispatchEvent(new Event("error"));
    expect(view.dom.querySelector(".ed-missing")?.textContent).toBe("Picture not available");
  });

  it("draws a placeholder and no address when the room holds one that is not on the list", () => {
    const view = person(room(para("b_1", "x"), image("b_i", "javascript:alert(1)"), image("b_j", "//evil.example/x.png"), image("b_k", "/api/artifacts")));
    expect(view.dom.querySelectorAll("img")).toHaveLength(0);
    expect(view.dom.querySelectorAll(".ed-missing")).toHaveLength(3);
    expect(view.dom.innerHTML).not.toMatch(/javascript:|evil\.example|\/api\//);
  });
});

describe("pasting an image element", () => {
  it("keeps one that comes from an upload, https or a small data picture, with its alt text", () => {
    const view = person(room(para("b_1", "")));
    view.pasteHTML('<p>a</p><img src="/files/abc/a.png" alt="Upload"><img src="https://example.com/b.png" alt="Web"><img src="data:image/png;base64,AAAA" alt="Inline">', new Event("paste") as ClipboardEvent);
    expect(blocks(view).filter((b) => b.type === "image").map((b) => [b.attrs as { src: string; alt: string }].map((a) => [a.src, a.alt])[0])).toEqual([
      ["/files/abc/a.png", "Upload"],
      ["https://example.com/b.png", "Web"],
      ["data:image/png;base64,AAAA", "Inline"],
    ]);
  });

  it("drops one whose address is not on the list and keeps no handler or style", () => {
    const view = person(room(para("b_1", "")));
    view.pasteHTML('<p>keep</p><img src="javascript:alert(1)" onerror="window.stolen=1"><img src="//evil.example/x.png"><img src="/api/artifacts" style="width:1px"><img src="data:image/svg+xml;base64,AAAA"><img src="file:///etc/passwd">', new Event("paste") as ClipboardEvent);
    expect(outline(view)).toBe("p(keep)");
    expect(JSON.stringify(view.state.doc.toJSON())).not.toMatch(/onerror|evil|stolen|style|passwd|svg/);
  });
});

describe("pictures pasted or dropped as files", () => {
  const png = () => new File(["x"], "image.png", { type: "image/png" });
  const clipboard = (files: File[], texts: Record<string, string> = {}) => Object.assign(new Event("paste", { cancelable: true }), { clipboardData: { files, getData: (type: string) => texts[type] ?? "" } }) as unknown as ClipboardEvent;

  const pasted = (event: ClipboardEvent) => {
    const onFiles = vi.fn<(files: File[]) => void>();
    const plugin = imageInput(onFiles);
    const view = person(room(para("b_1", "x")));
    const handled = plugin.props.handlePaste?.call(plugin, view, event, null as never) === true;
    return { onFiles, handled, event };
  };

  it("takes a screenshot (files and no text) and hands over its pictures", () => {
    const file = png();
    const { onFiles, handled, event } = pasted(clipboard([file, new File(["t"], "notes.txt", { type: "text/plain" })]));
    expect(handled).toBe(true);
    expect(event.defaultPrevented).toBe(true);
    expect(onFiles).toHaveBeenCalledExactlyOnceWith([file]);
  });

  it("leaves a paste that has text, such as a table copied from a spreadsheet with a picture of itself, to the paste rules", () => {
    for (const texts of [{ "text/html": "<table></table>" }, { "text/plain": "a\tb" }]) {
      const { onFiles, handled } = pasted(clipboard([png()], texts));
      expect(handled).toBe(false);
      expect(onFiles).not.toHaveBeenCalled();
    }
  });

  it("takes dropped pictures, puts the caret where they were dropped, and ignores a drop of anything else", () => {
    const onFiles = vi.fn<(files: File[]) => void>();
    const where = vi.fn();
    const plugin = imageInput(onFiles, where);
    const view = person(room(para("b_1", "x")));
    const drop = (files: File[]) => Object.assign(new Event("drop", { cancelable: true }), { dataTransfer: { files }, clientX: 4, clientY: 9 }) as unknown as DragEvent;
    const file = png();
    expect(plugin.props.handleDrop?.call(plugin, view, drop([file]), null as never, false)).toBe(true);
    expect(where).toHaveBeenCalledOnce();
    expect(onFiles).toHaveBeenCalledExactlyOnceWith([file]);
    expect(plugin.props.handleDrop?.call(plugin, view, drop([new File(["t"], "a.txt", { type: "text/plain" })]), null as never, false)).toBe(false);
    expect(onFiles).toHaveBeenCalledOnce();
  });

  it("lists only the pictures among files", () => {
    const files = [png(), new File(["t"], "a.pdf", { type: "application/pdf" })];
    expect(imageFilesOf(files as unknown as FileList).map((f) => f.name)).toEqual(["image.png"]);
    expect(imageFilesOf(undefined)).toEqual([]);
  });
});

describe("adding pictures", () => {
  const file = (name: string) => new File(["x"], name, { type: "image/png" });

  it("uploads each file and puts it in the doc, one after the other", async () => {
    const inserted: string[] = [];
    const refused = vi.fn<(reason: string | null) => void>();
    await addPictures([file("a.png"), file("b.png")], { upload: (f) => Promise.resolve(`/files/abc/${f.name}`), insert: (src) => inserted.push(src), refused });
    expect(inserted).toEqual(["/files/abc/a.png", "/files/abc/b.png"]);
    expect(refused.mock.calls).toEqual([[null]]);
  });

  it("tells the server's reason when it refuses one, and still adds the others", async () => {
    const inserted: string[] = [];
    const refused = vi.fn<(reason: string | null) => void>();
    const upload = (f: File) => (f.name === "big.png" ? Promise.reject(Object.assign(new Error("413"), { reason: "a file can be at most 15 MB" })) : Promise.resolve(`/files/abc/${f.name}`));
    await addPictures([file("big.png"), file("ok.png")], { upload, insert: (src) => inserted.push(src), refused });
    expect(refused).toHaveBeenLastCalledWith("a file can be at most 15 MB");
    expect(inserted).toEqual(["/files/abc/ok.png"]);
  });

  it("gives every upload a name of its own so a second screenshot does not replace the first, and keeps the type's ending", () => {
    expect(uploadName(file("image.png"), () => "1a2b3c4d")).toBe("image-1a2b3c4d.png");
    expect(uploadName(new File(["x"], "Q3 chart (final).JPG", { type: "image/jpeg" }), () => "x")).toBe("Q3-chart-final-x.jpg");
    expect(uploadName(new File(["x"], "", { type: "image/webp" }), () => "x")).toBe("picture-x.webp");
    expect(uploadName(file("image.png")).startsWith("image-")).toBe(true);
    expect(uploadName(file("image.png"))).not.toBe(uploadName(file("image.png")));
  });
});
