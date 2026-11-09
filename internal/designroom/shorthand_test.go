package designroom_test

import (
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/reearth/ygo/crdt"
)

func TestEditsAfterCoveringShorthandsPreserveAuthoredSourceAndResetOnlyTheOverride(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ style, prop, value string }{
		{"border-top-color:blue !important;border:2px solid red !important", "border-top-color", "green !important"},
		{"border-right-width:1px;border-width:2px 3px", "border-right-width", "4px"},
		{"border-bottom-style:dashed;border-bottom:2px solid red", "border-bottom-style", "dotted"},
		{"background-color:blue !important;background:red !important", "background-color", "transparent !important"},
		{"background-image:url(/files/abc/photo.png);background:red", "background-image", "none"},
	} {
		t.Run(c.prop, func(t *testing.T) {
			frame := normal(t, `<html><body><div data-pid="card" style="`+c.style+`;transform:rotate(2deg)">Keep</div></body></html>`)
			doc := peer(t, 1, seeded(t, designroom.Design{Manifest: manifest, Frames: map[string]string{heroFile: frame}}))
			if got := read(t, doc).Frames[heroFile]; got != frame {
				t.Fatalf("untouched source changed: %s", got)
			}
			edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
				return r.SetStyle(heroFile, "card", c.prop, c.value)
			})
			want := strings.Replace(frame, "transform:rotate(2deg)", "transform:rotate(2deg);"+c.prop+":"+c.value, 1)
			if got := read(t, doc).Frames[heroFile]; got != want {
				t.Fatalf("override must follow shorthand; want %s\ngot %s", want, got)
			}
			// A saved frame seeded into a new room must keep the winning declaration last.
			fresh := peer(t, 2, seeded(t, designroom.Design{Manifest: manifest, Frames: map[string]string{heroFile: want}}))
			if got := read(t, fresh).Frames[heroFile]; got != want {
				t.Fatalf("reload changed cascade order: %s", got)
			}
			edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
				return r.SetStyle(heroFile, "card", c.prop, "")
			})
			if got := read(t, doc).Frames[heroFile]; got != frame {
				t.Fatalf("reset changed authored declarations: %s", got)
			}
		})
	}
}

func legacyCoveredRoom(t *testing.T) *crdt.Doc {
	t.Helper()
	frame := normal(t, `<html><body><div data-pid="card" style="border-top-color:blue !important;border:2px solid red !important;background-color:blue !important;background:red !important;transform:rotate(2deg)">Keep</div></body></html>`)
	doc := peer(t, 1, seeded(t, designroom.Design{Manifest: manifest, Frames: map[string]string{heroFile: frame}}))
	setEntry(doc, heroFile+"|n|card|rest", "border-top-color:border-top-color;border:2px solid red !important;background-color:background-color;background:red !important;transform:rotate(2deg)")
	setEntry(doc, heroFile+"|n|card|s|border-top-color", "blue !important")
	setEntry(doc, heroFile+"|n|card|s|background-color", "blue !important")
	if got := read(t, doc).Frames[heroFile]; got != frame {
		t.Fatalf("legacy fixture changed source: %s", got)
	}
	return doc
}

func TestLegacyCoveredSlotsKeepConcurrentOverridesAfterShorthandsAndResetTheirOwnValues(t *testing.T) {
	t.Parallel()
	a := legacyCoveredRoom(t)
	original := read(t, a).Frames[heroFile]
	b := peer(t, 2, crdt.EncodeStateAsUpdateV1(a, nil))
	edit(t, a, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, "card", "border-top-color", "green !important")
	})
	edit(t, b, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, "card", "background-color", "transparent !important")
	})
	sync(t, a, b)
	want := strings.Replace(original, "transform:rotate(2deg)", "transform:rotate(2deg);background-color:transparent !important;border-top-color:green !important", 1)
	for _, doc := range []*crdt.Doc{a, b, peer(t, 3, crdt.EncodeStateAsUpdateV1(a, nil))} {
		if err := designroom.Check(doc); err != nil {
			t.Fatal(err)
		}
		if got := read(t, doc).Frames[heroFile]; got != want {
			t.Fatalf("legacy concurrent edits lost cascade order: want %s\ngot %s", want, got)
		}
	}
	for _, prop := range []string{"border-top-color", "background-color"} {
		edit(t, a, func(r *designroom.Room) (func(*crdt.Transaction), error) {
			return r.SetStyle(heroFile, "card", prop, "")
		})
	}
	if got := read(t, a).Frames[heroFile]; got != original {
		t.Fatalf("reset changed retained authored source: %s", got)
	}
}

func TestSourceEditsToLegacyCoveredSlotsKeepWinningOrderWithoutChangingAuthoredDeclarations(t *testing.T) {
	t.Parallel()
	doc := legacyCoveredRoom(t)
	base := read(t, doc)
	next := changed(t, base, "border-top-color:blue !important", "border-top-color:green !important", "background-color:blue !important", "background-color:transparent !important")
	agentMerges(t, doc, base, next)
	want := strings.Replace(base.Frames[heroFile], "transform:rotate(2deg)", "transform:rotate(2deg);background-color:transparent !important;border-top-color:green !important", 1)
	if got := read(t, doc).Frames[heroFile]; got != want {
		t.Fatalf("legacy source merge lost cascade order: want %s\ngot %s", want, got)
	}
}

func TestRepeatedSourceEditsAndResetKeepTheWinningCoveredOverrideIdentified(t *testing.T) {
	t.Parallel()
	doc := legacyCoveredRoom(t)
	original := read(t, doc)
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, "card", "border-top-color", "green !important")
	})
	base := read(t, doc)
	next := changed(t, base, "border-top-color:green !important", "border-top-color:purple !important")
	agentMerges(t, doc, base, next)
	if got := read(t, doc).Frames[heroFile]; got != next.Frames[heroFile] {
		t.Fatalf("second edit lost override identity: want %s\ngot %s", next.Frames[heroFile], got)
	}
	base = read(t, doc)
	next = changed(t, base, ";border-top-color:purple !important", "")
	agentMerges(t, doc, base, next)
	if got := read(t, doc).Frames[heroFile]; got != original.Frames[heroFile] {
		t.Fatalf("source reset failed to remove override: %s", got)
	}
}

func TestStyleOverridesCannotBypassOwnedPropertyOrValueValidation(t *testing.T) {
	t.Parallel()
	for key, value := range map[string]string{"border-top-color": "url(x)", "background-color": "red;position:fixed", "transform": "rotate(2deg)"} {
		doc := legacyCoveredRoom(t)
		setEntry(doc, heroFile+"|n|card|override|"+key, value)
		if err := designroom.Check(doc); err == nil {
			t.Fatalf("unsafe retained base %s accepted", key)
		}
	}
}
