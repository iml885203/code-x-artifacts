package designsourceedit

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

// BlockRestore carries exact removed declaration bytes and a conditional source
// placement for transported own Undo. It cannot replace an existing block.
type BlockRestore struct {
	Kind   string      `json:"kind"`
	Source string      `json:"source"`
	Anchor BlockAnchor `json:"anchor"`
}

// BlockAnchor names an element boundary through element-only source-tree indices.
// Context protects the gap between that boundary and the insertion point. Text
// in other gaps may change without moving or invalidating this placement.
type BlockAnchor struct {
	Path    []int  `json:"path,omitempty"`
	PID     string `json:"pid,omitempty"`
	Tag     string `json:"tag,omitempty"`
	End     bool   `json:"end,omitempty"`
	After   bool   `json:"after,omitempty"`
	Context string `json:"context,omitempty"`
}

type declarationState struct {
	logic, schema, values string
	hasLogic, hasProps    bool
}

func (e *frameEdit) declarations() (declarationState, error) {
	var s declarationState
	for _, d := range e.plan.Declarations {
		c, err := e.blockCell(d.Kind)
		if err != nil {
			return s, err
		}
		if d.Kind == "logic" {
			s.logic = c.next
			s.hasLogic = true
			continue
		}
		schema, ok, err := field(c.next, "schema")
		if err != nil || !ok {
			return s, fmt.Errorf("props.schema object is required")
		}
		values, _, err := field(c.next, "values")
		if err != nil {
			return s, err
		}
		s.schema = string(schema.raw)
		s.values = string(values.raw)
		s.hasProps = true
	}
	return s, nil
}

func desiredDeclarations(old declarationState, op Op) (declarationState, error) {
	desired := old
	desired.logic = op.Logic
	desired.hasLogic = op.Logic != ""
	if len(op.Schema) > 0 {
		desired.hasProps = strings.TrimSpace(string(op.Schema)) != "null"
		desired.schema = ""
		if desired.hasProps {
			if _, err := members(string(op.Schema)); err != nil {
				return declarationState{}, err
			}
			desired.schema = string(op.Schema)
		}
	}
	return desired, nil
}

func (e *frameEdit) logic(op Op) (Op, string, error) {
	old, err := e.declarations()
	if err != nil {
		return Op{}, "", err
	}
	desired, err := desiredDeclarations(old, op)
	if err != nil {
		return Op{}, "", err
	}
	current := UnitDigest(old.logic, json.RawMessage(old.schema))
	target := UnitDigest(desired.logic, json.RawMessage(desired.schema))
	if err := acceptUnit(op.Expected, current, target); err != nil {
		return Op{}, current, err
	}
	if old.hasProps && !desired.hasProps {
		c := e.cells["props"]
		if c.next != c.text {
			return Op{}, current, fmt.Errorf("props deletion cannot share a batch with prop edits")
		}
		valueDigest := ValueDigest(json.RawMessage(old.values))
		if err := conflict(op.ExpectedValuesSHA256, valueDigest); err != nil {
			return Op{}, valueDigest, err
		}
	}
	undo := Op{Kind: "set_logic", File: op.File, Expected: target, Logic: old.logic, Schema: json.RawMessage(`null`)}
	if old.hasProps {
		undo.Schema = json.RawMessage(old.schema)
	}
	kinds, restores, err := e.restoreOrder(op.RestoreBlocks)
	if err != nil {
		return Op{}, current, err
	}
	for _, kind := range kinds {
		if err := e.changeDeclaration(kind, old, desired, restores[kind], &undo); err != nil {
			return Op{}, current, err
		}
	}
	if !old.hasProps && desired.hasProps {
		undo.ExpectedValuesSHA256, err = restoredValuesDigest(op.File, restores["props"])
		if err != nil {
			return Op{}, current, err
		}
	}
	return undo, current, nil
}

func (e *frameEdit) restoreOrder(restores []BlockRestore) ([]string, map[string]BlockRestore, error) {
	byKind := map[string]BlockRestore{}
	kinds := []string{}
	for _, r := range restores {
		if (r.Kind != "logic" && r.Kind != "props") || byKind[r.Kind].Kind != "" {
			return nil, nil, fmt.Errorf("invalid or duplicate block restore")
		}
		byKind[r.Kind] = r
		kinds = append(kinds, r.Kind)
	}
	for _, kind := range []string{"logic", "props"} {
		if byKind[kind].Kind == "" {
			kinds = append(kinds, kind)
		}
	}
	return kinds, byKind, e.checkRestoredPlacements(restores)
}

func (e *frameEdit) changeDeclaration(kind string, old, desired declarationState, r BlockRestore, undo *Op) error {
	had, want := old.hasLogic, desired.hasLogic
	content := desired.logic
	if kind == "props" {
		had, want = old.hasProps, desired.hasProps
		content = `{"schema":` + desired.schema + `}`
	}
	if r.Kind != "" {
		if err := e.checkRestore(kind, want, had, r, desired); err != nil {
			return err
		}
		if had {
			return nil
		}
	}
	switch {
	case had && !want:
		if err := e.checkRemovedParent(kind); err != nil {
			return err
		}
		e.removeDeclaration(kind, undo)
	case !had && want:
		return e.createDeclaration(kind, content, r, desired)
	case had && want:
		return e.updateDeclaration(kind, content, desired.schema)
	}
	return nil
}

