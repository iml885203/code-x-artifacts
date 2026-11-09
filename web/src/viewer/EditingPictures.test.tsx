import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Artifact } from "../api";
import * as Y from "yjs";
import { FakeRoom } from "../editor/fakeRoom";
import { para, room as seeded } from "../editor/testRoom";
import { artifact, bootFor, renderIn } from "../test-support";
import { Viewer } from "./Viewer";

// Adding a picture to a doc from the toolbar: the file goes to the doc's files, and what the server says
// when it refuses is what the person reads. Pasting and dropping reach the same code (editor/image.test.ts).
let room: FakeRoom;
class RoomSocket extends EventTarget {
  binaryType = "arraybuffer";
  private readonly end: ReturnType<FakeRoom["open"]>;
  constructor(url: string) {
    super();
    this.end = room.open(url);
    this.end.onopen = () => this.dispatchEvent(new Event("open"));
    this.end.onmessage = (e) => this.dispatchEvent(new MessageEvent("message", { data: e.data }));
    this.end.onclose = (e) => this.dispatchEvent(Object.assign(new Event("close"), { code: e.code, reason: e.reason }));
  }
  send(data: Uint8Array) {
    this.end.send(data);
  }
  close() {
    this.end.close();
  }
}

const doc = artifact({ title: "Plan", author: "alice", kind: "doc", visibility: "organisation", canEdit: true, role: "editor" });
const uploads: { path: string; size: number }[] = [];

function open(answer: (path: string) => Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST" && url.endsWith("/live")) return Promise.resolve(new Response(JSON.stringify({ ticket: "t1", epoch: "e1", path: `/ws/${doc.id}` })));
      if (init?.method === "PUT" && url.includes("/assets/")) {
        uploads.push({ path: url, size: init.body instanceof Blob ? init.body.size : 0 });
        return Promise.resolve(answer(url));
      }
      return Promise.resolve(new Response(JSON.stringify(doc satisfies Artifact)));
    }),
  );
  return renderIn(<Viewer boot={bootFor("en", { page: "viewer", user: "alice" })} artifact={doc} />, "en", `/a/${doc.id}`);
}

async function edit() {
  return screen.findByRole("textbox", { name: "Document" });
}

beforeEach(() => {
  room = new FakeRoom();
  Y.applyUpdate(room.doc, Y.encodeStateAsUpdate(seeded(para("b_1", "Plan"))));
  uploads.length = 0;
  vi.stubGlobal("WebSocket", RoomSocket);
});
afterEach(() => {
  vi.unstubAllGlobals();
});

describe("adding a picture with the Image button", () => {
  it("uploads the file under a name of its own and puts the picture in the doc, selected, with its tools", async () => {
    open((path) => new Response(JSON.stringify({ name: decodeURIComponent(path.split("/").pop() ?? ""), url: `https://pictor.test${new URL(path, "https://pictor.test").pathname.replace("/api/artifacts/", "/files/").replace("/assets/", "/")}` }), { status: 201 }));
    const document = await edit();
    await userEvent.upload(screen.getByTestId("image-picker"), new File(["png"], "screenshot.png", { type: "image/png" }));

    const picture = () => document.querySelector("img");
    await waitFor(() => {
      expect(picture()).not.toBeNull();
    });
    expect(uploads).toHaveLength(1);
    expect(uploads[0]?.path).toMatch(new RegExp(`^/api/artifacts/${doc.id}/assets/screenshot-[0-9a-f]{8}\\.png$`));
    expect(picture()?.getAttribute("src")).toMatch(new RegExp(`^/files/${doc.id}/screenshot-[0-9a-f]{8}\\.png$`));
    const tools = await screen.findByRole("toolbar", { name: "Image tools" });
    expect(within(tools).getByLabelText("Alt text")).toHaveValue("");
    await userEvent.click(within(tools).getByRole("button", { name: "Large" }));
    await waitFor(() => {
      expect(picture()).toHaveAttribute("width", "504");
    });
    await userEvent.type(within(tools).getByLabelText("Alt text"), "A screenshot{Enter}");
    await waitFor(() => {
      expect(picture()).toHaveAttribute("alt", "A screenshot");
    });
    // Undo takes back what was done in one burst (half a second of the undo plugin's capture time): the deletion is its own.
    await new Promise((resolve) => setTimeout(resolve, 600));
    await userEvent.click(within(tools).getByRole("button", { name: "Delete image" }));
    await waitFor(() => {
      expect(picture()).toBeNull();
    });
    await userEvent.click(screen.getByRole("button", { name: "More" }));
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    await waitFor(() => {
      expect(picture()).not.toBeNull();
    });
  });

  it("shows the server's reason when it refuses the file, and nothing is put in the doc", async () => {
    open(() => new Response(JSON.stringify({ error: "a file can be at most 15 MB; send the file itself as the request body" }), { status: 413 }));
    const document = await edit();
    await userEvent.upload(screen.getByTestId("image-picker"), new File(["png"], "huge.png", { type: "image/png" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The picture was not added: a file can be at most 15 MB; send the file itself as the request body");
    expect(document.querySelector("img")).toBeNull();
  });

  it("tells that the picture was not added when the server gives no reason", async () => {
    open(() => new Response("", { status: 500 }));
    await edit();
    await userEvent.upload(screen.getByTestId("image-picker"), new File(["png"], "a.png", { type: "image/png" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The picture was not added.");
  });
});
