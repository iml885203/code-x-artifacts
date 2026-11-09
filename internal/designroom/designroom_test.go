package designroom_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

const manifest = `{"version":1,"pages":[{"id":"p","name":"P","rows":[[{"id":"hero","name":"Hero","file":"frames/hero.html","w":800,"h":600}]]}]}`

func normal(t *testing.T, text string) string {
	t.Helper()
	out, err := designfmt.Normalize(text)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func messy(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../designfmt/testdata/messy.html")
	if err != nil {
		t.Fatal(err)
	}
	return normal(t, string(b))
}

// peer is a replica of the room, with the client id fixed so runs are the same.
func peer(t *testing.T, id crdt.ClientID, state []byte) *crdt.Doc {
	t.Helper()
	doc := crdt.New(crdt.WithClientID(id))
	if state != nil {
		if err := crdt.ApplyUpdateV1(doc, state, nil); err != nil {
			t.Fatal(err)
		}
	}
	return doc
}

func seeded(t *testing.T, d designroom.Design) []byte {
	t.Helper()
	state, err := designroom.State(d)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// sync makes every replica hold what all of them hold, as a server relaying updates would.
func sync(t *testing.T, docs ...*crdt.Doc) {
	t.Helper()
	for range 2 {
		for i, from := range docs {
			for j, to := range docs {
				if i == j {
					continue
				}
				diff := crdt.EncodeStateAsUpdateV1(from, to.StateVector())
				if err := crdt.ApplyUpdateV1(to, diff, nil); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func edit(t *testing.T, doc *crdt.Doc, plan func(r *designroom.Room) (func(*crdt.Transaction), error)) {
	t.Helper()
	write, err := plan(designroom.Open(doc))
	if err != nil {
		t.Fatal(err)
	}
	doc.Transact(write)
}

func read(t *testing.T, doc *crdt.Doc) designroom.Design {
	t.Helper()
	d, err := designroom.Open(doc).Read()
	if err != nil {
		t.Fatalf("read the room: %v", err)
	}
	return d
}

const hero = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0"><div id="box" style="position:absolute;left:10px;top:20px;width:30px;height:40px;color:red"><p>Hello <b>big</b> world</p></div><section><h1>Title</h1></section></body></html>`

func heroDesign(t *testing.T) designroom.Design {
	return designroom.Design{Manifest: manifest, Frames: map[string]string{"frames/hero.html": normal(t, hero)}}
}

// pidOf finds the data-pid of the first element whose start tag contains marker.
func pidOf(t *testing.T, frame, marker string) string {
	t.Helper()
	i := strings.Index(frame, marker)
	if i < 0 {
		t.Fatalf("no %q in %s", marker, frame)
	}
	end := strings.Index(frame[i:], ">")
	tag := frame[i : i+end]
	j := strings.Index(tag, `data-pid="`)
	if j < 0 {
		t.Fatalf("no pid in %s", tag)
	}
	rest := tag[j+len(`data-pid="`):]
	closing := strings.Index(rest, `"`)
	if closing < 0 {
		t.Fatalf("unterminated pid in %s", tag)
	}
	return rest[:closing]
}

func TestAFrameGoesIntoTheRoomAndComesBackByteForByte(t *testing.T) {
	t.Parallel()
	frames := map[string]string{
		"frames/messy.html": messy(t),
		"frames/hero.html":  normal(t, hero),
		"frames/bare.html":  normal(t, `<p>one<p>two`),
		"frames/table.html": normal(t, `<table><tr><td>a<td>b<tr><td>c</table><pre>
 kept</pre><!-- note -->`),
	}
	d := designroom.Design{Manifest: manifest, Frames: frames}
	doc := peer(t, 1, seeded(t, d))
	got := read(t, doc)
	if got.Manifest != manifest {
		t.Errorf("manifest %q", got.Manifest)
	}
	for file, want := range frames {
		if got.Frames[file] != want {
			t.Errorf("%s did not come back as it went in:\nwant %q\ngot  %q", file, want, got.Frames[file])
		}
	}
	// And so it does for a peer that only has the encoded state.
	other := peer(t, 2, crdt.EncodeStateAsUpdateV1(doc, nil))
	if again := read(t, other); again.Frames["frames/messy.html"] != frames["frames/messy.html"] {
		t.Error("the messy frame changed on its way to another peer")
	}
}

func TestTwoPeopleChangingDifferentPropertiesOfOneElementKeepBothAndTheSameOneConverges(t *testing.T) {
	t.Parallel()
	d := heroDesign(t)
	state := seeded(t, d)
	a, b := peer(t, 11, state), peer(t, 22, state)
	box := pidOf(t, d.Frames["frames/hero.html"], `id="box"`)
	edit(t, a, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle("frames/hero.html", box, "left", "100px")
	})
	edit(t, b, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle("frames/hero.html", box, "top", "200px")
	})
	sync(t, a, b)
	for name, doc := range map[string]*crdt.Doc{"a": a, "b": b} {
		frame := read(t, doc).Frames["frames/hero.html"]
		if !strings.Contains(frame, "left:100px") || !strings.Contains(frame, "top:200px") {
			t.Errorf("%s lost one of the two changes: %s", name, frame)
		}
		// What the editor does not own is as it was, in its place.
		if !strings.Contains(frame, "position:absolute;left:100px;top:200px;width:30px;height:40px;color:red") {
			t.Errorf("%s: the style attribute is not in the order it had: %s", name, frame)
		}
	}
	// The same property: one wins and both agree.
	edit(t, a, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle("frames/hero.html", box, "width", "1px")
	})
	edit(t, b, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle("frames/hero.html", box, "width", "2px")
	})
	sync(t, a, b)
	fa, fb := read(t, a).Frames["frames/hero.html"], read(t, b).Frames["frames/hero.html"]
	if fa != fb || (!strings.Contains(fa, "width:1px") && !strings.Contains(fa, "width:2px")) {
		t.Errorf("the peers disagree about one property: %s | %s", fa, fb)
	}
}

func TestMovingAnElementWhileAnotherPersonEditsItsTextLosesNeither(t *testing.T) {
	t.Parallel()
	d := heroDesign(t)
	state := seeded(t, d)
	a, b := peer(t, 11, state), peer(t, 22, state)
	frame := d.Frames["frames/hero.html"]
	p, section := pidOf(t, frame, "<p "), pidOf(t, frame, "<section")
	// The text under the heading: the first text node of the h1 is "~t…"; find it by reading.
	textID := textNodeWith(t, a, "Title")
	edit(t, a, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.Move("frames/hero.html", p, section, 0)
	})
	edit(t, b, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.EditText("frames/hero.html", textID, "Title, changed")
	})
	edit(t, b, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.Move("frames/hero.html", pidOf(t, frame, "<h1"), pidOf(t, frame, "<body"), 0)
	})
	sync(t, a, b)
	fa, fb := read(t, a).Frames["frames/hero.html"], read(t, b).Frames["frames/hero.html"]
	if fa != fb {
		t.Fatalf("peers disagree:\n%s\n%s", fa, fb)
	}
	if !strings.Contains(fa, "Title, changed") {
		t.Errorf("the text edit was lost: %s", fa)
	}
	if !strings.Contains(fa, "<section") || strings.Index(fa, "<p ") < strings.Index(fa, "<section") || strings.Index(fa, "<p ") > strings.Index(fa, "</section>") {
		t.Errorf("the paragraph is not inside the section: %s", fa)
	}
	if strings.Index(fa, "<h1") > strings.Index(fa, "<div") {
		t.Errorf("the heading did not move to the top of the body: %s", fa)
	}
}

