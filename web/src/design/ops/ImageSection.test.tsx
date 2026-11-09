import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { ApiError, type Asset } from "../../api";
import { renderIn } from "../../test-support";
import type { Target } from "../controller";
import { DesignEdits } from "../edits";
import { FILE, key, seed } from "../test-room";
import { ImageSection } from "./ImageSection";
import type { Seams } from "./picture";

const asset = (name: string): Asset => ({ name, contentType: "image/png", size: 1, url: `/files/abc/${name}`, createdBy: "alice", createdAt: "" });

function setup(report: Partial<Target["report"]> = {}, seams: Seams = {}) {
  const doc = new Y.Doc();
  seed(doc);
  const edits = new DesignEdits(doc);
  const target: Target = { frameId: "hero", file: FILE, pid: "box", report: { pid: "box", tag: "div", rect: { x: 0, y: 0, w: 1, h: 1 }, attrs: [], inline: [], computed: {}, ...report } };
  const upload = seams.upload ?? ((_id: string, file: File) => Promise.resolve(asset(file.name)));
  renderIn(<ImageSection artifactId="art" edits={edits} targets={[target]} seams={{ size: () => Promise.resolve({ width: 40, height: 20 }), ...seams, upload }} />, "en", "/a/art");
  const field = (name: string) => edits.map.get(key("box", name));
  const choose = (file: File) => userEvent.upload(screen.getByLabelText("Fill with image…", { selector: "input" }), file);
  return { edits, field, choose };
}

const png = (name = "photo.png") => new File([new Uint8Array([1])], name, { type: "image/png" });

describe("the image of an element", () => {
  it("fills an element with an uploaded picture as one step: the address of the artifact's file, covering, centred, not repeated", async () => {
    const { edits, field, choose } = setup({}, { upload: (_id, file) => Promise.resolve(asset(file.name)) });
    await choose(png());
    await waitFor(() => {
      expect(String(field("s|background-image"))).toMatch(/^url\(\/files\/abc\/photo-[0-9a-f]{8}\.png\)$/);
    });
    expect([field("s|background-size"), field("s|background-position"), field("s|background-repeat")]).toEqual(["cover", "center", "no-repeat"]);
    expect(edits.steps().undo).toBe(1);
    edits.undo();
    expect(field("s|background-image")).toBeUndefined();
  });

  it("shows the server's reason when it refuses the file and writes nothing", async () => {
    const { edits, field, choose } = setup({}, { upload: () => Promise.reject(new ApiError("PUT: 413", 413, "a file is at most 15 MB")) });
    await choose(png("big.png"));
    expect(await screen.findByRole("alert")).toHaveTextContent("big.png wasn't added: a file is at most 15 MB");
    expect(field("s|background-image")).toBeUndefined();
    expect(edits.steps().undo).toBe(0);
  });

  it("says so when the browser cannot read the picture", async () => {
    const { choose } = setup({}, { size: () => Promise.reject(new Error("no")) });
    await choose(png("odd.png"));
    expect(await screen.findByRole("alert")).toHaveTextContent("odd.png isn't a picture this browser can read.");
  });

  it("changes how a fill fits and takes it away", async () => {
    const { edits, field } = setup({ inline: [["background-image", "url(/files/abc/a.png)"], ["background-size", "cover"]] });
    const user = userEvent.setup();
    edits.apply([{ file: FILE, pid: "box", style: { "background-image": "url(/files/abc/a.png)", "background-size": "cover" } }]);
    await user.click(screen.getByRole("button", { name: "Contain" }));
    expect(field("s|background-size")).toBe("contain");
    await user.click(screen.getByRole("button", { name: "Original" }));
    expect(field("s|background-size")).toBe("auto");
    await user.click(screen.getByRole("button", { name: "Remove image fill" }));
    expect([field("s|background-image"), field("s|background-size")]).toEqual(["", ""]);
  });

  it("gives an img its fit as object-fit and its alt as an attribute that only it changes", async () => {
    const { edits, field } = setup({ tag: "img", attrs: [["src", "/files/abc/a.png"], ["alt", "old"]], inline: [["object-fit", "cover"]] });
    edits.map.set(key("box", "attrs"), JSON.stringify([["", "src", "/files/abc/a.png"], ["", "alt", "old"]]));
    const user = userEvent.setup();
    expect(screen.queryByRole("button", { name: "Remove image fill" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Contain" }));
    expect(field("s|object-fit")).toBe("contain");
    const alt = screen.getByRole("textbox", { name: "Alt text" });
    expect(alt).toHaveValue("old");
    await user.clear(alt);
    await user.type(alt, "A cat{Enter}");
    expect(JSON.parse(String(field("attrs")))).toEqual([["", "src", "/files/abc/a.png"], ["", "alt", "A cat"]]);
    await user.clear(alt);
    await user.tab();
    expect(JSON.parse(String(field("attrs")))).toEqual([["", "src", "/files/abc/a.png"], ["", "alt", ""]]);
  });

  it("replaces the source of an img and gives it an alt from the file only when it has none", async () => {
    const { edits, field, choose } = setup({ tag: "img", attrs: [["src", "/files/abc/a.png"]] });
    edits.map.set(key("box", "attrs"), JSON.stringify([["", "src", "/files/abc/a.png"]]));
    await choose(png("my-cat.png"));
    await waitFor(() => {
      expect(String(field("attrs"))).toContain("my-cat-");
    });
    const rows = JSON.parse(String(field("attrs"))) as string[][];
    expect(rows.find((r) => r[1] === "alt")?.[2]).toBe("my cat");
  });
});
