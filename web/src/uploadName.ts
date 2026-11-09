// The name a picture is uploaded under, shared by the Docs editor and the board.

/** A name that is the file's own and no other upload's: a second screenshot called image.png must not replace the first. */
export function uploadName(file: File, random: () => string = () => crypto.randomUUID().slice(0, 8)): string {
  const dot = file.name.lastIndexOf(".");
  const typed = /^image\/(png|jpeg|gif|webp|svg\+xml)$/.exec(file.type)?.[1];
  const ext = dot > 0 ? file.name.slice(dot).toLowerCase() : typed === undefined ? "" : typed === "jpeg" ? ".jpg" : typed === "svg+xml" ? ".svg" : `.${typed}`;
  const stem = (dot > 0 ? file.name.slice(0, dot) : file.name).replace(/[^\p{L}\p{N}_-]+/gu, "-").replace(/^-+|-+$/g, "").slice(0, 40);
  return `${stem === "" ? "picture" : stem}-${random()}${ext}`;
}
