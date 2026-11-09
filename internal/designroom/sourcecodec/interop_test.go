package sourcecodec_test

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designroom/sourcecodec"
	"github.com/reearth/ygo/crdt"
)

type wireFixture struct {
	Update     string            `json:"update"`
	Manifest   string            `json:"manifest"`
	Entries    map[string]string `json:"entries"`
	LiteralKey string            `json:"literalKey"`
	After      string            `json:"after"`
}

func readWire(t *testing.T, path string) wireFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var fixture wireFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestGoAndRealYjsAgreeOnTypedSourceUnitsAndUnicodeLiteralEdits(t *testing.T) {
	t.Parallel()
	input := fixture(t)
	doc := crdt.New(crdt.WithClientID(15001))
	defer doc.Destroy()
	if err := sourcecodec.Seed(doc, input); err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{}
	for key, value := range doc.GetMap("design").Entries() {
		actual, _ := doc.GetMap("design").Get(key)
		if text, ok := actual.(*crdt.YText); ok {
			entries[key] = text.ToString()
		} else {
			entries[key] = value.(string)
		}
	}
	got := wireFixture{Update: hex.EncodeToString(crdt.EncodeStateAsUpdateV1(doc, nil)), Manifest: input.Manifest, Entries: entries, LiteralKey: sourcecodec.SlotKey("frames/source.html", "literal", "label", 0, 0, "")}
	if os.Getenv("PICTOR_WRITE_SOURCE_FIXTURE") == "1" {
		raw, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("testdata/go.json", append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	expected := readWire(t, "testdata/go.json")
	if !reflect.DeepEqual(got, expected) {
		t.Fatal("Go boundary fixture differs")
	}
	yjs := readWire(t, "testdata/yjs.json")
	update, err := hex.DecodeString(yjs.Update)
	if err != nil {
		t.Fatal(err)
	}
	other := crdt.New()
	defer other.Destroy()
	if err := crdt.ApplyUpdateV1(other, update, nil); err != nil {
		t.Fatal(err)
	}
	if err := sourcecodec.CheckTransition(doc, other); err != nil {
		t.Fatal(err)
	}
	projected, err := sourcecodec.Read(other)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(input.Files["frames/source.html"], "Hello &amp; ", yjs.After, 1)
	if projected.Files["frames/source.html"] != want {
		t.Fatal("Yjs literal edit changed source bytes outside its run")
	}
}
