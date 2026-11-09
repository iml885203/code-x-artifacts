# Authoring an html artifact

One HTML page, at most 16 MB. A small page is one self-contained file: put everything inline, link the kit below, or load
libraries from a public CDN (cdnjs, jsdelivr, unpkg). A bigger page keeps its scripts, styles and data in files that
come with it (see Files that come with the page). For images, fonts and PDFs use files uploaded to the artifact (see
Files: images, fonts, PDFs, CSV below), or small `data:` URIs or https URLs. Start with `<!doctype html>` and
`<meta charset="utf-8">`. Never put credentials, tokens or production customer data in a page, shared or not.

## The sandbox

The page runs in a sandboxed iframe with an opaque origin. That decides what works:

| Works | Does not |
|---|---|
| scripts, forms, `alert` / `confirm` | `localStorage`, `sessionStorage`, IndexedDB, `document.cookie`: they throw |
| links with `target="_blank"`, `window.open` | navigating the top window |
| `fetch` to public APIs that allow CORS | `fetch` to CodeXArtifacts's own API: use `window.pictor` |
| `pictor.download`: saves a file if the person agrees | a download the page starts itself |

State that only the current viewer needs lives in memory. State people share goes in `window.pictor.db`.

## window.pictor

Present on every page. Every call returns a promise.

```js
const me = await pictor.whoami();   // {username, displayName, email, groups, anonymous}
await pictor.db.set("votes", me.username, { choice: "A" });  // any JSON, at most 1 MB
await pictor.db.get("votes", me.username);                   // the data, or null
await pictor.db.list("votes");      // [{id, data, updatedBy, updatedAt, version}]
await pictor.db.delete("votes", me.username);

await pictor.db.query("tasks", { status: "open", score: { gte: 3 } }, { limit: 20 });  // only the matches, same shape as list
await pictor.db.update("tasks", "t1", { status: "done", note: null });  // merge fields; null removes one; returns the document
await pictor.db.batch([                                                 // all of them or none
  { op: "set", collection: "tasks", id: "t2", data: { status: "open" } },
  { op: "update", collection: "tasks", id: "t1", fields: { next: "t2" } },
  { op: "delete", collection: "tasks", id: "t0" },
]);
```

The data belongs to this page and is shared by everyone who can open it: every viewer can read and overwrite
every document, and `updatedBy` records who wrote last. For something per person, key the document by
`me.username`. `list` returns the whole collection, so keep collections to hundreds of documents.
A page that is public can be opened without signing in: then `whoami()` says `anonymous: true`, `db` is
read-only and `updatedBy` is empty.

Every document has a `version`: 1 when created, one more with each write. When two people may change the same
document (a claim, a counter, a sign-up slot), pass the version you read as `ifVersion` to `set`, `update`,
`delete` (`{ ifVersion: 3 }` as the last argument) or to an operation of `batch`. If the document has moved on the
write is refused with a message that says so, nothing changes, and you read again and retry; `ifVersion: 0` means "only
if it does not exist yet". A `batch` with any refused operation changes nothing at all. `get` returns just the data;
to get a document's version read it with `query("c", { id: "x" })` or `list`.

`query` filters are small: each key is a field (`"owner.name"` reaches inside a field; `"id"` is the document id) and
its value is a plain value to equal or an object of `in` (list), `gt` `gte` `lt` `lte` (numbers or strings), `prefix`
(string start) and `eq` (any JSON, for matching an object); everything given must hold, and a missing field matches nothing.
`update` needs an existing document that is an object. `query` works for a signed-out viewer of a public page; the writes do not.

Render something useful before the first `pictor` call resolves, and show an error in the page when a call
rejects instead of leaving it blank.

## Saving a file for the viewer

A page cannot start a download by itself (an `<a download>` click does nothing). To hand the viewer a file, call
`pictor.download`; the viewer shows the file name, type and size and the person chooses Save or Cancel, every time.

```js
const csv = "name,votes\nA,3\nB,5\n";
try {
  await pictor.download({ name: "votes.csv", type: "text/csv", data: csv });   // resolves {name, size} after the person saved it
} catch (e) { /* e.message says why: the person refused, or the limits below */ }
```

