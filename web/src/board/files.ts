// A board's pictures: each is a file uploaded to the board's artifact, named by
// the image element's fileId (internal/boardfmt). Nothing of a picture is in the
// room; each browser loads it from the artifact's files.
import { uploadName } from "../uploadName";

/** A file the editor shows, as Excalidraw's BinaryFileData has it. */
export interface BoardFile {
  id: string;
  mimeType: string;
  dataURL: string;
  created: number;
}

const types: Record<string, string> = { png: "image/png", jpg: "image/jpeg", jpeg: "image/jpeg", gif: "image/gif", webp: "image/webp", svg: "image/svg+xml" };

/** The file the editor loads for a picture: only ever one of this artifact's. */
export function boardFile(artifactId: string, name: string, now: () => number = Date.now): BoardFile {
  const ext = name.slice(name.lastIndexOf(".") + 1).toLowerCase();
  return { id: name, mimeType: types[ext] ?? "image/png", dataURL: `/files/${encodeURIComponent(artifactId)}/${encodeURIComponent(name)}`, created: now() };
}

/**
 * The pictures to give the editor: those its images show that it does not have
 * yet and that the artifact has. A picture whose file is gone is left out, and
 * the editor draws its placeholder instead of asking for a file that is not there.
 */
export function picturesToLoad(elements: readonly { type?: unknown; fileId?: unknown; isDeleted?: unknown }[], loaded: ReadonlySet<string>, files: ReadonlySet<string>): string[] {
  const out = new Set<string>();
  for (const el of elements) {
    if (el.type !== "image" || el.isDeleted === true || typeof el.fileId !== "string") continue;
    if (!loaded.has(el.fileId) && files.has(el.fileId)) out.add(el.fileId);
  }
  return [...out];
}

/**
 * Uploads a picture someone pasted or dropped and gives the name the board keeps
 * it by. A refusal tells the person the server's reason and throws, so the
 * editor adds nothing.
 */
export async function uploadPicture(file: File, upload: (file: File) => Promise<{ name: string }>, refused: (reason: string) => void): Promise<string> {
  try {
    return (await upload(new File([file], uploadName(file), { type: file.type }))).name;
  } catch (err) {
    const reason = (err as { reason?: unknown }).reason;
    refused(typeof reason === "string" ? reason : "");
    throw err;
  }
}
