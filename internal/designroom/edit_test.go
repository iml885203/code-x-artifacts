package designroom_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

const heroFile = "frames/hero.html"

// replaceOnce is the text with exactly one change, so a test can say the whole
// file it expects instead of looking at a few places in it.
func replaceOnce(t *testing.T, text, old, now string) string {
	t.Helper()
	if strings.Count(text, old) != 1 {
		t.Fatalf("%q is in the text %d times, not once", old, strings.Count(text, old))
	}
	return strings.Replace(text, old, now, 1)
}

func TestMovingAndResizingChangesOnlyTheDeclarationsTheEditorOwnsInTheSavedFile(t *testing.T) {
	t.Parallel()
	frames := map[string]string{"frames/messy.html": messy(t), heroFile: normal(t, hero)}
	// Each case is an element, the owned declarations it has, and what the editor writes; the file after is the file before with exactly those declarations replaced.
	cases := []struct {
		file, marker string
		set          [][2]string
		from, to     string
	}{
		{"frames/messy.html", `class="badge"`, [][2]string{{"left", "100px"}, {"top", "330px"}}, "left:48px;top:300px;", "left:100px;top:330px;"},
		{"frames/messy.html", `class="badge"`, [][2]string{{"width", "10px"}}, "width:220px;background", "width:10px;background"},
		{heroFile, `id="box"`, [][2]string{{"left", "11px"}, {"top", "22px"}, {"width", "33px"}, {"height", "44px"}}, "left:10px;top:20px;width:30px;height:40px;color:red", "left:11px;top:22px;width:33px;height:44px;color:red"},
		{heroFile, `id="box"`, [][2]string{{"height", ""}}, "height:40px;color:red", "color:red"},
	}
	for _, c := range cases {
		doc := peer(t, 1, seeded(t, designroom.Design{Manifest: manifest, Frames: frames}))
		before := read(t, doc)
		id := pidOf(t, before.Frames[c.file], c.marker)
		for _, kv := range c.set {
			edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.SetStyle(c.file, id, kv[0], kv[1]) })
		}
		after := read(t, doc)
		if want := replaceOnce(t, before.Frames[c.file], c.from, c.to); after.Frames[c.file] != want {
			t.Errorf("%s %s: the saved file differs from the file before in more than the owned declarations:\nwant %s\ngot  %s", c.file, c.marker, want, after.Frames[c.file])
		}
		for file, text := range before.Frames {
			if file != c.file && after.Frames[file] != text {
				t.Errorf("%s changed when %s was edited", file, c.file)
			}
		}
		if after.Manifest != before.Manifest {
			t.Error("the manifest changed")
		}
	}
}

func TestDeletingAnElementTakesAwayItAndEverythingInsideItAndNothingElse(t *testing.T) {
	t.Parallel()
	d := heroDesign(t)
	doc := peer(t, 1, seeded(t, d))
	before := read(t, doc).Frames[heroFile]
	box := pidOf(t, before, `id="box"`)
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Delete(heroFile, box) })
	after := read(t, doc).Frames[heroFile]
	start := strings.Index(before, "<div id=\"box\"")
	end := strings.Index(before, "</div>") + len("</div>")
	if want := before[:start] + before[end:]; after != want {
		t.Errorf("after the delete:\nwant %s\ngot  %s", want, after)
	}
	for k := range doc.GetMap(designroom.Map).Entries() {
		if strings.HasPrefix(k, heroFile+"|n|"+box+"|") {
			t.Errorf("an entry of the deleted element is left: %s", k)
		}
	}
	// What was inside it (the paragraph, its bold text, its text nodes) left no entry behind either.
	for k := range doc.GetMap(designroom.Map).Entries() {
		if strings.Contains(k, "|n|") && strings.HasSuffix(k, "|text") {
			if v, _ := doc.GetMap(designroom.Map).Get(k); v != nil && strings.Contains(v.(*crdt.YText).ToString(), "Hello") {
				t.Errorf("text of the deleted element is left: %s", k)
			}
		}
	}
}

