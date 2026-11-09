// Every action a person can pick for an artifact from a menu: the card's "⋯" in
// the overview, the viewer's "⋯" and the dropdown on the viewer's title. A new action is one entry in `artifactActions`;
// the menus render whatever is visible, so neither grows a prop per action.
// How an action runs lives in `ActionEnv`, which each surface fills in
// (artifactActions.ts holds the shared behaviour behind it).
import type { Artifact } from "./api";
import { format, type Key, type Translate } from "./i18n";

export type Surface = "card" | "viewer" | "title";
export type Panel = "history" | "files" | "comments";

/** Who is looking, and from where. */
export interface ActionViewer {
  signedIn: boolean;
  surface: Surface;
}

/** What an action can do; each surface supplies it. A card has no panels to open. */
export interface ActionEnv {
  copyLink: (a: Artifact) => void;
  copy: (a: Artifact) => void;
  /** Flips the pin of the artifact. */
  togglePin: (a: Artifact) => void;
  openPanel: (panel: Panel) => void;
  /** Asks whether to delete; the surface owns the dialog. */
  askDelete: () => void;
  /** Starts renaming where the title is; only a surface that renames in place has it. */
  rename?: () => void;
}

export interface ArtifactAction {
  id: string;
  /** The menus that list it, each with its place there (lower first). */
  at: Partial<Record<Surface, number>>;
  label: (a: Artifact, t: Translate) => string;
  visible: (a: Artifact, viewer: ActionViewer) => boolean;
  /** A link that opens in a new tab, instead of `run`. */
  href?: (a: Artifact) => string;
  run?: (a: Artifact, env: ActionEnv) => void;
  /** The letter that runs it while a menu that shows letters is open (uppercase). */
  key?: string;
  /** Listed after everything else, set apart. */
  danger?: boolean;
  /** On a phone the bar has no room for it, so the viewer's menu carries it; CSS hides it where the bar has it. */
  phoneOnly?: boolean;
}

/** Where an action goes in a menu is its `at`, not its place in this list. */
export const artifactActions: ArtifactAction[] = [
  {
    id: "comments",
    at: { viewer: 0 },
    label: (a, t) => (a.comments.unread > 0 ? format(t("comments.openWithNew"), { count: String(a.comments.unread) }) : t("comments.title")),
    visible: (a) => a.canComment,
    run: (_, env) => {
      env.openPanel("comments");
    },
    phoneOnly: true,
  },
  {
    id: "versions",
    at: { viewer: 1, title: 0 },
    label: (_, t) => t("viewer.versions"),
    visible: (a) => a.canEdit,
    run: (_, env) => {
      env.openPanel("history");
    },
  },
  {
    id: "files",
    at: { viewer: 2 },
    label: (_, t) => t("viewer.files"),
    visible: (a) => a.canEdit,
    run: (_, env) => {
      env.openPanel("files");
    },
  },
  {
    id: "pin",
    at: { viewer: 4, card: 0 },
    key: "P",
    label: (a, t) => (a.pinned ? t("pin.remove") : t("pin.add")),
    // A pin is a person's own.
    visible: (_, v) => v.signedIn,
    run: (a, env) => {
      env.togglePin(a);
    },
  },
  {
    id: "rename",
    at: { card: 1 },
    key: "R",
    label: (_, t) => t("create.rename"),
    // The same rule as the viewer's title: whoever may edit it may rename it. Delete stays the author's.
    visible: (a, v) => v.signedIn && a.canEdit,
    run: (_, env) => {
      env.rename?.();
    },
  },
  {
    id: "copyLink",
    at: { card: 3, title: 1 },
    key: "C",
    label: (_, t) => t("viewer.copyLink"),
    visible: () => true,
    run: (a, env) => {
      env.copyLink(a);
    },
  },
  {
    id: "copy",
    at: { viewer: 5, card: 2 },
    label: (_, t) => t("viewer.copy"),
    // A copy needs someone to be its author.
    visible: (_, v) => v.signedIn,
    run: (a, env) => {
      env.copy(a);
    },
  },
  {
    id: "openRaw",
    at: { viewer: 7 },
    label: (_, t) => t("viewer.openRaw"),
    visible: () => true,
    href: (a) => `/a/${a.id}?view=full`,
  },
  {
    id: "delete",
    at: { viewer: 8, card: 4, title: 2 },
    label: (_, t) => t("viewer.delete"),
    visible: (a) => a.canChange,
    run: (_, env) => {
      env.askDelete();
    },
    danger: true,
  },
];

/** The actions on offer to this viewer for this artifact, in menu order. */
export function visibleActions(a: Artifact, viewer: ActionViewer): ArtifactAction[] {
  return artifactActions
    .filter((action) => action.at[viewer.surface] !== undefined && action.visible(a, viewer))
    .sort((x, y) => (x.at[viewer.surface] ?? 0) - (y.at[viewer.surface] ?? 0));
}

/** The action a letter runs in this menu, or undefined. */
export function actionForKey(a: Artifact, viewer: ActionViewer, key: string): ArtifactAction | undefined {
  const k = key.toUpperCase();
  return visibleActions(a, viewer).find((action) => action.key === k && action.run !== undefined);
}

/** Whether the menu has an action to set apart from the others. */
export function hasDangerousAction(a: Artifact, viewer: ActionViewer): boolean {
  return visibleActions(a, viewer).some((action) => action.danger === true);
}

/** The formats the menus' Export group knows by name: each one's label is in the catalogue (export.json). */
const exportLabels: Record<string, Key> = { md: "export.md", docx: "export.docx", pptx: "export.pptx", html: "export.html", pdf: "export.pdf", zip: "export.zip", svg: "export.svg", png: "export.png" };

/** A format's name in the Export group; one the catalogue has no entry for shows as its file type. */
export function exportLabel(t: Translate, f: string): string {
  const key = exportLabels[f];
  return key === undefined ? `.${f}` : t(key);
}
