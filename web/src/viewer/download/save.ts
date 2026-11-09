import type { Accepted } from "./rules";

/** Hands the bytes the page sent to the browser to save, as they are, under the cleaned name. Only called from the viewer's own click on "Save". */
export function saveFile(file: Accepted): void {
  // A text is saved as the UTF-8 it is; bytes go through as they are.
  const blob = new Blob([file.data as BlobPart], { type: file.type });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = file.name;
  link.hidden = true;
  document.body.append(link);
  link.click();
  link.remove();
  // The browser has started reading the blob by the next task.
  setTimeout(() => {
    URL.revokeObjectURL(url);
  }, 0);
}
