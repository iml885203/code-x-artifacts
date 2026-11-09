// Typing into an element of a frame, in place. The typing happens in the frame
// (the inspector makes the element editable and reports what it became when the
// person is done); this is the page's side: which element, what to show the frame
// to start from, and, at the end, the least that has to be written to the room.
// Everything is one step of the person's history, and what other people wrote to
// the same text while this person typed stays.
import * as Y from "yjs";
import type { DesignEdits } from "../edits";
import { isFrameGone } from "../inspect";
import { contentOf, deltaOf, htmlOfContent, type Content } from "./content";
import { diffUnits, freshId, throughChanges, writeDiff, type DeltaStep } from "./diff";
import { isFormatted, parseRuns, unitsOfDelta, unitsOfRuns, type Unit } from "./units";

/** What the page asks of the frames; FrameLinks (../inspect.ts) is the real one. */
export interface TextLinks {
  chain(frameId: string, x: number, y: number): Promise<string[]>;
  edit(frameId: string, pid: string, html: string): Promise<boolean>;
  release(frameId: string): void;
  onEdited(fn: (frameId: string, pid: string, runs: unknown) => void): () => void;
}

export type Begin = "started" | "not text" | "busy" | "gone";

interface Session {
  frameId: string;
  file: string;
  pid: string;
  content: Content;
  /** The content as the person began from. */
  before: Unit[];
  /** What others did to the text since, as the text's observer saw it. */
  changes: DeltaStep[][];
  stop: () => void;
}

const nodeKey = (file: string, pid: string, field: string) => `${file}|n|${pid}|${field}`;

export class TextEditing {
  private session: Session | null = null;
  private readonly ended = new Set<(frameId: string, pid: string) => void>();
  private readonly offEdited: () => void;

  constructor(
    private readonly edits: DesignEdits,
    private readonly links: TextLinks,
  ) {
    this.offEdited = links.onEdited((frameId, pid, runs) => {
      this.finish(frameId, pid, runs);
    });
  }

  /** The element being edited, if any. */
  get editing(): { frameId: string; pid: string } | null {
    return this.session && { frameId: this.session.frameId, pid: this.session.pid };
  }

  /** Called when an edit is over, committed or not, once the room has what it is to have. Returns the unsubscribe. */
  onEnded(fn: (frameId: string, pid: string) => void): () => void {
    this.ended.add(fn);
    return () => this.ended.delete(fn);
  }

  /** Starts editing the text under a point of a frame (in the frame's own pixels). */
  async beginAt(frameId: string, file: string, x: number, y: number): Promise<Begin> {
    if (this.session) return "busy";
    const tree = this.edits.tree(file);
    const pid = (await this.links.chain(frameId, x, y)).find((p) => tree.nodes.get(p)?.kind === "el");
    return pid === undefined ? "not text" : this.begin(frameId, file, pid);
  }

  /** Starts editing the text of an element. */
  async begin(frameId: string, file: string, pid: string): Promise<Begin> {
    if (this.session) return "busy";
    const content = contentOf(this.edits.map, file, this.edits.tree(file), pid);
    if (!content) return "not text";
    const session: Session = { frameId, file, pid, content, before: unitsOfDelta(deltaOf(content)), changes: [], stop: () => undefined };
    if (content.text) {
      const text = content.text;
      const watch = (e: Y.YTextEvent) => {
        session.changes.push(e.delta as DeltaStep[]);
      };
      text.observe(watch);
      session.stop = () => {
        text.unobserve(watch);
      };
    }
    this.session = session;
    let started = false;
    try {
      started = await this.links.edit(frameId, pid, htmlOfContent(content));
    } catch (err) {
      if (!isFrameGone(err)) console.error("start editing text in a frame", err);
    }
    if (!started && this.session === session) {
      session.stop();
      this.session = null;
    }
    return started ? "started" : "gone";
  }

  private finish(frameId: string, pid: string, said: unknown): void {
    const session = this.session;
    if (session?.frameId !== frameId || session.pid !== pid) return;
    this.session = null;
    session.stop();
    if (said !== null) {
      const runs = parseRuns(said);
      if (runs) this.commit(session, unitsOfRuns(runs, session.before));
      else console.error("a frame said something that is not the text it was editing", said);
    }
    this.links.release(frameId);
    for (const fn of this.ended) fn(frameId, pid);
  }

  private commit(session: Session, after: Unit[]): void {
    const { file, pid } = session;
    const now = contentOf(this.edits.map, file, this.edits.tree(file), pid);
    if (!now) return; // the element is gone: someone deleted it as this person typed
    // The difference is against what the person began from, found again in the
    // text as it is now; when the text itself was replaced there is nothing to
    // find it in, and what is there now is what the difference is against.
    const same = now.kind === session.content.kind && now.text === session.content.text;
    const before = same ? session.before : unitsOfDelta(deltaOf(now));
    const changes = same ? session.changes : [];
    const diff = diffUnits(before, after);
    if (diff.start === diff.end && diff.insert.length === 0 && diff.formats.length === 0) return;
    const map = (index: number, later: boolean) => throughChanges(index, changes, later);
    this.edits.transact((room) => {
      if (now.kind === "rich") { writeDiff(now.text, diff, map); return; }
      if (!isFormatted(after)) {
        if (now.text) { writeDiff(now.text, diff, map); return; }
        // An empty element gets its first text node.
        const text = new Y.Text();
        const node = freshId("~t");
        room.set(nodeKey(file, node, "kind"), "text");
        room.set(nodeKey(file, node, "parent"), pid);
        room.set(nodeKey(file, node, "order"), "V");
        room.set(nodeKey(file, node, "text"), text);
        writeDiff(text, diff, map); return;
      }
      // Text with formats is one formatted text in place of its text node, written in the same step.
      const rich = new Y.Text();
      room.set(nodeKey(file, pid, "rich"), rich);
      if (now.text) rich.insert(0, now.text.toJSON(), {});
      writeDiff(rich, diff, map);
      if (now.node) for (const key of [...room.keys()]) if (key.startsWith(nodeKey(file, now.node, ""))) room.delete(key);
    });
  }

  destroy(): void {
    this.offEdited();
    this.session?.stop();
    this.session = null;
    this.ended.clear();
  }
}
