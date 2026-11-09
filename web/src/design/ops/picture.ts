// A picture for the canvas: the file goes to the artifact through the same
// upload the Files panel uses (so the server's limits and reasons are the
// ones the person already knows), and what comes back is an address of the
// artifact (/files/<id>/<name>), never the picture's bytes in a frame.
import { uploadAsset, type Asset } from "../../api";
import type { Picture } from "./nodes";

/** A name that is the file's own and no other upload's: a second screenshot called image.png must not replace the first. */
export function pictureName(file: File, random: () => string = () => crypto.randomUUID().slice(0, 8)): string {
  const dot = file.name.lastIndexOf(".");
  const typed = /^image\/(png|jpeg|gif|webp|svg\+xml)$/.exec(file.type)?.[1];
  const ext = dot > 0 ? file.name.slice(dot).toLowerCase() : typed === undefined ? "" : typed === "jpeg" ? ".jpg" : typed === "svg+xml" ? ".svg" : `.${typed}`;
  const stem = (dot > 0 ? file.name.slice(0, dot) : file.name).replace(/[^\p{L}\p{N}_-]+/gu, "-").replace(/^-+|-+$/g, "").slice(0, 40);
  return `${stem === "" ? "picture" : stem}-${random()}${ext}`;
}

/** What the picture's alt starts as: the file's own name without its extension, for the person to say better. */
export const altOf = (file: File): string => file.name.replace(/\.[^.]*$/, "").replace(/[-_]+/g, " ").trim();

/** The size an SVG says it has (width and height, or its viewBox), or null when it says none. */
function svgSize(source: string): { width: number; height: number } | null {
  const root = new DOMParser().parseFromString(source, "image/svg+xml").documentElement;
  const positive = (n: number | undefined) => (n !== undefined && n > 0 ? n : null);
  const own = (name: string) => positive(Number.parseFloat(root.getAttribute(name) ?? ""));
  const box = (root.getAttribute("viewBox") ?? "").split(/[\s,]+/).map(Number);
  const width = own("width") ?? positive(box[2]);
  const height = own("height") ?? positive(box[3]);
  return width !== null && height !== null ? { width, height } : null;
}

/** The size the browser draws the picture at; an SVG with none gets a picture's worth. A picture the browser cannot read rejects. */
export async function sizeOf(file: File): Promise<{ width: number; height: number }> {
  if (file.type === "image/svg+xml") return svgSize(await file.text()) ?? { width: 320, height: 240 };
  // Decoded without loading an address, which the page's policy does not allow for a blob.
  const bitmap = await createImageBitmap(file);
  const size = { width: bitmap.width, height: bitmap.height };
  bitmap.close();
  return size;
}

export interface Seams {
  upload?: (id: string, file: File) => Promise<Asset>;
  size?: (file: File) => Promise<{ width: number; height: number }>;
}

/** Uploads a picture to the artifact and says what to put in a frame to show it. A refusal is the upload's ApiError, with the server's reason. */
export async function addPicture(artifactId: string, file: File, seams: Seams = {}): Promise<Picture> {
  const size = await (seams.size ?? sizeOf)(file);
  const asset = await (seams.upload ?? uploadAsset)(artifactId, new File([file], pictureName(file), { type: file.type }));
  return { src: new URL(asset.url, window.location.origin).pathname, alt: altOf(file), ...size };
}
