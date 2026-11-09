// The slide routes of #42 (/api/artifacts/{id}/slides…), the same service calls
// as the get_slide, replace_slide, insert_slide, move_slide and delete_slide tools. Every edit
// carries the version the filmstrip shows as baseVersion, so the server refuses
// it (409) when a newer one was saved meanwhile.
import { call } from "../api";

export interface SlideEntry {
  number: number;
  title: string;
  /** The review's estimate: the slide is likely to overflow the canvas. */
  overflows?: boolean;
}

/** The slides of a deck at a version: what the filmstrip shows. An edit answers with the same. */
export interface SlideList {
  version: number;
  slides: SlideEntry[];
}

/** One slide as the deck stores it, and the list it is in. */
export interface SlideRead extends SlideList {
  number: number;
  slide: { title?: unknown; notes?: unknown } & Record<string, unknown>;
}

const slides = (id: string) => `/api/artifacts/${encodeURIComponent(id)}/slides`;

async function answer<T>(res: Promise<Response>): Promise<T> {
  return (await (await res).json()) as T;
}

export function listSlides(id: string): Promise<SlideList> {
  return answer(call("GET", slides(id)));
}

export function getSlide(id: string, n: number): Promise<SlideRead> {
  return answer(call("GET", `${slides(id)}/${String(n)}`));
}

/** Puts `slide` after slide `after` (0 for the start). */
export function insertSlide(id: string, after: number, slide: unknown, baseVersion: number): Promise<SlideList> {
  return answer(call("POST", slides(id), { after, slide, baseVersion }));
}

/** Puts `slide` in the place of slide `n`: the call replace_slide makes. */
export function replaceSlide(id: string, n: number, slide: unknown, baseVersion: number): Promise<SlideList> {
  return answer(call("PUT", `${slides(id)}/${String(n)}`, { slide, baseVersion }));
}

/** Moves slide `n` so that it becomes slide `to`. */
export function moveSlide(id: string, n: number, to: number, baseVersion: number): Promise<SlideList> {
  return answer(call("POST", `${slides(id)}/${String(n)}/move`, { to, baseVersion }));
}

export function deleteSlide(id: string, n: number, baseVersion: number): Promise<SlideList> {
  return answer(call("DELETE", `${slides(id)}/${String(n)}?baseVersion=${String(baseVersion)}`));
}
