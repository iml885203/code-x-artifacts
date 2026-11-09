// The page's side of the conversation with the inspector in each frame
// (internal/kind/design_inspector.js). The frames are sandboxed documents the
// page cannot reach into, so everything it learns about an element is a message
// the frame sends back. Only the window of a frame the page itself drew is
// listened to; a request is answered by the reply that carries its number, and
// the page waits for that reply (and for the frame to say it is ready) instead
// of guessing how long a frame takes.

export interface ElementReport {
  pid: string;
  tag: string;
  /** Where the element is, in the frame's own pixels from the frame's top left corner. */
  rect: { x: number; y: number; w: number; h: number };
  attrs: [string, string][];
  inline: [string, string][];
  computed: Record<string, string>;
}

type Reply = { pid: string | null } | { items: ElementReport[] } | { pids: string[] } | { ok: boolean };

/** A style to show in a frame: an element, an owned property, and its value as the room holds it; null puts back what the frame had, "" removes it. */
export type StyleChange = [pid: string, prop: string, value: string | null];

/** A room-derived inline cascade, replayed without expanding authored shorthand. */
export type InlineChange = [pid: string, value: string | null];

/** A parent and its element children, in order. */
export type ChildOrder = [parent: string, children: string[]];

/** An element the frame does not have, as markup to put under `parent`, right before `before` (the end when null); `ns` is "svg" for markup that is svg. The markup carries the data-pid of every element in it. */
export interface Build {
  parent: string;
  before: string | null;
  html: string;
  ns?: string;
}

/** An attribute of an element that is there, to show (null takes it away): the alt and src of an image. */
export type AttrChange = [pid: string, name: "alt" | "src", value: string | null];

interface Pending {
  frameId: string;
  resolve: (reply: Reply) => void;
  reject: (reason: Error) => void;
  timer: ReturnType<typeof setTimeout>;
}

/** How long a frame may take to answer before the question is given up on, so one that never loads cannot hang the page. */
const ANSWER_LIMIT_MS = 5000;

/** What a question to a frame ends in when the frame is loaded again or taken away before it answers: the question no longer has anyone to answer it. */
export class FrameGoneError extends Error {
  constructor() {
    super("the frame is gone");
    this.name = "FrameGoneError";
  }
}

/** Whether a failed question failed only because its frame went: whoever asked has nothing to report, the next frame is told everything once it is ready. */
export const isFrameGone = (err: unknown): boolean => err instanceof FrameGoneError;

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null;

export class FrameLinks {
  private readonly frames = new Map<string, HTMLIFrameElement>();
  private readonly ready = new Set<string>();
  private readonly waiting = new Map<string, (() => void)[]>();
  private readonly pending = new Map<number, Pending>();
  private next = 1;

  private readonly refs = new Map<string, (el: HTMLIFrameElement | null) => void>();
  private readonly pids = new Map<string, Set<string>>();
  private plain = "";
  private granted = "";

  /**
   * What a frame is told about the artifact's own files is written with the grant
   * the frame loads them by, which the room (and so the saved file) does not hold:
   * `/files/<id>/` becomes `granted`.
   */
  setFiles(plain: string, granted: string): void {
    this.plain = plain;
    this.granted = granted;
  }

  private grant(text: string): string {
    return this.plain === "" || text.includes(this.granted) ? text : text.split(this.plain).join(this.granted);
  }
  private readonly readyListeners = new Set<(frameId: string) => void>();
  private readonly editedListeners = new Set<(frameId: string, pid: string, runs: unknown) => void>();

  /** Called when a person is done typing into an element of a frame: with what it became, or null when they gave up. Returns the unsubscribe. */
  onEdited(fn: (frameId: string, pid: string, runs: unknown) => void): () => void {
    this.editedListeners.add(fn);
    return () => this.editedListeners.delete(fn);
  }

  /** Called each time a frame says it is ready (a frame that is loaded again says it again). Returns the unsubscribe. */
  onReady(fn: (frameId: string) => void): () => void {
    this.readyListeners.add(fn);
    return () => this.readyListeners.delete(fn);
  }

  /** The ref callback for the element that shows a frame; the same function every time, so a render does not detach and attach it again. */
  refFor(frameId: string): (el: HTMLIFrameElement | null) => void {
    let ref = this.refs.get(frameId);
    if (!ref) {
      ref = (el) => {
        this.attach(frameId, el);
      };
      this.refs.set(frameId, ref);
    }
    return ref;
  }

  /** Remembers the element that shows a frame, or forgets the frame when it goes. A new element is a frame that has not said it is ready. */
  attach(frameId: string, el: HTMLIFrameElement | null): void {
    if (el === null) {
      this.frames.delete(frameId);
      this.ready.delete(frameId);
      this.pids.delete(frameId);
      this.abandon(frameId);
      return;
    }
    if (this.frames.get(frameId) !== el) {
      this.frames.set(frameId, el);
      this.ready.delete(frameId);
      this.pids.delete(frameId);
      this.abandon(frameId);
    }
  }

  /** Ends the questions a frame was asked and has not answered: the window that was asked is no longer the frame. */
  private abandon(frameId: string): void {
    for (const [id, p] of this.pending) {
      if (p.frameId !== frameId) continue;
      clearTimeout(p.timer);
      this.pending.delete(id);
      p.reject(new FrameGoneError());
    }
  }

