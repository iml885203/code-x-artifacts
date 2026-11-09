package designsource

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

// Property is a bounded JSON schema. Objects reject undeclared keys; arrays
// replace as whole authored values rather than performing implicit deep merges.
type Property struct {
	Type       string              `json:"type"`
	Default    any                 `json:"default,omitempty"`
	Min        *float64            `json:"min,omitempty"`
	Max        *float64            `json:"max,omitempty"`
	MaxLength  int                 `json:"maxLength,omitempty"`
	Enum       []string            `json:"enum,omitempty"`
	Properties map[string]Property `json:"properties,omitempty"`
	Items      *Property           `json:"items,omitempty"`
	MaxItems   int                 `json:"maxItems,omitempty"`
}

// Props is a declared schema and authored per-key values.
type Props struct {
	Schema map[string]Property `json:"schema"`
	Values map[string]any      `json:"values"`
}

func decodeJSON(source string, out any) error {
	if e := checkUniqueJSON(source); e != nil {
		return e
	}
	d := json.NewDecoder(strings.NewReader(source))
	d.UseNumber()
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return fmt.Errorf("expected exactly one JSON value")
	}
	return nil
}

func parseProps(source string) (Props, error) {
	var p Props
	if len(source) > 64<<10 {
		return p, fmt.Errorf("props block exceeds 64 KiB")
	}
	if e := decodeJSON(source, &p); e != nil {
		return p, e
	}
	if p.Schema == nil {
		return p, fmt.Errorf("props.schema is required")
	}
	for _, name := range sortedKeys(p.Schema) {
		s := p.Schema[name]
		if !safeName(name) || name == "props" || name == "state" {
			return p, fmt.Errorf("props.schema.%s: reserved or invalid name", name)
		}
		if e := s.checkSchema("props.schema."+name, 0); e != nil {
			return p, e
		}
	}
	_, e := p.Resolve(nil)
	return p, e
}

func (s Property) checkSchema(path string, depth int) error {
	if depth > 16 {
		return fmt.Errorf("%s: schema exceeds depth 16", path)
	}
	switch s.Type {
	case "boolean", "asset":
	case "number":
		if s.Min != nil && s.Max != nil && *s.Min > *s.Max {
			return fmt.Errorf("%s: min exceeds max", path)
		}
	case "string":
		if s.MaxLength < 1 || s.MaxLength > 262144 {
			return fmt.Errorf("%s: string needs maxLength from 1 to 262144", path)
		}
	case "object":
		if e := s.objectSchema(path, depth); e != nil {
			return e
		}
	case "array":
		if e := s.arraySchema(path, depth); e != nil {
			return e
		}
	default:
		return fmt.Errorf("%s: unknown type %q", path, s.Type)
	}
	return s.checkDefault(path, depth)
}

func (s Property) validate(v any, path string, depth int, leaves *int) error {
	if depth > 16 {
		return fmt.Errorf("%s: value exceeds depth 16", path)
	}
	*leaves++
	if *leaves > 10000 {
		return fmt.Errorf("%s: value exceeds 10000 leaves", path)
	}
	valid := false
	switch s.Type {
	case "boolean":
		_, valid = v.(bool)
	case "number":
		valid = s.numberValue(v)
	case "string":
		valid = s.stringValue(v)
	case "asset":
		x, ok := v.(string)
		valid = ok && validAsset(x)
	case "object":
		return s.objectValue(v, path, depth, leaves)
	case "array":
		return s.arrayValue(v, path, depth, leaves)
	}
	if !valid {
		return fmt.Errorf("%s: incompatible %s value", path, s.Type)
	}
	return nil
}

func validAsset(s string) bool {
	return strings.HasPrefix(s, "/files/") && path.Clean(s) == s && !strings.ContainsAny(s, "?#\\%\r\n\t") && len(designfmt.CheckAttribute("img", "src", s)) == 0
}

