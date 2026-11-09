# Authoring a doc artifact

Write Markdown, at most 2 MB, UTF-8. The platform converts it to blocks (headings, paragraphs, lists, tables, ...) that
each have a stable id, and draws them as a calm reading page that follows the viewer's light or dark setting; you
write content, not layout. `read` gives the document back as Markdown, and anyone who can open it can download the
Markdown (the export `md`). A person can also save the page as a PDF from the viewer (it prints in the browser); there is
no PDF for an agent, `read` gives you the content. Never put credentials, tokens or production customer data in a document.

## What to write

CommonMark plus the GitHub extras:

| Use | Syntax |
|---|---|
| Title and sections | `# Title` once at the top, then `##` and `###`; the first heading also names the browser tab |
| Emphasis, code | `**bold**`, `*italic*`, `` `code` ``, fenced blocks with a language |
| Lists | `- item`, `1. item`, task lists `- [x] done` |
| Tables | pipe tables with a header row; keep them narrow, wide ones scroll sideways |
| Links | `[text](https://example.com)`; `#section-name` jumps within the page; `mailto:` works |
| Images | `![alt](URL)` where URL is a file uploaded to this document (see Images below), an https URL, or a small `data:image/png` URI |
| Strikethrough, bare URLs | `~~old~~`; a plain `https://...` becomes a link |
| A part you will write next | a line `<!-- pictor:pending Goals: the three outcomes -->`; it is drawn as a grey placeholder with that text |
| Tabs | a line `<!-- pictor:tab Title -->` starts a tab; a document without one is a single tab |

To build a document in steps, publish a skeleton first (the title and one `pictor:pending` line per section) so the
person sees it take shape, then publish again with each section filled in. Republish the whole Markdown with the
`id` and `baseVersion` you have: a block whose content did not change keeps its id, and an edited block keeps its id
too, so comments and later edits stay attached.

Write headings in order and give every image alt text. Lead with the conclusion, then the detail; a report
or cheat sheet reads best as short sections that can be scanned.

## Images

Never inline a big picture as base64. Upload it to the document with `upload_asset` (MCP; the document must exist
first) or `PUT /api/artifacts/{id}/assets/{name}`, and use the url it returns exactly as it is:
`![Revenue by month](https://pictor.example/files/AbC123/chart.png)`. It shows for everyone who can open the document.
PNG, JPEG, GIF, WebP and SVG work as images; at most 50 files and 100 MB per document. The download (`export/md`)
keeps the url, which stays readable only by those who can open the document.

## What is dropped or refused

There is no raw HTML: `<script>`, `<img>`, `<iframe>`, `<style>`, `<b>` and every other tag are left out of the
page (only the two comment lines above are read). A link must start with `https:`, `http:`, `mailto:` or `#`, and an
image must be an https URL, a file uploaded to the document (`/files/...`) or a small `data:image` URI; anything
else (`javascript:`, a relative link, `file:`) refuses the whole publish and the message names the line. A document
cannot run code, call `window.pictor` or keep shared data. If you need layout, a chart or interactivity, publish an
`html` artifact instead; if you paste a complete HTML page here, publishing is refused and says so.

What Markdown cannot say (underline, highlight, an empty paragraph, a table without a header row) is plain text or
left out when the document is read back as Markdown, and a comment at the end names it.

## Blocks (format 2)

A document can also be structured blocks instead of Markdown: pass `blocks` to `publish` instead of `content`, as a JSON
list of blocks (they become the only tab) or `{"tabs": [{"id"?, "title", "blocks": [...]}]}`. Send the value itself, not a
string. Every tab and block has a stable `id`; leave it out and the server adds one, send it and it is kept (send the ids you
read when you republish, so comments and later edits stay attached to the right block).

```json
[
  {"type": "heading", "level": 1, "content": [{"text": "Plan"}]},
  {"type": "paragraph", "content": [{"text": "Ship "}, {"text": "beta", "marks": ["bold"]}, {"text": " docs", "marks": ["link"], "href": "https://example.com"}]},
  {"type": "pending", "intent": "Goals: the three outcomes this quarter commits to"}
]
```

| type | fields |
|---|---|
| `paragraph` | `content` |
| `heading` | `level` 1-6, `content` |
| `bullet_list`, `numbered_list` (`start?`), `task_list` | `items`: `{content, checked (task list only, required), items (nested)}` |
| `quote` | `blocks` (paragraphs, headings, lists, quotes, code, rules) |
| `code` | `lang?`, `text` |
| `rule` | |
| `table` | `header?` (first row is the header), `rows[].cells[]`: `{align? left\|center\|right, content}` |
| `image` | `src`, `alt` |
| `pending` | `intent`: a placeholder for a part you will write next, drawn grey with the intent |

