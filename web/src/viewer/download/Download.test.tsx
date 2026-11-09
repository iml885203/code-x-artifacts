import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Artifact } from "../../api";
import { artifact, bootFor, renderIn } from "../../test-support";
import { App } from "../../App";
import { MAX_BYTES } from "./rules";

const page = (kind: string) => artifact({ title: `Report ${kind}`, author: "alice", kind, canChange: true, canEdit: true, role: "author" });

/** Everything the browser was told to save, with the bytes of each. */
let saved: { name: string; type: string; bytes: Uint8Array }[];

function read(blob: Blob): Promise<Uint8Array> {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => {
      resolve(new Uint8Array(r.result as ArrayBuffer));
    };
    r.onerror = () => {
      reject(new Error("could not read the blob"));
    };
    r.readAsArrayBuffer(blob);
  });
}

beforeEach(() => {
  saved = [];
  const blobs = new Map<string, Blob>();
  vi.stubGlobal("URL", Object.assign(URL, {
    createObjectURL: (b: Blob) => {
      const url = `blob:test/${String(blobs.size)}`;
      blobs.set(url, b);
      return url;
    },
    revokeObjectURL: () => undefined,
  }));
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
    const blob = blobs.get(this.href);
    if (!blob) return;
    void read(blob).then((bytes) => saved.push({ name: this.download, type: blob.type, bytes }));
  });
  vi.stubGlobal("fetch", vi.fn((url: string) => Promise.resolve(new Response(JSON.stringify(url.startsWith("/api/artifacts/") && !url.includes("/", 15) ? opened : {}), { status: 200 }))));
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

let opened: Artifact;

/** The viewer open on a page, and the page's side of the conversation: a function that asks and returns what the page was told. */
function viewing(a: Artifact, fullPage = false, version?: number) {
  opened = a;
  const path = `/a/${a.id}${fullPage ? `?view=full${version === undefined ? "" : `&version=${String(version)}`}` : ""}`;
  renderIn(<App boot={bootFor("en", { page: "viewer", artifact: a, here: path })} />, "en", path);
  const target = screen.getByTitle<HTMLIFrameElement>(a.title).contentWindow;
  if (!target) throw new Error("the iframe has no window");
  const replies: { id: number; result?: unknown; error?: string }[] = [];
  vi.spyOn(target, "postMessage").mockImplementation((m: unknown) => {
    replies.push(m as { id: number; result?: unknown; error?: string });
  });
  let seq = 0;
  const call = (op: string, args: unknown) => {
    const id = ++seq;
    act(() => {
      window.dispatchEvent(new MessageEvent("message", { data: { pictor: "call", id, op, args }, source: target }));
    });
    return async () => {
      await waitFor(() => {
        expect(replies.some((r) => r.id === id)).toBe(true);
      });
      return replies.find((r) => r.id === id) as { result?: unknown; error?: string };
    };
  };
  return { ask: (args: unknown) => call("download", args), call, replies };
}

const csv = { name: "votes.csv", type: "text/csv", data: "name,votes\nA,3\n" };
const prompt = () => screen.findByRole("alertdialog");

describe.each([
  { kind: "html", fullPage: false }, { kind: "react", fullPage: false },
  { kind: "html", fullPage: true }, { kind: "react", fullPage: true },
])("a $kind page (full-page: $fullPage) that asks to save a file", ({ kind, fullPage }) => {
  it("is asked about with the cleaned name, the type and the size, and saves exactly its data once the person accepts", async () => {
    const user = userEvent.setup();
    const { ask } = viewing(page(kind), fullPage);
    const answer = ask({ ...csv, name: "../../votes" });
    const dialog = await prompt();
    expect(within(dialog).getByRole("heading")).toHaveTextContent("Save votes.csv?");
    expect(dialog).toHaveTextContent("text/csv");
    expect(dialog).toHaveTextContent("15 B");
    expect(saved).toEqual([]);

    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await answer()).toMatchObject({ id: 1, result: { name: "votes.csv", size: 15 } });
    await waitFor(() => {
      expect(saved).toHaveLength(1);
    });
    expect(saved[0]).toMatchObject({ name: "votes.csv", type: "text/csv" });
    expect(new TextDecoder().decode(saved[0]?.bytes)).toBe(csv.data);
  });

  it("is refused, and saves nothing, when the person cancels", async () => {
    const user = userEvent.setup();
    const { ask } = viewing(page(kind), fullPage);
    const answer = ask(csv);
    await user.click(within(await prompt()).getByRole("button", { name: "Cancel" }));
    expect(await answer()).toMatchObject({ error: "the viewer refused the download" });
    expect(saved).toEqual([]);
  });
});