`data` is text (saved as UTF-8), an `ArrayBuffer`, a typed array or a `Blob`; the file saved is exactly those bytes. Limits, checked by
the viewer and not by the page: at most 10 MB; type one of `text/plain`, `text/csv`, `application/json`, `text/markdown`,
`application/pdf`, `image/png`, `image/jpeg`, `image/gif`, `image/webp`, `application/zip`; anything else is rejected before the
person is asked, and so is a second call made while the first is still waiting for an answer. The name loses path parts, control
characters and leading dots, is cut to 100 characters, and gets the extension of the type unless it already ends in one. Call it from a click
handler and show what happens when the person refuses. It works in CodeXArtifacts's viewer and its full-page view (Open in new tab, `/a/<id>?view=full`), not in the page's own address (`/raw/<id>`). Both views ask for consent through the same trusted parent; the page keeps its opaque-origin sandbox.

## The look: link the kit

Every page links CodeXArtifacts's kit in its `<head>`, so it matches CodeXArtifacts and follows the viewer's light or dark setting:

```html
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="stylesheet" href="/kit/v5/pictor.css">
```

The path is versioned and never changes, so the page keeps its look. With the kit linked, plain HTML is already
styled: headings, paragraphs, lists, links, `code`, `pre`, `table`, `details`, buttons and form controls need no class.
The kit gives tokens and defaults; the layout is yours. Use `<main class="pk-page">` for the page column (phone
gutters). `class="pk-prose"` is for running text only: it caps paragraphs, lists and headings inside it at a
readable line length. Grids, charts, diagrams, tables and `pre` go outside it, or use the full page width. Do not render or screenshot
the page to check it unless the person asks. What needs a class:

| Component | Markup |
|---|---|
| Callout | `<div class="pk-callout pk-callout--ok">` (also `--warn`, `--danger`; none = info), start with `<strong>Title</strong>` |
| Card | `<div class="pk-card">` or `<a class="pk-card">`; `<div class="pk-grid">` lays cards out |
| Table | wrap in `<div class="pk-table-wrap">`; numeric cells get `class="num"` and line up |
| Buttons | `<button>`; `class="pk-btn pk-btn--primary"` for the one main action, `--quiet`, `--danger` |
| Form row | `<div class="pk-field"><label>…</label><input>` with `<p class="pk-help">` or `<p class="pk-error">` |
| Facts | `class="pk-meta"` (monospace, muted) for times, ids, counts; `class="pk-tag"` for a label |
| Chart | colours only: `stroke="var(--chart-1)"` … `--chart-6`, grid `var(--chart-grid)`; `pk-legend`, `pk-bars` for simple CSS bars |

Colours are tokens, never hard-coded: `--night` (page), `--plate` (surfaces), `--line`, `--text`, `--muted`,
`--star` (the one accent), `--danger`, `--ok`, `--info`. Fonts are `--font-sans` and `--font-mono`; sizes are
`--fs-body`, `--fs-h1` and the rest of the `--fs-*` scale; spacing is `--s-1` to `--s-8`. The reference with
every component in both themes is the Kit page of this server.

## Page contract

- Light and dark: set no page background and no text colour of your own; use the tokens. A colour that is
  fine on white and unreadable on dark is a bug. Look at both (`prefers-color-scheme`) before publishing.
