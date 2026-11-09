package realtime_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/reearth/ygo/crdt"
	ygsync "github.com/reearth/ygo/sync"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
)

// fixture is what interop/generate.mjs writes for the cases the real yjs
// library produced, and what these tests write for the cases the Go room
// produced (testdata/go), which interop/check.mjs applies with yjs.
type fixture struct {
	Name    string   `json:"name"`
	Updates []string `json:"updates"`
	Expect  struct {
		Text string `json:"text,omitempty"`
		XML  string `json:"xml,omitempty"`
	} `json:"expect"`
}

func readFixtures(t *testing.T, dir string) map[string]fixture {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures in %s: %v", dir, err)
	}
	out := map[string]fixture{}
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // G304: paths come from a glob of testdata
		if err != nil {
			t.Fatal(err)
		}
		var fx fixture
		if err := json.Unmarshal(raw, &fx); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out[fx.Name] = fx
	}
	return out
}

// roomContent reads what the room holds, in the form the fixtures state it.
func roomContent(t *testing.T, e *env, room string) (text, xml string) {
	t.Helper()
	err := e.hub.Read(t.Context(), room, func(doc *crdt.Doc) error {
		text = doc.GetText("t").ToString()
		xml = doc.GetXmlFragment("content").ToXML()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return text, xml
}

func TestUpdatesFromTheRealYjsLibraryApplyInTheRoom(t *testing.T) {
	t.Parallel()
	for name, fx := range readFixtures(t, "testdata/yjs") {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, nil)
			room := e.room()
			editor, watcher := e.connect(room, alice), e.connect(room, reader)
			for i, u := range fx.Updates {
				raw, err := hex.DecodeString(u)
				if err != nil {
					t.Fatal(err)
				}
				// Each update arrives the way a browser sends it: as a sync update message.
				if err := editor.send(frame(0, ygsync.EncodeUpdate(raw))); err != nil {
					t.Fatalf("update %d of %s was refused: %v", i+1, name, err)
				}
			}
			settle(t, editor, watcher)

			text, xml := roomContent(t, e, room)
			if fx.Expect.Text != "" && text != fx.Expect.Text {
				t.Errorf("text = %q, yjs built %q", text, fx.Expect.Text)
			}
			if fx.Expect.XML != "" && xml != fx.Expect.XML {
				t.Errorf("fragment = %q, yjs built %q", xml, fx.Expect.XML)
			}
			// Another client, over the wire, ends with the same content as yjs built.
			if fx.Expect.Text != "" && watcher.text() != fx.Expect.Text {
				t.Errorf("a client of the room holds %q, yjs built %q", watcher.text(), fx.Expect.Text)
			}
			// And it is still the same after a restart of the hub.
			e.start()
			if text2, xml2 := roomContent(t, e, room); text2 != text || xml2 != xml {
				t.Errorf("after a restart: %q / %q, before: %q / %q", text2, xml2, text, xml)
			}
		})
	}
}

// goFixtures builds, through the room, the cases the JavaScript side applies
// with yjs. The client id is fixed so the bytes are reproducible.
func goFixtures(t *testing.T) map[string]fixture {
	t.Helper()
	build := func(name string, steps func(e *env, room string)) fixture {
		e := newEnv(t, func(c *realtime.Config) {
			c.NewClientID = func() crdt.ClientID { return 401 }
		})
		room := e.room()
		steps(e, room)
		fx := fixture{Name: name}
		for _, u := range e.seen() {
			fx.Updates = append(fx.Updates, hex.EncodeToString(u.Bytes))
		}
		text, xml := roomContent(t, e, room)
		if name == "text" {
			fx.Expect.Text = text
		} else {
			fx.Expect.XML = xml
		}
		return fx
	}
	edit := func(e *env, room string, fn func(doc *crdt.Doc) func(*crdt.Transaction)) {
		err := e.hub.Edit(t.Context(), room, alice, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
			return fn(doc), nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	textOps := func(e *env, room string) {
		insert := func(index int, s string) {
			edit(e, room, func(doc *crdt.Doc) func(*crdt.Transaction) {
				txt := doc.GetText("t")
				at := index
				if at < 0 {
					at = txt.Len()
				}
				return func(txn *crdt.Transaction) { txt.Insert(txn, at, s, nil) }
			})
		}
		insert(0, "Hello ")
		insert(6, "😀 世界 ")
		insert(-1, "𠮷野家")
		edit(e, room, func(doc *crdt.Doc) func(*crdt.Transaction) {
			txt := doc.GetText("t")
			return func(txn *crdt.Transaction) { txt.Delete(txn, 2, 2) }
		})
		insert(-1, "!")
	}
	xmlOps := func(e *env, room string) {
		edit(e, room, func(doc *crdt.Doc) func(*crdt.Transaction) {
			fragment := doc.GetXmlFragment("content")
			heading := crdt.NewYXmlElement("heading")
			paragraph := crdt.NewYXmlElement("paragraph")
			return func(txn *crdt.Transaction) {
				fragment.InsertElement(txn, 0, heading)
				heading.SetAttribute(txn, "id", "b1")
				heading.SetAttributeValue(txn, "level", 2)
				headingText := crdt.NewYXmlText()
				heading.InsertText(txn, 0, headingText)
				headingText.Insert(txn, 0, "標題 Title", nil)
				fragment.InsertElement(txn, 1, paragraph)
				paragraph.SetAttribute(txn, "id", "b2")
				paragraphText := crdt.NewYXmlText()
				paragraph.InsertText(txn, 0, paragraphText)
				paragraphText.Insert(txn, 0, "first line 😀", nil)
			}
		})
	}
	return map[string]fixture{
		"text": build("text", textOps),
		"xml":  build("xml", xmlOps),
	}
}

func TestUpdatesTheRoomProducedAreWhatTheCommittedFixturesHoldAndApplyInYjs(t *testing.T) {
	// The updates committed under testdata/go are applied by interop/check.mjs
	// with the real yjs library (make verify-interop). Here the room must
	// produce exactly those bytes, so the committed files are what the room does,
	// and the text it built is what yjs builds for the same edits.
	built := goFixtures(t) // once: building a fixture opens a store and a hub
	if os.Getenv("PICTOR_UPDATE_FIXTURES") != "" {
		for name, fx := range built {
			var raw bytes.Buffer
			enc := json.NewEncoder(&raw)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(fx); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join("testdata", "go", name+".json"), raw.Bytes(), 0o644); err != nil { //nolint:gosec // G306: a committed fixture, readable by everyone
				t.Fatal(err)
			}
		}
	}
	committed := readFixtures(t, "testdata/go")
	for name, fx := range built {
		want, ok := committed[name]
		if !ok {
			t.Errorf("no committed fixture %q; run with PICTOR_UPDATE_FIXTURES=1", name)
			continue
		}
		got, _ := json.Marshal(fx)
		wantJSON, _ := json.Marshal(want)
		if !bytes.Equal(got, wantJSON) {
			t.Errorf("fixture %q differs from the committed one (PICTOR_UPDATE_FIXTURES=1 rewrites it):\n got: %s\nwant: %s", name, got, wantJSON)
		}
	}
	// The same edits made in yjs (testdata/yjs) build the same text.
	yjs := readFixtures(t, "testdata/yjs")
	if got, want := committed["text"].Expect.Text, yjs["text"].Expect.Text; got != want {
		t.Errorf("the Go room built %q from the edits yjs built %q from", got, want)
	}
}
