package designroom_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

// manyFrames is a design of n copies of the hero frame, one per row.
func manyFrames(t *testing.T, n int) designroom.Design {
	t.Helper()
	rows := make([]string, n)
	frames := map[string]string{}
	for i := range n {
		file := fmt.Sprintf("frames/f%d.html", i)
		rows[i] = fmt.Sprintf(`[{"id":"f%d","name":"F%d","file":"%s","w":800,"h":600}]`, i, i, file)
		frames[file] = normal(t, hero)
	}
	return designroom.Design{Manifest: `{"version":1,"pages":[{"id":"p","name":"P","rows":[` + strings.Join(rows, ",") + `]}]}`, Frames: frames}
}

// A canvas asks for every frame of a design at once, so reading one frame
// builds that frame alone: a frame elsewhere in the room that cannot be built
// does not stop it, and it gives the same frame and design.json as reading the
// whole room.
func TestReadingOneFrameBuildsThatFrameAloneAndAgreesWithReadingTheRoom(t *testing.T) {
	t.Parallel()
	b := fromState(t, 1, seeded(t, manyFrames(t, 3)))
	whole := read(t, b.doc)
	one, err := designroom.Open(b.doc).FrameNow("frames/f0.html")
	if err != nil || !one.Present || one.Text != whole.Frames["frames/f0.html"] || one.Manifest != whole.Manifest {
		t.Fatalf("one frame = %+v, %v; the whole room's = %q, %q", one, err, whole.Frames["frames/f0.html"], whole.Manifest)
	}
	if gone, err := designroom.Open(b.doc).FrameNow("frames/none.html"); err != nil || gone.Present || gone.Manifest != whole.Manifest {
		t.Errorf("a frame the room does not hold = %+v, %v", gone, err)
	}

	b.set("frames/f2.html|n|"+pidOf(t, whole.Frames["frames/f2.html"], "<h1")+"|rich", "not a text")
	if _, err := designroom.Open(b.doc).Read(); err == nil {
		t.Fatal("the whole room reads although a frame in it cannot be built")
	}
	if again, err := designroom.Open(b.doc).FrameNow("frames/f0.html"); err != nil || again.Text != one.Text {
		t.Errorf("one frame beside a frame that cannot be built = %+v, %v", again, err)
	}
}
