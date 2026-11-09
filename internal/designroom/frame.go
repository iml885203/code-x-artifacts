package designroom

import "fmt"

// Frame is the text of one frame as the room holds it now, for a viewer that
// draws the frame before any version holds it. False when the room has no such
// frame (or, for a room that keeps its structure per value, the design does not
// include it).
func (r *Room) Frame(file string) (string, bool, error) {
	now, err := r.FrameNow(file)
	return now.Text, now.Present, err
}

// OneFrame is one frame and the design.json beside it, read together.
type OneFrame struct {
	Manifest string
	Text     string
	Present  bool
}

// FrameNow reads one frame and the design.json from the same look at the room.
// It builds only that frame: serving one frame of a design must not cost every
// frame, because a canvas asks for each of them at once.
func (r *Room) FrameNow(file string) (OneFrame, error) {
	if r.writing.Load() {
		return OneFrame{}, ErrReadWhileWriting
	}
	e := r.entries()
	manifest := manifestOf(e)
	out := OneFrame{Manifest: manifest}
	if _, ok := e[shellKey(file, "root")]; !ok {
		return out, nil
	}
	if p, ok := projectStructure(e); ok && !p.hasFile(file) {
		return out, nil
	}
	m, err := r.model(e, file)
	if err != nil {
		return OneFrame{}, fmt.Errorf("%s: %w", file, err)
	}
	text, err := build(m)
	if err != nil {
		return OneFrame{}, fmt.Errorf("%s: %w", file, err)
	}
	out.Text, out.Present = text, true
	return out, nil
}
