# Authoring a design artifact

A design is a canvas of frames, like artboards. You publish it as `kind: "design"` with two things: the **content** is
`design.json`, a small manifest that names the pages and their frames, and the **files** are the frames themselves, one
static HTML file each at `frames/<name>.html`. People see all frames side by side; to change one frame you send only that
file and every other frame is carried over unchanged.

```text
publish { kind: "design", title: "Landing page",
          content: "<the text of design.json>",
          files: { "frames/hero.html": "<html of the frame>", "frames/mobile.html": "...", "frames/footer.html": "..." } }
```

## design.json

One object with `version` (always `1`), `pages`, and optional `tokens`. A page has an `id`, a `name` and `rows`: a list of rows, each a list of
frames that sit side by side. A frame has an `id`, a `name`, the `file` that holds its HTML, and its size `w` and `h` in
pixels (1 to 8192). Other fields are refused.

```json design.json
{
  "version": 1,
  "pages": [
    {
      "id": "home",
      "name": "Home",
      "rows": [
        [
          { "id": "hero", "name": "Hero", "file": "frames/hero.html", "w": 1440, "h": 900 },
          { "id": "hero-mobile", "name": "Hero mobile", "file": "frames/mobile.html", "w": 390, "h": 844 }
        ],
        [
          { "id": "footer", "name": "Footer", "file": "frames/footer.html", "w": 1440, "h": 240 }
        ]
      ]
    }
  ]
}
```

### Shared design tokens

`tokens` is an optional flat object of CSS custom property names and CSS strings, for example
`"tokens": {"--accent": "#336699", "--font": "system-ui, sans-serif", "--space": "16px", "--radius": "8px", "--shadow": "0 2px 8px rgba(0,0,0,.2)"}`.
Use them in every frame as `color:var(--accent)`, `font-family:var(--font)`, `gap:var(--space)`,
`border-radius:var(--radius)` or `box-shadow:var(--shadow)`. Keep these references when editing a frame;
never expand them to computed colours or lengths. The properties colour picker writes `var(--name)`.
The Design tokens section edits shared values for all frames; each commit is one Undo step.

Names include `--`, followed by 1–80 ASCII letters, digits, underscores or dashes, starting with a letter
or underscore. Values are nonempty strings of at most 1000 bytes. Colours, quoted font names, lengths,
shadows, `var()` aliases and arithmetic/colour functions are supported; URLs, escapes, comments,
control characters, declaration/rule delimiters, markup and `!important` are refused. Undefined or cyclic
aliases follow the browser's CSS fallback rules. Omitting `tokens` means no shared variables.

Agents use the same `publish`/`read` path: read the latest manifest, change only the relevant token and
publish the manifest with `baseVersion`; frame files can be omitted when unchanged. Existing live clients
receive token changes through the room. Version history retains its own token values. Raw frames, Play,
print/PDF and offline ZIP frames receive shared variables at render time; stored frame bytes retain bindings.

A frame may include `<link rel="stylesheet" href="/kit/v5/pictor.css">` (released v1–v5 paths only).
The platform inlines that trusted CSS in the rendered copy and includes its fonts in ZIP downloads;
frames cannot load arbitrary stylesheets. Put shared token references in your authored CSS.

- Ids are 1 to 40 characters of lower case letters, digits, dash and underscore, starting with a letter or digit. A frame id is
  unique in the whole design, a page id among the pages. Ids are the names comments and later edits refer to, so keep them
  when you rename.
- Names are the text people see, at most 80 characters.
- `file` is `frames/<name>.html`, one level deep, a different file for each frame. Every frame needs its file in the same
  publish or already in the design.
- At most 20 pages and 60 frames, design.json at most 256 KB, a frame at most 512 KB.
- Live editing has a separate 8 MiB encoded room/history budget. Valid source files can still exceed it after expansion. A capacity refusal names the bound: retain your draft, reduce additions if the current room fits, or ask the operator to recover an oversized retained log. Do not retry unchanged.

## A frame

A frame is a static page: markup and styles, no scripts. Write a complete document or just the body; the frame is always
drawn in standards mode. Size it to its `w` and `h` in design.json, and lay it out with flexbox or grid so it holds together
without computing coordinates.