func TestTheBodyAndWhatIsNotThereCannotBeDeleted(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, heroDesign(t)))
	frame := read(t, doc).Frames[heroFile]
	r := designroom.Open(doc)
	if _, err := r.Delete(heroFile, pidOf(t, frame, "<body")); err == nil {
		t.Error("the body was deleted")
	}
	if _, err := r.Delete(heroFile, "p999"); err == nil {
		t.Error("an element that is not there was deleted")
	}
	if _, err := r.Delete("frames/none.html", "p1"); err == nil {
		t.Error("an element of a frame that is not there was deleted")
	}
	if read(t, doc).Frames[heroFile] != frame {
		t.Error("a refused delete changed the frame")
	}
}

func TestAStyleChangeInsideAnElementSomeoneDeletedLeavesTheFrameReadableAndWithoutIt(t *testing.T) {
	t.Parallel()
	d := heroDesign(t)
	state := seeded(t, d)
	a, b := peer(t, 11, state), peer(t, 22, state)
	frame := d.Frames[heroFile]
	box, para := pidOf(t, frame, `id="box"`), pidOf(t, frame, "<p")
	edit(t, a, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Delete(heroFile, box) })
	edit(t, b, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, para, "left", "5px")
	})
	sync(t, a, b)
	fa, fb := read(t, a).Frames[heroFile], read(t, b).Frames[heroFile]
	if fa != fb || strings.Contains(fa, `id="box"`) || strings.Contains(fa, "left:5px") {
		t.Errorf("peers: %s | %s", fa, fb)
	}
}

// The browser's editor (web/src/design/edits.ts) writes the room's map directly,
// with these literal keys; its tests spell the same ones. If this test and that
// one drift apart, an edit would be written where nothing reads it.
func TestTheEntriesTheBrowsersEditorWritesAreTheOnesTheRoomReads(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, heroDesign(t)))
	frame := read(t, doc).Frames[heroFile]
	box := pidOf(t, frame, `id="box"`)
	m := doc.GetMap(designroom.Map)
	doc.Transact(func(txn *crdt.Transaction) {
		m.Set(txn, heroFile+"|n|"+box+"|s|left", " 110px") // the old value's spacing kept, as edits.ts does
		m.Set(txn, heroFile+"|n|"+box+"|s|top", "25px")
	})
	got := read(t, doc).Frames[heroFile]
	if want := replaceOnce(t, frame, "left:10px;top:20px;", "left: 110px;top:25px;"); got != want {
		t.Errorf("want %s\ngot  %s", want, got)
	}
}

// The outline writes a rename and a hide the way these tests do, straight into
// the room's map (web/src/design/outline.ts): a name is one attribute in the
// element's attribute list, a hide is the owned `display` property.
func TestRenamingAnElementChangesOnlyItsDataNameAndHidingItWritesDisplayNoneAndKeepsIt(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, heroDesign(t)))
	before := read(t, doc).Frames[heroFile]
	box := pidOf(t, before, `id="box"`)
	m := doc.GetMap(designroom.Map)
	attrsKey := heroFile + "|n|" + box + "|attrs"
	old, _ := m.Get(attrsKey)
	var rows [][3]string
	if err := json.Unmarshal([]byte(old.(string)), &rows); err != nil {
		t.Fatal(err)
	}
	rows = append(rows, [3]string{"", "data-name", "Hero card"})
	next, _ := json.Marshal(rows)
	doc.Transact(func(txn *crdt.Transaction) { m.Set(txn, attrsKey, string(next)) })

	renamed := read(t, doc).Frames[heroFile]
	if got := strings.Replace(renamed, ` data-name="Hero card"`, "", 1); got != before || got == renamed {
		t.Errorf("a rename changed more than the data-name attribute:\nbefore  %s\nrenamed %s", before, renamed)
	}

	doc.Transact(func(txn *crdt.Transaction) { m.Set(txn, heroFile+"|n|"+box+"|s|display", "none") })
	hidden := read(t, doc).Frames[heroFile]
	if want := replaceOnce(t, renamed, "color:red", "color:red;display:none"); hidden != want {
		t.Errorf("hiding:\nwant %s\ngot  %s", want, hidden)
	}
	if !strings.Contains(hidden, "Hello") || !strings.Contains(hidden, `id="box"`) {
		t.Error("the hidden element is not in the file any more")
	}

	// Showing it again takes the declaration away and leaves the rest.
	doc.Transact(func(txn *crdt.Transaction) { m.Delete(txn, heroFile+"|n|"+box+"|s|display") })
	if shown := read(t, doc).Frames[heroFile]; shown != renamed {
		t.Errorf("showing it again:\nwant %s\ngot  %s", renamed, shown)
	}
}
