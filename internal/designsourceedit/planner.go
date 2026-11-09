package designsourceedit

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

type cell struct {
	sourceBlock
	next string
}
type frameEdit struct {
	plan           designsource.FramePlan
	cells          map[string]*cell
	removed        map[string]int
	literalParents map[string]bool
}

func (e *frameEdit) blockCell(kind string) (*cell, error) {
	if c := e.cells[kind]; c != nil {
		return c, nil
	}
	b, err := block(e.plan, kind)
	if err != nil {
		return nil, err
	}
	c := &cell{b, b.text}
	e.cells[kind] = c
	return c, nil
}

func (e *frameEdit) apply(op Op) (Op, string, error) {
	switch op.Kind {
	case "set_prop":
		return e.prop(op)
	case "set_logic":
		return e.logic(op)
	case "set_literal":
		if err := e.checkLiteralParent(op.PID); err != nil {
			return Op{}, "", err
		}
		if e.literalParents == nil {
			e.literalParents = map[string]bool{}
		}
		e.literalParents[op.PID] = true
		change, undo, current, err := literalChange(e.plan, op)
		if err != nil {
			return Op{}, current, err
		}
		key, _ := json.Marshal([]any{"literal", op.PID, op.TextChild, op.Segment})
		e.cells[string(key)] = &cell{change.block, change.next}
		return undo, current, nil
	default:
		return Op{}, "", fmt.Errorf("unknown operation %q", op.Kind)
	}
}

func (e *frameEdit) prop(op Op) (Op, string, error) {
	if _, removed := e.removed["props"]; removed {
		return Op{}, "", fmt.Errorf("props deletion cannot share a batch with prop edits")
	}
	props, err := e.blockCell("props")
	if err != nil {
		return Op{}, "", err
	}
	values, ok, err := field(props.next, "values")
	if err != nil {
		return Op{}, "", err
	}
	if !ok {
		props.next, err = replaceField(props.next, "values", json.RawMessage(`{}`))
		if err != nil {
			return Op{}, "", err
		}
		values, _, err = field(props.next, "values")
		if err != nil {
			return Op{}, "", err
		}
	}
	old, _, err := field(string(values.raw), op.Key)
	if err != nil {
		return Op{}, "", err
	}
	current := ValueDigest(old.raw)
	if err := conflict(op.Expected, current); err != nil {
		return Op{}, current, err
	}
	if len(op.Value) > 0 {
		if _, err := canonical(op.Value); err != nil {
			return Op{}, current, err
		}
	}
	changed, err := replaceField(string(values.raw), op.Key, op.Value)
	if err != nil {
		return Op{}, current, err
	}
	props.next = props.next[:values.start] + changed + props.next[values.end:]
	return Op{Kind: "set_prop", File: op.File, Key: op.Key, Expected: ValueDigest(op.Value), Value: old.raw}, current, nil
}

func (e *frameEdit) source() (string, error) {
	changes := make([]*cell, 0, len(e.cells))
	for _, c := range e.cells {
		if c.next != c.text {
			changes = append(changes, c)
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].start == changes[j].start {
			return changes[i].end < changes[j].end
		}
		return changes[i].start < changes[j].start
	})
	last := 0
	for _, c := range changes {
		if c.start < last || c.end < c.start || c.end > len(e.plan.Source) {
			return "", fmt.Errorf("overlapping source operations")
		}
		last = c.end
	}
	source := e.plan.Source
	for i := len(changes) - 1; i >= 0; i-- {
		source = splice(source, changes[i].sourceBlock, changes[i].next)
	}
	return source, nil
}

func acceptUnit(expected, current, desired string) error {
	if current == desired {
		return nil
	}
	return conflict(expected, current)
}

func editOf(editors map[string]*frameEdit, file, source string) (*frameEdit, error) {
	if editor := editors[file]; editor != nil {
		return editor, nil
	}
	plan, err := designsource.ParseFrame(file, source)
	if err != nil {
		return nil, err
	}
	editor := &frameEdit{plan: plan, cells: map[string]*cell{}}
	editors[file] = editor
	return editor, nil
}
