package designroom

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

// MergeError is why a design an agent made cannot be merged into the room: a
// Conflict says someone changed the same thing meanwhile (read the room again
// and redo the change); otherwise what the agent wrote is not something the room
// can hold.
type MergeError struct {
	Conflict bool
	Msg      string
}

func (e *MergeError) Error() string { return e.Msg }

func conflict(format string, args ...any) error {
	return &MergeError{Conflict: true, Msg: fmt.Sprintf(format, args...)}
}

func notHeld(format string, args ...any) error {
	return &MergeError{Msg: fmt.Sprintf(format, args...)}
}

// Touched is what a merge changed: the pids of the elements in each frame file,
// sorted. An element that was removed is not there; its parent is.
type Touched map[string][]string

// Merge is the change that makes the room hold what an agent made of a version:
// next is base with the agent's changes, and only what differs between the two
// is written, into whatever the room holds now. What the agent did not change
// is not touched, so what people changed in the room meanwhile (another
// property of an element, another element, other words of a text) stays. What
// the agent changed is what the agent says, except where it cannot be both
// (the same words changed in both: a *MergeError that says Conflict).
//
// Elements are matched by their data-pid, which is why an element the agent
// leaves as it read it keeps its place. What a person deleted stays deleted; an
// element the agent adds gets an id the room does not already use.
func (r *Room) Merge(base, next Design) (func(*crdt.Transaction), Touched, error) {
	if r.writing.Load() {
		return nil, nil, ErrReadWhileWriting
	}
	e := r.entries()
	var steps []func(*crdt.Transaction)
	structure, err := r.mergeStructure(e, base.Manifest, next.Manifest)
	if err != nil {
		return nil, nil, err
	}
	if structure != nil {
		steps = append(steps, structure)
	}
	inRoom := map[string]bool{}
	for _, file := range frameFiles(e) {
		inRoom[file] = true
	}
	touched := Touched{}
	for _, file := range filesOf(base.Frames, next.Frames) {
		fm, err := r.mergeFile(e, file, base.Frames, next.Frames, inRoom[file])
		if err != nil {
			return nil, nil, err
		}
		steps = append(steps, fm.steps...)
		if pids := fm.touchedPIDs(); len(pids) > 0 {
			touched[file] = pids
		}
	}
	return func(txn *crdt.Transaction) {
		r.writing.Store(true)
		defer r.writing.Store(false)
		for _, step := range steps {
			step(txn)
		}
	}, touched, nil
}

// mergeFile merges one frame file of the agent's design: a frame the agent
// took out is taken out, one it added is added, and one it kept is merged by
// element. A frame someone deleted from the room since the version stays deleted.
func (r *Room) mergeFile(e map[string]any, file string, base, next map[string]string, inRoom bool) (*frameMerge, error) {
	baseText, hadBase := base[file]
	nextText, has := next[file]
	f := &frameMerge{}
	switch {
	case !has:
		if inRoom {
			keys := keysOf(e, file)
			f.step(func(txn *crdt.Transaction) {
				for _, k := range keys {
					r.m.Delete(txn, k)
				}
			})
		}
	case !hadBase:
		if inRoom {
			return nil, conflict("%s was added to the design by someone else while you were adding it; read the design again", file)
		}
		m, err := modelOf(nextText)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		f.step(func(txn *crdt.Transaction) { r.putFrame(txn, file, m) })
	case inRoom:
		return r.mergeFrame(e, file, baseText, nextText)
	}
	return f, nil
}

func filesOf(a, b map[string]string) []string {
	seen := map[string]bool{}
	for f := range a {
		seen[f] = true
	}
	for f := range b {
		seen[f] = true
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// mergeStructure is the change to the pages and frames the agent made to the
// manifest, and only that: an entry the agent's manifest changed from the
// version's is set (or dropped), so a page or frame someone added meanwhile
// stays. A room that still holds the one-entry manifest takes the agent's.
func (r *Room) mergeStructure(e map[string]any, base, next string) (func(*crdt.Transaction), error) {
	if sameDesign(base, next) {
		return nil, nil
	}
	if _, legacy := e[manifestKey]; legacy {
		return r.setStructure(e, next)
	}
	b, problems := designfmt.Parse(base)
	if len(problems) > 0 {
		return nil, fmt.Errorf("design.json of the version: %s", problems[0])
	}
	n, problems := designfmt.Parse(next)
	if len(problems) > 0 {
		return nil, notHeld("design.json: %s", problems[0])
	}
	was, want := structureEntries(b), structureEntries(n)
	var set, drop []string
	for k, v := range want {
		if was[k] != v {
			if cur, ok := e[k]; !ok || str(cur) != v {
				set = append(set, k)
			}
		}
	}
	for k := range was {
		if _, kept := want[k]; !kept {
			if _, there := e[k]; there {
				drop = append(drop, k)
			}
		}
	}
	sort.Strings(set)
	sort.Strings(drop)
	return func(txn *crdt.Transaction) {
		for _, k := range drop {
			r.m.Delete(txn, k)
		}
		for _, k := range set {
			r.m.Set(txn, k, want[k])
		}
	}, nil
}

// sameDesign says whether two design.json texts name the same pages and frames.
func sameDesign(a, b string) bool {
	if a == b {
		return true
	}
	x, problemsX := designfmt.Parse(a)
	y, problemsY := designfmt.Parse(b)
	return len(problemsX) == 0 && len(problemsY) == 0 && reflect.DeepEqual(x, y)
}
