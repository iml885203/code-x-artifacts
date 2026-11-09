package designroom

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"unicode/utf16"
)

// The diff of one formatted text against another is written twice, here
// (mergerich.go) and in the editor (web/src/design/text/diff.ts), because the
// room and the browser are not one program. Every file under testdata/diff is a
// case both run: what the least change between `before` and `after` is.
type diffCase struct {
	Name   string            `json:"name"`
	Before []json.RawMessage `json:"before"`
	After  []json.RawMessage `json:"after"`
	Want   struct {
		Start   int              `json:"start"`
		End     int              `json:"end"`
		Insert  []any            `json:"insert"`
		Formats []map[string]any `json:"formats"`
	} `json:"want"`
}

func diffCases(t *testing.T) []diffCase {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "diff", "*.json"))
	if err != nil || len(files) < 10 {
		t.Fatalf("no diff cases: %v", err)
	}
	sort.Strings(files)
	var out []diffCase
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Clean(f))
		if err != nil {
			t.Fatal(err)
		}
		var c diffCase
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out = append(out, c)
	}
	return out
}

// opsOf reads the delta of a case: a run of characters or an embed, with its formats.
func opsOf(t *testing.T, delta []json.RawMessage) []richOp {
	t.Helper()
	var out []richOp
	for _, raw := range delta {
		var op struct {
			Insert     json.RawMessage `json:"insert"`
			Attributes map[string]any  `json:"attributes"`
		}
		if err := json.Unmarshal(raw, &op); err != nil {
			t.Fatal(err)
		}
		var text string
		if json.Unmarshal(op.Insert, &text) == nil {
			out = append(out, richOp{text: text, attrs: op.Attributes})
			continue
		}
		var embed map[string]any
		if err := json.Unmarshal(op.Insert, &embed); err != nil {
			t.Fatal(err)
		}
		out = append(out, richOp{embed: embed, attrs: op.Attributes})
	}
	return out
}

// deltaOfUnits is units as the runs of a delta: consecutive characters with the same formats are one run, an embed is its own.
func deltaOfUnits(units []unit) []any {
	out := []any{}
	var chars []uint16
	flush := func(attrs map[string]any) {
		if len(chars) == 0 {
			return
		}
		out = append(out, runOf(textOfUnits(chars), attrs))
		chars = nil
	}
	var last map[string]any
	for i, u := range units {
		if u.embed != nil {
			flush(last)
			out = append(out, runOf(u.embed, u.attrs))
			continue
		}
		if i > 0 && units[i-1].embed == nil && !sameFormats(last, u.attrs) {
			flush(last)
		}
		chars = append(chars, u.c)
		last = u.attrs
	}
	flush(last)
	return out
}

func runOf(insert any, attrs map[string]any) map[string]any {
	run := map[string]any{"insert": insert}
	if len(attrs) > 0 {
		run["attributes"] = attrs
	}
	return run
}

func textOfUnits(chars []uint16) string {
	return string(utf16.Decode(chars))
}

func TestTheDiffOfAFormattedTextIsTheLeastChangeTheSharedFixturesSay(t *testing.T) {
	t.Parallel()
	for _, c := range diffCases(t) {
		was, now := unitsOf(opsOf(t, c.Before)), unitsOf(opsOf(t, c.After))
		start, end, endNow := unitRange(was, now)
		formats := []map[string]any{}
		for _, f := range changedFormats(was, now, start, end) {
			formats = append(formats, map[string]any{"index": f.index, "attrs": map[string]any(f.attrs)})
		}
		got := map[string]any{"start": start, "end": end, "insert": deltaOfUnits(now[start:endNow]), "formats": formats}
		// Compared as JSON values, the way the fixtures hold them.
		raw, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var gotJSON map[string]any
		if err := json.Unmarshal(raw, &gotJSON); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"start": float64(c.Want.Start), "end": float64(c.Want.End), "insert": nonNil(c.Want.Insert), "formats": nonNilMaps(c.Want.Formats)}
		if !reflect.DeepEqual(gotJSON, want) {
			t.Errorf("%s: Go finds\n %v\nthe fixture says\n %v", c.Name, gotJSON, want)
		}
	}
}

func nonNil(v []any) []any {
	if v == nil {
		return []any{}
	}
	return v
}

func nonNilMaps(v []map[string]any) []any {
	out := []any{}
	for _, m := range v {
		out = append(out, m)
	}
	return out
}
