package docfmt

import (
	"strings"
	"testing"
)

func TestADateIsAValidCalendarDayAndHasNoOtherRunFields(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"0001-01-01", "9999-12-31", "2024-02-29", "2000-02-29", "2026-10-24"} {
		raw := one(`{"id":"p","type":"paragraph","content":[{"date":"` + value + `"}]}`)
		d, err := Parse(raw)
		if err != nil {
			t.Errorf("%s: %v", value, err)
			continue
		}
		got, err := Canonical(d)
		if err != nil || got != raw {
			t.Errorf("date was lost: %s, %v", got, err)
		}
	}
	for _, run := range []string{`{"date":""}`, `{"date":null}`, `{"date":2026}`, `{"date":"0000-01-01"}`, `{"date":"2026-02-29"}`, `{"date":"1900-02-29"}`, `{"date":"2026-04-31"}`, `{"date":"2026-13-01"}`, `{"date":"2026-00-01"}`, `{"date":"2026-01-00"}`, `{"date":"2026-1-01"}`, `{"date":"2026-01-01T00:00:00Z"}`, `{"date":" 2026-01-01"}`, `{"date":"2026-01-01","text":"x"}`, `{"date":"2026-01-01","marks":[]}`, `{"date":"2026-01-01","href":""}`, `{"date":"2026-01-01","user":"a","name":"A"}`, `{"date":"2026-01-01","name":"A"}`} {
		err := Validate(one(`{"id":"p","type":"paragraph","content":[` + run + `]}`))
		if err == nil || !strings.Contains(err.Error(), "tabs[0].blocks[0].content[0]") {
			t.Errorf("%s: %v", run, err)
		}
	}
}

func TestADateExportsOnlyItsVisibleValueAndContributesToTheTitle(t *testing.T) {
	t.Parallel()
	d := mustParse(t, one(`{"id":"h","type":"heading","level":1,"content":[{"text":"Due "},{"date":"2026-10-24"},{"text":" now"}]}`))
	if got := Title(d); got != "Due 2026-10-24 now" {
		t.Errorf("title: %s", got)
	}
	if got := ToMarkdown(d); !strings.HasSuffix(got, "# Due 2026-10-24 now\n") {
		t.Errorf("markdown: %q", got)
	}
	if got := RenderBody(d); !strings.Contains(got, `Due <time datetime="2026-10-24">2026-10-24</time> now`) || strings.Contains(got, "<input") {
		t.Errorf("render: %s", got)
	}
	if got := RenderBody(Doc{Tabs: []Tab{{Blocks: []Block{{Type: Paragraph, Content: []Inline{{Date: `<script>"`}}}}}}}); strings.Contains(got, "<script>") {
		t.Errorf("unsafe date: %s", got)
	}
}