`content` is a list of runs `{text, marks?, href?}`; marks are bold, italic, underline, strike, code, highlight and
link (a link run needs `href`). A newline in `text` is a line break. Links are `https:`, `http:`, `mailto:` or `#anchor`;
images are `https:` addresses, files uploaded to the document, or small `data:image` addresses. At most 5000 blocks, 50
tabs, 8 levels of nesting and 2 MB. A refusal starts with the path of the problem, such as `tabs[0].blocks[2].level`, and
nothing is saved. Read `docs/doc-format.md` in the repository for the whole definition.

To mention a person, use a run `{"user":"anna.lee","name":"Anna Lee"}` in any paragraph, heading, list item or table cell, or write `[@Anna Lee](mention:anna.lee)` in Markdown. The account is 1–64 ASCII letters, digits, `.`, `_` or `-`; the name is nonblank, one line, at most 100 Unicode characters, without control characters. A mention has no text, marks or href. `doc_edit` inserts or replaces these runs and `read` with `view: "blocks"` returns them unchanged. Mentions show the stored name after @; they grant no access and send no notification. Name the account directly; there is no people-search MCP tool. The browser picker searches the organisation directory, including colleagues who have never signed in to CodeXArtifacts. Stored mention names remain readable even when an account is no longer available.

Both ways give the same document: the Markdown you publish is converted to these blocks, and `read` converts them back.
Send blocks when you need to say something Markdown cannot (underline, highlight) or to keep ids you read elsewhere.

## Changing part of a document

For a small change use `doc_edit`, not `publish`: `read` with `view` `blocks` gives every block with its id and hash (the default
view gives the Markdown and the ids of the top-level blocks). `doc_edit` takes `ops`, done in order, all or none: `replace` a block
(by id), `insert` blocks (`before`, `after` a block, or at the `start` or `end` of a tab), `delete` a block, with the new content
as `markdown` or `blocks`. Send `ifHash` with every block you rewrite from what you read: if a person changed that block meanwhile
the whole call is refused and the block as it is now comes back, so you never overwrite what someone is typing.

Tabs have four ops of their own: `tab_add` (`title`, placed with `at` start or end, or `before` or `after` a tab id; the new tab
starts with an empty heading 1 unless you send `markdown` or `blocks`), `tab_rename` (`tab`, `title`), `tab_delete` (`tab`, with its
blocks; the last tab stays) and `tab_move` (`tab`, with `before`, `after` or `at`). `read` lists every tab with its id and title,
and a call that used a tab op answers with the tabs, including the id of the one it added.

When people have the document open, a `doc_edit` goes into the live copy and they see it at once, what they are typing elsewhere is
kept, and a version follows a couple of minutes after the document goes quiet; a whole `publish` also goes through the live copy,
replacing it, and is refused when your `baseVersion` is older than work people did that you have not seen.

## Comments on words

People select words in the editor and comment on them; `comments` then returns that comment with an `anchor` of `tab` (the tab id),
`block` (the id of the block the words start in) and `quote` (the words, at most 300 characters); an anchor from the editor also has
`start` and `end`, which the editor uses to follow the words, so send them back unchanged or leave them out. To read what a comment is
about, find the block with `read` and look for the quote in it. To comment on words yourself, call `comment` with
`anchor: {"tab": "...", "block": "...", "quote": "the exact words"}`; the editor marks those words, and a thread whose words were
since changed or deleted stays, shown with its quote and a note that the original text changed.

## Format version

The format is `2` (blocks) and there is no other: leave `format` out. Documents that were Markdown before blocks existed
were converted once, and read back as Markdown like any other.

## A long document that is already a file

A Markdown file on disk, or text over about 50 KB, does not go through `publish`. Use `begin_publish` with `files` listing
`content` (the Markdown file) with its sha256 and size, `curl` the bytes to the upload URL, then `commit_publish`; the
exact commands are in the `begin_publish` description. A doc written as `blocks` goes through `publish`.

## Choosing who can see it

Same as every artifact: new documents are visible only to you; pass `visibility` as `organisation` to share
with everyone who can sign in, or `public` where the deployment allows public links.
