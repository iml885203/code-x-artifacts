// The outer half of window.pictor. The artifact runs in an opaque origin and
// cannot reach the API; internal/web/bridge.js (injected into the page) posts
// {pictor:"call", id, op, args} here, and this answers with {pictor:"reply"}.
// Only messages from the artifact's own frame are answered, and every call is
// made as the viewer, with the cookie and X-Pictor (see web/AGENTS.md).

import type { Ask } from "./download/DownloadPrompt";
import { checkDownload } from "./download/rules";
import { saveFile } from "./download/save";

class ApiError extends Error {
  readonly status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

interface Args {
  collection?: string;
  id?: string;
  data?: unknown;
  fields?: unknown;
  filter?: unknown;
  limit?: number;
  ops?: unknown;
  ifVersion?: number;
}

async function api(method: string, path: string, body?: unknown): Promise<unknown> {
  const res = await fetch(path, {
    method,
    headers: { "Content-Type": "application/json", "X-Pictor": "1" },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  if (!res.ok) {
    const detail = (await res.json().catch(() => ({}))) as { error?: string };
    throw new ApiError(res.status, detail.error ?? res.statusText);
  }
  return res.status === 204 ? null : ((await res.json()) as unknown);
}

/** What a frame showing an older version gets when it tries to change the shared data. */
export const OLD_VERSION_WRITE = "viewing an old version; restore it to make changes";

/**
 * The operations a page may ask for, for one artifact. A frame showing an older
 * version is read only: its code could otherwise overwrite what the current
 * version keeps in the shared data. Whether it is old is decided by the viewer
 * from the version it opened the frame with, never by anything the page says.
 */
export function operations(artifactId: string, readOnly = false): Record<string, (a: Args) => Promise<unknown>> {
  const base = `/api/artifacts/${encodeURIComponent(artifactId)}`;
  const collection = (a: Args) => `${base}/db/${encodeURIComponent(a.collection ?? "")}`;
  // The version a write was built on goes in the address, as for any caller of the API.
  const doc = (a: Args) => `${collection(a)}/${encodeURIComponent(a.id ?? "")}${typeof a.ifVersion === "number" ? `?ifVersion=${String(a.ifVersion)}` : ""}`;
  const write = (run: () => Promise<unknown>) => (readOnly ? Promise.reject(new Error(OLD_VERSION_WRITE)) : run());
  return {
    whoami: () => api("GET", `${base}/me`),
    "db.get": (a) =>
      api("GET", doc(a)).then(
        (d) => (d as { data: unknown }).data,
        (err: unknown) => (err instanceof ApiError && err.status === 404 ? null : Promise.reject(err as Error)),
      ),
    "db.set": (a) => write(() => api("PUT", doc(a), a.data).then((d) => (d as { data: unknown }).data)),
    "db.update": (a) => write(() => api("PATCH", doc(a), a.fields)),
    "db.delete": (a) => write(() => api("DELETE", doc(a))),
    "db.list": (a) => api("GET", collection(a)),
    "db.query": (a) => api("POST", `${collection(a)}/query`, { filter: a.filter, limit: a.limit }),
    "db.batch": (a) => write(() => api("POST", `${base}/db/batch`, { ops: a.ops }).then((r) => (r as { results: unknown }).results)),
  };
}

interface Call {
  pictor: "call";
  id: number;
  op: string;
  args?: Args;
}

function isCall(m: unknown): m is Call {
  return typeof m === "object" && m !== null && (m as { pictor?: unknown }).pictor === "call";
}

/** What a page that asks to save a file gets when the person said no. */
export const DOWNLOAD_REFUSED = "the viewer refused the download";
/** What it gets when the viewer has nowhere to ask. */
export const DOWNLOAD_UNAVAILABLE = "this viewer cannot save files from a page";

/**
 * pictor.download: the limits are checked here, before anyone is asked, and
 * the file is saved only after the person's own click (`ask` resolves true).
 * Nothing the page sends can skip the question.
 */
async function download(args: Args, ask: Ask | undefined, stillThere: () => boolean): Promise<unknown> {
  if (!ask) throw new Error(DOWNLOAD_UNAVAILABLE);
  const file = checkDownload(args);
  if (!file.ok) throw new Error(file.error);
  // A page replaced while the person was reading the prompt (a newer version) is not the one they were asked about.
  if (!(await ask(file)) || !stillThere()) throw new Error(DOWNLOAD_REFUSED);
  saveFile(file);
  return { name: file.name, size: file.size };
}

/**
 * The listener for the window's "message" events: it answers calls from `frame` and ignores everything else.
 * `ask` is how the viewer asks the person about a file the page wants to save.
 */
export function bridgeHost(artifactId: string, frame: () => Window | null, readOnly = false, ask?: Ask) {
  const base = operations(artifactId, readOnly);
  return async (e: MessageEvent) => {
    const target = frame();
    if (!target || e.source !== target || !isCall(e.data)) return;
    const m = e.data;
    const ops: Record<string, (a: Args) => Promise<unknown>> = { ...base, download: (a) => download(a, ask, () => frame() === target) };
    // An opaque origin has no name to address, so "*" is the only way to reach it.
    const reply = (body: { result?: unknown; error?: string }) => {
      target.postMessage({ pictor: "reply", id: m.id, ...body }, "*");
    };
    const op = Object.hasOwn(ops, m.op) ? ops[m.op] : undefined;
    if (!op) {
      reply({ error: `unknown operation ${m.op}` });
      return;
    }
    try {
      reply({ result: await op(m.args ?? {}) });
    } catch (err) {
      reply({ error: err instanceof Error ? err.message : String(err) });
    }
  };
}
