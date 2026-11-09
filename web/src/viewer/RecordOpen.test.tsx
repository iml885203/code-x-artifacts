import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Artifact } from "../api";
import { App } from "../App";
import { artifact, bootFor, renderIn } from "../test-support";

const page = artifact({ title: "Cheat sheet", author: "bob", visibility: "organisation" });
const other = artifact({ title: "Sprint report", author: "alice", openedAt: "2026-10-19T09:00:00Z" });

/** The server: the list, each artifact by its id, and an open that it records and answers with. */
function serve(list: Artifact[], openedAt: string) {
  let held = list;
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    const opening = init?.method === "POST" && url.endsWith("/open");
    if (opening) held = held.map((a) => (url === `/api/artifacts/${a.id}/open` ? { ...a, openedAt } : a));
    const one = held.find((a) => url === `/api/artifacts/${a.id}` + (opening ? "/open" : ""));
    const answer = url === "/api/artifacts" ? held : (one ?? {});
    return Promise.resolve(new Response(JSON.stringify(answer)));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const opens = (fetchMock: ReturnType<typeof serve>) => (fetchMock.mock.calls as unknown as [string, RequestInit | undefined][]).filter(([url, init]) => init?.method === "POST" && url.endsWith("/open")).map(([url]) => url);

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("opening an artifact in the viewer", () => {
  it("records an open each time the person opens the artifact, once per opening", async () => {
    const user = userEvent.setup();
    const fetchMock = serve([page, other], "2026-10-20T12:00:00Z");
    renderIn(<App boot={bootFor("en", { page: "gallery" })} />, "en", "/recent");
    await screen.findAllByRole("listitem");
    await user.click(screen.getByRole("link", { name: /Sprint report/ }));
    fireEvent.load(await screen.findByTitle("Sprint report"));
    await user.click(screen.getByRole("link", { name: "Back to artifacts" }));
    await user.click(await screen.findByRole("link", { name: "Recently opened" }));
    await screen.findAllByRole("listitem");
    expect(opens(fetchMock)).toEqual([`/api/artifacts/${other.id}/open`]);

    fetchMock.mockClear();
    await user.click(screen.getByRole("link", { name: /Sprint report/ }));
    fireEvent.load(await screen.findByTitle("Sprint report"));
    await waitFor(() => {
      expect(opens(fetchMock)).toEqual([`/api/artifacts/${other.id}/open`]);
    });
  });

  it("puts what the server recorded into the library's list, so the open shows first under Recently opened", async () => {
    const user = userEvent.setup();
    serve([other, page], "2026-10-20T12:00:00Z");
    renderIn(<App boot={bootFor("en", { page: "viewer", artifact: page })} />, "en", `/a/${page.id}`);
    await screen.findByTitle("Cheat sheet");
    await user.click(screen.getByRole("link", { name: "Back to artifacts" }));
    await user.click(await screen.findByRole("link", { name: "Recently opened" }));
    await waitFor(() => {
      expect(screen.getAllByRole("listitem").map((li) => within(li).getByRole("heading").textContent)).toEqual(["Cheat sheet", "Sprint report"]);
    });
  });

  it("records the open of a signed-out reader of a public link once, for the usage counts", async () => {
    const fetchMock = serve([page], "2026-10-20T12:00:00Z");
    renderIn(<App boot={bootFor("en", { page: "viewer", user: "", artifact: { ...page, visibility: "public" } })} />, "en", `/a/${page.id}`);
    await screen.findByTitle("Cheat sheet");
    await waitFor(() => {
      expect(opens(fetchMock)).toEqual([`/api/artifacts/${page.id}/open`]);
    });
  });
});
