# Authoring a board artifact

A board is a whiteboard for drawing ideas: flow charts, architecture sketches, the drawing a meeting talks over. People
draw on it together live in the browser; for how a web page looks, make a `design` instead. The content is JSON, at
most 1 MB, UTF-8:

```
{"version": 1, "elements": [ ...Excalidraw elements... ]}
```

Nothing else goes at the top level: an Excalidraw file's `type`, `source`, `appState` and `files` are refused. `read`
gives the board back in its stored form: deleted elements left out, elements in drawing order, and every label placed
on its shape. Publish the whole board each time with the `id` and `baseVersion` you have; while people have the board
open, your board is written into what they see, field by field, and what you did not change stays as they left it.

## The short way: a skeleton

Most diagrams are boxes with words in them and arrows between them. Write those, and the server fills in the rest:

- A `rectangle`, `ellipse` or `diamond` may have `label`: the words inside it. The server adds the text element
  (`<shape id>-label`), sizes it, centres it in the shape and binds the two. A labelled shape without `width` and
  `height` gets a size that fits its label.
- An `arrow` or `line` may have `from` and `to`: the ids of the shapes it joins. The server works out where it leaves
  and meets each shape's outline and binds it to both, so it follows when a person moves a shape. An arrow gets an
  arrowhead at its end unless you give `endArrowhead`.
- When no element has an `index`, they are drawn in the order you wrote them.

Every shape still needs its `x` and `y`: the server places nothing by itself. Lay shapes out on a grid, for example
left to right 200 apart. `read` gives the board back in full, the way the editor keeps it.

```json
{"version": 1, "elements": [
  {"id": "open", "type": "rectangle", "x": 0, "y": 0, "label": "Open app", "backgroundColor": "#a5d8ff", "fillStyle": "solid"},
  {"id": "check", "type": "diamond", "x": 220, "y": -20, "width": 160, "height": 100, "label": "Signed in?"},
  {"id": "home", "type": "rectangle", "x": 460, "y": 0, "label": "Home"},
  {"id": "login", "type": "rectangle", "x": 220, "y": 160, "label": "Sign in"},
  {"id": "yes", "type": "arrow", "from": "open", "to": "check"},
  {"id": "go", "type": "arrow", "from": "check", "to": "home"},
  {"id": "no", "type": "arrow", "from": "check", "to": "login", "strokeStyle": "dashed"}
]}
```

## Elements

Every element needs an `id` (letters, digits, `-` and `_`, at most 64) unique on the board, and a `type`:

| `type` | What it is | Its own fields |
|---|---|---|
| `rectangle`, `ellipse`, `diamond` | A shape | `x`, `y`, `width`, `height`, `roundness` (`{"type": 3}` rounds corners) |
| `text` | Text, free or a shape's label | `text`, `originalText`, `fontSize`, `fontFamily`, `lineHeight`, `textAlign` (`left`, `center`, `right`), `verticalAlign` (`top`, `middle`, `bottom`), `containerId`, `autoResize` |
| `arrow`, `line` | A line through `points` | `points` (`[[0, 0], [dx, dy], ...]`, relative to `x`, `y`), `startBinding`, `endBinding`, `startArrowhead`, `endArrowhead` (`arrow`, `bar`, `dot`, `circle`, `triangle`, `diamond`, the `_outline` forms and the `crowfoot_` forms, or null), `elbowed` |
| `freedraw` | A pen stroke | `points`, `pressures`, `simulatePressure` |
| `frame` | A named area that holds other elements | `name`; an element joins it with `frameId` |
| `image` | A picture: a file uploaded to this board | `fileId` (the file's name, see Pictures), `x`, `y`, `width`, `height`, `status` (`saved`), `scale`, `crop` |

Any element may also have `angle` (radians), `strokeColor` and `backgroundColor` (`transparent` or `#hex`), `fillStyle`
(`hachure`, `cross-hatch`, `solid`, `zigzag`), `strokeWidth`, `strokeStyle` (`solid`, `dashed`, `dotted`), `roughness`
(0 neat to 2 sketchy), `opacity` (0 to 100), `seed` (0 to 2147483647; the sketch's randomness), `groupIds`, `index`
(drawing order, compared as plain strings, so `a10` comes before `a2`; Excalidraw's own sequence is `a0` … `a9`, `aA` … `aZ`, `aa` … `az`, then `b00`, `b01`, …), `locked`, `isDeleted` and `link` (an `https:` or `mailto:`
address only). Coordinates are numbers within ±10,000,000; one text is at most 20,000 characters; a line has at most
10,000 points; a board has at most 10,000 elements.

A field not named here is refused, and so is any `data:` value anywhere: a picture is a file uploaded to the board,
never data in it, and `embeddable` and `iframe` elements are never drawn (they would load another site). `version`, `versionNonce` and
`updated` (each editor's own bookkeeping) are accepted and dropped. A refusal names the element and the field, for
example `board: r1.link is "javascript:x"; a link is an https: or mailto: address`.

## Labels and arrows

- **A label inside a shape** is a `text` with `containerId` set to the shape's id, and the shape lists it in
  `boundElements` as `{"id": "<text id>", "type": "text"}`. Give the label a `width` and `height` that fit its text
  (about `0.6 × fontSize` per Latin character and `1 × fontSize` per CJK character wide, `lineHeight × fontSize` per
  line high); its `x` and `y` are worked out from the shape and its alignment, so leave them out.
- **An arrow from one shape to another** starts at the arrow's `x`, `y` and ends at the last point. It sticks to its
  shapes through `startBinding` and `endBinding`, `{"elementId": "<shape id>", "focus": 0, "gap": 5}`, and each shape
  lists it in `boundElements` as `{"id": "<arrow id>", "type": "arrow"}`. Then the arrow follows when a person moves a
  shape. A bound arrow may bend: give it more `points`; the first and last are the ends that stick.

## Example in full

```json
{"version": 1, "elements": [
  {"id": "start", "type": "rectangle", "x": 0, "y": 0, "width": 160, "height": 70, "index": "a0",
   "roundness": {"type": 3}, "backgroundColor": "#a5d8ff", "fillStyle": "solid",
   "boundElements": [{"id": "start-label", "type": "text"}, {"id": "to-check", "type": "arrow"}]},
  {"id": "start-label", "type": "text", "width": 90, "height": 25, "text": "Sign in", "originalText": "Sign in",
   "fontSize": 20, "lineHeight": 1.25, "containerId": "start", "textAlign": "center", "verticalAlign": "middle", "index": "a1"},
  {"id": "check", "type": "diamond", "x": 260, "y": -20, "width": 160, "height": 110, "index": "a2",
   "boundElements": [{"id": "check-label", "type": "text"}, {"id": "to-check", "type": "arrow"}]},
  {"id": "check-label", "type": "text", "width": 60, "height": 25, "text": "OK?", "originalText": "OK?",
   "fontSize": 20, "lineHeight": 1.25, "containerId": "check", "textAlign": "center", "verticalAlign": "middle", "index": "a3"},
  {"id": "to-check", "type": "arrow", "x": 165, "y": 35, "points": [[0, 0], [90, 0]], "index": "a4", "endArrowhead": "arrow",
   "startBinding": {"elementId": "start", "focus": 0, "gap": 5}, "endBinding": {"elementId": "check", "focus": 0, "gap": 5}}
]}
```

## Changing part of a board

To change a board people may be drawing on, do not publish all of it again: use `board_edit`. `read` gives each
element's hash in `hashes`; then send ops: `add` an element (a skeleton's `label`, `from` and `to` work here too),
`change` some `fields` of one (`label` sets a shape's words), or `delete` one, each change and delete with the
`ifHash` you read. Only what you change is written, so what people drew meanwhile stays; if someone changed an element
you name, the call is refused with that element as it is now. The board_edit tool's description has the details.

## Pictures

A picture on a board is a file uploaded to the board. Create the board first, upload the picture with `upload_asset`
(its result's `name` is the file's name on the board), then add an `image` element whose `fileId` is that name, with
`board_edit` or a publish: `{"id": "shot", "type": "image", "x": 0, "y": 0, "width": 640, "height": 400, "fileId":
"<name>", "status": "saved"}`. An image can only show a file of its own board: a `fileId` that is not one is refused, and
the board's files are listed in the refusal. When a file is deleted the image stays and is drawn as an empty frame. A
person pastes or drops a picture onto the board and it is uploaded the same way.

## Seeing a board as a picture

`export` with `format: "svg"` gives the board as an SVG drawn by the server: the same shapes, words and arrows with
neat lines rather than the editor's hand-drawn ones. It is also what people see when they open the board without the
editor and on its gallery card. A person exporting from the open board gets the editor's own drawing, as SVG or PNG.

## A big board that is already a file

A board's JSON on disk, or over about 50 KB, does not go through `publish`. Use `begin_publish` with `files` listing
`content` (the JSON file) with its sha256 and size, `curl` the bytes to the upload URL, then `commit_publish`; the exact
commands are in the `begin_publish` description. The same checks apply.

## Writing a good board

Keep one idea per board, a few dozen elements, labels of a few words. Lay shapes out left to right or top to bottom
with even gaps (60 to 100 between shapes), give shapes of one role one colour, and use arrows for what flows or calls.
Never put credentials, tokens or production customer data on a board.