func (e *frameEdit) checkRestore(kind string, want, had bool, r BlockRestore, desired declarationState) error {
	if !want {
		return fmt.Errorf("restore requires a desired declaration")
	}
	if err := validateDeclaration(e.plan.File, kind, r.Source, desired); err != nil {
		return err
	}
	if !had {
		return nil
	}
	for _, d := range e.plan.Declarations {
		if d.Kind == kind && e.plan.Source[d.Span.StartByte:d.Span.EndByte] == r.Source {
			return nil
		}
	}
	return fmt.Errorf("%w: restore cannot replace an existing declaration", ErrConflict)
}

func (e *frameEdit) checkRemovedParent(kind string) error {
	for _, d := range e.plan.Declarations {
		if d.Kind == kind && e.literalParents[d.ParentPID] {
			return fmt.Errorf("declaration deletion and its parent literal edits overlap")
		}
	}
	return nil
}

func (e *frameEdit) updateDeclaration(kind, content, desiredSchema string) error {
	c := e.cells[kind]
	if kind == "logic" {
		c.next = content
		return nil
	}
	schema, _, err := field(c.next, "schema")
	if err != nil {
		return err
	}
	before, _ := canonical(schema.raw)
	after, _ := canonical(json.RawMessage(desiredSchema))
	if before != after {
		c.next = c.next[:schema.start] + desiredSchema + c.next[schema.end:]
	}
	return nil
}

func (e *frameEdit) createDeclaration(kind, content string, r BlockRestore, desired declarationState) error {
	raw := `<script type="text/x-pictor">` + content + `</script>`
	if kind == "props" {
		raw = `<script type="application/x-pictor-props+json">` + content + `</script>`
	}
	at := 0
	if r.Kind != "" {
		raw = r.Source
		var err error
		at, err = resolveAnchor(e.plan, r.Anchor)
		if err != nil {
			return err
		}
	}
	if err := validateDeclaration(e.plan.File, kind, raw, desired); err != nil {
		return err
	}
	e.insertDeclaration(at, raw)
	return nil
}

func restoredValuesDigest(file string, r BlockRestore) (string, error) {
	if r.Kind == "" {
		return ValueDigest(nil), nil
	}
	p, err := designsource.ParseFrame(file, r.Source)
	if err != nil {
		return "", err
	}
	b, err := block(p, "props")
	if err != nil {
		return "", err
	}
	v, _, err := field(b.text, "values")
	if err != nil {
		return "", err
	}
	return ValueDigest(v.raw), nil
}

func validateDeclaration(file, kind, raw string, want declarationState) error {
	p, err := designsource.ParseFrame(file, raw)
	if err != nil {
		return err
	}
	if len(p.Declarations) != 1 || p.Declarations[0].Kind != kind || p.Declarations[0].Span.StartByte != 0 || p.Declarations[0].Span.EndByte != len(raw) {
		return fmt.Errorf("expected exactly one whole %s declaration", kind)
	}
	b, err := block(p, kind)
	if err != nil {
		return err
	}
	if kind == "logic" {
		if b.text != want.logic {
			return fmt.Errorf("restored logic differs from requested source")
		}
	} else {
		s, _, err := field(b.text, "schema")
		if err != nil {
			return err
		}
		before, _ := canonical(s.raw)
		after, _ := canonical(json.RawMessage(want.schema))
		if before != after {
			return fmt.Errorf("restored schema differs from requested declarations")
		}
	}
	return nil
}

func (e *frameEdit) removeDeclaration(kind string, undo *Op) {
	for _, d := range e.plan.Declarations {
		if d.Kind != kind {
			continue
		}
		raw := e.plan.Source[d.Span.StartByte:d.Span.EndByte]
		undo.RestoreBlocks = append(undo.RestoreBlocks, BlockRestore{Kind: kind, Source: raw})
		if e.removed == nil {
			e.removed = map[string]int{}
		}
		e.removed[kind] = d.Span.StartByte
		e.cells[kind] = &cell{sourceBlock{d.Span.StartByte, d.Span.EndByte, raw}, ""}
	}
	sort.Slice(undo.RestoreBlocks, func(i, j int) bool {
		return e.removed[undo.RestoreBlocks[i].Kind] < e.removed[undo.RestoreBlocks[j].Kind]
	})
}

func (e *frameEdit) insertDeclaration(at int, raw string) {
	key := fmt.Sprintf("insert:%d", at)
	if c := e.cells[key]; c != nil {
		c.next += raw
		return
	}
	e.cells[key] = &cell{sourceBlock{at, at, ""}, raw}
}

func (e *frameEdit) anchorUndo(source string, undo *Op) error {
	if len(undo.RestoreBlocks) == 0 {
		return nil
	}
	plan, err := designsource.ParseFrame(e.plan.File, source)
	if err != nil {
		return err
	}
	for i, r := range undo.RestoreBlocks {
		at := e.removed[r.Kind]
		for _, c := range e.cells {
			if c.end <= e.removed[r.Kind] && c.next != c.text {
				at += len(c.next) - (c.end - c.start)
			}
		}
		undo.RestoreBlocks[i].Anchor = anchorAt(plan, at)
	}
	return nil
}

func (e *frameEdit) checkLiteralParent(pid string) error {
	for _, d := range e.plan.Declarations {
		if _, removed := e.removed[d.Kind]; removed && d.ParentPID == pid {
			return fmt.Errorf("declaration deletion and its parent literal edits overlap")
		}
	}
	return nil
}
