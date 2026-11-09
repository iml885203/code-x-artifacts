// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { DesignEdits, ROOM_MAP } from "../edits";
import type { AttrChange, Build, FrameLinks } from "../inspect";
import { FILE, key, seed } from "../test-room";
import { buildsFor, foreignDeclarations, mountBuilt, pictureChanges, pictures } from "./built";
import { DesignOps } from "./DesignOps";

// The frame has what seed() wrote and nothing the editor made since: body b, box, para and h1.
const FILE_HAS = new Set(["b", "box", "para", "h1"]);

function setup() {
  const doc = new Y.Doc();
  seed(doc);
  const edits = new DesignEdits(doc);
  return { doc, edits, map: doc.getMap(ROOM_MAP) };
}

describe("what a frame builds from the room", () => {
  it("is nothing while the frame has every element of the room", () => {
    const { edits } = setup();
    expect(buildsFor(edits, FILE, FILE_HAS)).toEqual([]);
  });

  it("is each new element under the element the frame has, before the next element it has, with its tag, attributes and text", () => {
    const { edits, map } = setup();
    const ops = new DesignOps(edits);
    const made = ops.insertShape(FILE, "text", { left: 12, top: 34 }, "Text");
    const builds = buildsFor(edits, FILE, FILE_HAS);
    expect(builds).toHaveLength(1);
    const [build] = builds as [Build];
    expect(build.parent).toBe("b");
    expect(build.before).toBeNull();
    expect(build.html).toBe(`<div data-pid="${made?.pid ?? ""}">Text</div>`);
    // A copy goes where the original is: before the element that followed it.
    map.set(key("n1", "kind"), "el");
    map.set(key("n1", "tag"), "div");
    map.set(key("n1", "parent"), "b");
    map.set(key("n1", "order"), "Z");
    const next = buildsFor(edits, FILE, FILE_HAS).find((b) => b.html.includes('data-pid="n1"'));
    expect(next?.before).toBe("h1");
  });

  it("builds the elements inside a new one as part of it, not on their own", () => {
    const { edits } = setup();
    const ops = new DesignOps(edits);
    ops.duplicate([{ frameId: "hero", file: FILE, pid: "box", report: { pid: "box", tag: "div", rect: { x: 0, y: 0, w: 1, h: 1 }, attrs: [], inline: [], computed: {} } }]);
    const builds = buildsFor(edits, FILE, FILE_HAS);
    expect(builds).toHaveLength(1);
    expect(builds[0]?.html).toMatch(/^<div data-pid="n\w+"><div data-pid="n\w+">Hello<\/div><\/div>$/);
  });

  it("builds the markup of a formatted text and the attributes of an image", () => {
    const { edits, map } = setup();
    new DesignOps(edits).insertImage(FILE, { src: "/files/a/p.png", alt: "P", width: 10, height: 10 }, { left: 0, top: 0 });
    const rich = new Y.Text();
    rich.applyDelta([{ insert: "go " }, { insert: "now", attributes: { "f:e9": '{"t":"b","d":0,"a":[["","data-pid","e9"]]}' } }]);
    map.set(key("n2", "kind"), "el");
    map.set(key("n2", "tag"), "p");
    map.set(key("n2", "parent"), "b");
    map.set(key("n2", "order"), "zz");
    map.set(key("n2", "rich"), rich);
    map.set(key("n2", "rest"), "color:red;font-size:font-size;margin:0");
    const builds = buildsFor(edits, FILE, FILE_HAS);
    expect(builds.find((b) => b.html.startsWith("<img"))?.html).toMatch(/^<img src="\/files\/a\/p.png" alt="P" data-pid="n\w+">$/);
    expect(builds.find((b) => b.html.includes('data-pid="n2"'))?.html).toBe('<p style="color:red;margin:0" data-pid="n2">go <b data-pid="e9">now</b></p>');
  });

  it("leaves out an element whose parent is nowhere", () => {
    const { edits, map } = setup();
    map.set(key("n3", "kind"), "el");
    map.set(key("n3", "tag"), "div");
    map.set(key("n3", "parent"), "b");
    map.set(key("n3", "order"), "z");
    // The body is not in the frame: nothing can be put in it.
    expect(buildsFor(edits, FILE, new Set(["box", "para", "h1"]))).toEqual([]);
  });
});

