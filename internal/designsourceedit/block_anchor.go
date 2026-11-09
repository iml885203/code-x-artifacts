package designsourceedit

import (
	"fmt"
	"slices"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

type elementBoundary struct {
	path     []int
	end      bool
	at       int
	pid, tag string
}

func elementBoundaries(plan designsource.FramePlan) []elementBoundary {
	out := []elementBoundary{{at: 0}, {end: true, at: len(plan.Source)}}
	var walk func([]*designsource.Node, []int)
	walk = func(nodes []*designsource.Node, path []int) {
		index := 0
		for _, n := range nodes {
			if n.Kind == "text" {
				continue
			}
			here := append(slices.Clone(path), index)
			index++
			out = append(out, elementBoundary{here, false, n.Span.StartByte, n.PID, n.Tag}, elementBoundary{here, true, n.Span.EndByte, n.PID, n.Tag})
			walk(n.Children, here)
		}
	}
	walk(plan.Nodes, nil)
	return out
}

func anchorAt(plan designsource.FramePlan, at int) BlockAnchor {
	boundaries := elementBoundaries(plan)
	best := boundaries[0]
	distance := func(n int) int {
		if n < 0 {
			return -n
		}
		return n
	}
	for _, b := range boundaries {
		if distance(at-b.at) < distance(at-best.at) {
			best = b
		}
	}
	anchor := BlockAnchor{Path: best.path, PID: best.pid, Tag: best.tag, End: best.end, After: at >= best.at}
	if anchor.After {
		anchor.Context = plan.Source[best.at:at]
	} else {
		anchor.Context = plan.Source[at:best.at]
	}
	return anchor
}

func resolveAnchor(plan designsource.FramePlan, anchor BlockAnchor) (int, error) {
	for _, b := range elementBoundaries(plan) {
		if b.end != anchor.End || b.pid != anchor.PID || b.tag != anchor.Tag || !slices.Equal(b.path, anchor.Path) {
			continue
		}
		start, end := b.at-len(anchor.Context), b.at
		at := start
		if anchor.After {
			start, end = b.at, b.at+len(anchor.Context)
			at = end
		}
		if start < 0 || end > len(plan.Source) || plan.Source[start:end] != anchor.Context {
			break
		}
		return at, nil
	}
	return 0, fmt.Errorf("%w: declaration placement changed", ErrConflict)
}

type presentRestore struct {
	restore   BlockRestore
	start, at int
}

// Resolve placement in the deletion baseline: restored blocks must not become
// gap context or move the element boundaries used by their own Undo.
func (e *frameEdit) checkRestoredPlacements(restores []BlockRestore) error {
	var present []presentRestore
	source := e.plan.Source
	for _, r := range restores {
		for _, d := range e.plan.Declarations {
			if d.Kind == r.Kind {
				present = append(present, presentRestore{restore: r, start: d.Span.StartByte})
			}
		}
	}
	if len(present) == 0 {
		return nil
	}
	for i := len(e.plan.Declarations) - 1; i >= 0; i-- {
		d := e.plan.Declarations[i]
		if slices.ContainsFunc(present, func(p presentRestore) bool { return d.Kind == p.restore.Kind }) {
			source = source[:d.Span.StartByte] + source[d.Span.EndByte:]
		}
	}
	plan, err := designsource.ParseFrame(e.plan.File, source)
	if err != nil {
		return err
	}
	for i := range present {
		present[i].at, err = resolveAnchor(plan, present[i].restore.Anchor)
		if err != nil {
			return err
		}
	}
	for i, p := range present {
		if p.start != restoredStart(present, i) {
			return fmt.Errorf("%w: declaration placement changed", ErrConflict)
		}
	}
	return nil
}

func restoredStart(present []presentRestore, index int) int {
	at := present[index].at
	expected := at
	for j, other := range present {
		if other.at < at || (other.at == at && j < index) {
			expected += len(other.restore.Source)
		}
	}
	return expected
}