```html frames/hero.html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<style>
  :root { --ink: #1c1917; --paper: #faf7f2; --accent: #0f766e; }
  * { box-sizing: border-box; margin: 0; }
  body { width: 1440px; height: 900px; background: var(--paper); color: var(--ink); font: 400 20px/1.5 "Helvetica Neue", Arial, sans-serif; }
  .hero { display: grid; grid-template-columns: 1fr 1fr; align-items: center; gap: 64px; height: 100%; padding: 0 120px; }
  h1 { font-size: 72px; line-height: 1.05; letter-spacing: -0.02em; }
  p { margin-top: 24px; max-width: 28em; }
  .cta { display: inline-block; margin-top: 40px; padding: 16px 32px; border-radius: 999px; background: var(--accent); color: #fff; }
  .art { height: 560px; border-radius: 32px; background: linear-gradient(135deg, var(--accent), #134e4a); }
</style>
</head>
<body>
  <main class="hero">
    <section>
      <h1>Ship the page, not the meeting</h1>
      <p>CodeXArtifacts turns a prompt into something people can open, comment on and change together.</p>
      <a class="cta" href="#start">Start a page</a>
    </section>
    <div class="art" role="img" aria-label="A teal gradient"></div>
  </main>
</body>
</html>
```

```html frames/mobile.html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<style>
  * { box-sizing: border-box; margin: 0; }
  body { width: 390px; height: 844px; background: #faf7f2; color: #1c1917; font: 400 17px/1.5 "Helvetica Neue", Arial, sans-serif; }
  main { display: flex; flex-direction: column; justify-content: center; gap: 24px; height: 100%; padding: 0 28px; }
  h1 { font-size: 44px; line-height: 1.05; letter-spacing: -0.02em; }
  .cta { align-self: flex-start; padding: 14px 28px; border-radius: 999px; background: #0f766e; color: #fff; }
</style>
</head>
<body>
  <main>
    <h1>Ship the page, not the meeting</h1>
    <p>CodeXArtifacts turns a prompt into something people can open, comment on and change together.</p>
    <a class="cta" href="#start">Start a page</a>
  </main>
</body>
</html>
```

```html frames/footer.html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<style>
  * { box-sizing: border-box; margin: 0; }
  body { width: 1440px; height: 240px; background: #1c1917; color: #faf7f2; font: 400 18px/1.5 "Helvetica Neue", Arial, sans-serif; }
  footer { display: flex; justify-content: space-between; align-items: center; height: 100%; padding: 0 120px; }
  ul { display: flex; gap: 32px; list-style: none; padding: 0; }
</style>
</head>
<body>
  <footer>
    <strong>CodeXArtifacts</strong>
    <ul><li>Docs</li><li>Pricing</li><li>Contact</li></ul>
  </footer>
</body>
</html>
```

What a frame may contain:

- **Elements**: text and structure (`div span p h1`-`h6 a b i em strong small code pre blockquote br hr`, lists, tables,
  `header footer nav main section article aside figure`), form controls as they look (`form input button select textarea label`),
  `img`, and inline `svg` with its drawing elements. Not allowed: `script`, `iframe`, `object`, `embed`, `video`, `audio`,
  `canvas`, `base`, `template`. A `link` is accepted only for a released CodeXArtifacts kit stylesheet.
- **Attributes**: any, including `data-*` and `aria-*`, except `on*` event handlers, `srcdoc` and `http-equiv`.
- **CSS**: inline `style` attributes and `<style>` elements. Every property that draws, lays out, animates or sets a font is
  allowed, with any value (gradients, grid, custom properties, `calc()`), and so are `@media`, `@supports`, `@keyframes`,
  `@font-face`, `@layer` and `@container`. Not allowed: `@import`, `expression()`, and a property outside the list (the
  message names it, so you can say what you need).
- **Addresses**: a frame loads nothing from outside. An address (`src`, `href`, `url()`) may be a `#fragment`, a `data:`
  picture or font, or a file uploaded to this artifact (`/files/<id>/<name>`, from `upload_asset`). An `https:` address or a
  relative file name is refused, except in one place: an `<a href>` may link out with `https:` or `mailto:`, and the link
  opens in a new tab (publish sets `target="_blank"` and `rel="noopener noreferrer"`). Pictures, fonts and `url()` never
  load from outside.
- **Fonts**: system fonts by name, or a font uploaded as a file and declared with `@font-face`.

A small piece that follows the same rules, to show a few of them together:

```html
<ul class="plans" style="display: grid; grid-template-columns: repeat(3, 1fr); gap: 24px">
  <li style="padding: 24px; border: 1px solid #d6d3d1; border-radius: 16px">
    <h3>Team</h3>
    <img alt="" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=">
  </li>
</ul>
```