describe("the declarations a built element has of its own", () => {
  it.each([
    ["color:red;left:left;margin:0", "color:red;margin:0"],
    ["left", undefined],
    ["", undefined],
    ["background:url(a.png?x;y);width:width", "background:url(a.png?x;y)"],
    ['content:";";color:blue', 'content:";";color:blue'],
  ])("%j gives %j", (skeleton, want) => {
    expect(foreignDeclarations(skeleton)).toBe(want);
  });
});

describe("the alt and src of images", () => {
  it("reports what changed and nothing the first time an image is seen", () => {
    const { edits, map } = setup();
    new DesignOps(edits).insertImage(FILE, { src: "/files/a/p.png", alt: "P", width: 10, height: 10 }, { left: 0, top: 0 });
    const first = pictures(edits, FILE);
    expect(pictureChanges(first, undefined)).toEqual([]);
    const pid = [...first.keys()][0] ?? "";
    map.set(key(pid, "attrs"), JSON.stringify([["", "src", "/files/a/q.png"], ["", "alt", ""]]));
    expect(pictureChanges(pictures(edits, FILE), first)).toEqual([
      [pid, "alt", ""],
      [pid, "src", "/files/a/q.png"],
    ] satisfies AttrChange[]);
    map.set(key(pid, "attrs"), JSON.stringify([["", "src", "/files/a/q.png"]]));
    expect(pictureChanges(pictures(edits, FILE), pictures(edits, FILE))).toEqual([]);
  });
});

describe("keeping a frame in step with the room", () => {
  function links(has: Set<string> | null) {
    const ready: ((id: string) => void)[] = [];
    const mounts: { builds: Build[]; attrs: AttrChange[] }[] = [];
    const fake = {
      has: () => has,
      mount: vi.fn((_id: string, builds: Build[], attrs: AttrChange[] = []) => {
        mounts.push({ builds, attrs });
        for (const b of builds) for (const m of b.html.matchAll(/data-pid="([^"]*)"/g)) has?.add(m[1] ?? "");
        return Promise.resolve();
      }),
      onReady: (fn: (id: string) => void) => {
        ready.push(fn);
        return () => undefined;
      },
    };
    return { fake: fake as unknown as FrameLinks, mounts, ready };
  }

  it("builds a new element when the room changes, once, and everything missing when the frame says it is ready", () => {
    const { edits } = setup();
    const have = new Set(FILE_HAS);
    const { fake, mounts, ready } = links(have);
    const stop = mountBuilt(edits, fake, () => [{ id: "hero", file: FILE }]);
    expect(mounts).toEqual([]);
    new DesignOps(edits).insertShape(FILE, "rect", { left: 1, top: 2 }, "Text");
    expect(mounts).toHaveLength(1);
    expect(mounts[0]?.builds).toHaveLength(1);
    // Another change finds the element built and builds nothing more.
    edits.apply([{ file: FILE, pid: "box", style: { left: "5px" } }]);
    expect(mounts).toHaveLength(1);
    // A frame that is loaded again has only what its file has.
    have.clear();
    for (const p of FILE_HAS) have.add(p);
    ready[0]?.("hero");
    expect(mounts).toHaveLength(2);
    expect(mounts[1]?.builds).toHaveLength(1);
    stop();
  });

  it("waits for a frame that has not said what it has", () => {
    const { edits } = setup();
    const { fake, mounts } = links(null);
    mountBuilt(edits, fake, () => [{ id: "hero", file: FILE }]);
    new DesignOps(edits).insertShape(FILE, "rect", { left: 1, top: 2 }, "Text");
    expect(mounts).toEqual([]);
  });

  it("shows a new alt of an image that is there", () => {
    const { edits, map } = setup();
    const ops = new DesignOps(edits);
    const img = ops.insertImage(FILE, { src: "/files/a/p.png", alt: "P", width: 10, height: 10 }, { left: 0, top: 0 });
    const have = new Set([...FILE_HAS, img?.pid ?? ""]);
    const { fake, mounts } = links(have);
    mountBuilt(edits, fake, () => [{ id: "hero", file: FILE }]);
    map.set(key(img?.pid ?? "", "attrs"), JSON.stringify([["", "src", "/files/a/p.png"], ["", "alt", "A cat"]]));
    expect(mounts).toEqual([{ builds: [], attrs: [[img?.pid, "alt", "A cat"]] }]);
  });
});
