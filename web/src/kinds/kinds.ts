import { BarChart, Board, Brush, CodeIcon, FileText, PlayCircle, Screen, Shapes } from "../icons/kind";
import type { Key } from "../i18n";

/** The kinds people see: a web page (html and react alike), a document, slides, a design, a board; and three whose look is reserved. */
export type DisplayedKind = "web" | "doc" | "slides" | "design" | "board" | "dashboard" | "motion" | "design-system";

interface KindLook {
  icon: () => React.JSX.Element;
  label: Key;
  /** The name in the --kind-<token>-bg and -fg colour tokens of style.css, which hold both themes. */
  token: string;
  /** Not offered yet: only its colour and icon exist. */
  reserved: boolean;
}

/** The one table that knows how a kind looks; its colours are the --kind-* tokens (DESIGN.md). A new kind is one row here, one token pair, one label in kinds.json and one icon. */
export const kindsTable: Record<DisplayedKind, KindLook> = {
  web: { icon: CodeIcon, label: "kinds.web", token: "web", reserved: false },
  doc: { icon: FileText, label: "kinds.doc", token: "doc", reserved: false },
  slides: { icon: Screen, label: "kinds.slides", token: "slides", reserved: false },
  design: { icon: Brush, label: "kinds.design", token: "design", reserved: false },
  board: { icon: Board, label: "kinds.board", token: "board", reserved: false },
  dashboard: { icon: BarChart, label: "kinds.dashboard", token: "dashboard", reserved: true },
  motion: { icon: PlayCircle, label: "kinds.motion", token: "motion", reserved: true },
  "design-system": { icon: Shapes, label: "kinds.designSystem", token: "system", reserved: true },
};

export const displayedKinds: DisplayedKind[] = ["web", "doc", "slides", "design", "board"];
export const reservedKinds: DisplayedKind[] = ["dashboard", "motion", "design-system"];

const byKind: Record<string, DisplayedKind> = {
  html: "web",
  react: "web",
  doc: "doc",
  slides: "slides",
  design: "design",
  board: "board",
  dashboard: "dashboard",
  motion: "motion",
  "design-system": "design-system",
};

/** What an artifact's kind looks like; a kind this bundle does not know (a newer deployment) looks like a web page. */
export function displayOf(kind: string): DisplayedKind {
  return byKind[kind] ?? "web";
}