// Resolve applies defaults, saved values, then declared import overrides. Every
// result is freshly decoded JSON; callers cannot mutate source defaults/values.
func (p Props) Resolve(overrides map[string]any) (map[string]any, error) {
	result := map[string]any{}
	for k, s := range p.Schema {
		if s.Default != nil {
			result[k] = s.Default
		}
	}
	for _, layer := range []map[string]any{p.Values, overrides} {
		for _, k := range sortedKeys(layer) {
			s, ok := p.Schema[k]
			if !ok {
				return nil, fmt.Errorf("props.values.%s: undeclared property", k)
			}
			leaves := 0
			if e := s.validate(layer[k], "props.values."+k, 0, &leaves); e != nil {
				return nil, e
			}
			result[k] = layer[k]
		}
	}
	if e := CheckData(result); e != nil {
		return nil, e
	}
	b, e := json.Marshal(result)
	if e != nil {
		return nil, e
	}
	if len(b) > 256<<10 {
		return nil, fmt.Errorf("props exceeds 256 KiB")
	}
	var detached map[string]any
	if e := decodeJSON(string(b), &detached); e != nil {
		return nil, e
	}
	return detached, nil
}

func (s Property) objectSchema(path string, depth int) error {
	if s.Properties == nil {
		return fmt.Errorf("%s: object needs properties", path)
	}
	for _, k := range sortedKeys(s.Properties) {
		if !safeName(k) {
			return fmt.Errorf("%s.%s: invalid name", path, k)
		}
		if e := s.Properties[k].checkSchema(path+"."+k, depth+1); e != nil {
			return e
		}
	}

	return nil
}

func (s Property) arraySchema(path string, depth int) error {
	if s.Items == nil || s.MaxItems < 1 || s.MaxItems > 10000 {
		return fmt.Errorf("%s: array needs items and maxItems from 1 to 10000", path)
	}
	if e := s.Items.checkSchema(path+"[]", depth+1); e != nil {
		return e
	}
	return nil
}

func (s Property) numberValue(v any) bool {
	n, ok := v.(json.Number)
	if !ok {
		return false
	}
	x, e := n.Float64()
	return e == nil && !math.IsInf(x, 0) && !math.IsNaN(x) && (s.Min == nil || x >= *s.Min) && (s.Max == nil || x <= *s.Max)
}

func (s Property) stringValue(v any) bool {
	x, ok := v.(string)
	if !ok || len([]rune(x)) > s.MaxLength {
		return false
	}
	if len(s.Enum) == 0 {
		return true
	}
	for _, allowed := range s.Enum {
		if x == allowed {
			return true
		}
	}
	return false
}

func (s Property) objectValue(v any, path string, depth int, leaves *int) error {
	obj, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: incompatible object value", path)
	}
	for _, k := range sortedKeys(obj) {
		child, exists := s.Properties[k]
		if !exists {
			return fmt.Errorf("%s.%s: undeclared property", path, k)
		}
		if e := child.validate(obj[k], path+"."+k, depth+1, leaves); e != nil {
			return e
		}
	}
	return nil
}

func (s Property) arrayValue(v any, path string, depth int, leaves *int) error {
	items, ok := v.([]any)
	if !ok || len(items) > s.MaxItems {
		return fmt.Errorf("%s: incompatible array value", path)
	}
	for i, item := range items {
		if e := s.Items.validate(item, fmt.Sprintf("%s[%d]", path, i), depth+1, leaves); e != nil {
			return e
		}
	}
	return nil
}

func (s Property) checkDefault(path string, depth int) error {
	if s.Default == nil {
		return nil
	}
	leaves := 0
	return s.validate(s.Default, path+".default", depth, &leaves)
}

// UnmarshalJSON refuses declarations with type-incompatible schema options or
// null defaults; no supported property type treats null as an authored value.
func (s *Property) UnmarshalJSON(data []byte) error {
	type plain Property
	var value plain
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.UseNumber()
	d.DisallowUnknownFields()
	if e := d.Decode(&value); e != nil {
		return e
	}
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(data, &fields); e != nil {
		return e
	}
	allowed := words("type default")
	for _, name := range strings.Fields(map[string]string{"number": "min max", "string": "maxLength enum", "object": "properties", "array": "items maxItems"}[value.Type]) {
		allowed[name] = true
	}
	for _, name := range sortedKeys(fields) {
		if !allowed[name] {
			return fmt.Errorf("%s schema does not accept %q", value.Type, name)
		}
	}
	if raw, ok := fields["default"]; ok && string(raw) == "null" {
		return fmt.Errorf("default must match its declared type; null is unsupported")
	}
	*s = Property(value)
	return nil
}
