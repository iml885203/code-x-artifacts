// The browser calls the same JSON API the agents do, with the session cookie.
// A cookie call must carry X-Pictor (see web/AGENTS.md).
import type { TokenItem } from "./boot";

export type Visibility = "private" | "organisation" | "public";

export interface Artifact {
  id: string;
  title: string;
  kind: string;
  /** The number of the newest saved version. */
  version: number;
  author: string;
  /** Presentation only; author remains the stable account identity. */
  authorDisplayName?: string;
  visibility: string;
  createdAt: string;
  updatedAt: string;
  url: string;
  /** Whether the viewer may change who sees or edits it, or delete it: the server's answer, not the front end's. */
  canChange: boolean;
  /** Whether the viewer may publish new versions, restore and manage files: the author and the editors. */
  canEdit: boolean;
  /** Whether the viewer pinned it: their own, nobody else's. */
  pinned: boolean;
  /** When the viewer last opened it in the viewer: their own, absent when they never did. */
  openedAt?: string;
  /** What the viewer is here: the author, an editor, or a reader. */
  role: "author" | "editor" | "reader";
  /** The accounts the author named as editors. */
  editors: string[];
  /** The author's switch: everyone who can sign in may edit (only counts while the organisation can see it). */
  organisationEdit: boolean;
  /** Whether the viewer may read and write its comments: signed in and able to open it. */
  canComment: boolean;
  /** How many comments it has and, on the author's own artifact, how many they have not seen. */
  comments: CommentSummary;
  /** Where this artifact was copied from. `id` is empty when the viewer cannot open the source, so there is nothing to link to. */
  copiedFrom?: { id: string; title: string };
}

export interface CommentSummary {
  count: number;
  unread: number;
}

/** One comment. A reply names the comment that started its thread. */
export interface Comment {
  id: string;
  author: string;
  body: string;
  parentId?: string;
  deleted?: boolean;
  canDelete?: boolean;
  /** Where the comment points, when it points at a place: its meaning belongs to the kind of artifact (a doc's tab, block and quote). */
  anchor?: Record<string, unknown>;
  /** Set for the artifact's author on comments by others they have not seen. */
  unread?: boolean;
  createdAt: string;
}

/** The most characters one comment may have; the server enforces it. */
export const maxCommentChars = 4000;

export class SignedOutError extends Error {}

/** A call the server answered with an error; the status is what a caller decides on. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    /** What the server said went wrong, in its own words, when it said. */
    readonly reason = "",
    /** How many seconds the server asked to wait before the same request (`Retry-After`), 0 when it did not say. */
    readonly retryAfter = 0,
  ) {
    super(message);
  }
}

/** One file an artifact holds. `url` is what to put in the page to show it. */
export interface Asset {
  name: string;
  contentType: string;
  size: number;
  url: string;
  createdBy: string;
  createdAt: string;
}

/** One saved version, as the history lists it. */
export interface VersionRow {
  version: number;
  title: string;
  /** The first of the people who made it. */
  author: string;
  /** Everyone who made it, in the order they first edited; a version made in a live room can have several. */
  authors?: string[];
  /** The agents that made it, each as the person whose account it worked for and the client's name. */
  agents?: { by: string; client: string }[];
  /** The version this one copies, when it was made by restoring. */
  restoredFrom?: number;
  createdAt: string;
}

// A Blob is sent as it is (a file); anything else is JSON.
export async function call(method: string, path: string, body?: unknown, signal?: AbortSignal, keepalive = false): Promise<Response> {
  const file = body instanceof Blob;
  const res = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: { "X-Pictor": "1", ...(body === undefined || file ? {} : { "Content-Type": "application/json" }) },
    ...(body === undefined ? {} : { body: file ? body : JSON.stringify(body) }),
    ...(signal ? { signal } : {}),
    ...(keepalive ? { keepalive } : {}),
  });
  if (res.status === 401) throw new SignedOutError("the session ended");
  if (!res.ok) throw new ApiError(`${method} ${path}: ${String(res.status)}`, res.status, await reasonOf(res), Number(res.headers.get("Retry-After")) || 0);
  return res;
}

async function reasonOf(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: unknown };
    return typeof body.error === "string" ? body.error : "";
  } catch {
    return "";
  }
}

export async function listArtifacts(signal?: AbortSignal): Promise<Artifact[]> {
  return (await (await call("GET", "/api/artifacts", undefined, signal)).json()) as Artifact[];
}

export async function getArtifact(id: string): Promise<Artifact> {
  return (await (await call("GET", `/api/artifacts/${encodeURIComponent(id)}`)).json()) as Artifact;
}

/** What happened to an artifact since a version: where it is now, and what is new. Not signed in, only `versions` is filled. */
export interface Changes {
  id: string;
  version: number;
  title: string;
  updatedAt: string;
  versions: { version: number; author?: string; restoredFrom?: number; createdAt: string }[];
  comments: Comment[];
  access: { by: string; visibility: string; editors: string[]; organisationEdit: boolean; at: string }[];
}

