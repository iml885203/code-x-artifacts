import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { beforeAll, beforeEach, describe, expect, it } from "vitest";

// The script the server puts in every frame, run in a document of its own.
const source = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "../../../internal/kind/design_inspector.js"), "utf8");

// The frame's world as the script sees it: a parent that records what it is told, and the listeners the script registers.
const sent: unknown[] = [];
const parent = { postMessage: (message: unknown) => sent.push(message) };
const stranger = { postMessage: () => undefined };
const listeners: Record<string, ((e: unknown) => void)[]> = {};
const emit = (type: string, event: object) => {
  for (const listener of listeners[type] ?? []) listener(event);
};
const fromParent = (data: unknown) => {
  emit("message", { data, source: parent });
};
const from = (win: unknown, data: unknown) => {
  emit("message", { data, source: win });
};

beforeAll(() => {
  // The inspector is the script the server serves; running its text is the test.
  // eslint-disable-next-line @typescript-eslint/no-implied-eval
  const run = new Function("parent", "addEventListener", "scrollX", "scrollY", source) as (...args: unknown[]) => void;
  run(parent, (type: string, listener: (e: unknown) => void) => (listeners[type] ??= []).push(listener), 0, 0);
});

beforeEach(() => {
  sent.length = 0;
  const page = new DOMParser().parseFromString(`<h1 data-pid="e2" style="font-size:40px;color:red" class="t">Hello</h1><p data-pid="e3"><span data-pid="e4">x</span></p>`, "text/html");
  document.body.replaceChildren(...page.body.childNodes);
  document.body.setAttribute("data-pid", "e1");
  document.elementFromPoint = (): Element | null => document.querySelector("span");
});

