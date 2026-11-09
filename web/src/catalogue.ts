// The interface text is one JSON file per area in internal/i18n/locales/<tag>/
// (an area is a key namespace: nav.json holds the keys "nav.*"). Only the types
// of the English files are imported, as the set of valid keys: a key that is
// in none of them fails the type check, and the bundle carries none of the text.
// A new area is two lines here, its name in `areas` and its type in EnglishAreas
// (catalogue.test.ts fails until both are there and match the files). The tests
// load the text from this list, not from a glob: a glob's result is cached by
// Vite and went stale when a file was added.
export const areas = [
  "anchors",
  "card",
  "comments",
  "connect",
  "copy",
  "create",
  "deck",
  "design",
  "download",
  "editor",
  "editors",
  "error",
  "export",
  "files",
  "gallery",
  "kinds",
  "kit",
  "lang",
  "login",
  "mention",
  "meta",
  "nav",
  "ops",
  "pen",
  "pin",
  "presence",
  "props",
  "share",
  "shell",
  "slideedit",
  "structure",
  "tabs",
  "textedit",
  "time",
  "toolbar",
  "versions",
  "viewer",
  "visibility",
] as const;

export type Area = (typeof areas)[number];

interface EnglishAreas extends Record<Area, unknown> {
  anchors: typeof import("../../internal/i18n/locales/en/anchors.json");
  card: typeof import("../../internal/i18n/locales/en/card.json");
  comments: typeof import("../../internal/i18n/locales/en/comments.json");
  connect: typeof import("../../internal/i18n/locales/en/connect.json");
  copy: typeof import("../../internal/i18n/locales/en/copy.json");
  create: typeof import("../../internal/i18n/locales/en/create.json");
  deck: typeof import("../../internal/i18n/locales/en/deck.json");
  design: typeof import("../../internal/i18n/locales/en/design.json");
  download: typeof import("../../internal/i18n/locales/en/download.json");
  editor: typeof import("../../internal/i18n/locales/en/editor.json");
  editors: typeof import("../../internal/i18n/locales/en/editors.json");
  error: typeof import("../../internal/i18n/locales/en/error.json");
  export: typeof import("../../internal/i18n/locales/en/export.json");
  files: typeof import("../../internal/i18n/locales/en/files.json");
  gallery: typeof import("../../internal/i18n/locales/en/gallery.json");
  kinds: typeof import("../../internal/i18n/locales/en/kinds.json");
  kit: typeof import("../../internal/i18n/locales/en/kit.json");
  lang: typeof import("../../internal/i18n/locales/en/lang.json");
  login: typeof import("../../internal/i18n/locales/en/login.json");
  mention: typeof import("../../internal/i18n/locales/en/mention.json");
  meta: typeof import("../../internal/i18n/locales/en/meta.json");
  nav: typeof import("../../internal/i18n/locales/en/nav.json");
  ops: typeof import("../../internal/i18n/locales/en/ops.json");
  pen: typeof import("../../internal/i18n/locales/en/pen.json");
  pin: typeof import("../../internal/i18n/locales/en/pin.json");
  presence: typeof import("../../internal/i18n/locales/en/presence.json");
  props: typeof import("../../internal/i18n/locales/en/props.json");
  share: typeof import("../../internal/i18n/locales/en/share.json");
  shell: typeof import("../../internal/i18n/locales/en/shell.json");
  slideedit: typeof import("../../internal/i18n/locales/en/slideedit.json");
  tabs: typeof import("../../internal/i18n/locales/en/tabs.json");
  structure: typeof import("../../internal/i18n/locales/en/structure.json");
  textedit: typeof import("../../internal/i18n/locales/en/textedit.json");
  time: typeof import("../../internal/i18n/locales/en/time.json");
  toolbar: typeof import("../../internal/i18n/locales/en/toolbar.json");
  versions: typeof import("../../internal/i18n/locales/en/versions.json");
  viewer: typeof import("../../internal/i18n/locales/en/viewer.json");
  visibility: typeof import("../../internal/i18n/locales/en/visibility.json");
}

type KeysOf<T> = T extends unknown ? keyof T : never;

export type Key = KeysOf<EnglishAreas[keyof EnglishAreas]>;
