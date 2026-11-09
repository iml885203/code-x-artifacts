import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach, vi } from "vitest";

afterEach(cleanup);

// jsdom has no layout, scrolling or pointer capture; Radix and the router ask for them.
window.scrollTo = vi.fn();
Element.prototype.scrollIntoView = vi.fn();
Element.prototype.hasPointerCapture = () => false;
Element.prototype.setPointerCapture = vi.fn();
Element.prototype.releasePointerCapture = vi.fn();
// jsdom has <dialog> but not showModal/close; the open attribute is what the tests see.
HTMLDialogElement.prototype.showModal = function showModal() {
  this.setAttribute("open", "");
};
HTMLDialogElement.prototype.close = function close() {
  this.removeAttribute("open");
};
globalThis.ResizeObserver = class {
  observe() {
    /* nothing to measure */
  }
  unobserve() {
    /* nothing to measure */
  }
  disconnect() {
    /* nothing to measure */
  }
};
// jsdom draws nothing: a canvas gives no context, and there is no media-query engine.
HTMLCanvasElement.prototype.getContext = () => null;
// jsdom has no layout: ProseMirror asks ranges where they are when it scrolls a selection into view.
Range.prototype.getClientRects = () => ({ length: 0, item: () => null, [Symbol.iterator]: [][Symbol.iterator] });
Range.prototype.getBoundingClientRect = () => new DOMRect();