describe("the inspector in a frame", () => {
  it("says it is ready once the frame is parsed", () => {
    emit("DOMContentLoaded", {});
    expect(sent).toEqual([{ pictor: "inspect", type: "ready", pids: ["e1", "e2", "e3", "e4"] }]);
  });

  it("answers what is under a point with the nearest element that has an id", () => {
    fromParent({ pictor: "inspect", id: 7, type: "hit", x: 5, y: 6 });
    expect(sent).toEqual([{ pictor: "inspect", id: 7, type: "hit", pid: "e4" }]);
    document.elementFromPoint = () => null;
    sent.length = 0;
    fromParent({ pictor: "inspect", id: 8, type: "hit", x: 5, y: 6 });
    expect(sent).toEqual([{ pictor: "inspect", id: 8, type: "hit", pid: null }]);
  });

  it("describes an element: its place, attributes, inline style and computed values", () => {
    fromParent({ pictor: "inspect", id: 9, type: "report", pids: ["e2", "missing"] });
    const [reply] = sent as [{ id: number; items: { pid: string; tag: string; attrs: [string, string][]; inline: [string, string][]; computed: Record<string, string>; rect: object }[] }];
    expect(reply.id).toBe(9);
    expect(reply.items).toHaveLength(1);
    const h1 = reply.items[0];
    expect(h1).toMatchObject({ pid: "e2", tag: "h1" });
    expect(h1?.attrs).toContainEqual(["class", "t"]);
    expect(h1?.inline).toContainEqual(["font-size", "40px"]);
    expect(h1?.computed["font-size"]).toBe("40px");
    expect(h1?.rect).toHaveProperty("w");
  });

  it("reports asymmetric borders and stack wrap from the rendered element", () => {
    document.querySelector("h1")?.setAttribute("style", "display:flex;flex-wrap:wrap;border-top:2px solid red;border-right:4px dashed blue;border-bottom:6px dotted green;border-left:8px double black");
    fromParent({ pictor: "inspect", id: 10, type: "report", pids: ["e2"] });
    const [reply] = sent as [{items: {computed: Record<string,string>}[]}];
    expect(reply.items[0]?.computed).toMatchObject({"flex-wrap":"wrap", "border-top-width":"2px", "border-right-width":"4px", "border-bottom-style":"dotted", "border-left-style":"double"});
  });

  it("keeps seeded longhands in their authored position before a covering shorthand", () => {
    const element = document.querySelector("h1");
    if (!element) throw new Error("Missing inspector fixture heading");
    const source = "border-top-color:blue!important;border:2px solid red!important";
    element.setAttribute("style", source);
    fromParent({ pictor:"inspect", id:14, type:"apply", styles:[["e2","border-top-color","blue!important"]] });
    expect(element.getAttribute("style")).toBe(source);
  });

  it("replays authored shorthands when resetting an edited longhand and restores source on Undo", () => {
    const element = document.querySelector("h1");
    if (!element) throw new Error("Missing inspector fixture heading");
    element.setAttribute("style", "background-color:blue!important;background:yellow!important;border-top-color:blue!important;border:2px solid red!important");
    const source = element.getAttribute("style");
    fromParent({ pictor:"inspect", id:11, type:"apply", styles:[["e2","background-color","transparent !important"],["e2","background-image","none !important"],["e2","border-top-color","green !important"]] });
    expect(element.getAttribute("style")).toContain("background-color:transparent !important;");
    expect(element.getAttribute("style")).toMatch(/border:2px solid red!important;.*border-top-color:green !important;/);
    fromParent({ pictor:"inspect", id:12, type:"apply", styles:[["e2","background-color",""],["e2","background-image",""]] });
    expect(element.getAttribute("style")).toContain("background:yellow!important;");
    expect(element.getAttribute("style")).not.toContain("background-color:");
    expect(element.getAttribute("style")).not.toContain("background-image:");
    fromParent({ pictor:"inspect", id:13, type:"apply", styles:[["e2","background-color",null],["e2","background-image",null],["e2","border-top-color",null]] });
    expect(element.getAttribute("style")).toBe(source);
  });

  it("replays the room cascade after a snapshot already contains the winning override", () => {
    const element = document.querySelector("h1");
    if (!element) throw new Error("Missing inspector fixture heading");
    element.setAttribute("style", "background-color:blue!important;background:red!important;background-color:green!important");
    fromParent({ pictor:"inspect", id:16, type:"apply", styles:[], inline:[["e2","background-color:blue!important;background:red!important"]] });
    expect(element.getAttribute("style")).toBe("background-color:blue!important;background:red!important");
    fromParent({ pictor:"inspect", id:17, type:"apply", styles:[], inline:[["e2","background-color:blue!important;background:red!important;background-color:green!important"]] });
    expect(element.getAttribute("style")).toBe("background-color:blue!important;background:red!important;background-color:green!important");
  });

  it("changes nothing in the frame when it is only asked", () => {
    const before = document.body.innerHTML;
    fromParent({ pictor: "inspect", id: 1, type: "report", pids: ["e2"] });
    fromParent({ pictor: "inspect", id: 2, type: "hit", x: 1, y: 1 });
    expect(document.body.innerHTML).toBe(before);
  });

  it("ignores a message that does not come from its parent", () => {
    from(stranger, { pictor: "inspect", id: 3, type: "hit", x: 1, y: 1 });
    from(null, { pictor: "inspect", id: 3, type: "report", pids: ["e2"] });
    expect(sent).toEqual([]);
  });

  it("ignores what is not a question it knows", () => {
    for (const data of ["hello", null, { pictor: "other", id: 1, type: "hit", x: 1, y: 1 }, { pictor: "inspect", type: "hit", x: 1, y: 1 }, { pictor: "inspect", id: 1, type: "hit", x: "1", y: 1 }, { pictor: "inspect", id: 1, type: "report", pids: "e2" }, { pictor: "inspect", id: 1, type: "delete", pids: ["e2"] }]) {
      fromParent(data);
    }
    expect(sent).toEqual([]);
  });

  describe("showing the room", () => {
    const apply = (id: number, body: object) => {
      fromParent({ pictor: "inspect", id, type: "apply", ...body });
    };
    const add = (pid: string, style: string) => {
      const el = document.createElement("div");
      el.setAttribute("data-pid", pid);
      el.setAttribute("style", style);
      document.body.append(el);
      return el;
    };
    const h1 = () => document.querySelector<HTMLElement>("[data-pid=e2]");

    it("sets the owned style properties of an element and removes one given an empty value, and says it is done", () => {
      apply(1, { styles: [["e2", "left", "10px"], ["e2", "top", "20px"], ["e2", "width", "30px"], ["e2", "height", "40px"]] });
      expect(sent).toEqual([{ pictor: "inspect", id: 1, type: "apply", ok: true }]);
      expect(h1()?.style.left).toBe("10px");
      expect(h1()?.style.height).toBe("40px");
      apply(2, { styles: [["e2", "left", ""]] });
      expect(h1()?.style.left).toBe("");
      expect(h1()?.style.top).toBe("20px");
    });

    it("keeps an !important and puts back what the frame had when told null, whether it had a value or none", () => {
      const box = add("e9", "width: 220px !important");
      apply(1, { styles: [["e9", "width", " 300px !important"], ["e9", "left", "5px"]] });
      expect(box.style.getPropertyValue("width")).toBe("300px");
      expect(box.style.getPropertyPriority("width")).toBe("important");
      apply(2, { styles: [["e9", "width", null], ["e9", "left", null]] });
      expect(box.style.getPropertyValue("width")).toBe("220px");
      expect(box.style.getPropertyPriority("width")).toBe("important");
      expect(box.style.left).toBe("");
    });

    it("hides the elements the room no longer has, shows them again when it has them, and gives back a display the frame set", () => {
      const flex = add("e8", "display:flex");
      apply(1, { styles: [], alive: ["e1", "e2"] });
      expect(flex.style.display).toBe("none");
      expect(flex.style.getPropertyPriority("display")).toBe("important");
      expect(document.querySelector<HTMLElement>("[data-pid=e3]")?.style.display).toBe("none");
      expect(h1()?.style.display).toBe("");
      apply(2, { styles: [], alive: ["e1", "e2", "e3", "e4", "e8"] });
      expect(flex.style.display).toBe("flex");
      expect(document.querySelector<HTMLElement>("[data-pid=e3]")?.style.display).toBe("");
      expect(document.querySelector<HTMLElement>("[data-pid=e3]")?.getAttribute("style")).toBe("");
    });

    it("does not describe an element that is hidden", () => {
      apply(1, { styles: [], alive: ["e1", "e3", "e4"] });
      sent.length = 0;
      fromParent({ pictor: "inspect", id: 2, type: "report", pids: ["e2", "e3"] });
      const [reply] = sent as [{ items: { pid: string }[] }];
      expect(reply.items.map((i) => i.pid)).toEqual(["e3"]);
    });

    it("touches no other property and no other attribute, whatever it is told", () => {
      const before = document.body.innerHTML;
      for (const [i, styles] of [[["e2", "transform", "rotate(9deg)"]], [["e2", "z-index", "9"]], [["e2", "background", "url(http://x)"]], [["e2", "left"]], [["e2", "left", 4]], [[1, "left", "1px"]], "left"].entries()) {
        apply(i + 1, { styles });
      }
      apply(9, { styles: [], alive: "e2" });
      apply(10, { alive: ["e1"] });
      expect(document.body.innerHTML).toBe(before);
      expect(sent).toEqual([]);
    });

    it("hides an element with display:none when told and gives its own display back when told null", () => {
      const flex = add("e8", "display:flex");
      apply(1, { styles: [["e8", "display", "none"]] });
      expect(flex.style.display).toBe("none");
      apply(2, { styles: [["e8", "display", null]] });
      expect(flex.style.display).toBe("flex");
    });

    describe("the order of the elements", () => {
      const list = () => {
        const page = new DOMParser().parseFromString(`<div data-pid="p">one <i data-pid="a"></i>two<i data-pid="b"></i><i data-pid="c"></i> three</div><section data-pid="s"></section>`, "text/html");
        document.body.replaceChildren(...page.body.childNodes);
      };
      const kids = (pid: string) => Array.from(document.querySelector(`[data-pid=${pid}]`)?.children ?? []).map((el) => el.getAttribute("data-pid"));

      it("puts the elements of a parent in the order given, together where the last of them was, and leaves the text", () => {
        list();
        apply(1, { styles: [], order: [["p", ["c", "a", "b"]]] });
        expect(sent).toEqual([{ pictor: "inspect", id: 1, type: "apply", ok: true }]);
        expect(kids("p")).toEqual(["c", "a", "b"]);
        expect(document.querySelector("[data-pid=p]")?.textContent).toBe("one two three");
      });

      it("moves an element into another parent, and does not touch a parent that is already in order", () => {
        list();
        const moved = document.querySelector("[data-pid=a]");
        apply(1, { styles: [], order: [["s", ["a"]]] });
        expect(kids("s")).toEqual(["a"]);
        expect(kids("p")).toEqual(["b", "c"]);
        apply(2, { styles: [], order: [["p", ["b", "c"]]] });
        expect(document.querySelector("[data-pid=a]")).toBe(moved);
      });

      it("does not put an element inside itself, whatever it is told", () => {
        list();
        const before = document.body.innerHTML;
        apply(1, { styles: [], order: [["a", ["p"]]] });
        expect(document.body.innerHTML).toBe(before);
      });

      it("ignores an order that is not a list of parents with their children", () => {
        list();
        const before = document.body.innerHTML;
        for (const [i, order] of ["p", [["p"]], [["p", "a"]], [["p", [1]]], [[1, ["a"]]]].entries()) apply(i + 1, { styles: [], order });
        expect(document.body.innerHTML).toBe(before);
        expect(sent).toEqual([]);
      });
    });

    describe("the elements the room has and the file does not", () => {
      const placed = (parent: string, before: string | null | undefined, html: string, ns?: string) => ({ parent, before, html, ...(ns ? { ns } : {}) });

      it("builds the markup under the element it was told, before the one it was told, with everything inside it", () => {
        apply(1, { styles: [], create: [placed("e1", "e3", '<div class="card" style="color:red" data-pid="n1">Hi <img src="/files/a/p.png" alt="P" data-pid="n2"><b>bold</b></div>')] });
        expect(sent).toEqual([{ pictor: "inspect", id: 1, type: "apply", ok: true }]);
        const made = document.querySelector("[data-pid=n1]");
        expect(made?.outerHTML).toBe('<div class="card" style="color:red" data-pid="n1">Hi <img src="/files/a/p.png" alt="P" data-pid="n2"><b>bold</b></div>');
        expect(made?.nextElementSibling?.getAttribute("data-pid")).toBe("e3");
      });

      it("builds svg markup in the svg namespace and does not build an element it already has", () => {
        apply(1, { styles: [], create: [placed("e3", null, '<circle data-pid="n1"></circle>', "svg")] });
        expect(document.querySelector("[data-pid=n1]")?.namespaceURI).toBe("http://www.w3.org/2000/svg");
        const before = document.body.innerHTML;
        apply(2, { styles: [], create: [placed("e3", null, '<div data-pid="n1"></div>'), placed("e3", null, '<div data-pid="e4"></div>')] });
        expect(document.body.innerHTML).toBe(before);
      });

      it("styles and shows an element it has just built, like any other", () => {
        apply(1, { styles: [["n1", "left", "9px"]], alive: ["e1", "e2", "e3", "e4", "n1"], create: [placed("e1", null, '<div data-pid="n1"></div>')] });
        const made = document.querySelector<HTMLElement>("[data-pid=n1]");
        expect(made?.style.left).toBe("9px");
        expect(made?.style.display).toBe("");
      });

      it("runs no script from the markup, and builds nothing from a message that is not well formed", () => {
        apply(1, { styles: [], create: [placed("e1", null, '<div data-pid="n1"><script>window.ran = 1</script></div>')] });
        expect((window as { ran?: number }).ran).toBeUndefined();
        const before = document.body.innerHTML;
        sent.length = 0;
        const bad = [[{ parent: "e1", before: null }], "n2", [placed("e1", undefined, '<div data-pid="n2"></div>')], [{ parent: 1, before: null, html: "x" }], [null]];
        for (const [i, create] of bad.entries()) apply(i + 2, { styles: [], create });
        expect(document.body.innerHTML).toBe(before);
        expect(sent).toEqual([]);
      });

      it("changes the alt and src of an image and takes one away, and no other attribute", () => {
        const img = document.createElement("img");
        img.setAttribute("data-pid", "i1");
        img.setAttribute("alt", "old");
        document.body.append(img);
        apply(1, { styles: [], attrs: [["i1", "alt", "A cat"], ["i1", "src", "/files/a/c.png"]] });
        expect(img.getAttribute("alt")).toBe("A cat");
        expect(img.getAttribute("src")).toBe("/files/a/c.png");
        apply(2, { styles: [], attrs: [["i1", "alt", null]] });
        expect(img.hasAttribute("alt")).toBe(false);
        const before = document.body.innerHTML;
        apply(3, { styles: [], attrs: [["i1", "onload", "x"], ["i1", "data-pid", "z"], ["i1", "style", "x"]] });
        expect(document.body.innerHTML).toBe(before);
      });
    });

    it("ignores a style for an element it does not have", () => {
      apply(1, { styles: [["nope", "left", "1px"], ["e2", "left", "2px"]] });
      expect(h1()?.style.left).toBe("2px");
      expect(sent).toHaveLength(1);
    });
  });
});
