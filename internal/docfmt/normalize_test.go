package docfmt

import (
	"fmt"
	"strings"
	"testing"
)

func counting() func(string) string {
	n := 0
	return func(prefix string) string { n++; return fmt.Sprintf("%s%d", prefix, n) }
}

func TestNormalizeGivesMissingIdsAndKeepsTheOnesSent(t *testing.T) {
	t.Parallel()
	got, err := Normalize([]byte(`{"tabs":[{"title":"A","blocks":[{"type":"paragraph"},{"id":"mine","type":"quote","blocks":[{"type":"rule"}]}]}]}`), counting())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"tabs":[{"id":"t_1","title":"A","blocks":[{"id":"b_2","type":"paragraph"},{"id":"mine","type":"quote","blocks":[{"id":"b_3","type":"rule"}]}]}]}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestNormalizeAListOfBlocksIsTheOnlyTab(t *testing.T) {
	t.Parallel()
	got, err := Normalize([]byte(`[{"type":"heading","level":1,"content":[{"text":"Hi"}]}]`), counting())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"tabs":[{"id":"t_1","title":"Document","blocks":[{"id":"b_2","type":"heading","level":1,"content":[{"text":"Hi"}]}]}]}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestNormalizeNeverReusesAnIdThatWasSent(t *testing.T) {
	t.Parallel()
	got, err := Normalize([]byte(`[{"id":"b_1","type":"rule"},{"type":"rule"}]`), counting())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"id":"b_1"`) || !strings.Contains(got, `"id":"b_2"`) || strings.Count(got, `"id":"b_1"`) != 1 {
		t.Errorf("ids collide: %s", got)
	}
}

func TestNormalizeIsCanonicalSoEqualDocsAreEqualBytes(t *testing.T) {
	t.Parallel()
	a, err := Normalize([]byte(`[ {"type" : "paragraph", "id":"x", "content":[{"marks":["link"],"href":"https://a.co/?a=1&b=2","text":"l"}]} ]`), counting())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Normalize([]byte(a), counting())
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("not stable:\n%s\n%s", a, b)
	}
	if !strings.Contains(a, `a=1&b=2`) {
		t.Errorf("& was escaped: %s", a)
	}
	if err := Validate(a); err != nil {
		t.Errorf("normalised content does not validate: %v", err)
	}
}

func TestNormalizeRefusesWhatValidateRefusesAndNamesThePath(t *testing.T) {
	t.Parallel()
	_, err := Normalize([]byte(`[{"type":"paragraph"},{"type":"heading","level":9}]`), counting())
	if err == nil || !strings.Contains(err.Error(), "tabs[0].blocks[1].level") {
		t.Errorf("err = %v", err)
	}
}

func TestTheListItemLimitNamesTheCount(t *testing.T) {
	t.Parallel()
	items := strings.TrimSuffix(strings.Repeat(`{},`, MaxListItems+1), ",")
	err := Validate(one(`{"id":"b1","type":"bullet_list","items":[` + items + `]}`))
	want := fmt.Sprintf("this doc has %d list items; at most %d are allowed", MaxListItems+1, MaxListItems)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want %q", err, want)
	}
}
