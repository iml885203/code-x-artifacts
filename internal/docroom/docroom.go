// Package docroom is how a doc lives in a realtime room: its tabs and blocks as
// Yjs XmlFragments in the shape y-prosemirror writes, one per tab, and back to the block
// JSON of docs/doc-format.md. The editor writes the room, the server reads and
// writes the room, and both have to agree on what a document is; this package
// is the server's half, and the golden fixtures in testdata keep it honest
// against the editor schema in interop/schema.mjs (docs/doc-room.md).
//
// It maps the schema, not the editor: nothing here knows ProseMirror, only the
// element names, attributes and mark attributes the schema produces.
package docroom

import "github.com/iml885203/code-x-artifacts/internal/docfmt"

// The shared types of a document in a room (docs/doc-room.md): OrderName is the
// array of tab ids in order, TabPrefix plus a tab id names the fragment that
// holds that tab, and Fragment is the one fragment that held every tab before
// tabs could move; a room that still has it is migrated when it is loaded.
const (
	OrderName = "tabs"
	TabPrefix = "tab:"
	Fragment  = "content"
)

// Elements of the room. A block is an element named by its docfmt type; the
// others are the parts the editor schema needs around them.
const (
	nodeTab       = "tab"
	nodeListItem  = "list_item"
	nodeRow       = "table_row"
	nodeCell      = "table_cell"
	nodeBreak     = "hard_break"
	nodeMention   = "mention"
	nodeDropdown  = "dropdown"
	attrConfig    = "config"
	nodeDate      = "date"
	attrDate      = "date"
	nodeCode      = "code_block" // a node and a mark cannot share a name in the editor's schema, and `code` is a mark
	attrID        = "id"
	attrTitle     = "title"
	attrLevel     = "level"
	attrStart     = "start"
	attrChecked   = "checked"
	attrLang      = "lang"
	attrHeader    = "header"
	attrAlign     = "align"
	attrSrc       = "src"
	attrAlt       = "alt"
	attrSize      = "size"
	attrIntent    = "intent"
	attrHref      = "href"
	attrUser      = "user"
	attrName      = "name"
	markSeparator = "--"
	ignoredMark   = "ychange"
)

// markOrder is the order a run's marks are read back in: a mark in the room is
// an attribute of the text, a set with no order.
var markOrder = []string{
	docfmt.MarkBold, docfmt.MarkItalic, docfmt.MarkUnderline, docfmt.MarkStrike,
	docfmt.MarkCode, docfmt.MarkHighlight, docfmt.MarkLink,
}
