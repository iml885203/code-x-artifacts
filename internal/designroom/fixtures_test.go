package designroom_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

// A richCase is one element of formatted text: the markup it is, and the delta
// of the Y.Text that holds it in the room (docs/design-room.md). Both sides
// read the same file: Go writes the room and yjs must read the delta from it,
// and yjs writes the delta and Go must read the markup from it.
type richCase struct {
	Name       string            `json:"name"`
	Body       string            `json:"body"`
	PID        string            `json:"pid"`
	Delta      []json.RawMessage `json:"delta"`
	Edits      json.RawMessage   `json:"edits"`
	AfterEdits string            `json:"afterEdits"`
}

func richCases(t *testing.T) []richCase {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "cases", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no cases: %v", err)
	}
	sort.Strings(files)
	var out []richCase
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Clean(f))
		if err != nil {
			t.Fatal(err)
		}
		var c richCase
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out = append(out, c)
	}
	return out
}

type update struct {
	Name   string `json:"name"`
	Update string `json:"update"`
}

func readUpdate(t *testing.T, path string) update {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var u update
	if err := json.Unmarshal(raw, &u); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return u
}

// deltaOf is the delta of a text as JSON values, the way the case files hold it.
func deltaOf(t *testing.T, text *crdt.YText) []any {
	t.Helper()
	var out []any
	for _, op := range text.ToDelta() {
		m := map[string]any{"insert": op.Insert}
		if len(op.Attributes) > 0 {
			m["attributes"] = map[string]any(op.Attributes)
		}
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

func caseDelta(t *testing.T, c richCase) []any {
	t.Helper()
	var out []any
	for _, raw := range c.Delta {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

// The delta of a case is what Go writes into the room for its markup: this is
// the encoding, written down once for the editor that will write it too.
func TestTheFormattedTextOfEachCaseIsTheDeltaTheCaseHolds(t *testing.T) {
	t.Parallel()
	for _, c := range richCases(t) {
		d := oneFrame(t, c.Body)
		doc := peer(t, 1, seeded(t, d))
		text := richOf(doc, c.PID)
		if text == nil {
			t.Errorf("%s: no formatted text", c.Name)
			continue
		}
		if got, want := deltaOf(t, text), caseDelta(t, c); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: Go writes\n %v\nthe case holds\n %v", c.Name, got, want)
		}
	}
}

// What the browser editor writes is read by Go as the markup of the case: the
// updates under testdata/yjs are what the real yjs library produces for each
// delta (interop/generate.mjs; make verify-interop checks they are what it
// produces now).
func TestGoReadsTheFormattedTextYjsWrote(t *testing.T) {
	t.Parallel()
	for _, c := range richCases(t) {
		u := readUpdate(t, filepath.Join("testdata", "yjs", c.Name+".json"))
		raw, err := hex.DecodeString(u.Update)
		if err != nil {
			t.Fatal(err)
		}
		doc := crdt.New()
		if err := crdt.ApplyUpdateV1(doc, raw, nil); err != nil {
			t.Fatalf("%s: the update from yjs does not apply: %v", c.Name, err)
		}
		if got, want := read(t, doc).Frames[heroFile], frameOf(t, c.Body); got != want {
			t.Errorf("%s: Go reads\n %q\nwant\n %q", c.Name, got, want)
		}
		if err := designroom.Check(doc); err != nil {
			t.Errorf("%s: the room refuses what yjs wrote: %v", c.Name, err)
		}
	}
}

// innerBody is what is inside the body of a frame (its own pid is not the point here).
func innerBody(frame string) string {
	_, rest, _ := strings.Cut(frame, "<body")
	_, rest, _ = strings.Cut(rest, ">")
	inner, _, _ := strings.Cut(rest, "</body>")
	return inner
}

// Two clients make their changes at once, each in its own copy, and yjs merges
// them: Go reads the merge as the markup with both changes in it.
func TestGoReadsWhatYjsMergedWhenTwoPeopleChangedDifferentWords(t *testing.T) {
	t.Parallel()
	var found bool
	for _, c := range richCases(t) {
		if c.AfterEdits == "" {
			continue
		}
		found = true
		u := readUpdate(t, filepath.Join("testdata", "yjs", c.Name+".merged.json"))
		raw, err := hex.DecodeString(u.Update)
		if err != nil {
			t.Fatal(err)
		}
		doc := crdt.New()
		if err := crdt.ApplyUpdateV1(doc, raw, nil); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if got, want := innerBody(read(t, doc).Frames[heroFile]), c.AfterEdits; got != want {
			t.Errorf("%s: Go reads\n %q\nwant\n %q", c.Name, got, want)
		}
	}
	if !found {
		t.Error("no case has edits")
	}
}

// goFixtures are the rooms Go writes for the cases, with a client id fixed so
// the bytes are the same every time; interop/check.mjs reads them with yjs and
// interop/generate.mjs starts the yjs side from them.
func goFixtures(t *testing.T) map[string]update {
	t.Helper()
	out := map[string]update{}
	for i, c := range richCases(t) {
		d := oneFrame(t, c.Body)
		doc := crdt.New(crdt.WithClientID(crdt.ClientID(7100 + i)))
		write, err := designroom.Open(doc).Write(d)
		if err != nil {
			t.Fatal(err)
		}
		doc.Transact(write)
		out[c.Name] = update{Name: c.Name, Update: hex.EncodeToString(crdt.EncodeStateAsUpdateV1(doc, nil))}
	}
	// The pages and frames, which the structure interop reads and changes.
	doc := crdt.New(crdt.WithClientID(7200))
	write, err := designroom.Open(doc).Write(twoPageDesign(t))
	if err != nil {
		t.Fatal(err)
	}
	doc.Transact(write)
	out["structure"] = update{Name: "structure", Update: hex.EncodeToString(crdt.EncodeStateAsUpdateV1(doc, nil))}
	out["stroke"] = strokeFixture(t)
	return out
}

// Two people restructure the design at once with the real yjs library, from the
// room Go wrote (interop/generate.mjs): one renames and resizes a frame, the
// other puts it in a row of its own and renames the other page. Go reads the
// merge as one design with all of it, which the hub accepts.
func TestGoReadsWhatYjsMergedWhenTwoPeopleRestructuredAtOnce(t *testing.T) {
	t.Parallel()
	u := readUpdate(t, filepath.Join("testdata", "yjs", "structure.merged.json"))
	raw, err := hex.DecodeString(u.Update)
	if err != nil {
		t.Fatal(err)
	}
	doc := crdt.New()
	if err := crdt.ApplyUpdateV1(doc, raw, nil); err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"pages":[` +
		`{"id":"p","name":"P","rows":[[{"id":"hero","name":"Banner","file":"frames/hero.html","w":390,"h":600}]]},` +
		`{"id":"q","name":"Footer","rows":[[{"id":"foot","name":"Foot","file":"frames/foot.html","w":800,"h":200}]]}]}`
	got := read(t, doc)
	if got.Manifest != want {
		t.Errorf("Go reads\n %s\nwant\n %s", got.Manifest, want)
	}
	if err := designfmt.Validate(got.Manifest, got.Frames); err != nil {
		t.Error(err)
	}
	if err := designroom.Check(doc); err != nil {
		t.Errorf("the room refuses what yjs merged: %v", err)
	}
}

func TestTheRoomsGoWritesAreTheCommittedFixturesYjsReads(t *testing.T) {
	if os.Getenv("PICTOR_UPDATE_FIXTURES") != "" {
		for name, u := range goFixtures(t) {
			var raw bytes.Buffer
			enc := json.NewEncoder(&raw)
			enc.SetIndent("", "  ")
			if err := enc.Encode(u); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join("testdata", "go", name+".json"), raw.Bytes(), 0o644); err != nil { //nolint:gosec // G306: a committed fixture, readable by everyone
				t.Fatal(err)
			}
		}
	}
	for name, u := range goFixtures(t) {
		if committed := readUpdate(t, filepath.Join("testdata", "go", name+".json")); committed != u {
			t.Errorf("the fixture %q differs from what Go writes now (PICTOR_UPDATE_FIXTURES=1 go test ./internal/designroom rewrites it)", name)
		}
	}
}
