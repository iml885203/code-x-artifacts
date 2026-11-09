# Authoring a react artifact

One TSX file, at most 1 MB, whose default export is the component CodeXArtifacts mounts: `export default function App() { ... }`.
It takes no props. Write the source; CodeXArtifacts compiles it when you publish (types are stripped, not checked), refuses it
with a list of every problem if it does not compile, and serves it with React, Tailwind, components, icons and charts
it provides. No build step, no `import React`, nothing to install.

## What you can import

Only these specifiers, and the artifact's own files by relative path (see Files that come with the component). Any other package, a URL and a path that starts with `/` are refused at publish.

| Specifier | Gives you |
|---|---|
| `react` | hooks (`useState`, `useEffect`, `useMemo`, ...), `Fragment`, `Suspense`, `memo`, ... |
| `react-dom`, `react-dom/client` | `createPortal`, `flushSync`; you never call `createRoot`, CodeXArtifacts mounts `App` |
| `recharts` | charts: `LineChart`, `BarChart`, `AreaChart`, `PieChart`, `ResponsiveContainer`, `Tooltip`, ... |
| `lucide-react` | icons, by their lucide name: `import { Activity, Users } from "lucide-react"` (a name that does not exist is refused) |
| `@/components/ui/button` | `Button` (`variant`: default, outline, ghost, destructive; `size`: default, sm, lg, icon; `asChild`) |
| `@/components/ui/card` | `Card`, `CardHeader`, `CardTitle`, `CardDescription`, `CardContent`, `CardFooter` |
| `@/components/ui/badge` | `Badge` (`variant`: default, secondary, outline) |
| `@/components/ui/tabs` | `Tabs`, `TabsList`, `TabsTrigger`, `TabsContent` (`defaultValue` or `value` + `onValueChange`) |
| `@/components/ui/dialog` | `Dialog`, `DialogTrigger`, `DialogContent`, `DialogTitle`, `DialogDescription`, `DialogClose` |
| `@/components/ui/tooltip` | `TooltipProvider` (wrap the page once), `Tooltip`, `TooltipTrigger`, `TooltipContent` |
| `@/components/ui/progress` | `Progress` (`value` 0 to 100) |
| `@/lib/utils` | `cn(...classes)`: join class names, later Tailwind utilities win |

A dialog needs a `DialogTitle` (screen readers); a tooltip needs a `TooltipProvider` above it.

## Styling

Tailwind CSS 4 runs in the page: any utility works, including arbitrary values (`w-[calc(100%-2rem)]`) and variants
(`hover:`, `focus:`, `sm:`, `md:`); there is no `dark:` to write, the tokens already switch. Colours are the shadcn names, mapped to CodeXArtifacts's tokens, so the page follows the viewer's
light or dark setting by itself: `bg-background`, `text-foreground`, `bg-card`, `text-muted-foreground`, `border-border`,
`bg-primary` / `text-primary-foreground` (the one accent), `bg-secondary`, `bg-accent`, `bg-destructive`, `text-ok`,
`text-info`, and `chart-1` to `chart-6` (`fill-chart-1`, `text-chart-2`). Do not hard-code colours (`bg-white`,
`#fff`, `text-gray-500`): they are fine in one theme and unreadable in the other.

Inside Recharts, which wants colour strings, use the tokens as CSS variables:

```tsx
<CartesianGrid stroke="var(--chart-grid)" />
<XAxis dataKey="day" stroke="var(--muted)" />
<Bar dataKey="done" fill="var(--chart-1)" />
<Line dataKey="left" stroke="var(--chart-2)" />
<Tooltip contentStyle={{ background: "var(--plate)", border: "1px solid var(--line)", color: "var(--text)" }} />
```

The page already links CodeXArtifacts's kit (`/kit/v5/pictor.css`). Its variables work in `style` and arbitrary values: `--night` (page), `--plate` (surfaces), `--raised`,
`--line`, `--text`, `--muted`, `--star`, `--danger`, `--ok`, `--info`, `--font-sans`, `--font-mono`. Charts sit inside a
parent with a height (`h-64`) and a `ResponsiveContainer width="100%" height="100%"`. Do not render or screenshot the page to check it unless the person asks.

## Files that come with the component

A component that grows can be split. Publish it with `files`, an object from a relative path to the file's complete text, and
`import` them from the component or from each other by relative path; the platform compiles everything into the one page:

```json
{ "content": "import Chart from \"./Chart\"; import \"./styles.css\"; export default function App() { return <Chart />; }",
  "files": { "Chart.tsx": "…", "lib/format.ts": "…", "data/rows.json": "[…]", "styles.css": "…" } }
```

- `import Chart from "./Chart"` finds `Chart.tsx`, `.ts`, `.jsx`, `.js`, `.mjs`, `.json` or `.css` (the extension may be left out);
  a path is read from the importing file's folder (`../lib/format`) and can never leave the files. Sources are `.tsx`, `.ts`,
  `.jsx`, `.js`, `.mjs`; an imported `.json` gives its data; an imported `.css` becomes a style element of the page.
  Only the runtime's modules above are packages; a file never shadows one. A problem names the file and the line.
