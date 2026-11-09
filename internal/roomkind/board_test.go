package roomkind_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/boardedit"
	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
	"github.com/iml885203/code-x-artifacts/internal/boardroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

// labelled is a board with one shape and its label.
const labelled = `{"version": 1, "elements": [
  {"id": "box", "type": "rectangle", "x": 0, "y": 0, "width": 200, "height": 100, "index": "a0", "boundElements": [{"id": "label", "type": "text"}]},
  {"id": "label", "type": "text", "x": 0, "y": 0, "width": 60, "height": 25, "text": "Start", "originalText": "Start",
   "containerId": "box", "textAlign": "center", "verticalAlign": "middle", "index": "a1"}
]}`

// board publishes a board as alice and returns its id.
func (e *env) board(content string) string {
	e.t.Helper()
	p, err := e.svc.Create(e.t.Context(), e.svc.Viewer("alice"), service.PublishInput{Title: "Flow", Kind: "board", Content: content})
	if err != nil {
		e.t.Fatal(err)
	}
	return p.ID
}

// setFields writes element fields into a board's room as who, the way an editor
// does: one entry per field, each its JSON.
func (e *env) setFields(id string, who realtime.Identity, fields map[string]string) error {
	return e.hub.Edit(e.t.Context(), id, who, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		m := doc.GetMap(boardroom.Map)
		return func(txn *crdt.Transaction) {
			for k, v := range fields {
				m.Set(txn, k, v)
			}
		}, nil
	})
}

