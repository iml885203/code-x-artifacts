package designroom_test

import (
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

const footFile = "frames/foot.html"

// twoPages is a design with a page of one frame and a page of one.
const twoPages = `{"version":1,"pages":[` +
	`{"id":"p","name":"P","rows":[[{"id":"hero","name":"Hero","file":"frames/hero.html","w":800,"h":600}]]},` +
	`{"id":"q","name":"Q","rows":[[{"id":"foot","name":"Foot","file":"frames/foot.html","w":800,"h":200}]]}]}`

func twoPageDesign(t *testing.T) designroom.Design {
	t.Helper()
	d := heroDesign(t)
	d.Manifest = twoPages
	d.Frames[footFile] = normal(t, `<!doctype html><html><head><meta charset="utf-8"></head><body><p>Footer</p></body></html>`)
	return d
}

// What the browser writes (web/src/design/pages/structure.ts), entry by entry.
type browser struct {
	t   *testing.T
	doc *crdt.Doc
}

func (b browser) do(fn func(m *crdt.YMap, txn *crdt.Transaction)) {
	b.t.Helper()
	m := b.doc.GetMap(designroom.Map)
	b.doc.Transact(func(txn *crdt.Transaction) { fn(m, txn) })
}

func (b browser) set(kv ...string) {
	b.t.Helper()
	b.do(func(m *crdt.YMap, txn *crdt.Transaction) {
		for i := 0; i < len(kv); i += 2 {
			m.Set(txn, kv[i], kv[i+1])
		}
	})
}

func (b browser) keysWith(prefix string) []string {
	var out []string
	for k := range b.doc.GetMap(designroom.Map).Entries() {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out
}

func (b browser) del(prefixes ...string) {
	b.t.Helper()
	var keys []string
	for _, p := range prefixes {
		keys = append(keys, b.keysWith(p)...)
	}
	b.do(func(m *crdt.YMap, txn *crdt.Transaction) {
		for _, k := range keys {
			m.Delete(txn, k)
		}
	})
}

func (b browser) value(key string) string {
	v, _ := b.doc.GetMap(designroom.Map).Get(key)
	s, _ := v.(string)
	return s
}

// addFrame adds a frame with an empty body to a row, as the browser does: the frame's values and its content in one step.
func (b browser) addFrame(id, name, row, order string, w, h string) {
	b.t.Helper()
	file := "frames/" + id + ".html"
	b.do(func(m *crdt.YMap, txn *crdt.Transaction) {
		for k, v := range map[string]string{
			"s|frame|" + id + "|file": file, "s|frame|" + id + "|name": name, "s|frame|" + id + "|w": w, "s|frame|" + id + "|h": h,
			"s|frame|" + id + "|row": row, "s|frame|" + id + "|order": order,
			file + "|shell|docAfter": "", file + "|shell|docBefore": "", file + "|shell|htmlAttrs": "[]", file + "|shell|post": "",
			file + "|shell|pre": `<head><meta charset="utf-8"/></head>`, file + "|shell|root": "e1",
			file + "|n|e1|attrs": `[["","\u0000pid",""]]`, file + "|n|e1|kind": "el", file + "|n|e1|ns": "", file + "|n|e1|order": "",
			file + "|n|e1|parent": "", file + "|n|e1|rest": "", file + "|n|e1|tag": "body",
		} {
			m.Set(txn, k, v)
		}
	})
}

// deleteFrame removes the frame's values and its content.
func (b browser) deleteFrame(id string) { b.del("s|frame|"+id+"|", "frames/"+id+".html|") }

// deletePage removes the page, its rows, their frames and the frames' content.
func (b browser) deletePage(id string, frames ...string) {
	b.t.Helper()
	var rows []string
	for k := range b.doc.GetMap(designroom.Map).Entries() {
		if strings.HasPrefix(k, "s|row|") && strings.HasSuffix(k, "|page") && b.value(k) == id {
			rows = append(rows, strings.TrimSuffix(k, "page"))
		}
	}
	b.del(append([]string{"s|page|" + id + "|"}, rows...)...)
	for _, f := range frames {
		b.deleteFrame(f)
	}
}

func fromState(t *testing.T, id crdt.ClientID, state []byte) browser {
	return browser{t, peer(t, id, state)}
}

func frameIDs(t *testing.T, doc *crdt.Doc) [][]string {
	t.Helper()
	m, problems := designfmt.Parse(read(t, doc).Manifest)
	if len(problems) > 0 {
		t.Fatalf("the room's design.json does not parse: %v", problems)
	}
	var out [][]string
	for _, p := range m.Pages {
		for _, row := range p.Rows {
			var ids []string
			for _, f := range row {
				ids = append(ids, p.ID+"/"+f.ID)
			}
			out = append(out, ids)
		}
	}
	return out
}

func shape(rows [][]string) string {
	var parts []string
	for _, r := range rows {
		parts = append(parts, strings.Join(r, ","))
	}
	return strings.Join(parts, " | ")
}

func TestARoomThatHoldsADesignProjectsItsStructureBackToTheSameDesignJSON(t *testing.T) {
	t.Parallel()
	for name, d := range map[string]designroom.Design{"one page": heroDesign(t), "two pages": twoPageDesign(t)} {
		doc := peer(t, 1, seeded(t, d))
		got := read(t, doc)
		if got.Manifest != d.Manifest {
			t.Errorf("%s: design.json is\n%s\nwant\n%s", name, got.Manifest, d.Manifest)
		}
		if err := designfmt.Validate(got.Manifest, got.Frames); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if err := designroom.Check(doc); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := designroom.Check(crdt.New()); err != nil {
		t.Errorf("an empty room: %v", err)
	}
}

func TestAFrameTheBrowserAddsIsInTheDesignWithAnEmptyBodyAndTheVersionIsValid(t *testing.T) {
	t.Parallel()
	b := fromState(t, 1, seeded(t, heroDesign(t)))
	b.set("s|row|r2|page", "p", "s|row|r2|order", "V")
	b.addFrame("frame-2", "Frame 2", "r2", "V", "1440", "900")
	if err := designroom.Check(b.doc); err != nil {
		t.Fatalf("the room refused a frame the browser added: %v", err)
	}
	got := read(t, b.doc)
	if want := `<!DOCTYPE html><html><head><meta charset="utf-8"/></head><body data-pid="e1"></body></html>`; got.Frames["frames/frame-2.html"] != want {
		t.Errorf("the new frame is\n%s\nwant\n%s", got.Frames["frames/frame-2.html"], want)
	}
	if err := designfmt.Validate(got.Manifest, got.Frames); err != nil {
		t.Errorf("the design with the new frame cannot be saved: %v", err)
	}
	if !strings.Contains(got.Manifest, `"file":"frames/frame-2.html","w":1440,"h":900`) {
		t.Errorf("design.json: %s", got.Manifest)
	}
	frame, ok, err := designroom.Open(b.doc).Frame("frames/frame-2.html")
	if err != nil || !ok || frame != got.Frames["frames/frame-2.html"] {
		t.Errorf("the room does not serve the frame it holds: %q %v %v", frame, ok, err)
	}
}

func TestDeletingAPageTakesItsFramesOutOfTheVersion(t *testing.T) {
	t.Parallel()
	b := fromState(t, 1, seeded(t, twoPageDesign(t)))
	b.deletePage("q", "foot")
	if err := designroom.Check(b.doc); err != nil {
		t.Fatalf("the room refused the delete: %v", err)
	}
	got := read(t, b.doc)
	if _, has := got.Frames[footFile]; has {
		t.Error("the deleted page's frame is still among the files")
	}
	if shape(frameIDs(t, b.doc)) != "p/hero" {
		t.Errorf("pages %s", shape(frameIDs(t, b.doc)))
	}
	if err := designfmt.Validate(got.Manifest, got.Frames); err != nil {
		t.Errorf("the design after the delete cannot be saved: %v", err)
	}
}

// Two people restructure at the same time, each in a copy, and the copies meet.
// Whatever they did, both rooms end up the same, the hub accepts them, and the
// design is one a version can hold.
func merged(t *testing.T, d designroom.Design, alice, bob func(b browser)) (string, designroom.Design) {
	t.Helper()
	state := seeded(t, d)
	a, b := fromState(t, 11, state), fromState(t, 22, state)
	alice(a)
	bob(b)
	sync(t, a.doc, b.doc)
	ra, rb := read(t, a.doc), read(t, b.doc)
	if ra.Manifest != rb.Manifest {
		t.Fatalf("the rooms differ after they met:\n%s\n%s", ra.Manifest, rb.Manifest)
	}
	for _, doc := range []*crdt.Doc{a.doc, b.doc} {
		if err := designroom.Check(doc); err != nil {
			t.Fatalf("the hub refuses what two people did at once: %v", err)
		}
	}
	if err := designfmt.Validate(ra.Manifest, ra.Frames); err != nil {
		t.Fatalf("the merged design cannot be saved: %v", err)
	}
	return ra.Manifest, ra
}

func TestTwoPeopleAddingFramesAtOnceBothKeepTheirs(t *testing.T) {
	t.Parallel()
	d := heroDesign(t)
	state := seeded(t, d)
	a, b := fromState(t, 11, state), fromState(t, 22, state)
	a.set("s|row|ra|page", "p", "s|row|ra|order", "V")
	a.addFrame("from-a", "A", "ra", "V", "100", "100")
	b.set("s|row|rb|page", "p", "s|row|rb|order", "V")
	b.addFrame("from-b", "B", "rb", "V", "200", "200")
	sync(t, a.doc, b.doc)
	got := frameIDs(t, a.doc)
	if len(got) != 3 || !strings.Contains(shape(got), "p/from-a") || !strings.Contains(shape(got), "p/from-b") {
		t.Errorf("after both added a frame: %s", shape(got))
	}
	if err := designroom.Check(a.doc); err != nil {
		t.Error(err)
	}
}

func TestOnePersonRenamingAFrameWhileAnotherResizesItKeepsBoth(t *testing.T) {
	t.Parallel()
	manifest, _ := merged(t, heroDesign(t),
		func(b browser) { b.set("s|frame|hero|name", "Banner") },
		func(b browser) { b.set("s|frame|hero|w", "390", "s|frame|hero|h", "844") },
	)
	if !strings.Contains(manifest, `"name":"Banner","file":"frames/hero.html","w":390,"h":844`) {
		t.Errorf("design.json: %s", manifest)
	}
}

func TestTwoPeopleMovingDifferentFramesKeepBothMoves(t *testing.T) {
	t.Parallel()
	d := heroDesign(t)
	d.Manifest = `{"version":1,"pages":[{"id":"p","name":"P","rows":[[` +
		`{"id":"hero","name":"Hero","file":"frames/hero.html","w":800,"h":600},` +
		`{"id":"foot","name":"Foot","file":"frames/foot.html","w":800,"h":200}],[` +
		`{"id":"side","name":"Side","file":"frames/side.html","w":100,"h":100}]]}]}`
	d.Frames[footFile] = twoPageDesign(t).Frames[footFile]
	d.Frames["frames/side.html"] = d.Frames[footFile]
	_, got := merged(t, d,
		func(b browser) {
			b.set("s|row|ra|page", "p", "s|row|ra|order", "z", "s|frame|hero|row", "ra", "s|frame|hero|order", "V")
		}, // hero into a row of its own, last
		func(b browser) { b.set("s|frame|side|row", "p.r1", "s|frame|side|order", "zz") }, // side to the end of the first row
	)
	rows, _ := designfmt.Parse(got.Manifest)
	var shapeOut []string
	for _, row := range rows.Pages[0].Rows {
		var ids []string
		for _, f := range row {
			ids = append(ids, f.ID)
		}
		shapeOut = append(shapeOut, strings.Join(ids, ","))
	}
	if strings.Join(shapeOut, "|") != "foot,side|hero" {
		t.Errorf("rows %v", shapeOut)
	}
}

func TestAPageDeletedWhileAnotherPersonMovesAFrameIntoItConvergesAndLeavesTheFrameOut(t *testing.T) {
	t.Parallel()
	d := twoPageDesign(t)
	d.Manifest = strings.Replace(d.Manifest, `"w":800,"h":600}]]}`, `"w":800,"h":600},{"id":"side","name":"Side","file":"frames/side.html","w":100,"h":100}]]}`, 1)
	d.Frames["frames/side.html"] = d.Frames[footFile]
	_, got := merged(t, d,
		func(b browser) { b.deletePage("q", "foot") },
		func(b browser) { b.set("s|frame|side|row", "q.r1", "s|frame|side|order", "zz") }, // side into q's row as q goes
	)
	if strings.Contains(got.Manifest, "side") || strings.Contains(got.Manifest, "foot") {
		t.Errorf("a frame whose page is gone is still in the design: %s", got.Manifest)
	}
	if _, has := got.Frames["frames/side.html"]; has {
		t.Error("its file is still among the files of the version")
	}
}

func TestAFrameDeletedWhileAnotherRenamesItIsDeleted(t *testing.T) {
	t.Parallel()
	d := twoPageDesign(t)
	_, got := merged(t, d,
		func(b browser) { b.deleteFrame("foot") },
		func(b browser) { b.set("s|frame|foot|name", "Footer") },
	)
	if strings.Contains(got.Manifest, "foot") {
		t.Errorf("the frame is still in the design: %s", got.Manifest)
	}
	if _, has := got.Frames[footFile]; has {
		t.Error("its file is still among the files")
	}
}

func TestAPageThatLostAllItsFramesIsLeftOutOfTheDesignNotRefused(t *testing.T) {
	t.Parallel()
	b := fromState(t, 1, seeded(t, twoPageDesign(t)))
	b.deleteFrame("foot")
	if err := designroom.Check(b.doc); err != nil {
		t.Fatalf("the room refused a page with no frame: %v", err)
	}
	if shape(frameIDs(t, b.doc)) != "p/hero" {
		t.Errorf("design: %s", shape(frameIDs(t, b.doc)))
	}
}

func TestAValueOutsideTheFormatIsRefusedByTheRoomAndTheLimitIsNamed(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct{ key, value, want string }{
		"width over the limit": {"s|frame|hero|w", "9000", "w must be a whole number of pixels from 1 to 8192"},
		"height zero":          {"s|frame|hero|h", "0", "h must be a whole number of pixels from 1 to 8192"},
		"height not a number":  {"s|frame|hero|h", "tall", "h must be"},
		"empty name":           {"s|frame|hero|name", "  ", "a name is required"},
		"long name":            {"s|page|p|name", strings.Repeat("n", 81), "at most 80 characters"},
	} {
		b := fromState(t, 1, seeded(t, heroDesign(t)))
		b.set(c.key, c.value)
		err := designroom.Check(b.doc)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want a refusal saying %q", name, err, c.want)
		}
	}
}

func TestARoomThatKeepsItsManifestAsOneEntryStillReadsAndIsChecked(t *testing.T) {
	t.Parallel()
	doc := crdt.New(crdt.WithClientID(1))
	m := doc.GetMap(designroom.Map)
	doc.Transact(func(txn *crdt.Transaction) {
		m.Set(txn, "manifest", manifest)
		for _, kv := range [][2]string{{"shell|root", "e1"}, {"shell|htmlAttrs", "[]"}, {"n|e1|kind", "el"}, {"n|e1|tag", "body"}, {"n|e1|parent", ""}, {"n|e1|order", ""}, {"n|e1|attrs", "[]"}} {
			m.Set(txn, heroFile+"|"+kv[0], kv[1])
		}
	})
	if got := read(t, doc).Manifest; got != manifest {
		t.Errorf("design.json %s", got)
	}
	if err := designroom.Check(doc); err != nil {
		t.Errorf("a valid legacy room: %v", err)
	}
	doc.Transact(func(txn *crdt.Transaction) {
		m.Set(txn, "manifest", strings.Replace(manifest, `"w":800`, `"w":9000`, 1))
	})
	if err := designroom.Check(doc); err == nil || !strings.Contains(err.Error(), "8192") {
		t.Errorf("a legacy manifest over the limit: %v", err)
	}
}

func TestWritingADesignIntoARoomReplacesItsStructureAndNothingElse(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, twoPageDesign(t)))
	r := designroom.Open(doc)
	want := heroDesign(t)
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Write(want) })
	_ = r
	got := read(t, doc)
	if got.Manifest != want.Manifest {
		t.Errorf("design.json %s", got.Manifest)
	}
	if _, has := got.Frames[footFile]; has {
		t.Error("the frame the design no longer has is still there")
	}
}
