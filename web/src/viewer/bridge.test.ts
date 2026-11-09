import { afterEach, describe, expect, it, vi } from "vitest";
import { bridgeHost, operations } from "./bridge";

function respond(status: number, body: unknown) {
  const fetchMock = vi.fn(() => Promise.resolve(new Response(status === 204 ? null : JSON.stringify(body), { status })));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}
function lastCall(fetchMock: ReturnType<typeof respond>) {
  const [url, init] = fetchMock.mock.calls.at(-1) as unknown as [string, RequestInit];
  return { url, init };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("the operations a page may ask for", () => {
  it("answers whoami from the artifact's own /me, with the cookie header the API demands", async () => {
    const fetchMock = respond(200, { username: "alice" });
    expect(await operations("abc").whoami?.({})).toEqual({ username: "alice" });
    const { url, init } = lastCall(fetchMock);
    expect(url).toBe("/api/artifacts/abc/me");
    expect(init.method).toBe("GET");
    expect(init.headers).toMatchObject({ "X-Pictor": "1" });
  });

  it("reads, writes, deletes and lists documents under the artifact, escaping what the page names", async () => {
    const ops = operations("abc");
    let fetchMock = respond(200, { id: "d", data: { v: 1 } });
    expect(await ops["db.get"]?.({ collection: "a b", id: "x/y" })).toEqual({ v: 1 });
    expect(lastCall(fetchMock).url).toBe("/api/artifacts/abc/db/a%20b/x%2Fy");

    fetchMock = respond(200, { id: "d", data: { v: 2 } });
    expect(await ops["db.set"]?.({ collection: "c", id: "d", data: { v: 2 } })).toEqual({ v: 2 });
    expect(lastCall(fetchMock).init.method).toBe("PUT");
    expect(lastCall(fetchMock).init.body).toBe('{"v":2}');

    fetchMock = respond(204, null);
    expect(await ops["db.delete"]?.({ collection: "c", id: "d" })).toBeNull();
    expect(lastCall(fetchMock).init.method).toBe("DELETE");

    fetchMock = respond(200, [{ id: "d" }]);
    expect(await ops["db.list"]?.({ collection: "c" })).toEqual([{ id: "d" }]);
    expect(lastCall(fetchMock).url).toBe("/api/artifacts/abc/db/c");
  });

  it("queries, updates and batches through the API, and puts the version a write was built on in the address", async () => {
    const ops = operations("abc");
    let fetchMock = respond(200, [{ id: "d", version: 2 }]);
    expect(await ops["db.query"]?.({ collection: "c", filter: { n: { gte: 3 } }, limit: 5 })).toEqual([{ id: "d", version: 2 }]);
    expect(lastCall(fetchMock).url).toBe("/api/artifacts/abc/db/c/query");
    expect(lastCall(fetchMock).init.method).toBe("POST");
    expect(lastCall(fetchMock).init.body).toBe('{"filter":{"n":{"gte":3}},"limit":5}');

    fetchMock = respond(200, { id: "d", data: { a: 1 }, version: 3 });
    expect(await ops["db.update"]?.({ collection: "c", id: "d", fields: { a: 1 }, ifVersion: 2 })).toMatchObject({ version: 3 });
    expect(lastCall(fetchMock).url).toBe("/api/artifacts/abc/db/c/d?ifVersion=2");
    expect(lastCall(fetchMock).init.method).toBe("PATCH");

    fetchMock = respond(200, { id: "d", data: {}, version: 1 });
    await ops["db.set"]?.({ collection: "c", id: "d", data: {}, ifVersion: 0 });
    expect(lastCall(fetchMock).url).toBe("/api/artifacts/abc/db/c/d?ifVersion=0");

    fetchMock = respond(204, null);
    await ops["db.delete"]?.({ collection: "c", id: "d", ifVersion: 4 });
    expect(lastCall(fetchMock).url).toBe("/api/artifacts/abc/db/c/d?ifVersion=4");

    fetchMock = respond(200, { results: [{ id: "d", version: 5 }] });
    expect(await ops["db.batch"]?.({ ops: [{ op: "delete", collection: "c", id: "d" }] })).toEqual([{ id: "d", version: 5 }]);
    expect(lastCall(fetchMock).url).toBe("/api/artifacts/abc/db/batch");
    expect(lastCall(fetchMock).init.body).toBe('{"ops":[{"op":"delete","collection":"c","id":"d"}]}');
  });

  it("tells the page why a stale write was refused", async () => {
    respond(409, { error: "c/d was written against version 1 but now is at version 2; nothing was changed", conflict: { current: { version: 2 } } });
    await expect(operations("abc")["db.update"]?.({ collection: "c", id: "d", fields: {}, ifVersion: 1 })).rejects.toThrow("is at version 2");
  });

  it("gives a page null for a document that is not there, and the error for any other failure", async () => {
    const ops = operations("abc");
    respond(404, { error: "not found" });
    expect(await ops["db.get"]?.({ collection: "c", id: "d" })).toBeNull();
    respond(500, { error: "internal error" });
    await expect(ops["db.get"]?.({ collection: "c", id: "d" })).rejects.toThrow("internal error");
  });
});

describe("the host that answers a page's calls", () => {
  function frame() {
    const target = { postMessage: vi.fn() };
    return { target, window: target as unknown as Window };
  }
  const call = (source: unknown, data: unknown) => new MessageEvent("message", { data, source: source as MessageEventSource });

  it("replies to the frame that asked, with the id it sent, to any origin because the frame has none", async () => {
    respond(200, { username: "alice" });
    const { target, window: win } = frame();
    await bridgeHost("abc", () => win)(call(win, { pictor: "call", id: 7, op: "whoami" }));
    expect(target.postMessage).toHaveBeenCalledWith({ pictor: "reply", id: 7, result: { username: "alice" } }, "*");
  });

  it("saves nothing, and says it was refused, for a page that was replaced while the person was deciding, and asks nothing where there is nobody to ask", async () => {
    const first = frame();
    let shown: Window = first.window;
    const ask = vi.fn(() => {
      shown = frame().window; // a newer version's frame took its place
      return Promise.resolve(true);
    });
    const saved = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
    const args = { name: "a.csv", type: "text/csv", data: "x" };
    await bridgeHost("abc", () => shown, false, ask)(call(first.window, { pictor: "call", id: 1, op: "download", args }));
    expect(first.target.postMessage).toHaveBeenCalledWith({ pictor: "reply", id: 1, error: "the viewer refused the download" }, "*");

    const alone = frame();
    await bridgeHost("abc", () => alone.window)(call(alone.window, { pictor: "call", id: 2, op: "download", args }));
    expect(alone.target.postMessage).toHaveBeenCalledWith({ pictor: "reply", id: 2, error: "this viewer cannot save files from a page" }, "*");
    expect(saved).not.toHaveBeenCalled();
    saved.mockRestore();
  });

  it("answers nothing that did not come from the artifact's own frame", async () => {
    const fetchMock = respond(200, {});
    const { target, window: win } = frame();
    const host = bridgeHost("abc", () => win);
    await host(call(frame().window, { pictor: "call", id: 1, op: "whoami" }));
    await host(call(null, { pictor: "call", id: 2, op: "whoami" }));
    await host(call(win, { pictor: "reply", id: 3 }));
    await host(call(win, "whoami"));
    expect(fetchMock).not.toHaveBeenCalled();
    expect(target.postMessage).not.toHaveBeenCalled();
  });

  it("refuses an operation it does not have, including names every object has", async () => {
    const fetchMock = respond(200, {});
    const { target, window: win } = frame();
    const host = bridgeHost("abc", () => win);
    for (const op of ["nope", "constructor", "__proto__", "toString"]) {
      await host(call(win, { pictor: "call", id: 1, op }));
      expect(target.postMessage).toHaveBeenLastCalledWith({ pictor: "reply", id: 1, error: `unknown operation ${op}` }, "*");
    }
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("passes the API's error message back to the page", async () => {
    respond(403, { error: "cross-origin request refused" });
    const { target, window: win } = frame();
    await bridgeHost("abc", () => win)(call(win, { pictor: "call", id: 4, op: "db.set", args: { collection: "c", id: "d", data: {} } }));
    expect(target.postMessage).toHaveBeenCalledWith({ pictor: "reply", id: 4, error: "cross-origin request refused" }, "*");
  });

  it("refuses writes from a frame showing an older version, whatever the page claims, and still answers reads", async () => {
    const fetchMock = respond(200, { id: "d", data: { v: 1 }, username: "alice" });
    const { target, window: win } = frame();
    const host = bridgeHost("abc", () => win, true);
    const args = { collection: "c", id: "d", data: { v: 2 }, version: "current", readOnly: false };
    await host(call(win, { pictor: "call", id: 1, op: "db.set", args }));
    await host(call(win, { pictor: "call", id: 2, op: "db.delete", args }));
    await host(call(win, { pictor: "call", id: 6, op: "db.update", args: { ...args, fields: { v: 3 } } }));
    await host(call(win, { pictor: "call", id: 7, op: "db.batch", args: { ops: [] } }));
    for (const id of [1, 2, 6, 7]) {
      expect(target.postMessage).toHaveBeenCalledWith({ pictor: "reply", id, error: "viewing an old version; restore it to make changes" }, "*");
    }
    expect(fetchMock).not.toHaveBeenCalled();

    await host(call(win, { pictor: "call", id: 3, op: "whoami" }));
    await host(call(win, { pictor: "call", id: 4, op: "db.get", args }));
    await host(call(win, { pictor: "call", id: 5, op: "db.list", args }));
    await host(call(win, { pictor: "call", id: 8, op: "db.query", args }));
    expect(fetchMock.mock.calls.map((c) => (c as unknown as [string, RequestInit])[1].method)).toEqual(["GET", "GET", "GET", "POST"]);
    expect(target.postMessage).toHaveBeenCalledWith({ pictor: "reply", id: 4, result: { v: 1 } }, "*");
  });
});