/** Everything after `sinceVersion`: the same answer as the changes_since tool. */
export async function getChanges(id: string, sinceVersion: number, signal?: AbortSignal): Promise<Changes> {
  return (await (await call("GET", `/api/artifacts/${encodeURIComponent(id)}/changes?sinceVersion=${String(sinceVersion)}`, undefined, signal)).json()) as Changes;
}

export async function setVisibility(id: string, visibility: Visibility): Promise<Artifact> {
  return (await (await call("PATCH", `/api/artifacts/${encodeURIComponent(id)}`, { visibility })).json()) as Artifact;
}

/** What the author changes about who may edit; every part is optional. A refusal is an ApiError whose `reason` says why. */
export interface EditorsChange {
  add?: string[];
  remove?: string[];
  organisationEdit?: boolean;
}

export async function setEditors(id: string, change: EditorsChange): Promise<Artifact> {
  return (await (await call("PATCH", `/api/artifacts/${encodeURIComponent(id)}/editors`, change)).json()) as Artifact;
}

/** Pins or unpins for the signed-in person; both are idempotent. The card changes before the server answers, so the call outlives a navigation or reload that follows at once. */
export async function setPin(id: string, pinned: boolean): Promise<Artifact> {
  return (await (await call(pinned ? "PUT" : "DELETE", `/api/artifacts/${encodeURIComponent(id)}/pin`, undefined, undefined, true)).json()) as Artifact;
}

/** Records that the signed-in person opened the artifact in the viewer, for their "Recently opened" list. */
export async function recordOpen(id: string): Promise<Artifact> {
  return (await (await call("POST", `/api/artifacts/${encodeURIComponent(id)}/open`)).json()) as Artifact;
}

export async function deleteArtifact(id: string): Promise<void> {
  await call("DELETE", `/api/artifacts/${encodeURIComponent(id)}`);
}

/** What the canvas needs to draw a design: its manifest (design.json) and where its frames are served. */
export interface DesignData {
  /** The version these are for. */
  version: number;
  manifest: unknown;
  /** The address of the version's files, ending in a slash; a frame is at `base + file`. */
  base: string;
  /** Where a frame loads the artifact's uploaded files from, ending in a slash: `/files/<id>/<grant>/`. */
  files: string;
  /** For the newest version and someone who may join the room: the address of the frames as the room holds them now, ending in a slash; it outlasts versions. */
  live?: string;
}

/** The design of an artifact, of the newest version or of an older one that the caller may read. */
export async function getDesign(id: string, version?: number): Promise<DesignData> {
  const query = version === undefined ? "" : `?version=${String(version)}`;
  return (await (await call("GET", `/api/artifacts/${encodeURIComponent(id)}/design${query}`)).json()) as DesignData;
}

/** Newest first. Only the author and editors may ask. */
export async function listVersions(id: string): Promise<VersionRow[]> {
  return (await (await call("GET", `/api/artifacts/${encodeURIComponent(id)}/versions`)).json()) as VersionRow[];
}

/** Saves an old version as the newest. `base` is the newest version the caller saw: the server refuses (409) if there is a newer one. */
export async function restoreVersion(id: string, version: number, base: number): Promise<Artifact> {
  return (await (await call("POST", `/api/artifacts/${encodeURIComponent(id)}/versions/${String(version)}/restore`, { baseVersion: base })).json()) as Artifact;
}

/** Makes the caller's own private copy of an artifact, of the given version when one is named (editors only), else the newest. */
export async function copyArtifact(id: string, version?: number): Promise<Artifact> {
  return (await (await call("POST", `/api/artifacts/${encodeURIComponent(id)}/copy`, version === undefined ? undefined : { version })).json()) as Artifact;
}

/**
 * The file name of a Content-Disposition (RFC 6266): filename* (percent-encoded
 * UTF-8) when there is one, else filename, quoted or a plain token.
 */
