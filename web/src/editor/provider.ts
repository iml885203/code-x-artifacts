// The browser's end of the live socket (docs/realtime.md): the y-websocket framing
// the server speaks, a fresh one-use ticket for every connection, and what to do
// for each close code the server gives. The server answers every sync step 1 with
// a step 2 only after it has stored what came before it, which is how "saved" is
// known without a message of its own.
import * as decoding from "lib0/decoding";
import * as encoding from "lib0/encoding";
import * as awarenessProtocol from "y-protocols/awareness";
import * as syncProtocol from "y-protocols/sync";
import * as Y from "yjs";
import { ApiError, SignedOutError, type LiveTicket } from "../api";

const messageSync = 0;
const messageAwareness = 1;
const messageQueryAwareness = 3;

export type Connection = "connecting" | "live" | "reconnecting" | "offline";

/** The part of a WebSocket the provider uses; tests give it a fake. */
export interface SocketLike {
  binaryType: string;
  onopen: (() => void) | null;
  onmessage: ((e: { data: unknown }) => void) | null;
  onclose: ((e: { code: number; reason: string }) => void) | null;
  send(data: Uint8Array): void;
  close(code?: number): void;
}

/** A browser WebSocket, as the provider sees it. */
export function openWebSocket(url: string): SocketLike {
  const ws = new WebSocket(url);
  const socket: SocketLike = {
    get binaryType() {
      return ws.binaryType;
    },
    set binaryType(v: string) {
      ws.binaryType = v as BinaryType;
    },
    onopen: null,
    onmessage: null,
    onclose: null,
    send: (data) => {
      ws.send(new Uint8Array(data));
    },
    close: (code) => {
      ws.close(code);
    },
  };
  ws.addEventListener("open", () => socket.onopen?.());
  ws.addEventListener("message", (e: MessageEvent) => socket.onmessage?.({ data: e.data }));
  ws.addEventListener("close", (e) => socket.onclose?.({ code: e.code, reason: e.reason }));
  return socket;
}

export interface ProviderOptions {
  /** Asks for a ticket; every connection, including every reconnection, uses a new one. */
  ticket: () => Promise<LiveTicket>;
  open: (url: string) => SocketLike;
  /** What others see in the roster: the server replaces the name and id with the authenticated ones, and keeps the colour. */
  user: { id: string; name: string; color: string };
  /** Readers receive and never send: the server would drop it, and nothing is waiting to be saved. */
  readOnly: boolean;
  /** The server closed the connection because who may see or edit the artifact changed. */
  onAccessChanged?: () => void;
  /** The ticket was refused: the person cannot open the artifact any more, or is signed out. */
  onDenied?: () => void;
  /** Where the browser says it is online or not; window in the page. */
  network?: EventTarget & { readonly onLine?: boolean };
}

const firstRetryMs = 500;
const longestRetryMs = 15_000;

const frame = (build: (e: encoding.Encoder) => void): Uint8Array => {
  const e = encoding.createEncoder();
  build(e);
  return encoding.toUint8Array(e);
};

export class LiveProvider {
  doc!: Y.Doc;
  awareness!: awarenessProtocol.Awareness;
  state: Connection = "connecting";
  /** Everything this browser changed has been stored by the server. */
  saved = true;
  /**
   * The room's state has arrived in this document. A view must not be made on the document before: the editor
   * would write an empty page into it, which then merges with the room's as a second copy.
   */
  synced = false;

  private readonly options: ProviderOptions;
  private readonly changed = new Set<() => void>();
  private readonly reset = new Set<() => void>();
  private socket: SocketLike | null = null;
  private attempt = 0;
  private retries = 0;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private epoch = "";
  private stopped = false;
  private everLive = false;
  /** Step 1s sent whose step 2 has not come back. */
  private probes = 0;
  /** The room sends its whole state unprompted when someone joins: not the answer to anything we sent. */
  private unprompted = 0;
  private dirty = false;

  constructor(options: ProviderOptions) {
    this.options = options;
    this.fresh();
    const net = options.network;
    net?.addEventListener("offline", this.goOffline);
    net?.addEventListener("online", this.goOnline);
  }

