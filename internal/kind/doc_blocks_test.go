package kind

import (
	"strings"
	"testing"
)

const blocksSample = `{"tabs":[{"id":"t1","title":"Main","blocks":[
{"id":"h","type":"heading","level":1,"content":[{"text":"Plan"}]},
{"id":"p","type":"paragraph","content":[{"text":"Hello "},{"text":"world","marks":["bold"]}]},
{"id":"n","type":"pending","intent":"Goals: three outcomes"}]}]}`

func TestNoCodePathRendersFormat1AnyMore(t *testing.T) {
	d := Doc{}
	if _, err := d.Render("# Hi\n", "1"); err == nil {
		t.Error("Render drew format 1")
	}
	if _, err := d.Compose("1", "# Hi\n", nil, ""); err == nil {
		t.Error("Compose took format 1")
	}
	if _, err := d.Written("# Hi\n", "1"); err == nil {
		t.Error("Written read format 1")
	}
	if err := d.Validate("# Hi\n", "1"); err == nil {
		t.Error("Validate accepted format 1")
	}
}

func TestADocOfFormat2IsValidatedAndRenderedAsBlocks(t *testing.T) {
	d := Doc{}
	if err := d.Validate(blocksSample, "2"); err != nil {
		t.Fatalf("a valid block doc was refused: %v", err)
	}
	err := d.Validate(strings.Replace(blocksSample, `"level":1`, `"level":9`, 1), "2")
	if err == nil || !strings.Contains(err.Error(), "tabs[0].blocks[0].level") {
		t.Errorf("a bad block was refused with %v, want a message naming tabs[0].blocks[0].level", err)
	}
	page, err := d.Render(blocksSample, "2")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<title>Plan</title>", `<h1 id="plan" data-block="h">Plan</h1>`, "<strong>world</strong>", `class="pending"`, "Goals: three outcomes", "prefers-color-scheme: light"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if _, err := d.Render("# not json", "2"); err == nil {
		t.Error("rendering content that is not blocks succeeded")
	}
}

func TestDocComposeGivesBlocksTheirIdsAndRefusesTheWrongInput(t *testing.T) {
	d := Doc{}
	got, err := d.Compose("2", "", []byte(`[{"type":"rule"}]`), "")
	if err != nil || !strings.Contains(got, `"id":"b_`) || !strings.Contains(got, `"id":"t_`) {
		t.Errorf("Compose = %q, %v; want ids filled in", got, err)
	}
	for name, tc := range map[string]struct {
		format, written, structured, want string
	}{
		"blocks into format 1": {"1", "", `[{"type":"rule"}]`, "the only one is \"2\""},
		"both":                 {"2", "# Hi", `[{"type":"rule"}]`, "not both"},
		"neither":              {"2", "", ``, "needs content"},
		"an html page":         {"2", "<!doctype html><html></html>", ``, "kind html"},
		"a bad link":           {"2", "ok\n\n[x](javascript:alert(1))", ``, "line 3"},
		"bad blocks":           {"2", "", `[{"type":"nope"}]`, "tabs[0].blocks[0].type"},
	} {
		_, err := d.Compose(tc.format, tc.written, []byte(tc.structured), "")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want a refusal mentioning %q", name, err, tc.want)
		}
	}
}

func TestDocComposeConvertsMarkdownAndKeepsTheIdsOfBlocksThatDidNotChange(t *testing.T) {
	d := Doc{}
	first, err := d.Compose("2", "# Title\n\nOne.\n\nTwo.\n", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(first, "2"); err != nil {
		t.Fatalf("the converted doc does not validate: %v", err)
	}
	second, err := d.Compose("2", "# Title\n\nOne.\n\nTwo, edited.\n", nil, first)
	if err != nil {
		t.Fatal(err)
	}
	ids := func(content string) []string {
		var out []string
		for _, part := range strings.Split(content, `"id":"`)[1:] {
			id, _, _ := strings.Cut(part, `"`)
			out = append(out, id)
		}
		return out
	}
	a, b := ids(first), ids(second)
	if len(a) != 4 || len(b) != 4 || a[0] != b[0] || a[1] != b[1] || a[2] != b[2] || a[3] != b[3] {
		t.Errorf("ids changed on republish: %v then %v", a, b)
	}
	written, err := d.Written(second, "2")
	if err != nil || written != "# Title\n\nOne.\n\nTwo, edited.\n" {
		t.Errorf("Written = %q, %v", written, err)
	}
}
