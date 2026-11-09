// Images: a picture is a block that shows a file of the doc (an upload of the
// artifact, so it survives versions the way files do), a link on the web, or a
// small inline picture. People add one by the file picker, paste or drop; each
// becomes an upload first, and only the address it was given goes in the doc.
import type { Node } from "prosemirror-model";
import { NodeSelection, Plugin, TextSelection, type Command, type EditorState } from "prosemirror-state";
import type { EditorView, NodeView } from "prosemirror-view";
import { schema } from "./schema";

/** The widths a picture can be shown at, in the order of the toolbar (docs/doc-format.md). */
export const imageSizes = { small: 224, medium: 336, large: 504, xlarge: 672 } as const;
export type ImageSize = keyof typeof imageSizes;

export const sizeOf = (value: unknown): ImageSize | null => (typeof value === "string" && value in imageSizes ? (value as ImageSize) : null);

/** The selected picture: where it is, and what the tools show and change about it. */
export interface ImageInfo {
  pos: number;
  alt: string;
  size: ImageSize | null;
}

export function imageInfo(state: EditorState): ImageInfo | null {
  const { selection } = state;
  if (!(selection instanceof NodeSelection) || selection.node.type.name !== "image") return null;
  const alt: unknown = selection.node.attrs.alt;
  return { pos: selection.from, alt: typeof alt === "string" ? alt : "", size: sizeOf(selection.node.attrs.size) };
}

const imageNode = () => {
  const t = schema.nodes.image;
  if (t === undefined) throw new Error("the editor's schema has no node image");
  return t;
};

/** Puts the picture after the block the caret is in (in an empty paragraph's place) and selects it, so its tools show. */
export const insertImage = (src: string, alt = ""): Command => (state, dispatch) => {
  const { $from } = state.selection;
  const container = $from.depth > 0 ? $from.node($from.depth - 1) : null;
  if (container === null || !$from.parent.isTextblock) return false;
  const index = $from.index($from.depth - 1);
  if (!container.canReplaceWith(index, index + 1, imageNode())) return false;
  if (dispatch) {
    const empty = $from.parent.type.name === "paragraph" && $from.parent.content.size === 0;
    const from = empty ? $from.before() : $from.after();
    const last = index === container.childCount - 1;
    const made = imageNode().create({ src, alt: alt === "" ? null : alt });
    const tr = state.tr.replaceWith(from, empty ? $from.after() : from, last ? [made, schema.nodes.paragraph?.create() ?? made] : made);
    dispatch(tr.setSelection(NodeSelection.create(tr.doc, from)).scrollIntoView());
  }
  return true;
};

/** Changes the alt text or the size of the selected picture; null takes a size off. */
export const setImage = (changes: { alt?: string; size?: ImageSize | null }): Command => (state, dispatch) => {
  const info = imageInfo(state);
  const node = info === null ? null : state.doc.nodeAt(info.pos);
  if (info === null || node === null) return false;
  if (dispatch) {
    const had = node.attrs as { alt: string | null; size: string | null };
    const alt = changes.alt === undefined ? had.alt : changes.alt === "" ? null : changes.alt;
    const size = changes.size === undefined ? had.size : changes.size;
    const tr = state.tr.setNodeMarkup(info.pos, undefined, { ...node.attrs, alt, size });
    dispatch(tr.setSelection(NodeSelection.create(tr.doc, info.pos)));
  }
  return true;
};

/** Deletes the selected picture; Undo brings it back, because the file stays with the doc. */
export const deleteImage: Command = (state, dispatch) => {
  const info = imageInfo(state);
  if (info === null) return false;
  dispatch?.(state.tr.deleteSelection().scrollIntoView());
  return true;
};

/** The pictures among a clipboard's or a drop's files. */
export const imageFilesOf = (files: FileList | null | undefined): File[] => [...(files ?? [])].filter((f) => f.type.startsWith("image/"));

/** Where a picture goes when a file is dropped: at the block boundary nearest the pointer, as the caret there. */
export function placeCaretAt(view: EditorView, event: { clientX: number; clientY: number }): void {
  const at = view.posAtCoords({ left: event.clientX, top: event.clientY });
  if (at === null) return;
  const $pos = view.state.doc.resolve(at.pos);
  if ($pos.parent.isTextblock) view.dispatch(view.state.tr.setSelection(TextSelection.near($pos)));
}

