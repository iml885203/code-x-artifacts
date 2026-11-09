package docfmt

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func dropdownDocument(t *testing.T, config map[string]any, extra map[string]any) string {
	t.Helper()
	run := map[string]any{"dropdown": config}
	for key, value := range extra {
		run[key] = value
	}
	raw, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	return one(`{"id":"p","type":"paragraph","content":[` + string(raw) + `]}`)
}

func TestDropdownOptionsAreBoundedDistinctTrimmedLabelsAndTheValueIsAnOption(t *testing.T) {
	t.Parallel()
	fifty := make([]string, 50)
	for i := range fifty {
		fifty[i] = fmt.Sprint(i)
	}
	for _, options := range [][]string{{"A"}, {"<script>", "王😀"}, {strings.Repeat("😀", 100)}, fifty} {
		raw := dropdownDocument(t, map[string]any{"options": options, "value": options[0]}, nil)
		d, err := Parse(raw)
		if err != nil {
			t.Errorf("valid options: %v", err)
			continue
		}
		encoded, err := Canonical(d)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Parse(encoded)
		if err != nil || !reflect.DeepEqual(d, again) {
			t.Fatalf("normalization lost dropdown: %s %v", encoded, err)
		}
	}
	invalid := []map[string]any{
		{},
		{"options": []string{"A"}},
		{"options": []string{}, "value": "A"},
		{"options": append(fifty, "overflow"), "value": "0"},
		{"options": nil, "value": "A"},
		{"options": "A", "value": "A"},
		{"options": []any{1}, "value": "A"},
		{"options": []string{"A", "A"}, "value": "A"},
		{"options": []string{""}, "value": ""},
		{"options": []string{" A"}, "value": " A"},
		{"options": []string{"A "}, "value": "A "},
		{"options": []string{"A\nB"}, "value": "A\nB"},
		{"options": []string{"A\rB"}, "value": "A\rB"},
		{"options": []string{"A\tB"}, "value": "A\tB"},
		{"options": []string{"A\x00B"}, "value": "A\x00B"},
		{"options": []string{"A\u2028B"}, "value": "A\u2028B"},
		{"options": []string{strings.Repeat("😀", 101)}, "value": "x"},
		{"options": []string{"A"}, "value": "B"},
		{"options": []string{"A"}, "value": 1},
		{"options": []string{"A"}, "value": "A", "color": "red"},
	}
	for _, config := range invalid {
		if err := Validate(dropdownDocument(t, config, nil)); err == nil || !strings.Contains(err.Error(), "tabs[0].blocks[0].content[0].dropdown") {
			t.Errorf("accepted %v or lost path: %v", config, err)
		}
	}
	for _, extra := range []map[string]any{{"text": "x"}, {"marks": []string{}}, {"href": ""}, {"user": "a", "name": "A"}, {"name": "A"}, {"date": "2026-10-24"}} {
		if err := Validate(dropdownDocument(t, map[string]any{"options": []string{"A"}, "value": "A"}, extra)); err == nil {
			t.Errorf("accepted mixed run %v", extra)
		}
	}
}

func TestDropdownTrimmingMatchesTheEditorAtBOMBoundariesAndKeepsInteriorBOM(t *testing.T) {
	t.Parallel()
	for _, label := range []string{"\uFEFFA", "A\uFEFF", "\uFEFFA\uFEFF"} {
		err := Validate(dropdownDocument(t, map[string]any{"options": []string{label}, "value": label}, nil))
		if err == nil || !strings.Contains(err.Error(), "dropdown.options[0]") {
			t.Errorf("boundary BOM accepted or lost path: %q: %v", label, err)
		}
	}
	for _, label := range []string{"A\uFEFFB", "A B", "王😀"} {
		if err := Validate(dropdownDocument(t, map[string]any{"options": []string{label}, "value": label}, nil)); err != nil {
			t.Errorf("interior label %q refused: %v", label, err)
		}
	}
}

func TestDropdownExportsOnlyTheSelectedLabelAndEscapesMarkup(t *testing.T) {
	t.Parallel()
	raw := dropdownDocument(t, map[string]any{"options": []string{"<script>", "unselected"}, "value": "<script>"}, nil)
	d := mustParse(t, raw)
	body := RenderBody(d)
	if !strings.Contains(body, `<span class="dropdown">&lt;script&gt;</span>`) || strings.Contains(body, "unselected") {
		t.Fatalf("static field: %s", body)
	}
	if got := ToMarkdown(d); !strings.Contains(got, `\<script\>`) || strings.Contains(got, "unselected") || strings.Contains(got, "dropdown") {
		t.Fatalf("Markdown field: %s", got)
	}
	if got := PlainText(d.Tabs[0].Blocks[0].Content); got != "<script>" {
		t.Fatalf("visible label: %s", got)
	}
}
