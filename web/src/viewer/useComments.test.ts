import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useComments } from "./useComments";

const root = { id: "root", author: "alice", body: "Erase these words", createdAt: "2026-10-22T00:00:00Z" };
afterEach(() => { vi.unstubAllGlobals(); });

it("does not resurrect a deleted body from an earlier read and removes an empty deleted thread after its final reply", async () => {
  const reply = { ...root, id: "reply", parentId: "root", body: "Keep this reply", author: "bob" };
  let release: ((response: Response) => void) | undefined;
  vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(new Response(JSON.stringify([root, reply]))).mockImplementationOnce(() => new Promise<Response>((resolve) => { release = resolve; })));
  const hook = renderHook(() => useComments("artifact", true, 2));
  await waitFor(() => { expect(hook.result.current.load.status).toBe("ready"); });
  let stale: Promise<unknown> | undefined;
  act(() => { stale = hook.result.current.refresh(); });
  act(() => { hook.result.current.remove("root"); });
  await act(async () => { release?.(new Response(JSON.stringify([root, reply]))); await stale; });
  expect(hook.result.current.load).toMatchObject({ status: "ready", list: [{ id: "root", body: "", deleted: true }, reply] });
  act(() => { hook.result.current.remove("reply"); });
  expect(hook.result.current.load).toEqual({ status: "ready", list: [] });
});
