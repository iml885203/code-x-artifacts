package docroom_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
)

// fixture is a document as an update, which is what both sides exchange.
type fixture struct {
	Name   string `json:"name"`
	Update string `json:"update"`
}

func readFixture(t *testing.T, path string) fixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("%v; testdata/yjs is made by interop/generate.mjs (see docs/doc-room.md)", err)
	}
	var fx fixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	return fx
}

// The documents under testdata/yjs are what the editor's schema writes through
// y-prosemirror (interop/generate.mjs; make verify-interop checks they are
// what it produces now). Go has to read what the editor wrote.
func TestGoReadsWhatTheEditorSchemaWrote(t *testing.T) {
	t.Parallel()
	for name, want := range cases(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fx := readFixture(t, filepath.Join("testdata", "yjs", name+".json"))
			update, err := hex.DecodeString(fx.Update)
			if err != nil {
				t.Fatal(err)
			}
			doc := crdt.New()
			if err := crdt.ApplyUpdateV1(doc, update, nil); err != nil {
				t.Fatalf("the editor's document does not apply: %v", err)
			}
			if got, want := jsonOf(t, read(t, doc)), jsonOf(t, want); got != want {
				t.Errorf("Go reads:\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

// goFixtures are the cases as Go writes them, with a fixed client id so the
// bytes are the same every time. interop/check.mjs reads them with the
// editor's schema.
func goFixtures(t *testing.T) map[string]fixture {
	t.Helper()
	cs := cases(t)
	names := make([]string, 0, len(cs))
	for name := range cs {
		names = append(names, name)
	}
	sort.Strings(names)
	out := map[string]fixture{}
	for i, name := range names {
		doc := roomHolding(t, cs[name], crdt.ClientID(7000+i))
		out[name] = fixture{Name: name, Update: hex.EncodeToString(crdt.EncodeStateAsUpdateV1(doc, nil))}
	}
	return out
}

func TestTheDocumentsGoWritesAreTheCommittedFixturesTheEditorSchemaReads(t *testing.T) {
	// testdata/go is applied and read by interop/check.mjs with the editor's
	// schema (make verify-interop). Here Go must produce exactly those bytes, so
	// the committed files are what Go writes.
	if os.Getenv("PICTOR_UPDATE_FIXTURES") != "" {
		for name, fx := range goFixtures(t) {
			var raw bytes.Buffer
			enc := json.NewEncoder(&raw)
			enc.SetIndent("", "  ")
			if err := enc.Encode(fx); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join("testdata", "go", name+".json"), raw.Bytes(), 0o644); err != nil { //nolint:gosec // G306: a committed fixture, readable by everyone
				t.Fatal(err)
			}
		}
	}
	if os.Getenv("PICTOR_UPDATE_FIXTURES") != "" {
		var raw bytes.Buffer
		enc := json.NewEncoder(&raw)
		enc.SetIndent("", "  ")
		if err := enc.Encode(goMoveFixture(t)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join("testdata", "go", moveName+".json"), raw.Bytes(), 0o644); err != nil { //nolint:gosec // G306: a committed fixture, readable by everyone
			t.Fatal(err)
		}
	}
	committedMove, err := os.ReadFile(filepath.Join("testdata", "go", moveName+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var move moveFixture
	if err := json.Unmarshal(committedMove, &move); err != nil || move != goMoveFixture(t) {
		t.Errorf("the fixture %q differs from what Go writes now (PICTOR_UPDATE_FIXTURES=1 go test ./internal/docroom rewrites it): %v", moveName, err)
	}
	for name, fx := range goFixtures(t) {
		committed := readFixture(t, filepath.Join("testdata", "go", name+".json"))
		if committed != fx {
			t.Errorf("the fixture %q differs from what Go writes now (PICTOR_UPDATE_FIXTURES=1 go test ./internal/docroom rewrites it)", name)
		}
	}
}

// moveFixture is a document of three tabs and the update that moves the last to
// the front, as Go writes it. interop/check.mjs merges it with a person's typing
// in the moved tab (TestGoReadsWhatTheEditorSchemaWrote has the other direction).
type moveFixture struct {
	Name string `json:"name"`
	Base string `json:"base"`
	Move string `json:"move"`
}

const moveName = "move-while-typing"

func threeTabs() docfmt.Doc {
	d := docfmt.Doc{}
	for _, id := range []string{"t1", "t2", "t3"} {
		d.Tabs = append(d.Tabs, docfmt.Tab{ID: id, Title: strings.ToUpper(id), Blocks: []docfmt.Block{{ID: "b_" + id, Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: "text of " + id}}}}})
	}
	return d
}

func goMoveFixture(t *testing.T) moveFixture {
	t.Helper()
	doc := roomHolding(t, threeTabs(), 7100)
	base := crdt.EncodeStateAsUpdateV1(doc, nil)
	moved := threeTabs()
	moved.Tabs = []docfmt.Tab{moved.Tabs[2], moved.Tabs[0], moved.Tabs[1]}
	write, err := docroom.Open(doc).Apply(moved)
	if err != nil {
		t.Fatal(err)
	}
	before := doc.StateVector()
	doc.Transact(write)
	return moveFixture{Name: moveName, Base: hex.EncodeToString(base), Move: hex.EncodeToString(crdt.EncodeStateAsUpdateV1(doc, before))}
}

// editorMove is what the editor's two people made from one start: one moved a tab, the other typed in it.
type editorMove struct {
	Base   string `json:"base"`
	Move   string `json:"move"`
	Typing string `json:"typing"`
}

func readMoveFixture(t *testing.T) editorMove {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "yjs", moveName+".json"))
	if err != nil {
		t.Fatalf("%v; testdata/yjs is made by interop/generate.mjs (see docs/doc-room.md)", err)
	}
	var fx editorMove
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	return fx
}
