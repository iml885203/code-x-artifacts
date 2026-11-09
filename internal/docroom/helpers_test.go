package docroom_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
)

// cases are the documents in testdata/cases, shared with the JavaScript side.
func cases(t *testing.T) map[string]docfmt.Doc {
	t.Helper()
	files, err := filepath.Glob("testdata/cases/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no cases: %v", err)
	}
	out := map[string]docfmt.Doc{}
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Clean(f))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := docfmt.Parse(string(raw))
		if err != nil {
			t.Fatalf("%s is not a valid doc: %v", f, err)
		}
		out[strings.TrimSuffix(filepath.Base(f), ".json")] = doc
	}
	return out
}

// jsonOf is the JSON of a doc: two docs are the same when this is.
func jsonOf(t *testing.T, d docfmt.Doc) string {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// roomHolding is a document that holds d, written the way the server writes.
func roomHolding(t *testing.T, d docfmt.Doc, clientID crdt.ClientID) *crdt.Doc {
	t.Helper()
	doc := crdt.New(crdt.WithClientID(clientID))
	c := docroom.Open(doc)
	doc.Transact(c.Write(d))
	return doc
}

func read(t *testing.T, doc *crdt.Doc) docfmt.Doc {
	t.Helper()
	got, err := docroom.Open(doc).Read()
	if err != nil {
		t.Fatalf("read the room: %v", err)
	}
	return got
}

// overTheWire is a copy of doc made the way a peer gets it: its encoded state
// applied to an empty document.
func overTheWire(t *testing.T, doc *crdt.Doc) *crdt.Doc {
	t.Helper()
	other := crdt.New()
	if err := crdt.ApplyUpdateV1(other, crdt.EncodeStateAsUpdateV1(doc, nil), nil); err != nil {
		t.Fatal(err)
	}
	return other
}