```css
.card { background: linear-gradient(135deg, #fff, #eee); box-shadow: 0 2px 8px rgb(0 0 0 / 20%); }
@media (max-width: 600px) { .card { padding: 12px } }
```

## What comes back: normal form and `data-pid`

Each frame you send is stored in one normal form: parsed and written back once (the same bytes when you send it again),
in standards mode, with a `data-pid` attribute on every element of the body. The `data-pid` is the stable name of the
element: comments and edits refer to it, and it stays as long as you keep it in the file. The publish result's `rewritten`
holds the text of each frame as stored, so you have the ids without another call, and `read` with `file` returns it too.
When you change a frame, start from that text (read it first) so the ids stay; an element without one gets the next
free number, `e<number>`, and one that repeats an id gets a new one. Everything else you wrote is kept as written.

## Changing a design

- To change one frame, publish with `baseVersion` and `files: { "frames/hero.html": "<its complete new text>" }`.
  The frames you do not send are carried over unchanged.
- To add a frame, send its file and add it to design.json (content), in the same publish.
- To remove a frame, take it out of design.json and send its file as `null`.
- A frame in `files` that no frame in design.json uses is accepted, with a note in `review`: nothing shows it.

## While people are editing it

When people have the design open, a publish with `files` is merged into what they are editing instead of being saved as a
version: only what you changed from the version you started from is applied, matched by `data-pid`, so what they changed in
other properties, elements or words stays, and they see your change at once with your name over the elements you touched.
Start from the frame as `read` with `file` returns it (that is the live copy) and keep every `data-pid`; an element you leave out
is removed, one without an id is added. A frame sent with no ids at all replaces the live frame, so it is refused when people have changed that frame since you read it. The result says `live: true` and no new version is saved: it becomes one after they
pause. If they changed the same words, or moved an element into one you moved, the publish is refused with what to do: read
the frame again, redo your change on it and publish again. Values in an element's `style` must be ones the editor takes
(lengths, colours, keywords, a few functions); others are refused while the design is open.

## A large design that is already on disk

Frames you wrote to files, or a design over about 50 KB: use `begin_publish` with `files` listing `content` (`design.json`)
and every frame (`frames/<name>.html`), each with its sha256 and size, `curl` the bytes to the upload URLs it returns, then
`commit_publish`. The exact commands are in the `begin_publish` description; the result's `rewritten` still tells you which
frames were stored in their normal form.

## Pictures

Upload a picture or a font with `upload_asset`, then use the `url` path it returns as `/files/<id>/<name>` in `src` or
`url()`. A small picture can be a `data:` address instead. A frame over 512 KB is refused: move its images and fonts to
`upload_asset` and use the returned URL instead of writing them into the frame as `data:` addresses.

## Errors and review

A publish that breaks a rule saves nothing and lists every problem with where it is, for example:

```text
Page "Home" frame "Hero" element 3: tag script is not allowed (a frame is static markup and runs no scripts)
Page "Home" frame "Hero" element 7: attribute src: external address "https://x.test/a.png" is not allowed
Page "Home" frame "Footer": file frames/footer.html is not among this design's files; send it with files
```

`element 3` is the third start tag in the file you sent. Fix them all and publish again.

A publish that is accepted may carry `review` notes about what you cannot see: a picture without `alt` text, an element
placed outside its frame, a frame with no text, a file no frame uses. Act on them, or tell the person.

## Making a good design

- Real content: real headings, copy and numbers, not "Lorem ipsum" and not a box with the word "image".
- One size per frame and the content fills it. Make a mobile frame as its own frame at 390 x 844, not a squeezed desktop one.
- Put colours, spacing and type sizes in custom properties on `:root`, so a change is one line.
- Flow layout (flex, grid) over absolute positions: the frame holds together when text changes.
- Give every `img` an `alt`, empty if it is only decoration.
- Do not put credentials, tokens or customer data in a design.

## Comments on a frame or element

`comments` returns a thread's anchor unchanged, including after its element is removed. `comment` can send the same shape:

```text
{"type":"design","frameId":"hero","pid":"e2","name":"Heading","x":120,"y":80}
```

`frameId` is the manifest frame id, `pid` is its source element's `data-pid` (omit it for a frame comment), and `x`/`y` are the fallback point in unscaled frame pixels, from 0 to 8192. `name` is an optional short human-readable label. The canvas follows the element while it exists and uses the stored position within the frame when it is removed; the thread stays. Replies inherit their thread's anchor. Anchors are at most 2000 bytes; keep identifiers and labels short.

## Pencil strokes and Pen paths