// textNodeWith is the id of the text node holding text, found through the room.
func textNodeWith(t *testing.T, doc *crdt.Doc, text string) string {
	const file = heroFile
	t.Helper()
	m := doc.GetMap(designroom.Map)
	for k := range m.Entries() {
		if !strings.HasPrefix(k, file+"|n|") || !strings.HasSuffix(k, "|text") {
			continue
		}
		if v, ok := m.Get(k); ok {
			if tx, isText := v.(*crdt.YText); isText && tx.ToString() == text {
				return strings.Split(k, "|")[2]
			}
		}
	}
	t.Fatalf("no text node %q", text)
	return ""
}

func TestAMoveThatMakesACycleIsRefusedAndTheTreeIsUnchanged(t *testing.T) {
	t.Parallel()
	d := heroDesign(t)
	doc := peer(t, 1, seeded(t, d))
	frame := d.Frames["frames/hero.html"]
	body, section := pidOf(t, frame, "<body"), pidOf(t, frame, "<section")
	before := read(t, doc)
	r := designroom.Open(doc)
	for name, c := range map[string]struct{ id, into string }{
		"into itself":       {section, section},
		"into a child":      {section, pidOf(t, frame, "<h1")},
		"the body":          {body, section},
		"into a grandchild": {body, pidOf(t, frame, "<h1")},
	} {
		if _, err := r.Move("frames/hero.html", c.id, c.into, 0); err == nil {
			t.Errorf("%s: the move was allowed", name)
		}
	}
	if _, err := r.Move("frames/hero.html", section, pidOf(t, frame, "<h1"), 0); !errors.Is(err, designroom.ErrCycle) {
		t.Errorf("a move into a descendant is %v, want ErrCycle", err)
	}
	if after := read(t, doc); after.Frames["frames/hero.html"] != before.Frames["frames/hero.html"] {
		t.Error("a refused move changed the tree")
	}
}

