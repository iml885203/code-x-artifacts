// Package dbquery is the small language for asking a page's shared database
// for some of its documents, and the merge that updates one. It knows nothing
// about storage or who is asking; the rules are the whole package, so they are
// tested as a table.
//
// A filter is a JSON object. Each key names a field of the document's data
// ("score", or "owner.name" for a field inside a field), or is "id" for the
// document's id. The value is either a plain JSON value (the field must equal
// it) or an object of operators: "in" (one of a list), "gt", "gte", "lt",
// "lte" (numbers compare as numbers, strings as strings, nothing else
// compares) and "prefix" (a string field starts with it). "eq" is a plain
// equality that may hold an object. Every key and every operator must hold:
// they are ANDed. A missing field equals nothing, not even null. The empty
// filter matches every document.
package dbquery

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// IDField names the document's id in a filter; it is not a field of the data.
const IDField = "id"

var operators = []string{"eq", "gt", "gte", "in", "lt", "lte", "prefix"}

// Operators lists the operators a filter may use, for the guides and errors.
func Operators() []string { return slices.Clone(operators) }

type test struct {
	op  string
	arg any
}

type clause struct {
	field string
	path  []string
	tests []test
}

// Filter is a compiled filter.
type Filter struct{ clauses []clause }

// Compile reads a filter. An empty or null filter matches everything. A filter
// that is not understood is refused with a message that says what to send.
func Compile(raw json.RawMessage) (Filter, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return Filter{}, nil
	}
	var obj map[string]json.RawMessage
	if err := decode(raw, &obj); err != nil {
		return Filter{}, errors.New(`a filter is a JSON object such as {"status": "open", "score": {"gte": 3}}`)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var f Filter
	for _, k := range keys {
		if k == "" || slices.Contains(strings.Split(k, "."), "") {
			return Filter{}, fmt.Errorf("filter field %q is not a field name; name a field (\"score\"), a field inside a field (\"owner.name\") or \"id\"", k)
		}
		tests, err := compileTests(k, obj[k])
		if err != nil {
			return Filter{}, err
		}
		f.clauses = append(f.clauses, clause{field: k, path: strings.Split(k, "."), tests: tests})
	}
	return f, nil
}

func compileTests(field string, raw json.RawMessage) ([]test, error) {
	var ops map[string]json.RawMessage
	if err := decode(raw, &ops); err != nil || ops == nil {
		v, err := value(raw)
		if err != nil {
			return nil, fmt.Errorf("filter field %q: %w", field, err)
		}
		return []test{{"eq", v}}, nil
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("filter field %q: an empty object matches nothing useful; give an operator (%s) or use a plain value", field, strings.Join(operators, ", "))
	}
	names := make([]string, 0, len(ops))
	for n := range ops {
		names = append(names, n)
	}
	sort.Strings(names)
	var tests []test
	for _, n := range names {
		if !slices.Contains(operators, n) {
			return nil, fmt.Errorf("filter field %q: unknown operator %q; the operators are %s (to match an object, write {\"eq\": {...}})", field, n, strings.Join(operators, ", "))
		}
		v, err := value(ops[n])
		if err != nil {
			return nil, fmt.Errorf("filter field %q operator %q: %w", field, n, err)
		}
		switch n {
		case "in":
			if _, ok := v.([]any); !ok {
				return nil, fmt.Errorf("filter field %q: \"in\" takes a list, such as {\"in\": [\"a\", \"b\"]}", field)
			}
		case "prefix":
			if _, ok := v.(string); !ok {
				return nil, fmt.Errorf("filter field %q: \"prefix\" takes a string", field)
			}
		case "gt", "gte", "lt", "lte":
			switch v.(type) {
			case float64, string:
			default:
				return nil, fmt.Errorf("filter field %q: %q takes a number or a string to compare with", field, n)
			}
		}
		tests = append(tests, test{n, v})
	}
	return tests, nil
}

func value(raw json.RawMessage) (any, error) {
	var v any
	if err := decode(raw, &v); err != nil {
		return nil, errors.New("not a JSON value")
	}
	return v, nil
}

func decode(raw json.RawMessage, into any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := d.Decode(into); err != nil {
		return err
	}
	if d.More() {
		return errors.New("trailing data")
	}
	return nil
}

// Match reports whether a document with this id and JSON data satisfies the
// filter. Data that is not valid JSON matches nothing.
func (f Filter) Match(id string, data json.RawMessage) bool {
	if len(f.clauses) == 0 {
		return true
	}
	var doc any
	if err := decode(data, &doc); err != nil {
		return false
	}
	for _, c := range f.clauses {
		var got any
		var present bool
		if c.field == IDField {
			got, present = id, true
		} else {
			got, present = lookup(doc, c.path)
		}
		if !present {
			return false
		}
		for _, t := range c.tests {
			if !t.holds(got) {
				return false
			}
		}
	}
	return true
}

func lookup(v any, path []string) (any, bool) {
	for _, p := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok = m[p]; !ok {
			return nil, false
		}
	}
	return v, true
}

func (t test) holds(got any) bool {
	switch t.op {
	case "eq":
		return equal(got, t.arg)
	case "in":
		for _, want := range t.arg.([]any) {
			if equal(got, want) {
				return true
			}
		}
		return false
	case "prefix":
		s, ok := got.(string)
		return ok && strings.HasPrefix(s, t.arg.(string))
	}
	c, ok := compare(got, t.arg)
	if !ok {
		return false
	}
	switch t.op {
	case "gt":
		return c > 0
	case "gte":
		return c >= 0
	case "lt":
		return c < 0
	default:
		return c <= 0
	}
}

// compare orders two numbers or two strings; anything else is not comparable.
func compare(a, b any) (int, bool) {
	switch x := a.(type) {
	case float64:
		if y, ok := b.(float64); ok {
			switch {
			case x < y:
				return -1, true
			case x > y:
				return 1, true
			}
			return 0, true
		}
	case string:
		if y, ok := b.(string); ok {
			return strings.Compare(x, y), true
		}
	}
	return 0, false
}

func equal(a, b any) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !equal(v, w) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equal(x[i], y[i]) {
				return false
			}
		}
		return true
	}
	return a == b
}

// Merge applies a JSON merge patch (RFC 7396) to a document: fields in the
// patch replace or add, a field set to null is removed, and objects merge
// field by field. Arrays and scalars are replaced whole. Both must be JSON
// objects, since a document that is not one has no fields to update.
func Merge(data, patch json.RawMessage) (json.RawMessage, error) {
	var doc, p map[string]any
	if err := decode(data, &doc); err != nil || doc == nil {
		return nil, errors.New("this document is not a JSON object, so it has no fields to update; replace it with db_set")
	}
	if err := decode(patch, &p); err != nil || p == nil {
		return nil, errors.New("fields must be a JSON object, such as {\"status\": \"done\"}; a field set to null is removed")
	}
	mergeInto(doc, p)
	return json.Marshal(doc)
}

func mergeInto(doc, patch map[string]any) {
	for k, v := range patch {
		switch pv := v.(type) {
		case nil:
			delete(doc, k)
		case map[string]any:
			cur, ok := doc[k].(map[string]any)
			if !ok {
				cur = map[string]any{}
				doc[k] = cur
			}
			mergeInto(cur, pv)
		default:
			doc[k] = v
		}
	}
}