  /** Takes a message the page received; it matters only if it came from the window of a frame this page drew. */
  hear(e: MessageEvent): void {
    const frameId = this.frameOf(e.source);
    const data: unknown = e.data;
    if (frameId === null || !isRecord(data) || data.pictor !== "inspect") return;
    if (data.type === "ready") {
      this.ready.add(frameId);
      this.pids.set(frameId, new Set(Array.isArray(data.pids) ? data.pids.filter((p): p is string => typeof p === "string") : []));
      const waiting = this.waiting.get(frameId) ?? [];
      this.waiting.delete(frameId);
      // Queue the room's missing elements before questions waiting for this frame.
      for (const listener of this.readyListeners) listener(frameId);
      for (const wake of waiting) wake();
      return;
    }
    if (data.type === "edited") {
      if (typeof data.pid === "string") for (const listener of this.editedListeners) listener(frameId, data.pid, data.runs ?? null);
      return;
    }
    const id = data.id;
    if (typeof id !== "number") return;
    const pending = this.pending.get(id);
    // A reply counts only for the frame that was asked.
    if (pending?.frameId !== frameId) return;
    clearTimeout(pending.timer);
    this.pending.delete(id);
    pending.resolve(data as unknown as Reply);
  }

  private frameOf(source: MessageEventSource | null): string | null {
    if (source === null) return null;
    for (const [id, el] of this.frames) if (el.contentWindow === source) return id;
    return null;
  }

  private whenReady(frameId: string): Promise<void> {
    if (this.ready.has(frameId)) return Promise.resolve();
    return new Promise((resolve) => {
      this.waiting.set(frameId, [...(this.waiting.get(frameId) ?? []), resolve]);
    });
  }

  private async ask(frameId: string, message: Record<string, unknown>): Promise<Reply> {
    await this.whenReady(frameId);
    const target = this.frames.get(frameId)?.contentWindow;
    if (!target) throw new FrameGoneError();
    const id = this.next++;
    return new Promise<Reply>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error("the frame did not answer"));
      }, ANSWER_LIMIT_MS);
      this.pending.set(id, { frameId, resolve, reject, timer });
      target.postMessage({ pictor: "inspect", id, ...message }, "*");
    });
  }

  /** Shows shared variables without changing any element's authored declarations. */
  async tokens(frameId: string, tokens: Record<string, string>): Promise<void> {
    await this.ask(frameId, { type: "tokens", tokens });
  }

  /** The id of the element at a point of the frame (in its own pixels), or null where there is none. */
  async hit(frameId: string, x: number, y: number): Promise<string | null> {
    const reply = await this.ask(frameId, { type: "hit", x, y });
    return "pid" in reply ? reply.pid : null;
  }

  /** What the frame knows about each of the elements, for those it still has. */
  async report(frameId: string, pids: string[]): Promise<ElementReport[]> {
    const reply = await this.ask(frameId, { type: "report", pids });
    return "items" in reply ? reply.items : [];
  }

  /**
   * Has the frame show what the room holds: the styles, and when `alive` is
   * given the elements that are there (the others are hidden), and when `order`
   * is given the elements inside each parent in that order. Resolves when the
   * frame has done it, so what is asked of it next sees the result.
   */
  async apply(frameId: string, styles: StyleChange[], alive?: string[], order?: ChildOrder[], inline?: InlineChange[]): Promise<void> {
    const shown = styles.map(([pid, prop, value]): StyleChange => [pid, prop, value === null ? null : this.grant(value)]);
    await this.ask(frameId, { type: "apply", styles: shown, ...(alive ? { alive } : {}), ...(order ? { order } : {}), ...(inline ? { inline: inline.map(([pid, value]) => [pid, value === null ? null : this.grant(value)]) } : {}) });
  }

  /** The elements a frame has, as it said when it was loaded and as it has been built since; null until it says it is ready. */
  has(frameId: string): ReadonlySet<string> | null {
    return this.ready.has(frameId) ? (this.pids.get(frameId) ?? null) : null;
  }

  /** Has the frame build the elements it does not have and change the alt and src of the ones it does. Resolves when it has. */
  async mount(frameId: string, builds: Build[], attrs: AttrChange[] = []): Promise<void> {
    const made = builds.map((b) => ({ ...b, html: this.grant(b.html) }));
    const changed = attrs.map(([pid, name, value]): AttrChange => [pid, name, value === null ? null : this.grant(value)]);
    await this.ask(frameId, { type: "apply", styles: [], create: made, attrs: changed });
    const have = this.pids.get(frameId);
    for (const b of builds) for (const m of b.html.matchAll(/data-pid="([^"]*)"/g)) have?.add(m[1] ?? "");
  }

  /** Every element with an id from the one at a point of the frame outwards, nearest first. */
  async chain(frameId: string, x: number, y: number): Promise<string[]> {
    const reply = await this.ask(frameId, { type: "chain", x, y });
    return "pids" in reply ? reply.pids : [];
  }

  /** Has the frame let the person type into an element that holds `html`; false when the frame has no such element. */
  async edit(frameId: string, pid: string, html: string): Promise<boolean> {
    const reply = await this.ask(frameId, { type: "edit", pid, html });
    return "ok" in reply && reply.ok;
  }

  /** Has the frame show the content of elements that are only text, as markup. */
  async showTexts(frameId: string, texts: [pid: string, html: string][]): Promise<void> {
    await this.ask(frameId, { type: "apply", styles: [], texts });
  }

  /** Gives the keyboard back to the page, which is where it goes when a person is done typing in a frame. */
  release(frameId: string): void {
    this.frames.get(frameId)?.blur();
    window.focus();
  }

  /** Gives up on every question still waiting, for when the page goes. */
  close(): void {
    for (const [id, p] of this.pending) {
      clearTimeout(p.timer);
      p.reject(new Error("the viewer was closed"));
      this.pending.delete(id);
    }
  }
}