export function fileName(disposition: string): string | undefined {
  const encoded = /filename\*\s*=\s*utf-8''([^;]+)/i.exec(disposition)?.[1];
  if (encoded !== undefined) {
    try {
      return decodeURIComponent(encoded.trim());
    } catch {
      // a name the browser would not decode either: fall through to the plain one
    }
  }
  const quoted = /(?:^|[;\s])filename\s*=\s*"((?:[^"\\]|\\.)*)"/i.exec(disposition)?.[1];
  if (quoted !== undefined) return quoted.replace(/\\(.)/g, "$1");
  return /(?:^|[;\s])filename\s*=\s*([^;\s"]+)/i.exec(disposition)?.[1];
}

/**
 * The artifact in one of its kind's export formats, saved by the browser: the
 * given saved version, or the newest when there is none. The export route needs
 * the X-Pictor header, which a plain link cannot send, so the file is fetched
 * and handed over as a blob.
 */
export async function downloadArtifact(id: string, format: string, version?: number): Promise<void> {
  const query = version === undefined ? "" : `?version=${String(version)}`;
  const res = await call("GET", `/api/artifacts/${encodeURIComponent(id)}/export/${encodeURIComponent(format)}${query}`);
  saveDownload(await res.blob(), fileName(res.headers.get("Content-Disposition") ?? "") ?? `${id}.${format}`);
}

/** Hands the person a file the page made, under the name given. */
export function saveDownload(blob: Blob, name: string): void {
  const link = document.createElement("a");
  link.href = holdDownloadURL(blob);
  link.download = name;
  document.body.append(link);
  link.click();
  link.remove();
}

// The browser reads a blob URL after click() returns, so revoking it right away
// can lose the file and its name on a busy machine. One URL is held at a time:
// the next download, or leaving the page, releases it.
let heldDownloadURL: string | null = null;

function holdDownloadURL(blob: Blob): string {
  releaseDownloadURL();
  heldDownloadURL = URL.createObjectURL(blob);
  return heldDownloadURL;
}

function releaseDownloadURL(): void {
  if (heldDownloadURL !== null) URL.revokeObjectURL(heldDownloadURL);
  heldDownloadURL = null;
}

if (typeof window !== "undefined") window.addEventListener("pagehide", releaseDownloadURL);

/** Oldest first. */
export async function listComments(id: string): Promise<Comment[]> {
  return (await (await call("GET", `/api/artifacts/${encodeURIComponent(id)}/comments`)).json()) as Comment[];
}

export async function addComment(id: string, body: string, parentId?: string, anchor?: Record<string, unknown>): Promise<Comment> {
  return (await (await call("POST", `/api/artifacts/${encodeURIComponent(id)}/comments`, { body, ...(parentId ? { parentId } : {}), ...(anchor ? { anchor } : {}) })).json()) as Comment;
}

/** Removes only this author's content; a root with other replies remains a placeholder. */
export async function deleteComment(id: string, commentId: string): Promise<void> {
  await call("DELETE", `/api/artifacts/${encodeURIComponent(id)}/comments/${encodeURIComponent(commentId)}`);
}

/** The author has seen the comments up to this one, the newest that was shown. */
export async function markCommentsSeen(id: string, upTo: string): Promise<void> {
  await call("POST", `/api/artifacts/${encodeURIComponent(id)}/comments/seen`, { upTo });
}

/** The files of an artifact, as the server lists them. */
export async function listAssets(id: string): Promise<Asset[]> {
  return (await (await call("GET", `/api/artifacts/${encodeURIComponent(id)}/assets`)).json()) as Asset[];
}

/** Adds the file, or replaces the one with the same name. A refusal is an ApiError whose `reason` says why. */
export async function uploadAsset(id: string, file: File): Promise<Asset> {
  return (await (await call("PUT", `/api/artifacts/${encodeURIComponent(id)}/assets/${encodeURIComponent(file.name)}`, file)).json()) as Asset;
}

export async function deleteAsset(id: string, name: string): Promise<void> {
  await call("DELETE", `/api/artifacts/${encodeURIComponent(id)}/assets/${encodeURIComponent(name)}`);
}

/** The signed-in person's own tokens, newest first, without their secrets. Only a browser session may ask. */
export async function listTokens(): Promise<TokenItem[]> {
  return (await (await call("GET", "/api/tokens")).json()) as TokenItem[];
}

/** Revokes one of the person's own tokens; anything using it is refused from the next request. */
export async function revokeToken(id: string): Promise<void> {
  await call("DELETE", `/api/tokens/${encodeURIComponent(id)}`);
}

/** Makes a personal token and returns its secret, which the server never shows again. Only a signed-in browser may ask. */
export async function createToken(name: string): Promise<string> {
  return ((await (await call("POST", "/api/tokens", { name })).json()) as { token: string }).token;
}

/** What opens a live socket: a one-use ticket, the epoch of the document now, and the socket's path. */
export interface LiveTicket {
  ticket: string;
  epoch: string;
  path: string;
}

/** A person a doc can mention: the sign-in account, the name the directory gave, and whether they can open the doc now. */
export interface Person {
  user: string;
  name: string;
  canOpen: boolean;
}

/** Organisation colleagues whose account or name contains the text, for the editor of a doc to mention (docs/doc-format.md). */
export interface PeopleFound { people: Person[]; complete: boolean }

export async function searchPeople(id: string, query: string, signal?: AbortSignal): Promise<PeopleFound> {
  const res = await call("GET", `/api/artifacts/${encodeURIComponent(id)}/people?q=${encodeURIComponent(query)}`, undefined, signal);
  const result = (await res.json()) as { people: Person[]; directory?: { complete: boolean } };
  return { people: result.people, complete: result.directory?.complete === true };
}

/** Asks for a ticket to open the artifact's live socket (docs/realtime.md). */
export async function liveTicket(id: string, signal?: AbortSignal): Promise<LiveTicket> {
  return (await (await call("POST", `/api/artifacts/${encodeURIComponent(id)}/live`, undefined, signal)).json()) as LiveTicket;
}