// roomBoard is the board the room holds, in the stored form.
func (e *env) roomBoard(id string) string {
	e.t.Helper()
	var out string
	err := e.hub.Read(e.t.Context(), id, func(doc *crdt.Doc) error {
		b, err := boardroom.Read(doc)
		out = boardfmt.Encode(boardfmt.Canonical(b))
		return err
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return out
}

func (e *env) versionContent(id string, number int) string {
	e.t.Helper()
	v, err := e.st.Version(e.t.Context(), id, number)
	if err != nil {
		e.t.Fatal(err)
	}
	return v.Content
}

func elements(t *testing.T, content string) map[string]boardfmt.Element {
	t.Helper()
	b, err := boardfmt.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]boardfmt.Element{}
	for _, el := range b.Elements {
		out[el.ID()] = el
	}
	return out
}

func TestABoardsRoomStartsFromItsVersionAndAQuietRoomBecomesAVersionWithTheLabelOnItsShape(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.board(labelled)
	if got, want := e.roomBoard(id), e.versionContent(id, 1); got != want {
		t.Fatalf("the room holds\n%s\nthe version is\n%s", got, want)
	}
	// Alice moves the box; Bob retypes the label, and his editor writes the
	// label's x for the old box, as it does when the label's width changes.
	if err := e.setFields(id, alice, map[string]string{"box|x": "400", "box|y": "300"}); err != nil {
		t.Fatal(err)
	}
	if err := e.setFields(id, bob, map[string]string{"label|text": `"Begin"`, "label|originalText": `"Begin"`, "label|x": "70"}); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	if n := len(e.versions(id)); n != 2 {
		t.Fatalf("%d versions after the pause, want 2", n)
	}
	got := elements(t, e.versionContent(id, 2))
	if got["box"]["x"] != 400.0 || got["label"]["text"] != "Begin" {
		t.Errorf("the version does not hold both changes: %v", got)
	}
	if got["label"]["x"] != 470.0 || got["label"]["y"] != 337.5 {
		t.Errorf("the label is at %v,%v; the moved box centres it at 470,337.5", got["label"]["x"], got["label"]["y"])
	}
}

func TestWhatABoardMayNotHoldIsRefusedInItsRoomAndTheRoomKeepsWhatItHad(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.board(labelled)
	before := e.roomBoard(id)
	for name, fields := range map[string]map[string]string{
		"an embedded page":  {"e1|type": `"embeddable"`},
		"a javascript link": {"box|link": `"javascript:alert(1)"`},
		"a data URL":        {"box|groupIds": `["data:text/html,x"]`},
		"an unknown field":  {"box|customData": `{}`},
	} {
		if err := e.setFields(id, alice, fields); err == nil {
			t.Errorf("%s was kept", name)
		}
	}
	if got := e.roomBoard(id); got != before {
		t.Errorf("the room changed:\n%s", got)
	}
}

func TestAWholeBoardPublishWithTheRoomOpenGoesIntoTheRoomAndIsTheVersion(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.board(labelled)
	_ = e.roomBoard(id)
	next := `{"version": 1, "elements": [
  {"id": "box", "type": "rectangle", "x": 0, "y": 0, "width": 200, "height": 100, "index": "a0", "boundElements": [{"id": "label", "type": "text"}]},
  {"id": "label", "type": "text", "width": 60, "height": 25, "text": "Go", "originalText": "Go",
   "containerId": "box", "textAlign": "center", "verticalAlign": "middle", "index": "a1"},
  {"id": "end", "type": "ellipse", "x": 300, "y": 0, "width": 100, "height": 100, "index": "a2"}
]}`
	p, err := e.svc.Update(t.Context(), e.svc.Viewer("alice"), id, service.PublishInput{Title: "Flow", Content: next, BaseVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 2 {
		t.Errorf("version = %d", p.Version)
	}
	if room := e.roomBoard(id); room != e.versionContent(id, 2) {
		t.Errorf("the room holds\n%s\nversion 2 is\n%s", room, e.versionContent(id, 2))
	}
	if got := elements(t, e.roomBoard(id)); got["label"]["text"] != "Go" || got["end"] == nil {
		t.Errorf("the room holds %v", got)
	}
	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	if n := len(e.versions(id)); n != 2 {
		t.Errorf("%d versions after the room went quiet, want 2", n)
	}
}

func TestAnAgentsBoardEditChangesOneBoxsWordsWhileAPersonMovesAnotherAndBothStay(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.board(`{"version": 1, "elements": [
	  {"id": "a", "type": "rectangle", "x": 0, "y": 0, "label": "Start"},
	  {"id": "b", "type": "rectangle", "x": 300, "y": 0, "label": "End"}]}`)
	read := e.roomBoard(id)
	b, err := boardfmt.Parse(read)
	if err != nil {
		t.Fatal(err)
	}
	hashes := boardedit.Hashes(b)
	// A person moves b after the agent read the board.
	if err := e.setFields(id, bob, map[string]string{"b|x": "640"}); err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.BoardEdit(t.Context(), e.svc.Viewer("alice"), id, []boardedit.Op{
		{Op: "change", ID: "a", IfHash: hashes["a"], Fields: map[string]json.RawMessage{"label": json.RawMessage(`"Begin"`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Saved {
		t.Error("the edit was saved beside the open room")
	}
	got := elements(t, e.roomBoard(id))
	if got["b"]["x"] != 640.0 || got["a-label"]["text"] != "Begin" {
		t.Errorf("the room holds b at %v and a's label %v", got["b"]["x"], got["a-label"]["text"])
	}
	// The person's move made b stale for the agent: a change of b with the hash read before is refused.
	_, err = e.svc.BoardEdit(t.Context(), e.svc.Viewer("alice"), id, []boardedit.Op{
		{Op: "change", ID: "b", IfHash: hashes["b"], Fields: map[string]json.RawMessage{"x": json.RawMessage(`10`)}},
	})
	var refusal *service.Error
	if !errors.As(err, &refusal) || refusal.Code != service.Conflict || refusal.StaleElements["b"].Element["x"] != 640.0 {
		t.Errorf("a stale change = %v", err)
	}
	if got := elements(t, e.roomBoard(id)); got["b"]["x"] != 640.0 {
		t.Error("the refused change was written")
	}
}
