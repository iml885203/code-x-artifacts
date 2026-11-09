// What an editor feature is. Each feature (text formatting, lists, links,
// tables, pictures...) is one module in features/ that exports a `Feature`: its
// toolbar tools, the rows of tools it shows while it is relevant, its keys and
// the plugins and node views it needs. features.ts is the one list DocEditor
// assembles the editor from, so a new feature is a new module and a line there.
import type { ReactNode } from "react";
import type { Command, EditorState, Plugin } from "prosemirror-state";
import type { NodeViewConstructor } from "prosemirror-view";
import type { Person, PeopleFound } from "../api";
import type { Key, Translate } from "../i18n";
import type { Marking } from "./comments/marks";
import type { DropdownHooks } from "./features/dropdown";
import type { missingLabel } from "./image";

/** What mentioning a person needs from the page around the editor. */
export interface MentionHooks {
  /** The people whose account or name contains the text; the first answer for an empty text is who to offer before anything is typed. */
  people: (query: string) => Promise<PeopleFound>;
  /** A person was mentioned: the page may tell the author something about them. */
  mentioned: (person: Person) => void;
  /** The words of the list. */
  words: () => { label: string; none: string; incomplete: string };
}

/** What the editor tells the page around it, and the words it needs from it. */
export interface Hooks extends DropdownHooks {
  /** The state changed: the toolbar shows what the caret is in. */
  onUpdate: (state: EditorState) => void;
  /** Cmd+K. */
  onLink: () => void;
  /** Pictures were pasted or dropped. */
  onImages: (files: File[]) => void;
  /** What a picture says when its file cannot be shown. */
  say: (kind: keyof typeof missingLabel) => string;
  /** The words that have a comment, for everyone who opens the doc, and what a click on them does. */
  comments: Marking;
  /** What an empty doc says in its first block. */
  hint: () => string;
  /** Opens block discovery without writing its query into the room. */
  onInsert?: () => void;
  /** Mentioning people. */
  mention: MentionHooks;
}

/** What a tool can ask of the editor it belongs to. */
export interface ToolContext {
  /** Runs a command on the editor's view and gives the focus back to it. */
  run: (command: Command) => void;
  /** Opens the popover that makes or changes the link at the caret. */
  openLink: () => void;
  /** Adds pictures chosen with the picture button. */
  addImages: (files: File[]) => void;
}

/**
 * The tool groups, in order: a tool says which one it belongs to. `insert` is the insert row's own tools (table, picture,
 * checklist); the others are the row's More menu, except the tools that say `bubble`: those show over a selection.
 */
export const groups = [
  { id: "insert", bare: true },
  { id: "block", bare: true },
  { id: "marks", label: "editor.marks" },
  { id: "lists", label: "editor.lists" },
  { id: "blocks", label: "editor.blocks" },
  { id: "history" },
] as const satisfies readonly { id: string; label?: Key; bare?: boolean }[];

export type GroupId = (typeof groups)[number]["id"];

export interface Tool {
  id: string;
  group: GroupId;
  /** Shows in the bubble over a selection, at this place in it, instead of in More. */
  bubble?: number;
  /** Draws the tool for the state of the editor: what the caret is in decides whether it is pressed or disabled. */
  render: (state: EditorState, ctx: ToolContext, t: Translate) => ReactNode;
}

/** A row of tools under the toolbar that shows only while it applies (the table's, the selected picture's). */
export interface Row {
  id: string;
  render: (state: EditorState, ctx: ToolContext, t: Translate) => ReactNode;
}

export interface Insertion {
  id: string;
  label: Key;
  command: Command;
  keywords?: string[];
  face?: ReactNode;
}

export interface Feature {
  insertions?: Insertion[];
  id: string;
  tools?: Tool[];
  rows?: Row[];
  /** Keys, tried before the base keymap; only for people who may edit. */
  keymap?: (hooks: Hooks) => Record<string, Command>;
  /** Plugins for people who may edit. */
  plugins?: (hooks: Hooks) => Plugin[];
  nodeViews?: (hooks: Hooks) => Record<string, NodeViewConstructor>;
}

interface ButtonSpec {
  id: string;
  group: GroupId;
  label: Key;
  face: ReactNode;
  bubble?: number;
  pressed?: (state: EditorState) => boolean | undefined;
  disabled?: (state: EditorState) => boolean;
  run: (ctx: ToolContext) => void;
}

/** The usual tool: an icon button that is pressed or disabled by what the caret is in. */
export function button(spec: ButtonSpec): Tool {
  return {
    id: spec.id,
    group: spec.group,
    ...(spec.bubble === undefined ? {} : { bubble: spec.bubble }),
    render: (state, ctx, t) => (
      <button
        type="button"
        className="quiet icon-button"
        aria-label={t(spec.label)}
        title={t(spec.label)}
        aria-pressed={spec.pressed?.(state)}
        disabled={spec.disabled?.(state) ?? false}
        onClick={() => {
          spec.run(ctx);
        }}
      >
        {spec.face}
      </button>
    ),
  };
}
