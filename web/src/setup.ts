// A test file that needs no DOM declares `// @vitest-environment node` and
// skips jsdom, which is most of what such a file costs to start; the DOM
// stubs and Testing Library's cleanup are only for the files that have one.
if (typeof document !== "undefined") await import("./setup-dom");
export {};