  /** Opens the first connection. */
  start(): void {
    if (this.options.network?.onLine === false) this.setState("offline");
    else void this.connect();
  }

  /** Called when state or saved changes. Returns the unsubscribe. */
  onChange(fn: () => void): () => void {
    this.changed.add(fn);
    return () => this.changed.delete(fn);
  }

  /** Called when the document was thrown away and replaced (the room was rebuilt or a limit was hit): rebuild whatever shows it. */
  onReset(fn: () => void): () => void {
    this.reset.add(fn);
    return () => this.reset.delete(fn);
  }

  destroy(): void {
    this.stopped = true;
    this.options.network?.removeEventListener("offline", this.goOffline);
    this.options.network?.removeEventListener("online", this.goOnline);
    if (this.timer !== null) clearTimeout(this.timer);
    this.detach();
    this.socket = null;
    this.awareness.destroy();
    this.doc.destroy();
  }

  private fresh(): void {
    this.doc = new Y.Doc();
    this.awareness = new awarenessProtocol.Awareness(this.doc);
    this.awareness.setLocalStateField("user", this.options.user);
    this.doc.on("update", this.localUpdate);
    this.awareness.on("update", this.localAwareness);
    this.epoch = "";
    this.dirty = false;
  }

  private setState(state: Connection): void {
    if (this.state === state) return;
    this.state = state;
    this.notify();
  }

  private setSaved(saved: boolean): void {
    if (this.saved === saved) return;
    this.saved = saved;
    this.notify();
  }

  private notify(): void {
    for (const fn of this.changed) fn();
  }

  private async connect(): Promise<void> {
    if (this.stopped) return;
    const mine = ++this.attempt;
    this.setState(this.everLive || this.retries > 0 ? "reconnecting" : "connecting");
    let ticket: LiveTicket;
    try {
      ticket = await this.options.ticket();
    } catch (err) {
      if (this.superseded(mine)) return;
      if (err instanceof SignedOutError || (err instanceof ApiError && (err.status === 403 || err.status === 404))) {
        this.setState("offline");
        this.options.onDenied?.();
        return;
      }
      this.retry();
      return;
    }
    if (this.superseded(mine)) return;
    const url = new URL(ticket.path, location.href);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    url.searchParams.set("ticket", ticket.ticket);
    if (this.epoch !== "") url.searchParams.set("epoch", this.epoch);
    const socket = this.options.open(url.toString());
    socket.binaryType = "arraybuffer";
    this.socket = socket;
    const doc = this.doc;
    socket.onopen = () => {
      if (this.socket !== socket) return;
      this.probes = 0;
      this.unprompted = 1;
      this.send(frame((e) => {
        encoding.writeVarUint(e, messageSync);
        syncProtocol.writeSyncStep1(e, doc);
      }));
      this.probes++;
      this.send(frame((e) => {
        encoding.writeVarUint(e, messageAwareness);
        encoding.writeVarUint8Array(e, awarenessProtocol.encodeAwarenessUpdate(this.awareness, [this.doc.clientID]));
      }));
      this.epoch = ticket.epoch;
    };
    socket.onmessage = (e) => {
      if (this.socket === socket && e.data instanceof ArrayBuffer) this.receive(new Uint8Array(e.data));
    };
    socket.onclose = (e) => {
      if (this.socket !== socket) return;
      this.closed(e.code);
    };
  }

  /** Whether the connection being made was given up on while waiting: the provider was destroyed or another attempt began. */
  private superseded(attempt: number): boolean {
    return this.stopped || attempt !== this.attempt;
  }

  private send(message: Uint8Array): void {
    this.socket?.send(message);
  }

  private receive(message: Uint8Array): void {
    const decoder = decoding.createDecoder(message);
    switch (decoding.readVarUint(decoder)) {
      case messageSync: {
        const reply = encoding.createEncoder();
        encoding.writeVarUint(reply, messageSync);
        const kind = syncProtocol.readSyncMessage(decoder, reply, this.doc, this);
        if (encoding.length(reply) > 1) {
          this.send(encoding.toUint8Array(reply));
          // What we sent in answer to the room's step 1 is stored when this comes back.
          if (kind === syncProtocol.messageYjsSyncStep1) this.probe();
        }
        if (kind === syncProtocol.messageYjsSyncStep2) this.answered();
        break;
      }
      case messageAwareness:
        awarenessProtocol.applyAwarenessUpdate(this.awareness, decoding.readVarUint8Array(decoder), this);
        this.notify();
        break;
      case messageQueryAwareness:
        this.send(frame((e) => {
          encoding.writeVarUint(e, messageAwareness);
          encoding.writeVarUint8Array(e, awarenessProtocol.encodeAwarenessUpdate(this.awareness, [this.doc.clientID]));
        }));
        break;
    }
  }

