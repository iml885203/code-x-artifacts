import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { BOARD_MAP, bindBoard, type Element, type Scene } from "./binding";

const NEVER = "NEVER" as never;

/** A stand-in for the editor: the elements it shows, and the updates it was given. */
function fakeScene(): Scene & { elements: Element[]; captures: unknown[] } {
  const s = {
    elements: [] as Element[],
    captures: [] as unknown[],
    getSceneElementsIncludingDeleted: () => s.elements,
    updateScene: ({ elements, captureUpdate }: { elements: readonly Element[]; captureUpdate: never }) => {
      s.elements = [...elements];
      s.captures.push(captureUpdate);
    },
  };
  return s;
}

/**
 * Two browsers on one room: each update of one reaches the other, at once, or,
 * while they are apart, when they are joined again.
 */
function twoPeople(readOnlyB = false) {
  const a = new Y.Doc();
  const b = new Y.Doc();
  let apart = false;
  const held: [Y.Doc, Uint8Array][] = [];
  const send = (to: Y.Doc) => (u: Uint8Array, origin: unknown) => {
    if (origin === "remote") return;
    if (apart) held.push([to, u]);
    else Y.applyUpdate(to, u, "remote");
  };
  a.on("update", send(b));
  b.on("update", send(a));
  const sa = fakeScene();
  const sb = fakeScene();
  return {
    a, b, sa, sb,
    ba: bindBoard(a, sa, NEVER, false),
    bb: bindBoard(b, sb, NEVER, readOnlyB),
    part: () => {
      apart = true;
    },
    join: () => {
      apart = false;
      for (const [to, u] of held.splice(0)) Y.applyUpdate(to, u, "remote");
    },
  };
}

const box = (over: Partial<Record<string, unknown>> = {}): Element => ({
  id: "box", version: 1, versionNonce: 7, updated: 1, type: "rectangle", x: 0, y: 0, width: 200, height: 100, index: "a0", isDeleted: false,
  boundElements: [{ id: "label", type: "text" }], ...over,
});
const label = (over: Partial<Record<string, unknown>> = {}): Element => ({
  id: "label", version: 1, versionNonce: 8, updated: 1, type: "text", x: 70, y: 37.5, width: 60, height: 25, text: "Start",
  containerId: "box", textAlign: "center", verticalAlign: "middle", index: "a1", isDeleted: false, ...over,
});

/** A person drawing: their editor shows the elements, then reports the change. */
function draw(scene: { elements: Element[] }, binding: { onChange(els: readonly Element[]): void }, els: Element[]) {
  scene.elements = els;
  binding.onChange(els);
}

const byId = (els: readonly Element[]) => Object.fromEntries(els.map((e) => [e.id, e]));

describe("a board's scene and its room", () => {
  it("writes one entry per field and leaves each editor's bookkeeping out", () => {
    const { a, ba } = twoPeople();
    ba.onChange([box()]);
    const map = a.getMap<string>(BOARD_MAP);
    expect(map.get("box|x")).toBe("0");
    expect(map.get("box|boundElements")).toBe('[{"id":"label","type":"text"}]');
    for (const f of ["id", "version", "versionNonce", "updated"]) expect(map.has(`box|${f}`)).toBe(false);
  });

  it("shows what one person draws to the other, out of the other's Undo", () => {
    const { ba, sb } = twoPeople();
    ba.onChange([box(), label()]);
    expect(Object.keys(byId(sb.elements))).toEqual(["box", "label"]);
    expect(byId(sb.elements).label?.text).toBe("Start");
    expect(sb.captures.every((c) => c === NEVER)).toBe(true);
  });

  it("keeps both changes when one moves a box while the other retypes its label, and centres the label on the moved box", () => {
    const { ba, bb, sa, sb, part, join } = twoPeople();
    draw(sa, ba, [box(), label()]);
    const bobsBox = byId(sb.elements).box ?? box();
    part();
    draw(sa, ba, [box({ version: 2, x: 400, y: 300 }), label({ x: 470, y: 337.5 })]);
    // Bob's editor still has the box where it was, so it writes the label's position for the old box.
    draw(sb, bb, [bobsBox, label({ version: 5, text: "Begin", originalText: "Begin", width: 80, x: 60 })]);
    join();
    for (const scene of [sa, sb]) {
      const els = byId(scene.elements);
      expect(els.box?.x).toBe(400);
      expect(els.label?.text).toBe("Begin");
      expect([els.label?.x, els.label?.y]).toEqual([460, 337.5]);
    }
  });

  it("does not write back what the editor itself changes while taking a remote change in, such as deleting a label someone has only begun", () => {
    const a = new Y.Doc();
    const b = new Y.Doc();
    a.on("update", (u: Uint8Array, origin: unknown) => {
      if (origin !== "remote") Y.applyUpdate(b, u, "remote");
    });
    b.on("update", (u: Uint8Array, origin: unknown) => {
      if (origin !== "remote") Y.applyUpdate(a, u, "remote");
    });
    const sa = fakeScene();
    // Bob's editor, like Excalidraw's restore, drops an empty text to deleted when it is given one.
    const sb = fakeScene();
    sb.updateScene = ({ elements }) => {
      sb.elements = elements.map((el) => (el.type === "text" && el.text === "" ? { ...el, isDeleted: true, version: el.version + 1 } : el));
    };
    const ba = bindBoard(a, sa, NEVER, false);
    const bb = bindBoard(b, sb, NEVER, false);
    draw(sa, ba, [box(), label({ text: "", originalText: "" })]);
    // Bob's editor then reports its scene, as it does after any change.
    bb.onChange(sb.elements);
    expect(a.getMap<string>(BOARD_MAP).get("label|isDeleted")).toBe("false");
    draw(sa, ba, [box(), label({ version: 2, text: "你好", originalText: "你好" })]);
    expect(byId(sa.elements).label?.isDeleted).toBe(false);
  });

  it("writes only what changed since it last wrote or applied an element", () => {
    const { a, ba } = twoPeople();
    ba.onChange([box()]);
    let writes = 0;
    a.on("update", () => {
      writes++;
    });
    ba.onChange([box()]);
    expect(writes).toBe(0);
    ba.onChange([box({ version: 2, x: 5 })]);
    expect(writes).toBe(1);
    expect(a.getMap<string>(BOARD_MAP).get("box|x")).toBe("5");
  });

  it("never writes from a reader's editor, though it shows what others draw", () => {
    const { a, b, ba, bb, sb } = twoPeople(true);
    ba.onChange([box()]);
    const before = Y.encodeStateVector(b);
    bb.onChange([box({ version: 9, x: 999 })]);
    expect(Y.encodeStateVector(b)).toEqual(before);
    expect(a.getMap<string>(BOARD_MAP).get("box|x")).toBe("0");
    expect(byId(sb.elements).box?.x).toBe(0);
  });

  it("draws a room's existing board as soon as it is bound, in drawing order", () => {
    const doc = new Y.Doc();
    const map = doc.getMap<string>(BOARD_MAP);
    doc.transact(() => {
      for (const [k, v] of Object.entries({ "b|type": '"ellipse"', "b|index": '"a1"', "a|type": '"rectangle"', "a|index": '"a0"' })) map.set(k, v);
    });
    const scene = fakeScene();
    bindBoard(doc, scene, NEVER, false);
    expect(scene.elements.map((e) => e.id)).toEqual(["a", "b"]);
  });
});
