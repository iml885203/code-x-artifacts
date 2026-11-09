package designroom_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

const propertiesFrame = `<!doctype html><html><head><style>.card{color:navy;padding:4px}</style></head><body>` +
	`<div id="card" class="card" style="position:absolute;left:10px;top:20px;padding: 8px 12px;color:#112233;background:url(a.png);border-radius:4px;box-shadow:none;font-size:16px;transform:rotate(3deg)">Hi</div>` +
	`</body></html>`

func propertiesDesign(t *testing.T) designroom.Design {
	return designroom.Design{Manifest: manifest, Frames: map[string]string{heroFile: normal(t, propertiesFrame)}}
}

// The panel's list lives in one file the browser imports; the room has to own
// every property in it and nothing that the outline writes.
func TestEveryPropertyThePanelWritesIsOwnedByTheRoomAndDisplayIsNot(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../web/src/design/properties/owned.json")
	if err != nil {
		t.Fatal(err)
	}
	var list struct{ Owned, Shown []string }
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Owned) < 30 {
		t.Fatalf("the list holds %d properties", len(list.Owned))
	}
	for _, p := range list.Owned {
		if !designroom.Owned(p) {
			t.Errorf("%s is written by the panel and not owned by the room", p)
		}
	}
	for _, p := range list.Shown {
		if designroom.Owned(p) {
			t.Errorf("%s is shown in frames but owned: an authored display:none would stop being hidden", p)
		}
	}
	for _, p := range []string{"transform", "background", "z-index"} {
		if designroom.Owned(p) {
			t.Errorf("%s is owned but no control writes it", p)
		}
	}
}

func TestEachPropertyThePanelWritesChangesOnlyItsOwnDeclarationInTheSavedFile(t *testing.T) {
	t.Parallel()
	cases := []struct{ prop, value, from, to string }{
		{"position", "relative", "position:absolute;", "position:relative;"},
		{"padding", "10px", "padding: 8px 12px;", "padding:10px;"},
		{"color", "rgba(1,2,3,0.5)", "color:#112233;", "color:rgba(1,2,3,0.5);"},
		{"border-radius", "9px", "border-radius:4px;", "border-radius:9px;"},
		{"box-shadow", "0 2px 6px rgba(0,0,0,0.3)", "box-shadow:none;", "box-shadow:0 2px 6px rgba(0,0,0,0.3);"},
		{"font-size", "44px", "font-size:16px;", "font-size:44px;"},
		{"padding", "", "padding: 8px 12px;", ""},
	}
	for _, c := range cases {
		doc := peer(t, 1, seeded(t, propertiesDesign(t)))
		before := read(t, doc).Frames[heroFile]
		id := pidOf(t, before, `id="card"`)
		edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
			return r.SetStyle(heroFile, id, c.prop, c.value)
		})
		got := read(t, doc).Frames[heroFile]
		if want := replaceOnce(t, before, c.from, c.to); got != want {
			t.Errorf("%s=%q:\nwant %s\ngot  %s", c.prop, c.value, want, got)
		}
	}
}

func TestAPropertyTheElementDoesNotHaveYetIsAddedAfterWhatIsThereAndNothingElseMoves(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, propertiesDesign(t)))
	before := read(t, doc).Frames[heroFile]
	id := pidOf(t, before, `id="card"`)
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, id, "gap", "12px")
	})
	got := read(t, doc).Frames[heroFile]
	if want := replaceOnce(t, before, "rotate(3deg)", "rotate(3deg);gap:12px"); got != want {
		t.Errorf("want %s\ngot  %s", want, got)
	}
	if !strings.Contains(got, ".card{color:navy;padding:4px}") {
		t.Error("the class rule changed")
	}
}
