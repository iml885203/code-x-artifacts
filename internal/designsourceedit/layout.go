package designsourceedit

import (
	"fmt"
	"sort"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

// Slot is one source-owned room value. Literal is the only unprotected kind;
// logic/schema/prop and the envelope are writable only through validated CAS.
// Identity uses separate fields so imported/repeated source never aliases a DOM
// index or a delimiter-concatenated runtime instance path.
type Slot struct {
	Kind    string `json:"kind"`
	PID     string `json:"pid,omitempty"`
	Child   int    `json:"child,omitempty"`
	Segment int    `json:"segment,omitempty"`
	Key     string `json:"key,omitempty"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
}

// Layout partitions exact source bytes without a second template parser.
// Offsets address the normalized-once source supplied to this function.
func Layout(plan designsource.FramePlan) ([]Slot, error) {
	var out []Slot
	for _, declaration := range plan.Declarations {
		slots, err := blockSlots(plan, declaration.Kind)
		if err != nil {
			return nil, err
		}
		out = append(out, slots...)
	}
	if err := appendLayouts(plan, plan.Nodes, &out); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	last := 0
	for _, slot := range out {
		if slot.Start < last || slot.End < slot.Start || slot.End > len(plan.Source) {
			return nil, fmt.Errorf("overlapping source slots")
		}
		last = slot.End
	}
	return out, nil
}

func appendLayouts(plan designsource.FramePlan, nodes []*designsource.Node, out *[]Slot) error {
	for _, n := range nodes {

		// Style, script and head text are never editable literal runs.
		if n.Tag == "style" || n.Tag == "script" || n.Tag == "head" {
			continue
		}
		if n.PID != "" {
			slots, err := literalSlots(plan, n)
			if err != nil {
				return err
			}
			*out = append(*out, slots...)
		}
		if err := appendLayouts(plan, n.Children, out); err != nil {
			return err
		}
	}
	return nil
}

func blockSlots(plan designsource.FramePlan, kind string) ([]Slot, error) {
	b, err := block(plan, kind)
	if err != nil {
		return nil, err
	}
	if kind == "logic" {
		return []Slot{{Kind: "logic", Start: b.start, End: b.end}}, nil
	}
	schema, ok, err := field(b.text, "schema")
	if err != nil || !ok {
		return nil, fmt.Errorf("props.schema is required")
	}
	out := []Slot{{Kind: "schema", Start: b.start + schema.start, End: b.start + schema.end}}
	values, ok, err := field(b.text, "values")
	if err != nil || !ok {
		return out, err
	}
	fields, err := members(string(values.raw))
	if err != nil {
		return nil, err
	}
	for _, f := range fields {
		out = append(out, Slot{Kind: "prop", Key: f.key, Start: b.start + values.start + f.start, End: b.start + values.start + f.end})
	}
	return out, nil
}

func literalSlots(plan designsource.FramePlan, n *designsource.Node) ([]Slot, error) {
	var out []Slot
	gap := 0
	for _, child := range sourceChildren(plan, n) {
		c := child.node
		if c == nil || c.Kind != "text" {
			gap++
			continue
		}
		literal := 0
		for segment, s := range c.Segments {
			if s.Path != "" {
				literal++
				continue
			}
			start, end, err := segmentRange(plan, c, segment)
			if err != nil {
				return nil, err
			}
			out = append(out, Slot{Kind: "literal", PID: n.PID, Child: gap, Segment: literal, Start: start, End: end})
		}
	}
	return out, nil
}
