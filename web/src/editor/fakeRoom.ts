// A stand-in for the server's room (internal/realtime), for tests of the browser's
// side: it speaks the same frames in the same order (its step 1, the whole state
// unprompted, awareness, then an answer to each message) and stores what it is sent.
import * as decoding from "lib0/decoding";
import * as encoding from "lib0/encoding";
import * as awarenessProtocol from "y-protocols/awareness";
import * as syncProtocol from "y-protocols/sync";
import * as Y from "yjs";
import type { LiveTicket } from "../api";
import type { SocketLike } from "./provider";

class FakeSocket implements SocketLike {
  binaryType = "blob";
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: unknown }) => void) | null = null;
  onclose: ((e: { code: number; reason: string }) => void) | null = null;
  closed = false;
  constructor(
    readonly url: string,
    private readonly room: FakeRoom,
  ) {
    queueMicrotask(() => {
      if (this.room.refuse !== null) {
        this.end(this.room.refuse, "");
        return;
      }
      this.onopen?.();
      this.room.join(this);
    });
  }
  send(data: Uint8Array): void {
    if (!this.closed) this.room.receive(this, data);
  }
  close(): void {
    this.closed = true;
  }
  /** The server's end of the connection. */
  deliver(frame: Uint8Array): void {
    queueMicrotask(() => {
      if (!this.closed) this.onmessage?.({ data: frame.buffer.slice(frame.byteOffset, frame.byteOffset + frame.byteLength) });
    });
  }
  end(code: number, reason: string): void {
    queueMicrotask(() => {
      if (this.closed) return;
      this.closed = true;
      this.onclose?.({ code, reason });
    });
  }
}

export class FakeRoom {
  readonly doc = new Y.Doc();
  readonly awareness = new awarenessProtocol.Awareness(this.doc);
  readonly sockets: FakeSocket[] = [];
  /** Every URL a connection was opened with, in order. */
  readonly urls: string[] = [];
  /** Updates the room stored: what "saved" means. */
  stored = 0;
  epoch = "e1";
  tickets = 0;
  /** When set, connections are closed at once with this code. */
  refuse: number | null = null;
  readOnly = false;
  /** When set, what a connection sends is lost: nothing is stored and nothing is answered. */
  silent = false;

  constructor() {
    this.awareness.setLocalState(null);
    this.doc.on("update", (update: Uint8Array, origin: unknown) => {
      this.stored++;
      const out = encoding.createEncoder();
      encoding.writeVarUint(out, 0);
      syncProtocol.writeUpdate(out, update);
      for (const s of this.live) if (s !== origin) s.deliver(encoding.toUint8Array(out));
    });
  }

  readonly ticket = (): Promise<LiveTicket> => {
    this.tickets++;
    return Promise.resolve({ ticket: `t${String(this.tickets)}`, epoch: this.epoch, path: "/ws/room" });
  };

  readonly open = (url: string): SocketLike => {
    this.urls.push(url);
    const socket = new FakeSocket(url, this);
    this.sockets.push(socket);
    return socket;
  };

  get live(): FakeSocket[] {
    return this.sockets.filter((s) => !s.closed);
  }

  join(socket: FakeSocket): void {
    const step1 = encoding.createEncoder();
    encoding.writeVarUint(step1, 0);
    syncProtocol.writeSyncStep1(step1, this.doc);
    socket.deliver(encoding.toUint8Array(step1));
    const whole = encoding.createEncoder();
    encoding.writeVarUint(whole, 0);
    syncProtocol.writeSyncStep2(whole, this.doc);
    socket.deliver(encoding.toUint8Array(whole));
    const people = encoding.createEncoder();
    encoding.writeVarUint(people, 1);
    encoding.writeVarUint8Array(people, awarenessProtocol.encodeAwarenessUpdate(this.awareness, Array.from(this.awareness.getStates().keys())));
    socket.deliver(encoding.toUint8Array(people));
  }

  receive(socket: FakeSocket, data: Uint8Array): void {
    if (this.silent) return;
    const decoder = decoding.createDecoder(data);
    const kind = decoding.readVarUint(decoder);
    if (kind === 1) {
      // The room relays awareness to everyone, the sender included.
      const payload = decoding.readVarUint8Array(decoder);
      awarenessProtocol.applyAwarenessUpdate(this.awareness, payload, socket);
      const out = encoding.createEncoder();
      encoding.writeVarUint(out, 1);
      encoding.writeVarUint8Array(out, payload);
      for (const s of this.live) s.deliver(encoding.toUint8Array(out));
      return;
    }
    if (kind !== 0) return;
    // A reader's writes are dropped, as the real room does; its step 1 is still answered.
    if (this.readOnly && decoding.peekVarUint(decoder) !== syncProtocol.messageYjsSyncStep1) return;
    const reply = encoding.createEncoder();
    encoding.writeVarUint(reply, 0);
    syncProtocol.readSyncMessage(decoder, reply, this.doc, socket);
    if (encoding.length(reply) > 1) socket.deliver(encoding.toUint8Array(reply));
  }

  /** The server ends every connection with a close code. */
  closeAll(code: number, reason = ""): void {
    for (const s of this.live) s.end(code, reason);
  }
}

/** Lets what was queued by the sockets and the provider run. */
export async function settle(): Promise<void> {
  for (let i = 0; i < 10; i++) await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}
