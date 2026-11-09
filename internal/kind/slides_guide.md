# Authoring a slides artifact

A deck is JSON, at most 1 MB. You write content, never layout or styling: the platform draws every slide on a fixed
16:9 canvas that scales to the window, picks the type size from how much is on the slide, and centres it. People
open the deck as a page, press `f` for full screen, use the arrow keys, and press `n` to read the speaker notes.
Anyone who can open the deck can download it as an editable PowerPoint file (the export `pptx`), with the notes as speaker notes.
You cannot see the screen, so stay inside the limits below and read the notes the publish result sends back.
Never put credentials, tokens or production customer data in a deck.

## Shape

```json
{
  "version": 1,
  "slides": [
    { "layout": "title", "title": "Q3 review", "notes": "Welcome everyone. Three minutes of context.",
      "elements": [ { "type": "text", "text": "Platform team, 14 Oct" } ] },
    { "layout": "content", "title": "Where we are", "notes": "Lead with the 12-minute number.",
      "elements": [
        { "type": "stats", "items": [ { "value": "12 min", "label": "median deploy" }, { "value": "99.95%", "label": "uptime" } ] },
        { "type": "bullets", "items": [ "Deploys are **3x faster** than in Q2", "`main` is always releasable" ] }
      ] }
  ]
}
```

- `version` is `1`. `slides` is a list of 1 to 80 slides.
- A slide has `title` (required), `layout`, `notes` and `elements`.
- `layout` is `title` (a big centred title with a few elements under it: a subtitle, the date) or `content` (the title at the
  top and the elements below; the default). It is never guessed from the content: say `title` for the opening and closing slides.
- `notes` is the speaker's text for that slide, up to 4000 characters. Write notes on every slide; the publish result lists
  the slides that have none.
- `elements` is 1 to 8 elements, drawn top to bottom. A `title` slide may have none.

## Text

Every string may use `**bold**` and `` `code` `` and nothing else. Everything is plain text otherwise: there is no HTML,
no Markdown, no links and no images, and angle brackets show as typed. A line break in a `text` element is `\n`.

## Elements

| `type` | Fields | Shows |
|---|---|---|
| `text` | `text` | a paragraph |
| `bullets` | `items`: 1 to 8 strings | a bulleted list |
| `numbered` | `items`: 1 to 8 strings | a numbered list |
| `code` | `code`, `language` (a label only, no colouring) | a code block, up to 16 lines |
| `table` | `columns`: 1 to 6 strings; `rows`: 1 to 12 lists, one cell per column | a table. A cell is a string, or `{"text": "...", "tone": "success" or "warning" or "danger"}` to colour it |
| `stats` | `items`: 1 to 4 of `{"value", "label"}` | big numbers. `value` is a string or a number |
| `quote` | `text`, `by` (optional) | a pull quote |
| `steps` | `items`: 2 to 6, each a string or `{"title", "text"}` | a numbered sequence across the slide |
| `columns` | `columns`: 2 or 3 of `{"title" (optional), "elements": [...]}` | side by side, each holding up to 6 elements other than `columns` and `cards` |
| `cards` | `items`: 2 to 4 of `{"title", "text", "items": [strings], "highlight": true}` | side-by-side cards; at most one `highlight`, the one to look at (the recommended plan) |
| `progress` | `items`: 1 to 6 of `{"label", "value": 0 to 100, "text"}` | bars; `text` replaces the default `NN%` |

An unknown field or type is refused with the list of the ones that exist, so a typo does not silently drop content.

## How much fits

Plan for about 6 lines of text on a slide and tables of up to 7 rows. The renderer shrinks type on a full slide (down to 75%)
and enlarges it on a sparse one, but it cannot make room that is not there. The publish result has a `review` list: it names
the slides that are likely to overflow the canvas and the slides without notes. When it names a slide, split it in two or
cut text; do not just publish again.

- One idea per slide. Put detail in `notes`, not on the slide.
- Lead with the conclusion in the title ("Deploys are 3x faster"), not the topic ("Deploys").
- Use `stats`, `steps`, `columns` and `cards` where a bulleted list would be a wall of text.
- Keep wide tables to 3 or 4 columns; a long cell wraps and costs rows.

