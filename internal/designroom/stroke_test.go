package designroom_test

import (
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/reearth/ygo/crdt"
)

const strokeBody = `<svg data-pid="pen" data-pen="stroke" data-name="Pen stroke" viewBox="0 0 24 14" preserveAspectRatio="none" style="position:absolute;left:10px;top:20px;width:24px;height:14px"><path data-pid="pen-path" d="M2 2Q12 12 17 7L22 2" stroke="var(--accent)" stroke-width="4" fill="none" stroke-linecap="round" stroke-linejoin="round" pointer-events="none"></path></svg>`

func strokeDesign(t *testing.T) designroom.Design { t.Helper(); return oneFrame(t, strokeBody) }
func strokeFixture(t *testing.T) update {
	t.Helper()
	doc := crdt.New(crdt.WithClientID(7300))
	write, err := designroom.Open(doc).Write(strokeDesign(t))
	if err != nil {
		t.Fatal(err)
	}
	doc.Transact(write)
	return update{Name: "stroke", Update: hex.EncodeToString(crdt.EncodeStateAsUpdateV1(doc, nil))}
}

func TestAStrokeRoundTripsAsNativeSVGAndAnInvalidRoomPathNamesItsNode(t *testing.T) {
	t.Parallel()
	d := strokeDesign(t)
	doc := peer(t, 1, seeded(t, d))
	got := read(t, doc)
	if got.Frames[heroFile] != d.Frames[heroFile] {
		t.Fatal("the vector changed in the room")
	}
	if err := designfmt.Validate(got.Manifest, got.Frames); err != nil {
		t.Fatal(err)
	}
	if err := designroom.Check(doc); err != nil {
		t.Fatal(err)
	}
	m := doc.GetMap(designroom.Map)
	doc.Transact(func(txn *crdt.Transaction) {
		m.Set(txn, heroFile+"|n|pen-path|attrs", `[["","d","M0 0LNaN 5"]]`)
	})
	err := designroom.Check(doc)
	if err == nil || !strings.Contains(err.Error(), "pen-path") {
		t.Fatalf("got %v, want refusal naming pen-path", err)
	}
}

func TestGoReadsTheStrokePathColorWidthAndGeometryYjsWrote(t *testing.T) {
	t.Parallel()
	u := readUpdate(t, filepath.Join("testdata", "yjs", "stroke.json"))
	raw, err := hex.DecodeString(u.Update)
	if err != nil {
		t.Fatal(err)
	}
	doc := crdt.New()
	if err := crdt.ApplyUpdateV1(doc, raw, nil); err != nil {
		t.Fatal(err)
	}
	if err := designroom.Check(doc); err != nil {
		t.Fatal(err)
	}
	d := read(t, doc)
	for _, part := range []string{`d="M1 1L30 40"`, `stroke="#336699"`, `stroke-width="6"`, `left:100px`, `width:80px`} {
		if !strings.Contains(d.Frames[heroFile], part) {
			t.Errorf("saved SVG lost %s: %s", part, d.Frames[heroFile])
		}
	}
	if err := designfmt.Validate(d.Manifest, d.Frames); err != nil {
		t.Fatal(err)
	}
	// A fresh room from the saved design keeps the same vector, without a DOM snapshot.
	again := read(t, peer(t, 2, seeded(t, d)))
	if again.Frames[heroFile] != d.Frames[heroFile] {
		t.Fatal("reload changed the stroke")
	}
}