- Files can also be fetched by the running page: `fetch("data/rows.json")` (a relative path, never one that starts with `/`), and
  `.html`, `.js`, `.mjs`, `.css`, `.json` files are served at their path. Sources (`.tsx`, `.ts`, `.jsx`) are only compiled in.
- A publish says what changes: a path with its text adds or replaces the file, `null` removes it, every path not mentioned is
  carried over, and a publish without `files` keeps them all (the component is compiled again with them). Read a file with
  `read` and `file`, change it, publish only that path. A version keeps its own files; `restore` brings them back.
- Limits: 16 MB a file, 255 files in one publish, 511 files and 256 MB in all in a version; the compiled script and the style are
  each at most 16 MB. Pictures, fonts and PDFs are uploaded with `upload_asset`; their url works from any file.

## The sandbox

The page runs in a sandboxed iframe with an opaque origin: `localStorage`, `sessionStorage`, IndexedDB and cookies throw, so
keep state in React state. `fetch` works only for public APIs that allow CORS; for CodeXArtifacts itself use `window.pictor`. Scripts
load only from CodeXArtifacts, so there is no CDN, no `eval`, no `dangerouslySetInnerHTML` of text other people wrote.

## window.pictor

A global on every page (there are no type declarations: write `declare const pictor: any;` or use `window.pictor`).
Every call returns a promise.

```tsx
declare const pictor: any;
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

The data belongs to this page and is shared by everyone who can open it: every viewer can read and overwrite every document,
and `updatedBy` records who wrote last. For something per person, key the document by `me.username`. Load in `useEffect`,
render something useful before the first call resolves, and show an error in the page when a call rejects. A public page can be
opened without signing in: then `whoami()` says `anonymous: true` and `db` is read-only.

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

## Page contract

- Light and dark: set no page background or text colour of your own; use the tokens. Look at both before publishing.
- Phone width (390 px): no fixed widths, `max-w-*` and `grid sm:grid-cols-*` instead, let wide tables scroll.
- Say it with real content: real titles, numbers and names, not lorem ipsum. One primary `Button` per view.
- Charts: label the series or the axis; never colour alone.
- If the component throws, the page shows the message and stack instead of a blank frame; fix it and publish again.
- Never put credentials, tokens or production customer data in a page, shared or not.
- Leave out the tropes that make a page look machine-made: purple-blue gradients and gradient text, glow, emoji as icons
  (use lucide), a centred hero with three icon cards, big-number metric tiles, a small label above every heading, cards
  inside cards, coloured stripes on one side of a card.

A minimal page:

```tsx
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default function App() {
  const [n, setN] = useState(0);
  return (
    <main className="mx-auto max-w-xl p-6">
      <Card>
        <CardHeader><CardTitle>Clicks</CardTitle></CardHeader>
        <CardContent className="flex items-center gap-4">
          <span className="text-3xl font-semibold tabular-nums">{n}</span>
          <Button onClick={() => setN(n + 1)}>Add one</Button>
        </CardContent>
      </Card>
    </main>
  );
}
```

## Printing and PDF

People save the page as PDF from the viewer (Export → PDF): the browser prints the page itself, with its own
print stylesheet and none of the viewer around it. There is no PDF on the server, so make the page print well:

- Add `@media print` rules: hide controls that do nothing on paper (buttons, inputs, tabs; Tailwind's `print:hidden`) and show every tab or
  collapsed section the reader needs.
- Let the content flow: no `height: 100vh` or `overflow: auto` boxes that cut it off at one screen; a fixed or
  sticky header prints on every page, so make it `position: static` in print.
- Keep a figure, a table row or a card on one page with `break-inside: avoid`, and start a new section with
  `break-before: page` where it helps.
- Set the paper margin with `@page { margin: … }` if the page has none of its own.

## Choosing who can see it

A new page is `private`: only its author can open it. Say which audience you chose.

| The person says | visibility |
|---|---|
| nothing about sharing, "draft", "show me first" | `private` |
| "share with everyone", "the whole company" | `organisation` (everyone who can sign in) |
| "public", "anyone with the link", people without an account | `public`, only if `whoami` says public links are on |

## A large component that is already a file

Source over about 50 KB, or sources you wrote to disk: use `begin_publish` with `files` listing `content` (the component)
and each sibling file (`.tsx`, `.ts`, `.jsx`, `.js`, `.css`, `.json`), each with its sha256 and size, `curl` the bytes to the
upload URLs it returns, then `commit_publish`. The exact commands are in the `begin_publish` description.

## Updating

Publish again with the same `id` to keep the link people already have; the page keeps the library versions it was
published with, so an update never changes what `@/components/ui/*` or `recharts` do. Publishing without an `id`
makes a second page.
