package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/boardedit"
	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

const boardEditDoc = `Change some elements of a board without sending all of it, so what people draw meanwhile stays: use this to
add, change or delete a few elements; publish is for a new board. read the board first: it gives every element's hash in
hashes. ops is a list, done in order, all or none: if one cannot be done, or the board would not be valid, nothing is changed.
Each op has op and:
 add: element, one element as publish takes it, including the skeleton's label on a shape and from and to on an arrow (ids of
   shapes already on the board or added earlier in the call). Without index it draws above everything.
 change: id, ifHash, and fields: the fields to set by name, as in publish ({"x": 120, "backgroundColor": "#ffc9c9"}); null
   removes a field. fields.label on a rectangle, ellipse or diamond sets the words of its label (adding one when it has none).
   id and type cannot change.
 delete: id, ifHash. The element goes with its label, and arrows bound to it are unbound.
ifHash (needed on change and delete): the hash read gave for the element. When the element is not what you read, because a
person changed it meanwhile, the whole call is refused and the elements as they are now come back with their hashes: merge
your change into them and send the call again. When people have the board open, only the fields you change are written into
what they see, at once, and you show there under your name; the result says saved false, and a version follows a couple of
minutes after the board goes quiet. Otherwise it is saved as a new version now. The result lists the elements written with
their new hashes, and the ids deleted.`

// boardOp is one operation as an agent sends it; element and fields are the values themselves.
type boardOp struct {
	Op      string         `json:"op" jsonschema:"add, change or delete"`
	ID      string         `json:"id,omitempty" jsonschema:"change and delete: the id of the element"`
	IfHash  string         `json:"ifHash,omitempty" jsonschema:"change and delete: the hash read gave for the element"`
	Element any            `json:"element,omitempty" jsonschema:"add: the element, as an object"`
	Fields  map[string]any `json:"fields,omitempty" jsonschema:"change: the fields to set by name; null removes one; label sets a shape's words"`
}

type boardEditIn struct {
	ID  string    `json:"id" jsonschema:"the id of the board"`
	Ops []boardOp `json:"ops" jsonschema:"the operations, done in order, all or none"`
}

type boardEditOut struct {
	service.BoardEdited
	Next string `json:"next"`
}

func (t *tools) boardEdit(ctx context.Context, _ *mcp.CallToolRequest, in boardEditIn) (*mcp.CallToolResult, boardEditOut, error) {
	ops := make([]boardedit.Op, len(in.Ops))
	for i, o := range in.Ops {
		op := boardedit.Op{Op: o.Op, ID: o.ID, IfHash: o.IfHash}
		if o.Element != nil {
			op.Element, _ = json.Marshal(o.Element) // a value the JSON-RPC layer decoded
		}
		if o.Fields != nil {
			op.Fields = map[string]json.RawMessage{}
			for name, v := range o.Fields {
				op.Fields[name], _ = json.Marshal(v) // as above
			}
		}
		ops[i] = op
	}
	res, err := t.e.svc.BoardEdit(t.asAgent(ctx), t.viewer(ctx), in.ID, ops)
	if err != nil {
		return nil, boardEditOut{}, t.fail("board_edit", err)
	}
	next := fmt.Sprintf("Saved as version %d.", res.Version)
	if !res.Saved {
		next = "Done in the live copy: people who have the board open see it now, and a version follows when it has been quiet for a couple of minutes."
	}
	return nil, boardEditOut{BoardEdited: res, Next: next}, nil
}

// boardHashes are a board's elements' hashes; nothing for another kind.
func boardHashes(a store.Artifact) map[string]string {
	if a.Kind != "board" {
		return nil
	}
	b, err := boardfmt.Parse(a.Content)
	if err != nil {
		return nil
	}
	return boardedit.Hashes(b)
}

// registerBoardEdit adds the tools of this feature to the server.
func (t *tools) registerBoardEdit(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "board_edit", Description: boardEditDoc, Annotations: &mcp.ToolAnnotations{Title: "Change some elements of a board", DestructiveHint: ptr(true), OpenWorldHint: ptr(false)}}, t.boardEdit)
}
