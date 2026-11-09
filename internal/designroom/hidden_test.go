package designroom_test

import (
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

func setEntry(doc *crdt.Doc, key string, value any) {
	m := doc.GetMap(designroom.Map)
	doc.Transact(func(txn *crdt.Transaction) { m.Set(txn, key, value) })
}

func dropEntry(doc *crdt.Doc, key string) {
	m := doc.GetMap(designroom.Map)
	doc.Transact(func(txn *crdt.Transaction) { m.Delete(txn, key) })
}

// The panel makes a container with display:flex; hiding is the hidden state, so
// showing the element again gives the layout back instead of losing it.
func TestHidingAndShowingAnElementKeepsTheDisplayThePanelWrote(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, propertiesDesign(t)))
	plain := read(t, doc).Frames[heroFile]
	id := pidOf(t, plain, `id="card"`)
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, id, "flex-direction", "column")
	})
	setEntry(doc, heroFile+"|n|"+id+"|s|display", "flex")
	laidOut := read(t, doc).Frames[heroFile]
	if !strings.Contains(laidOut, "display:flex") {
		t.Fatalf("the layout is not in the file: %s", laidOut)
	}

	setEntry(doc, heroFile+"|n|"+id+"|hidden", "1")
	hidden := read(t, doc).Frames[heroFile]
	if !strings.Contains(hidden, "display:none") || strings.Contains(hidden, "display:flex") || !strings.Contains(hidden, "Hi") {
		t.Errorf("hidden: the element must say display:none and stay in the file: %s", hidden)
	}

	dropEntry(doc, heroFile+"|n|"+id+"|hidden")
	if shown := read(t, doc).Frames[heroFile]; shown != laidOut {
		t.Errorf("shown again:\nwant %s\ngot  %s", laidOut, shown)
	}
}

func TestHidingAnElementWithNoDisplayOfItsOwnAndShowingItRestoresTheFileByteForByte(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, propertiesDesign(t)))
	before := read(t, doc).Frames[heroFile]
	id := pidOf(t, before, `id="card"`)
	setEntry(doc, heroFile+"|n|"+id+"|hidden", "1")
	if got := read(t, doc).Frames[heroFile]; got != replaceOnce(t, before, "rotate(3deg)", "rotate(3deg);display:none") {
		t.Errorf("hidden: %s", got)
	}
	dropEntry(doc, heroFile+"|n|"+id+"|hidden")
	if got := read(t, doc).Frames[heroFile]; got != before {
		t.Errorf("shown:\nwant %s\ngot  %s", before, got)
	}
}

func TestShowingAnElementWhoseMarkupHidesItWritesRevertUnlessADisplayWasWritten(t *testing.T) {
	t.Parallel()
	frame := `<!doctype html><html><head></head><body><div id="gone" style="display:none;color:red">x</div></body></html>`
	doc := peer(t, 1, seeded(t, designroom.Design{Manifest: manifest, Frames: map[string]string{heroFile: normal(t, frame)}}))
	before := read(t, doc).Frames[heroFile]
	id := pidOf(t, before, `id="gone"`)

	setEntry(doc, heroFile+"|n|"+id+"|hidden", "0")
	if got := read(t, doc).Frames[heroFile]; got != replaceOnce(t, before, "display:none;color:red", "display:none;color:red;display:revert") {
		t.Errorf("shown: %s", got)
	}
	// A display of the panel's own is what shows it.
	setEntry(doc, heroFile+"|n|"+id+"|s|display", "flex")
	if got := read(t, doc).Frames[heroFile]; !strings.Contains(got, "display:flex") || strings.Contains(got, "revert") {
		t.Errorf("with a display written: %s", got)
	}
}

func TestAnElementAnOlderClientHidThroughTheDisplayEntryStaysHidden(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, propertiesDesign(t)))
	before := read(t, doc).Frames[heroFile]
	id := pidOf(t, before, `id="card"`)
	setEntry(doc, heroFile+"|n|"+id+"|s|display", "none")
	if got := read(t, doc).Frames[heroFile]; !strings.Contains(got, "display:none") {
		t.Errorf("%s", got)
	}
}
