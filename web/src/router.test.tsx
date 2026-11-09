import { act, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { RouterProvider, useRouter } from "./router";

function Go() {
  const router = useRouter();
  return <button onClick={() => { router.go("/a/abc"); }}>go</button>;
}

afterEach(() => {
  Reflect.deleteProperty(document, "startViewTransition");
});

describe("a view transition the browser aborts", () => {
  it("leaves no unhandled rejection: ready and the update callback both reject when the page is hidden or replaced", async () => {
    const unhandled: unknown[] = [];
    const record = (reason: unknown) => {
      unhandled.push(reason);
    };
    const others = process.listeners("unhandledRejection");
    process.removeAllListeners("unhandledRejection");
    process.on("unhandledRejection", record);
    const aborted = () => Promise.reject(new DOMException("Transition was aborted because of invalid state", "InvalidStateError"));
    Object.defineProperty(document, "startViewTransition", {
      configurable: true,
      value: () => ({ ready: aborted(), updateCallbackDone: aborted(), finished: aborted() }),
    });
    const { getByText } = render(
      <RouterProvider initial="/">
        <Go />
      </RouterProvider>,
    );
    await act(async () => {
      getByText("go").click();
      await vi.waitFor(() => undefined);
      await new Promise((r) => setTimeout(r, 20));
    });
    process.off("unhandledRejection", record);
    for (const l of others) process.on("unhandledRejection", l);
    expect(unhandled).toEqual([]);
  });
});

describe("the transition of an ordinary navigation, into a page or out of one", () => {
  // The script the shell loads in <head>, run as a browser runs it: before the app, on the page.
  const source = readFileSync(resolve(__dirname, "../../internal/web/static/transitions.js"), "utf8");
  const aborted = () => Promise.reject(new DOMException("Transition was aborted because of invalid state", "InvalidStateError"));

  function load() {
    const script = document.createElement("script");
    script.textContent = source;
    document.head.append(script);
    script.remove();
  }

  async function unhandledDuring(act: () => void): Promise<unknown[]> {
    const unhandled: unknown[] = [];
    const others = process.listeners("unhandledRejection");
    process.removeAllListeners("unhandledRejection");
    process.on("unhandledRejection", (reason) => unhandled.push(reason));
    act();
    await new Promise((r) => setTimeout(r, 20));
    process.removeAllListeners("unhandledRejection");
    for (const l of others) process.on("unhandledRejection", l);
    return unhandled;
  }

  it("leaves no unhandled rejection when the browser aborts it and hands the page its transition", async () => {
    load();
    const unhandled = await unhandledDuring(() => {
      for (const type of ["pagereveal", "pageswap"]) {
        const event = new Event(type);
        Object.assign(event, { viewTransition: { ready: aborted(), updateCallbackDone: aborted(), finished: aborted() } });
        window.dispatchEvent(event);
      }
    });
    expect(unhandled).toEqual([]);
  });

  it("keeps an aborted transition's rejection out of the console when the browser gave the page no transition to take it from", () => {
    load();
    const event = new Event("unhandledrejection", { cancelable: true });
    Object.assign(event, { reason: new DOMException("Transition was aborted because of invalid state. ViewTransition opt-in disabled", "InvalidStateError") });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
  });

  it("leaves every other rejection to be reported", () => {
    load();
    for (const reason of [new Error("Transition was aborted"), new DOMException("The operation was aborted", "AbortError"), new DOMException("nope", "InvalidStateError"), "Transition was aborted"]) {
      const event = new Event("unhandledrejection", { cancelable: true });
      Object.assign(event, { reason });
      window.dispatchEvent(event);
      expect(event.defaultPrevented).toBe(false);
    }
  });
});
