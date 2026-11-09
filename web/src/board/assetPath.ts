// Excalidraw loads its fonts from EXCALIDRAW_ASSET_PATH; the build copies them
// under /assets/excalidraw/ (vite.config.ts), so no request leaves the site.
// Set by the entry, before any chunk: Excalidraw reads it when its chunk is
// evaluated, and the chunk may run before the board's own module.
(window as unknown as { EXCALIDRAW_ASSET_PATH: string }).EXCALIDRAW_ASSET_PATH = "/assets/excalidraw/";