  private probe(): void {
    this.send(frame((e) => {
      encoding.writeVarUint(e, messageSync);
      syncProtocol.writeSyncStep1(e, this.doc);
    }));
    this.probes++;
  }

  private answered(): void {
    if (this.unprompted > 0) {
      this.unprompted--;
      return;
    }
    if (this.probes > 0) this.probes--;
    if (!this.synced) {
      this.synced = true;
      this.notify();
    }
    if (!this.everLive || this.state !== "live") {
      this.everLive = true;
      this.retries = 0;
      this.setState("live");
    }
    if (this.probes === 0) {
      this.dirty = false;
      this.setSaved(true);
    }
  }

  private readonly localUpdate = (update: Uint8Array, origin: unknown): void => {
    if (origin === this || this.options.readOnly) return;
    this.dirty = true;
    this.setSaved(false);
    if (this.state !== "live" || this.socket === null) return;
    this.send(frame((e) => {
      encoding.writeVarUint(e, messageSync);
      syncProtocol.writeUpdate(e, update);
    }));
    this.probe();
  };

  private readonly localAwareness = (change: { added: number[]; updated: number[]; removed: number[] }, origin: unknown): void => {
    if (origin === this || this.socket === null || this.state !== "live") return;
    const clients = change.added.concat(change.updated, change.removed).filter((id) => id === this.doc.clientID);
    if (clients.length === 0) return;
    this.send(frame((e) => {
      encoding.writeVarUint(e, messageAwareness);
      encoding.writeVarUint8Array(e, awarenessProtocol.encodeAwarenessUpdate(this.awareness, clients));
    }));
  };

  /** Whoever is gone from the connection is gone from the roster. */
  private forgetOthers(): void {
    const others = Array.from(this.awareness.getStates().keys()).filter((id) => id !== this.doc.clientID);
    awarenessProtocol.removeAwarenessStates(this.awareness, others, this);
  }

  private detach(): void {
    const socket = this.socket;
    this.socket = null;
    if (socket) {
      socket.onopen = null;
      socket.onmessage = null;
      socket.onclose = null;
      socket.close();
    }
  }

  private closed(code: number): void {
    this.socket = null;
    this.forgetOthers();
    this.probes = 0;
    if (this.dirty) this.setSaved(false);
    if (this.options.network?.onLine === false) {
      this.setState("offline");
      return;
    }
    // docs/realtime.md: what the client does for each code.
    switch (code) {
      case 4001:
      case 4002:
        this.discard();
        void this.connect();
        return;
      case 4403:
        this.options.onAccessChanged?.();
        void this.connect();
        return;
      default:
        this.retry();
    }
  }

  /** The room has a history this browser's copy is not part of: start from the room's. */
  private discard(): void {
    this.doc.off("update", this.localUpdate);
    this.awareness.off("update", this.localAwareness);
    const old = this.doc;
    const oldAwareness = this.awareness;
    this.fresh();
    this.dirty = false;
    this.synced = false;
    this.setSaved(true);
    for (const fn of this.reset) fn();
    oldAwareness.destroy();
    old.destroy();
  }

  private retry(): void {
    this.setState("reconnecting");
    const wait = Math.min(firstRetryMs * 2 ** this.retries, longestRetryMs);
    this.retries++;
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = setTimeout(() => {
      this.timer = null;
      void this.connect();
    }, wait);
  }

  private readonly goOffline = (): void => {
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = null;
    this.attempt++;
    this.detach();
    this.forgetOthers();
    this.probes = 0;
    this.setState("offline");
  };

  private readonly goOnline = (): void => {
    if (this.stopped || this.state !== "offline") return;
    this.retries = 0;
    void this.connect();
  };
}
