import { describe, expect, it } from "vitest";
import { whenLoaded, type Loading } from "./loaded";

// An editor that hands out its API while it is still loading its first scene, as Excalidraw does.
function editor(loading: boolean) {
  const listeners = new Set<(elements: readonly unknown[], state: { isLoading: boolean }) => void>();
  const state = { isLoading: loading };
  const api: Loading = {
    getAppState: () => state,
    onChange: (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
  };
  const finish = () => {
    state.isLoading = false;
    for (const cb of [...listeners]) cb([], state);
  };
  return { api, finish, listening: () => listeners.size };
}

const settled = async (p: Promise<void>) => {
  let done = false;
  void p.then(() => (done = true));
  await new Promise((r) => setTimeout(r, 0));
  return done;
};

describe("whenLoaded", () => {
  it("waits while the editor is still loading its first scene, which would replace whatever was put on it", async () => {
    const e = editor(true);
    const loaded = whenLoaded(e.api);
    expect(await settled(loaded)).toBe(false);
    e.finish();
    expect(await settled(loaded)).toBe(true);
    expect(e.listening()).toBe(0);
  });

  it("is ready at once for an editor that has loaded", async () => {
    expect(await settled(whenLoaded(editor(false).api))).toBe(true);
  });
});
