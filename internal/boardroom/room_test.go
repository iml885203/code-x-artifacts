package boardroom

import (
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
)

func parse(t *testing.T, content string) boardfmt.Board {
	t.Helper()
	b, err := boardfmt.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	return boardfmt.Canonical(b)
}

func seeded(t *testing.T, b boardfmt.Board) *crdt.Doc {
	t.Helper()
	state, err := State(b)
	if err != nil {
		t.Fatal(err)
	}
	doc := crdt.New()
	if err := crdt.ApplyUpdateV1(doc, state, nil); err != nil {
		t.Fatal(err)
	}
	return doc
}

func version(t *testing.T, doc *crdt.Doc) string {
	t.Helper()
	b, err := Read(doc)
	if err != nil {
		t.Fatal(err)
	}
	return boardfmt.Encode(boardfmt.Canonical(b))
}

const twoBoxes = `{"version": 1, "elements": [
  {"id": "a", "type": "rectangle", "x": 0, "y": 0, "width": 200, "height": 100, "index": "a0", "boundElements": [{"id": "la", "type": "text"}]},
  {"id": "la", "type": "text", "x": 1, "y": 1, "width": 60, "height": 25, "text": "Start", "containerId": "a", "textAlign": "center", "verticalAlign": "middle", "index": "a1"},
  {"id": "b", "type": "ellipse", "x": 300, "y": 0, "width": 100, "height": 100, "index": "a2"}
]}`

func TestARoomSeededFromABoardGivesTheSameBoardBack(t *testing.T) {
	t.Parallel()
	b := parse(t, twoBoxes)
	doc := seeded(t, b)
	if err := Check(doc); err != nil {
		t.Fatalf("a seeded room fails its own check: %v", err)
	}
	if got, want := version(t, doc), boardfmt.Encode(b); got != want {
		t.Errorf("room gives back\n%s\nwant\n%s", got, want)
	}
}

func TestWritingABoardIntoARoomChangesOnlyWhatDiffersAndMarksWhatIsGoneDeleted(t *testing.T) {
	t.Parallel()
	doc := seeded(t, parse(t, twoBoxes))
	m := doc.GetMap(Map)
	// Someone moved b in the room; the board written next changes only a's label.
	doc.Transact(func(txn *crdt.Transaction) { m.Set(txn, "b|x", "320") })
	next := parse(t, `{"version": 1, "elements": [
  {"id": "a", "type": "rectangle", "x": 0, "y": 0, "width": 200, "height": 100, "index": "a0", "boundElements": [{"id": "la", "type": "text"}]},
  {"id": "la", "type": "text", "x": 1, "y": 1, "width": 60, "height": 25, "text": "Begin", "containerId": "a", "textAlign": "center", "verticalAlign": "middle", "index": "a1"},
  {"id": "b", "type": "ellipse", "x": 320, "y": 0, "width": 100, "height": 100, "index": "a2"},
  {"id": "c", "type": "diamond", "x": 0, "y": 200, "width": 80, "height": 80, "index": "a3"}
]}`)
	var changed []string
	m.Observe(func(e crdt.YMapEvent) {
		for k := range e.KeysChanged {
			changed = append(changed, k)
		}
	})
	write, err := Write(doc, next)
	if err != nil {
		t.Fatal(err)
	}
	doc.Transact(write)
	for _, k := range changed {
		if k[:2] == "b|" || k[:2] == "a|" {
			t.Errorf("%s was written; nothing of it changed", k)
		}
	}
	if got := version(t, doc); got != boardfmt.Encode(next) {
		t.Errorf("room holds\n%s\nwant\n%s", got, boardfmt.Encode(next))
	}

	// A board without c: c stays in the room, deleted, so open editors drop it.
	write, err = Write(doc, parse(t, twoBoxes))
	if err != nil {
		t.Fatal(err)
	}
	doc.Transact(write)
	if v, _ := m.Get("c|isDeleted"); v != "true" {
		t.Errorf("c|isDeleted = %v", v)
	}
	if got := version(t, doc); got != boardfmt.Encode(parse(t, twoBoxes)) {
		t.Errorf("room holds\n%s", got)
	}
	// Bringing c back undeletes it.
	write, _ = Write(doc, next)
	doc.Transact(write)
	if v, _ := m.Get("c|isDeleted"); v != "false" {
		t.Errorf("c|isDeleted after it came back = %v", v)
	}
}

func TestARoomHoldingWhatNoBoardCanIsReplacedByTheBoardWrittenIntoIt(t *testing.T) {
	t.Parallel()
	doc := write(t, map[string]string{"x|type": `"iframe"`, "x|link": `"https://example.com"`})
	b := parse(t, twoBoxes)
	w, err := Write(doc, b)
	if err != nil {
		t.Fatal(err)
	}
	doc.Transact(w)
	if err := Check(doc); err != nil {
		t.Fatalf("still refused: %v", err)
	}
	if got := version(t, doc); got != boardfmt.Encode(b) {
		t.Errorf("room holds %s", got)
	}
}

func TestMergingAnAgentsChangeKeepsWhatAPersonChangedInTheRoomSince(t *testing.T) {
	t.Parallel()
	base := parse(t, twoBoxes)
	doc := seeded(t, base)
	m := doc.GetMap(Map)
	// A person moves b after the agent read the board.
	doc.Transact(func(txn *crdt.Transaction) { m.Set(txn, "b|x", "999") })
	next := parse(t, strings.Replace(twoBoxes, `"text": "Start"`, `"text": "Go"`, 1))
	write, touched, err := Merge(doc, base, next)
	if err != nil {
		t.Fatal(err)
	}
	doc.Transact(write)
	if v, _ := m.Get("b|x"); v != "999" {
		t.Errorf("the person's move was undone: b|x = %v", v)
	}
	if v, _ := m.Get("la|text"); v != `"Go"` {
		t.Errorf("la|text = %v", v)
	}
	if strings.Join(touched, ",") != "la" {
		t.Errorf("touched = %v", touched)
	}
	// An element the agent's board no longer has is marked deleted, not removed.
	gone := boardfmt.Board{Version: 1}
	for _, el := range next.Elements {
		if el.ID() != "b" {
			gone.Elements = append(gone.Elements, el)
		}
	}
	write, _, err = Merge(doc, next, gone)
	if err != nil {
		t.Fatal(err)
	}
	doc.Transact(write)
	if v, _ := m.Get("b|isDeleted"); v != "true" {
		t.Errorf("b|isDeleted = %v", v)
	}
}
