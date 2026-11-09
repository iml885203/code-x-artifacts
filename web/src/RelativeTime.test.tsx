import { fireEvent } from "@testing-library/react";
import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { catalogues } from "./test-support";
import { I18nProvider } from "./i18n";
import { RelativeTime } from "./RelativeTime";

const start = "2026-10-20T12:00:00Z";

function show(...isos: string[]) {
  return render(
    <I18nProvider strings={catalogues.en} lang="en">
      {isos.map((iso, i) => (
        <RelativeTime key={i} iso={iso} />
      ))}
    </I18nProvider>,
  );
}

function hide(hidden: boolean) {
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
  document.dispatchEvent(new Event("visibilitychange"));
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(start);
});
afterEach(() => {
  hide(false);
  vi.useRealTimers();
});

describe("relative time", () => {
  it("moves from now to a minute ago without anyone reloading", () => {
    show(start);
    expect(screen.getByText("now")).toBeInTheDocument();
    act(() => {
      vi.advanceTimersByTime(70_000);
    });
    expect(screen.getByText("1 minute ago")).toBeInTheDocument();
  });

  it("keeps the exact time as the tooltip and in the accessible name", () => {
    show("2026-10-19T08:05:00Z");
    const time = screen.getByText("yesterday");
    fireEvent.focus(time);
    const tip = screen.getByRole("tooltip");
    expect(tip.textContent).toMatch(/^2026-10-19 \d\d:\d\d$/);
    expect(time).toHaveAttribute("aria-describedby", tip.id);
    expect(time.getAttribute("aria-label")).toBe(`yesterday, ${tip.textContent}`);
    expect(time).not.toHaveAttribute("title");
  });

  it("runs one timer for every time on the page, and none when there is none", () => {
    const set = vi.spyOn(globalThis, "setInterval");
    const view = show(start, start, start, start);
    expect(set).toHaveBeenCalledTimes(1);
    view.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("does not tick in a hidden tab and catches up the moment the tab is shown", () => {
    show(start);
    hide(true);
    expect(vi.getTimerCount()).toBe(0);
    act(() => {
      vi.setSystemTime(new Date(Date.parse(start) + 3 * 3_600_000));
      vi.advanceTimersByTime(60_000);
    });
    expect(screen.getByText("now")).toBeInTheDocument();
    act(() => {
      hide(false);
    });
    expect(screen.getByText("3 hours ago")).toBeInTheDocument();
    expect(vi.getTimerCount()).toBe(1);
  });
});
