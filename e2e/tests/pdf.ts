import { getDocument } from "pdfjs-dist/legacy/build/pdf.mjs";

/** The text of each page of a PDF, in the order the page draws it. */
export async function pdfPages(bytes: Buffer): Promise<string[]> {
  const task = getDocument({ data: new Uint8Array(bytes), useSystemFonts: false, verbosity: 0 });
  const doc = await task.promise;
  const pages: string[] = [];
  for (let n = 1; n <= doc.numPages; n++) {
    const content = await (await doc.getPage(n)).getTextContent();
    pages.push(content.items.map((i) => ("str" in i ? i.str : "")).join(" "));
  }
  await task.destroy();
  return pages;
}

/** The size of each page of a PDF in CSS pixels (a PDF point is 3/4 of one). */
export async function pdfPageSizes(bytes: Buffer): Promise<{ width: number; height: number }[]> {
  const task = getDocument({ data: new Uint8Array(bytes), useSystemFonts: false, verbosity: 0 });
  const doc = await task.promise;
  const sizes: { width: number; height: number }[] = [];
  for (let n = 1; n <= doc.numPages; n++) {
    const [, , x1 = 0, y1 = 0] = (await doc.getPage(n)).view;
    sizes.push({ width: Math.round((x1 * 4) / 3), height: Math.round((y1 * 4) / 3) });
  }
  await task.destroy();
  return sizes;
}
