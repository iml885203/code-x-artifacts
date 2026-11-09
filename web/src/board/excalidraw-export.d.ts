// Excalidraw 0.18's types re-export its export functions from
// "@excalidraw/utils/export", a module the package does not ship, so without
// this they would be untyped. Only what the board uses is declared.
declare module "@excalidraw/utils/export" {
  import type { ExcalidrawElement } from "@excalidraw/excalidraw/element/types";
  import type { AppState, BinaryFiles } from "@excalidraw/excalidraw/types";

  interface ExportOpts {
    elements: readonly ExcalidrawElement[];
    appState?: Partial<AppState>;
    files: BinaryFiles | null;
    exportPadding?: number;
  }
  export function exportToBlob(opts: ExportOpts & { mimeType?: string; quality?: number }): Promise<Blob>;
  export function exportToSvg(opts: ExportOpts & { skipInliningFonts?: true }): Promise<SVGSVGElement>;
}