describe("what a page may save", () => {
  it.each([false, true])("is exactly the bytes it sent, for binary data too (full-page: %s)", async (fullPage) => {
    const user = userEvent.setup();
    const { ask } = viewing(page("html"), fullPage);
    const bytes = new Uint8Array([0, 255, 1, 254, 0x50, 0x4b, 3, 4, 128]);
    const answer = ask({ name: "pack", type: "application/zip", data: bytes });
    await user.click(within(await prompt()).getByRole("button", { name: "Save" }));
    await answer();
    await waitFor(() => {
      expect(saved).toHaveLength(1);
    });
    expect(saved[0]).toMatchObject({ name: "pack.zip", type: "application/zip" });
    expect([...(saved[0]?.bytes ?? [])]).toEqual([...bytes]);
  });

  it.each([
    ["a type that is not on the list", { ...csv, type: "text/html" }, /text\/csv/],
    ["a file over the limit", { name: "big", type: "application/zip", data: new Uint8Array(MAX_BYTES + 1) }, /10 MB/],
    ["data that is not text or bytes", { ...csv, data: { a: 1 } }, /text, an ArrayBuffer or a typed array/],
  ])("is rejected without a prompt: %s", async (_label, args, error) => {
    const { ask } = viewing(page("html"));
    expect(await ask(args)()).toMatchObject({ error: expect.stringMatching(error) as string });
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(saved).toEqual([]);
  });

  it.each([false, true])("needs a prompt of its own each time: accepting one does not allow the next (full-page: %s)", async (fullPage) => {
    const user = userEvent.setup();
    const { ask } = viewing(page("html"), fullPage);
    const first = ask(csv);
    await user.click(within(await prompt()).getByRole("button", { name: "Save" }));
    await first();
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });

    const second = ask({ ...csv, name: "again.csv" });
    const dialog = await prompt();
    expect(dialog).toHaveTextContent("Save again.csv?");
    expect(saved.map((s) => s.name)).toEqual(["votes.csv"]);
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(await second()).toMatchObject({ error: "the viewer refused the download" });
    expect(saved.map((s) => s.name)).toEqual(["votes.csv"]);
  });

  it("is refused while another request still waits, so a loop cannot pile up prompts", async () => {
    const user = userEvent.setup();
    const { ask } = viewing(page("html"));
    const first = ask(csv);
    await prompt();
    const second = await ask({ ...csv, name: "b.csv" })();
    expect(second.error).toMatch(/waiting for the viewer/);
    expect(screen.getAllByRole("alertdialog")).toHaveLength(1);
    await user.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Cancel" }));
    await first();
  });

  it("does not let the frame start a download on its own: the sandbox has no allow-downloads", () => {
    viewing(page("html"));
    expect(screen.getByTitle("Report html").getAttribute("sandbox")).not.toContain("allow-downloads");
  });
});


describe("the full-page viewer", () => {
  it.each(["html", "doc", "design"])("fills the page with the sandboxed %s presentation and offers no editor or viewer toolbar", (kind) => {
    viewing(page(kind), true);
    expect(screen.queryByRole("banner")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Share" })).not.toBeInTheDocument();
    expect(screen.getByTitle(`Report ${kind}`)).toHaveAttribute("sandbox", "allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-modals");
  });

  it.each([undefined, 1])("uses the same bridge with historical writes refused (version: %s)", async (version) => {
    const user = userEvent.setup();
    const { call, ask } = viewing(page("html"), true, version);
    const answer = await call("db.set", { collection: "notes", id: "n1", data: { text: "hi" } })();
    if (version === undefined) expect(answer.error).toBeUndefined();
    else expect(answer.error).toBe("viewing an old version; restore it to make changes");
    expect(screen.getByTitle("Report html")).toHaveAttribute("src", `/raw/${opened.id}${version === undefined ? "" : "?version=1"}`);
    const download = ask(csv);
    await user.click(within(await prompt()).getByRole("button", { name: "Cancel" }));
    expect(await download()).toMatchObject({ error: "the viewer refused the download" });
  });
});