Pencil (P) draws freehand samples; Pen (Shift+P) creates connected anchors.
Both persist ordinary static SVG through Design format 1.
The outer SVG is the editable box: move or resize its inline `left`, `top`, `width` and
`height`, or delete it together with its path. Its `viewBox` keeps the vector geometry
independent of that box. Keep `preserveAspectRatio="none"` for free resizing and round
caps and joins for freehand marks. `pointer-events="none"` on the path lets canvas
selection target the outer SVG. The path and SVG each keep their own stable `data-pid`.

```html
<svg data-pid="pen1" data-pen="stroke" data-name="Pencil stroke"
     viewBox="0 0 104 44" preserveAspectRatio="none"
     style="position:absolute;left:40px;top:50px;width:104px;height:44px">
  <path data-pid="pen1-path" d="M2 2Q52 42 77 22L102 2"
        stroke="#336699" stroke-width="4" fill="none"
        stroke-linecap="round" stroke-linejoin="round" pointer-events="none"></path>
</svg>
```

A Pencil click is a visible dot: use a padded box and a round-capped path such as
`M2 2L2 2`. Empty or nonfinite gestures produce no node. The browser samples at most
4096 points, simplifies them to at most 1024 control points and smooths corners into quadratic segments; a stroke
is one local Undo step. Both tools use widths of 0.5–64 frame pixels. Published SVG path data
is at most 65536 bytes and 32768 coordinates, each finite and between -1000000 and
1000000. These limits apply to the serialized local coordinates, including control
points after rounding, rather than only the frame coordinates. Invalid path data
is refused with the path node's identity.

Pen clicks add straight segments. Press and drag an anchor to set its symmetric
Bézier handles: outgoing control = anchor + handle; incoming control = anchor - handle.
It continues between gestures until clicking the initial anchor closes the path,
or Escape finalizes an open path. A usable path needs at least two distinct anchors;
Escape before that writes nothing. Interrupted input discards only the unfinished
anchor. Switching tools or losing editing access discards an unfinished draft.
Pen accepts at most 512 anchors without simplifying their geometry. More anchors
are ignored; close or finalize the existing path. Each completed path is one Undo step.

Use M/L/C/Z SVG commands for connected paths: M starts, L connects clicked anchors,
C connects through dragged controls, and Z closes. An open endpoint uses only the
handle that participates in a segment. Use frame-local coordinates after undoing
canvas pan and zoom. Pad the participating control hull by half the stroke width
for the SVG's absolute box; d coordinates are relative to that box's origin.

```html
<svg data-pid="path1" data-pen="stroke" data-name="Path"
     viewBox="0 0 113 63" preserveAspectRatio="none"
     style="position:absolute;left:8.5px;top:18.5px;width:113px;height:63px">
  <path data-pid="path1-path"
        d="M1.5 1.5L51.5 1.5C51.5 1.5 71.5 21.5 91.5 41.5C111.5 61.5 1.5 1.5 1.5 1.5Z"
        stroke="#336699" stroke-width="3" fill="none"
        stroke-linecap="round" stroke-linejoin="round" pointer-events="none"></path>
</svg>
```

An agent reads and republishes these SVG nodes through the same `read` and `publish`
path as other design elements. Keep color variables such as `stroke="var(--accent)"`
as references in the source; do not replace them with a computed color. A room save
and reload preserve `d`, `stroke`, `stroke-width`, the viewBox and editable box. There
is no pressure, eraser, brush type, or shape recognition. Read the current frame,
keep its exact sourceBases proof and baseVersion, and preserve the SVG and path
data-pid values when updating geometry. Assign new distinct ids only to new nodes.
Publish only changed source; independently edited box position and other unchanged
content survive the merge. Readers cannot publish; editing permission and native SVG
validation are checked by the same service used by the human editor.

## Publishing into live work

Read the current manifest and each frame you will change. Keep each returned
`sourceBases` proof with its exact source text and send that map with `publish`,
along with `baseVersion`. The manifest key is `design.json`; frame keys are their
normalized paths. MCP file reads also expose `file.sourceBase` and `file.present`.
Reader and explicit saved-version reads do not issue writable bases. A second
read does not replace an earlier proof. Missing, expired, evicted or retired
bases refuse the whole publish: retain the draft, read current source and
reconcile it before retrying. Never guess a baseline from the saved version.
For a large publish, pass the map at `commit_publish`, without repeating source
bodies. Metadata-only, identical-current and static publishes retain their
ordinary behavior. See `docs/design-source-edit.md` for the source contract.
