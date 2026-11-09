package designroom_test

import (
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

func TestDirectYjsOverridesKeepCascadeOrderThroughConcurrentEditsUndoRedoAndReload(t *testing.T) {
	t.Parallel()
	var original string
	for _, c := range []struct{ name, tail string }{
		{"base", ""},
		{"concurrent", ";background-color:transparent !important;border-top-color:green !important"},
		{"undo", ";background-color:transparent !important"},
		{"redo", ";background-color:transparent !important;border-top-color:green !important"},
		{"later", ";background-color:transparent !important;border-top-color:yellow !important"},
		{"first-race", ";border-top-color:yellow !important"},
	} {
		fixture := readUpdate(t, filepath.Join("testdata", "yjs", "cascade."+c.name+".json"))
		raw, err := hex.DecodeString(fixture.Update)
		if err != nil {
			t.Fatal(err)
		}
		doc := peer(t, 9001, raw)
		if err := designroom.Check(doc); err != nil {
			t.Fatalf("%s: browser room rejected: %v", c.name, err)
		}
		held := read(t, doc)
		got := held.Frames[heroFile]
		if c.name == "base" {
			original = got
		}
		want := strings.Replace(original, "transform:rotate(2deg)", "transform:rotate(2deg)"+c.tail, 1)
		if got != want {
			t.Fatalf("%s: direct Yjs update changed unrelated source or cascade order:\nwant %s\ngot %s", c.name, want, got)
		}
		fresh := peer(t, 9002, seeded(t, held))
		if got := read(t, fresh).Frames[heroFile]; got != want {
			t.Fatalf("%s: source reload changed winning declarations: %s", c.name, got)
		}
	}
}
