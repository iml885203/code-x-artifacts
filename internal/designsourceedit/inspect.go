package designsourceedit

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

// AuthoredProp distinguishes a missing override from default/resolved values.
// Clients consume its server digest rather than maintaining a JSON canonicalizer.
type AuthoredProp struct {
	Present        bool   `json:"present"`
	ExpectedSHA256 string `json:"expectedSHA256"`
}

// ByteSpan uses exact half-open UTF-8 coordinates, never DOM/UTF-16 positions.
type ByteSpan struct {
	File      string `json:"file"`
	StartByte int    `json:"startByte"`
	EndByte   int    `json:"endByte"`
}

// LiteralUnit maps an authored IR Segment.Span to its source edit selector.
// Visual child indices cannot determine TextChild across omitted source blocks.
type LiteralUnit struct {
	File           string   `json:"file"`
	PID            string   `json:"pid"`
	TextChild      int      `json:"textChild"`
	Segment        int      `json:"segment"`
	Span           ByteSpan `json:"span"`
	ExpectedSHA256 string   `json:"expectedSHA256"`
}

// Units is read-only source inspection, including for historical/read-only
// viewers. A write proof belongs to the service's separately authorized snapshot.
type Units struct {
	PropsValuesSHA256      string                  `json:"propsValuesSHA256,omitempty"`
	LogicDeclarationSHA256 string                  `json:"logicDeclarationSHA256"`
	Props                  map[string]AuthoredProp `json:"props"`
	Literals               []LiteralUnit           `json:"literals"`
}

// Inspect reads one successful parser plan. It does not grant access or issue a
// source revision token. Source/runtime/version/epoch belong to service context.
func Inspect(plan designsource.FramePlan) (Units, error) {
	result := Units{Props: map[string]AuthoredProp{}, Literals: []LiteralUnit{}}
	var logic string
	var schema json.RawMessage
	if plan.Logic != nil {
		content, err := block(plan, "logic")
		if err != nil {
			return Units{}, err
		}
		logic = content.text
	}
	if plan.Props != nil {
		content, err := block(plan, "props")
		if err != nil {
			return Units{}, err
		}
		declaration, ok, err := field(content.text, "schema")
		if err != nil || !ok {
			return Units{}, fmt.Errorf("props.schema source is missing")
		}
		schema = declaration.raw
		props, err := inspectProps(plan, content.text)
		if err != nil {
			return Units{}, err
		}
		result.Props = props
		values, _, err := field(content.text, "values")
		if err != nil {
			return Units{}, err
		}
		result.PropsValuesSHA256 = ValueDigest(values.raw)
	}
	result.LogicDeclarationSHA256 = UnitDigest(logic, schema)
	slots, err := Layout(plan)
	if err != nil {
		return Units{}, err
	}
	for _, slot := range slots {
		if slot.Kind != "literal" {
			continue
		}
		span := ByteSpan{plan.File, slot.Start, slot.End}
		result.Literals = append(result.Literals, LiteralUnit{plan.File, slot.PID, slot.Child, slot.Segment, span, Digest(plan.Source[slot.Start:slot.End])})
	}
	return result, nil
}

func inspectProps(plan designsource.FramePlan, source string) (map[string]AuthoredProp, error) {
	result := map[string]AuthoredProp{}
	values, _, err := field(source, "values")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(plan.Props.Schema))
	for name := range plan.Props.Schema {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := member{}
		present := false
		if len(values.raw) > 0 {
			value, present, err = field(string(values.raw), name)
			if err != nil {
				return nil, err
			}
		}
		result[name] = AuthoredProp{present, ValueDigest(value.raw)}
	}
	return result, nil
}
