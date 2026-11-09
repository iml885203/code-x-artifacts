import { act, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useServed } from "./useServed";

function Probe({ urls }: { urls: string[] }) {
  const served = useServed(urls);
  return <output>{urls.filter((u) => served.has(u)).join(",")}</output>;
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("a frame the room was just given", () => {
  it("is loaded only once the server has it, and asked for again until then", async () => {
    vi.useFakeTimers();
    const answers = [404, 404, 200];
    const fetch = vi.fn(() => Promise.resolve({ ok: (answers.shift() ?? 200) === 200 }));
    vi.stubGlobal("fetch", fetch);
    render(<Probe urls={["/raw/x/g/frames/new.html"]} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
    });
    expect(screen.getByRole("status")).toHaveTextContent("");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(600);
    });
    expect(screen.getByRole("status")).toHaveTextContent("/raw/x/g/frames/new.html");
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it("is let through when the server never has it, so the frame shows its own error", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve({ ok: false })));
    render(<Probe urls={["/raw/x/g/frames/gone.html"]} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(9000);
    });
    expect(screen.getByRole("status")).toHaveTextContent("gone.html");
  });
});