/**
 * Pasting a picture (a screenshot: files and no text) and dropping files hand the pictures to `onFiles`. A paste that has
 * text too (a table copied from a spreadsheet comes with a picture of itself) goes through the paste rules instead.
 */
export const imageInput = (onFiles: (files: File[]) => void, where: (view: EditorView, event: { clientX: number; clientY: number }) => void = placeCaretAt): Plugin =>
  new Plugin({
    props: {
      handlePaste: (_view, event) => {
        const data = event.clipboardData;
        const files = imageFilesOf(data?.files);
        if (data === null || files.length === 0 || data.getData("text/html") !== "" || data.getData("text/plain") !== "") return false;
        event.preventDefault();
        onFiles(files);
        return true;
      },
      handleDrop: (view, event) => {
        const files = imageFilesOf(event.dataTransfer?.files);
        if (files.length === 0) return false;
        event.preventDefault();
        where(view, event);
        onFiles(files);
        return true;
      },
    },
  });

/** What adding pictures needs from the page around the editor. */
export interface Adding {
  /** Stores the file with the doc and gives its address. A refusal throws an error that says why in its `reason`. */
  upload: (file: File) => Promise<string>;
  /** Puts a picture with this address in the doc. */
  insert: (src: string) => void;
  /** What the person is told: null when nothing went wrong, otherwise the server's reason ("" when it gave none). */
  refused: (reason: string | null) => void;
}

/** Uploads the files one after the other and puts each in the doc; a refusal is told with the server's words and the others still go in. */
export async function addPictures(files: File[], adding: Adding): Promise<void> {
  adding.refused(null);
  for (const file of files) {
    try {
      adding.insert(await adding.upload(file));
    } catch (e) {
      const reason = (e as { reason?: unknown }).reason;
      adding.refused(typeof reason === "string" ? reason : "");
    }
  }
}

/** What a picture says when its file is not there. */
export const missingLabel = { deleted: "Picture deleted", unavailable: "Picture not available" } as const;

/**
 * The picture's node view: the same `<img>` the schema draws, and when the file cannot be loaded (an older version whose
 * upload was deleted, an address that is gone) a placeholder instead of a broken picture. `say` gives the words.
 */
export class ImageView implements NodeView {
  readonly dom: HTMLElement;
  private node: Node;
  private failed = "";

  constructor(
    node: Node,
    private readonly say: (kind: keyof typeof missingLabel) => string,
  ) {
    this.node = node;
    this.dom = document.createElement("div");
    this.dom.className = "ed-image";
    this.dom.contentEditable = "false";
    this.draw();
  }

  private draw(): void {
    const src = schema.nodes.image?.spec.toDOM?.(this.node);
    this.dom.replaceChildren();
    const attrs = this.node.attrs as { src: unknown; alt: unknown };
    const wanted = attrs.src;
    const placeholder = (kind: keyof typeof missingLabel) => {
      const span = document.createElement("span");
      span.className = "ed-missing";
      const alt = typeof attrs.alt === "string" ? attrs.alt : "";
      span.textContent = alt === "" ? this.say(kind) : `${this.say(kind)}: ${alt}`;
      this.dom.append(span);
    };
    if (Array.isArray(src) && src[0] === "img" && this.failed !== wanted) {
      const img = document.createElement("img");
      const attrs = src[1] as Record<string, string>;
      for (const [k, v] of Object.entries(attrs)) img.setAttribute(k, v);
      img.addEventListener("error", () => {
        this.failed = typeof wanted === "string" ? wanted : "";
        this.draw();
      });
      this.dom.append(img);
    } else {
      placeholder(typeof wanted === "string" && wanted.startsWith("/files/") ? "deleted" : "unavailable");
    }
  }

  update(node: Node): boolean {
    if (node.type !== this.node.type) return false;
    this.node = node;
    this.draw();
    return true;
  }

  selectNode(): void {
    this.dom.classList.add("ed-selected");
  }

  deselectNode(): void {
    this.dom.classList.remove("ed-selected");
  }

  stopEvent(): boolean {
    return false;
  }
}