func TestTwoMovesThatTogetherMakeACycleAreFoundWhenTheyMeet(t *testing.T) {
	t.Parallel()
	d := designroom.Design{Manifest: manifest, Frames: map[string]string{"frames/hero.html": normal(t, `<div id="x"><p>x</p></div><div id="y"><p>y</p></div>`)}}
	state := seeded(t, d)
	a, b := peer(t, 11, state), peer(t, 22, state)
	frame := d.Frames["frames/hero.html"]
	x, y := pidOf(t, frame, `id="x"`), pidOf(t, frame, `id="y"`)
	edit(t, a, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Move("frames/hero.html", x, y, 0) })
	edit(t, b, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Move("frames/hero.html", y, x, 0) })
	sync(t, a, b)
	for name, doc := range map[string]*crdt.Doc{"a": a, "b": b} {
		if err := designroom.Check(doc); !errors.Is(err, designroom.ErrCycle) {
			t.Errorf("%s: Check says %v, want ErrCycle", name, err)
		}
		if _, err := designroom.Open(doc).Read(); !errors.Is(err, designroom.ErrCycle) {
			t.Errorf("%s: Read says %v, want ErrCycle", name, err)
		}
	}
	// Each move alone is fine: the room before the merge is sound.
	if err := designroom.Check(peer(t, 3, state)); err != nil {
		t.Errorf("the room before the moves: %v", err)
	}
}

func TestAnElementMovedIntoOneThatSomeoneDeletedIsKeptAtTheEndOfTheBody(t *testing.T) {
	t.Parallel()
	d := designroom.Design{Manifest: manifest, Frames: map[string]string{"frames/hero.html": normal(t, `<div id="x"><p>x</p></div><div id="y"></div><p id="z">z</p>`)}}
	state := seeded(t, d)
	a, b := peer(t, 11, state), peer(t, 22, state)
	frame := d.Frames["frames/hero.html"]
	y, z := pidOf(t, frame, `id="y"`), pidOf(t, frame, `id="z"`)
	edit(t, a, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Move("frames/hero.html", z, y, 0) })
	// b removes y's entries, as a delete of the element would.
	bm := b.GetMap(designroom.Map)
	var gone []string
	for k := range bm.Entries() {
		if strings.HasPrefix(k, "frames/hero.html|n|"+y+"|") {
			gone = append(gone, k)
		}
	}
	b.Transact(func(txn *crdt.Transaction) {
		for _, k := range gone {
			bm.Delete(txn, k)
		}
	})
	sync(t, a, b)
	got := read(t, a).Frames["frames/hero.html"]
	if !strings.Contains(got, `id="z"`) || strings.Contains(got, `id="y"`) {
		t.Errorf("z was lost or y was not deleted: %s", got)
	}
}

func TestWritingADesignIntoARoomChangesOnlyTheFramesThatDiffer(t *testing.T) {
	t.Parallel()
	d := designroom.Design{Manifest: manifest, Frames: map[string]string{
		"frames/a.html": normal(t, `<h1>A</h1>`),
		"frames/b.html": normal(t, `<h1>B</h1>`),
		"frames/c.html": normal(t, `<h1>C</h1>`),
	}}
	doc := peer(t, 1, seeded(t, d))
	// Someone is working in b; the write brings a changed and c gone.
	frameB := d.Frames["frames/b.html"]
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle("frames/b.html", pidOf(t, frameB, "<h1"), "left", "5px")
	})
	next := designroom.Design{Manifest: manifest, Frames: map[string]string{
		"frames/a.html": normal(t, `<h1>A, new</h1>`),
		"frames/b.html": read(t, doc).Frames["frames/b.html"],
	}}
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Write(next) })
	got := read(t, doc)
	if got.Frames["frames/a.html"] != next.Frames["frames/a.html"] || got.Frames["frames/b.html"] != next.Frames["frames/b.html"] {
		t.Errorf("frames after the write: %v", got.Frames)
	}
	if _, there := got.Frames["frames/c.html"]; there {
		t.Error("a frame the design lacks is still in the room")
	}
	if !strings.Contains(got.Frames["frames/b.html"], "left:5px") {
		t.Error("what was done in the frame that did not change was lost")
	}
}

func TestEditingATextChangesOnlyWhatDiffers(t *testing.T) {
	t.Parallel()
	d := heroDesign(t)
	state := seeded(t, d)
	a, b := peer(t, 11, state), peer(t, 22, state)
	id := textNodeWith(t, a, "Title")
	edit(t, a, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.EditText("frames/hero.html", id, "My Title")
	})
	edit(t, b, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.EditText("frames/hero.html", id, "Title 😀")
	})
	sync(t, a, b)
	fa, fb := read(t, a).Frames["frames/hero.html"], read(t, b).Frames["frames/hero.html"]
	if fa != fb || !strings.Contains(fa, "My Title 😀") {
		t.Errorf("two edits of one text: %s | %s", fa, fb)
	}
}