- Phone width (390 px): keep the viewport meta, use no fixed widths, let tables scroll inside `pk-table-wrap`,
  keep touch targets at least 44 px (the kit's buttons and inputs already are).
- Say it with real content: real titles, numbers and names, not lorem ipsum or "John Doe". Do not tell the
  reader what the page is, show it. One primary button per view.
- Charts: never colour alone; label the series or the axis.
- Leave out the tropes that make a page look machine-made: purple-blue gradients and gradient text, frosted
  glass, glow, emoji or symbols as icons, a centred hero with three icon cards, big-number metric tiles, a small
  label above every heading, cards inside cards, coloured stripes on one side of a card or callout, and Inter,
  Roboto or Arial.

## Printing and PDF

People save the page as PDF from the viewer (Export → PDF): the browser prints the page itself, with its own
print stylesheet and none of the viewer around it. There is no PDF on the server, so make the page print well:

- Add `@media print` rules: hide controls that do nothing on paper (buttons, inputs, tabs) and show every tab or
  collapsed section the reader needs.
- Let the content flow: no `height: 100vh` or `overflow: auto` boxes that cut it off at one screen; a fixed or
  sticky header prints on every page, so make it `position: static` in print.
- Keep a figure, a table row or a card on one page with `break-inside: avoid`, and start a new section with
  `break-before: page` where it helps.
- Set the paper margin with `@page { margin: … }` if the page has none of its own.

## Files: images, fonts, PDFs, CSV

Never inline a big picture as base64. Upload it to the artifact and reference the url the upload returns:
`upload_asset` (MCP) with the artifact `id`, a file `name` and the file as base64, or `PUT /api/artifacts/{id}/assets/{name}`
with the file as the body. The artifact has to exist first, so publish it, upload, then publish again with the url.

```html
<img src="https://pictor.example/files/AbC123/chart.png" alt="Revenue by month">
<style>@font-face { font-family: Brand; src: url(https://pictor.example/files/AbC123/brand.woff2); }</style>
<script>const rows = await (await fetch("https://pictor.example/files/AbC123/data.csv")).text();</script>
```

Write the url exactly as the upload returned it. It loads inside the sandbox for everyone who can open the page,
and `fetch` and fonts work on it. Accepted: PNG, JPEG, GIF, WebP, SVG, PDF, WOFF, WOFF2, TTF, OTF and UTF-8 CSV, at most
50 files and 100 MB per artifact. Files follow the artifact's audience and go when the artifact is deleted.

## Files that come with the page: scripts, styles, data

A page that has grown past one file can keep its parts apart. Publish it with `files`, an object from a relative path to
the file's complete text, and refer to each file by that same relative path:

```json
{ "content": "<!doctype html>… <link rel=\"stylesheet\" href=\"styles/site.css\"> … <script src=\"app.js\"></script>",
  "files": { "app.js": "…", "styles/site.css": "…", "data/rows.json": "[…]" } }
```

```html
<link rel="stylesheet" href="styles/site.css">
<script src="app.js"></script>
<script type="module">import { draw } from "./lib/chart.mjs"; const rows = await (await fetch("data/rows.json")).json();</script>
<a href="other.html">another page</a>
```

- The path is relative: `app.js`, `styles/site.css`, `data/rows.json`. Never start one with `/`: it would point at the site, not at your file. `<script src>`, `<link>`,
  `import`, `import()`, `fetch` and CSS `url()` all resolve it, and a page in a folder reaches its neighbours with `../`.
  `index.html` is the page itself: you send it as `content`, never as a file. A file ending in `.html` is another page of
  the same artifact, with the same sandbox and `window.pictor`.
- Files are text: `.html`, `.js`, `.mjs`, `.css`, `.json` (a `.json` file must parse). Names are letters, digits, dot, dash
  and underscore, at most 5 folders deep, and two names that differ only in case are one. Pictures, fonts and PDFs are
  uploaded with `upload_asset` as above; their url is absolute and works from any file.
- A file is part of the version it was published with. Each publish says what changes: a path with its text adds or
  replaces the file, a path with `null` removes it, and every path you do not mention is carried over. To change one file
  read it (`read` with `file`), then publish with just that path. A publish without `files` keeps all of them. `restore`
  brings back the files of that version along with its page, and an older version keeps showing its own.
- Limits: 16 MB a file, 255 files in one publish, 511 files and 256 MB in all in a version. A page without files is
  unchanged: nothing here applies.
- The page has no `localStorage`, `sessionStorage` or cookies, files or not: what must be saved goes in `pictor.db`.

## Choosing who can see it

A new page is `private`: only its author can open it. Say which audience you chose.

| The person says | visibility |
|---|---|
| nothing about sharing, "draft", "show me first" | `private` |
| "share with everyone", "the whole company" | `organisation` (everyone who can sign in) |
| "public", "anyone with the link", people without an account | `public`, only if `whoami` says public links are on |

## A large page that is already a file

Over about 50 KB, or a page you wrote to disk (a generated report, a build output): do not type it into `publish`. Use
`begin_publish` with `files` listing `content` (the page) and every file that comes with it, each with its sha256 and size,
`curl` the bytes to the upload URLs it returns, then `commit_publish`. The exact commands are in the `begin_publish`
description. An update sends `id` and `baseVersion` as for `publish`, and a file the artifact already holds is not uploaded again.

## Updating

Publish again with the same `id` to keep the link people already have. Publishing without an `id` makes a
second page.
