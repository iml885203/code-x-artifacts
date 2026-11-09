// A board's export in the browser: the file name the server gives an export
// (exportFileName in internal/web/export.go), and a PNG made from an SVG.

/** The title as a file name: what a file system refuses becomes a space, at most 100 characters, the id when nothing is left. */
export function exportFileName(title: string, id: string, extension: string): string {
  // eslint-disable-next-line no-control-regex -- control characters are what is being removed
  const cleaned = title.replace(/[\u0000-\u001f\u007f/\\:*?"<>|]/g, " ");
  let name = cleaned.split(/\s+/).filter(Boolean).join(" ");
  const chars = Array.from(name); // code points, as the server counts runes
  if (chars.length > 100) name = chars.slice(0, 100).join("").trim();
  name = name.replace(/^\.+/, "");
  return `${name === "" ? id : name}.${extension}`;
}

/** The SVG drawn on a white canvas twice its size, so the picture stays sharp. */
export async function pngFromSvg(svg: string): Promise<Blob> {
  const image = new Image();
  // A data: address, which the site's CSP allows for images; blob: it does not.
  image.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
  await image.decode();
  const scale = 2;
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(image.naturalWidth * scale));
  canvas.height = Math.max(1, Math.round(image.naturalHeight * scale));
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("no 2d canvas");
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.drawImage(image, 0, 0, canvas.width, canvas.height);
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob);
      else reject(new Error("the canvas made no PNG"));
    }, "image/png");
  });
}
