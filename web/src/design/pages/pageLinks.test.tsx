import { act, renderHook } from "@testing-library/react";
import { expect, it } from "vitest";
import { pageLink, usePageLink } from "./pageLinks";

it("copies the exact page identity while retaining version and comment navigation", () => {
  const url = new URL(pageLink("https://example.test/a/design?version=2#comment-4", "頁面 / 2"));
  expect(url.searchParams.get("page")).toBe("頁面 / 2");
  expect(url.searchParams.get("version")).toBe("2");
  expect(url.hash).toBe("#comment-4");
});

it("reads a page deep link, updates it on switching and follows browser history", () => {
  window.history.replaceState(null, "", "/a/design?page=about");
  const { result, unmount } = renderHook(usePageLink);
  expect(result.current[0]).toBe("about");
  act(() => { result.current[1]("home"); });
  expect(new URL(window.location.href).searchParams.get("page")).toBe("home");
  act(() => { window.history.replaceState(null, "", "/a/design?page=about"); window.dispatchEvent(new PopStateEvent("popstate")); });
  expect(result.current[0]).toBe("about");
  unmount();
  window.history.replaceState(null, "", "/");
});