## One complete example

```json
{
  "version": 1,
  "slides": [
    { "layout": "title", "title": "Pricing proposal", "notes": "Ask for a decision on the Team plan by Friday.",
      "elements": [ { "type": "text", "text": "Finance review, **October**" } ] },
    { "title": "Three plans", "notes": "Team is the one we recommend: it covers 80% of accounts.",
      "elements": [ { "type": "cards", "items": [
        { "title": "Starter", "text": "For trying it out", "items": [ "1 project", "Community support" ] },
        { "title": "Team", "text": "Recommended", "highlight": true, "items": [ "10 projects", "Email support", "SSO" ] },
        { "title": "Scale", "text": "For large orgs", "items": [ "Unlimited projects", "Named engineer" ] } ] } ] },
    { "title": "How a request flows", "notes": "Walk left to right; stop on step 3.",
      "elements": [ { "type": "steps", "items": [
        { "title": "Queue", "text": "Accepted in under 50 ms" }, { "title": "Validate" }, { "title": "Charge", "text": "Idempotent" } ] },
        { "type": "code", "language": "http", "code": "POST /v1/charges\nIdempotency-Key: 7f3a" } ] },
    { "title": "Targets", "notes": "Revenue is behind; churn is on track.",
      "elements": [
        { "type": "table", "columns": [ "Metric", "Now", "Target" ],
          "rows": [ [ "Revenue", { "text": "$1.1M", "tone": "warning" }, "$1.4M" ], [ "Churn", { "text": "2.1%", "tone": "success" }, "2.5%" ] ] },
        { "type": "progress", "items": [ { "label": "Revenue to target", "value": 78 } ] } ] },
    { "layout": "title", "title": "Questions?", "notes": "Leave this up during discussion.",
      "elements": [ { "type": "quote", "text": "Price is what you pay; value is what you get.", "by": "Warren Buffett" } ] }
  ]
}
```

## Errors

A deck that breaks a rule is refused before anything is saved, with every problem named by its place, for example
`Slide 3 element 2 (table): table needs "columns" (a list of header strings) and "rows" (a list of rows, each a list with one
cell per column)`. Fix those and publish again. If the content was wrapped in a Markdown code fence, send the JSON itself.

## A large deck that is already a file

A deck as a JSON file on disk, or over about 50 KB: use `begin_publish` with `files` listing `content` (the JSON file)
with its sha256 and size, `curl` the bytes to the upload URL, then `commit_publish`. The exact commands are in the
`begin_publish` description.

## Updating

Change one slide at a time with the slide tools instead of resending the deck, which can drop or misplace slides:

| Tool | Does |
|---|---|
| `get_slide` | `number`: that slide, with the list of all slides (number, title, and `overflows: true` on one likely to overflow) and the version |
| `replace_slide` | `number`, `slide`: puts the new slide object in its place |
| `insert_slide` | `after`, `slide`: the new slide goes after slide `after` (`0` puts it first) |
| `move_slide` | `number`, `to`: the slide ends up as slide `to` |
| `delete_slide` | `number`: removes it; a deck keeps at least one slide |

Slides are numbered from 1. Each change is saved as a new version and returns the slide list as it is now, so check it,
plus the same `review` as a publish. Send `baseVersion` (the version you read): slide numbers move when someone else
changes the deck, and a newer version refuses the call rather than changing the wrong slide. A slide that breaks a rule
or a number that does not exist is refused and nothing is saved.

To rework most of the deck, read the artifact, change the JSON and publish the whole deck again with the same id and
`baseVersion`. The publish result says when the slide count changed ("from 6 slides to 5 slides"); check it matches
what you meant. The kind of an artifact cannot change.

## Format version

The format version is `1`, and a new deck gets it. An artifact keeps the version it was published under, so an upgrade of the
renderer never changes a published deck.

## Choosing who can see it

Same as every artifact: new decks are visible only to you; pass `visibility` as `organisation` to share with everyone who
can sign in, or `public` where the deployment allows public links. Present a deck from `/a/<id>?slide=3` to open at slide 3.
