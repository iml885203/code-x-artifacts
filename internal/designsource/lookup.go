package designsource

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

var (
	identifier = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
	pidPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
)

func safeName(s string) bool {
	return identifier.MatchString(s) && s != "__proto__" && s != "prototype" && s != "constructor"
}

// CheckPath accepts only own dotted paths, never JavaScript expressions.
func CheckPath(s string) error {
	for _, part := range strings.Split(s, ".") {
		if !safeName(part) {
			return fmt.Errorf("%q is not an own dotted lookup path", s)
		}
	}
	return nil
}

func segments(value string) ([]Segment, error) { return segmentsAt(value, SourceSpan{}) }

func segmentsAt(value string, span SourceSpan) ([]Segment, error) {
	offset := span.StartByte
	var out []Segment
	for value != "" {
		i := strings.Index(value, "{{")
		if i < 0 {
			if strings.Contains(value, "}}") {
				return nil, fmt.Errorf("unmatched binding close")
			}
			out = append(out, Segment{Literal: value, Span: SourceSpan{span.File, offset, offset + len(value)}})
			break
		}
		if i > 0 {
			out = append(out, Segment{Literal: value[:i], Span: SourceSpan{span.File, offset, offset + i}})
		}
		rest := value[i+2:]
		j := strings.Index(rest, "}}")
		if j < 0 {
			return nil, fmt.Errorf("unclosed binding")
		}
		path := strings.TrimSpace(rest[:j])
		if err := CheckPath(path); err != nil {
			return nil, err
		}
		out = append(out, Segment{Path: path, Span: SourceSpan{span.File, offset + i, offset + i + 2 + j + 2}})
		offset += i + 2 + j + 2
		value = rest[j+2:]
	}
	return out, nil
}

func wholePath(value string) (string, error) {
	s, e := segments(value)
	if e != nil {
		return "", e
	}
	if len(s) != 1 || s[0].Path == "" {
		return "", fmt.Errorf("must be one complete {{own.path}} binding")
	}
	return s[0].Path, nil
}

// Lookup traverses plain JSON objects only. No reflection, getters, prototypes
// or array pseudo-properties are evaluated. Missing paths yield nil.
func Lookup(data map[string]any, path string) (any, error) {
	if err := CheckPath(path); err != nil {
		return nil, err
	}
	var v any = data
	for _, s := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, nil
		}
		v = m[s]
	}
	return v, nil
}

func scalar(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", nil
	case string:
		return x, nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	case float64:
		if !math.IsInf(x, 0) && !math.IsNaN(x) {
			return fmt.Sprint(x), nil
		}
	case json.Number:
		if n, e := x.Float64(); e == nil && !math.IsInf(n, 0) && !math.IsNaN(n) {
			return string(x), nil
		}
	}
	return "", fmt.Errorf("binding requires a finite JSON scalar")
}

// ResolveText returns text content, never markup. Rich text callers retain the
// surrounding literal/mark nodes and cannot edit a binding as literal source.
func ResolveText(parts []Segment, data map[string]any) (string, error) {
	var out strings.Builder
	for _, p := range parts {
		if p.Path == "" {
			out.WriteString(p.Literal)
			continue
		}
		v, e := Lookup(data, p.Path)
		if e != nil {
			return "", e
		}
		s, e := scalar(v)
		if e != nil {
			return "", e
		}
		out.WriteString(s)
	}
	return out.String(), nil
}

// Truth follows scalar truth semantics; containers are rejected, not coerced.
func Truth(v any) (bool, error) {
	if _, e := scalar(v); e != nil {
		return false, e
	}
	switch x := v.(type) {
	case nil:
		return false, nil
	case bool:
		return x, nil
	case string:
		return x != "", nil
	case float64:
		return x != 0, nil
	case json.Number:
		n, _ := x.Float64()
		return n != 0, nil
	}
	return false, fmt.Errorf("invalid scalar")
}

// InstanceKey is typed identity: the number 1 differs from the string "1".
type InstanceKey struct{ Kind, Value string }

// RepeatKeys returns unique typed instance identities without mutating scope.
func RepeatKeys(r Repeat, items []any, data map[string]any) ([]InstanceKey, error) {
	if e := CheckData(items); e != nil {
		return nil, e
	}
	if len(items) > 1000 {
		return nil, fmt.Errorf("repeat has more than 1000 items")
	}
	seen := map[InstanceKey]bool{}
	out := make([]InstanceKey, 0, len(items))
	for i, item := range items {
		key := InstanceKey{"position", fmt.Sprint(i)}
		if !r.Positional {
			scope := make(map[string]any, len(data)+1)
			for k, v := range data {
				scope[k] = v
			}
			scope[r.As] = item
			v, e := Lookup(scope, r.Key)
			if e != nil {
				return nil, e
			}
			key, e = repeatKey(v)
			if e != nil {
				return nil, e
			}
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate repeat key %s %q", key.Kind, key.Value)
		}
		seen[key] = true
		out = append(out, key)
	}
	return out, nil
}

func repeatKey(v any) (InstanceKey, error) {
	switch x := v.(type) {
	case string:
		if len(x) > 128 {
			return InstanceKey{}, fmt.Errorf("repeat key exceeds 128 bytes")
		}
		return InstanceKey{"string", x}, nil
	case float64:
		return numericKey(x)
	case json.Number:
		n, e := x.Float64()
		if e == nil {
			return numericKey(n)
		}
	}
	return InstanceKey{}, fmt.Errorf("repeat key must be a finite integer or string")
}

func numericKey(n float64) (InstanceKey, error) {
	if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n {
		return InstanceKey{}, fmt.Errorf("repeat key must be a finite integer or string")
	}
	if n == 0 {
		n = 0
	} // Canonicalize negative zero, matching JSON/JavaScript key equality.
	encoded, e := json.Marshal(n)
	if e != nil {
		return InstanceKey{}, e
	}
	return InstanceKey{"number", string(encoded)}, nil
}
