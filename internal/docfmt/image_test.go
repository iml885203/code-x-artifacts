package docfmt

import (
	"strconv"
	"strings"
	"testing"
)

func TestAnImageHasOneOfFourSizesOrNone(t *testing.T) {
	t.Parallel()
	for size, px := range map[string]int{"small": 224, "medium": 336, "large": 504, "xlarge": 672} {
		doc := one(`{"id":"i","type":"image","src":"/files/abc/p.png","alt":"a","size":"` + size + `"}`)
		if err := Validate(doc); err != nil {
			t.Errorf("size %s: %v", size, err)
		}
		page := RenderBody(mustParse(t, doc))
		want := `<img src="/files/abc/p.png" alt="a" width="` + strconv.Itoa(px) + `" loading="lazy">`
		if !strings.Contains(page, want) {
			t.Errorf("size %s is not drawn at %d pixels: %s", size, px, page)
		}
	}
	if page := RenderBody(mustParse(t, one(`{"id":"i","type":"image","src":"/files/abc/p.png"}`))); strings.Contains(page, "width=") {
		t.Errorf("a picture with no size has a width: %s", page)
	}
	err := Validate(one(`{"id":"i","type":"image","src":"/files/abc/p.png","size":"huge"}`))
	if err == nil || !strings.Contains(err.Error(), "tabs[0].blocks[0].size: must be one of small, medium, large, xlarge") {
		t.Errorf("a size outside the four is refused with the choices, got %v", err)
	}
	if err := Validate(one(`{"id":"i","type":"rule","size":"small"}`)); err == nil {
		t.Error("a block that is not an image has a size")
	}
}

func TestAnImageSizeSurvivesMarkdownAsATitleAndOtherTitlesAreIgnored(t *testing.T) {
	t.Parallel()
	d := fromMD(t, "![a chart](/files/abc/c.png \"large\")\n\n![plain](/files/abc/d.png \"Quarterly numbers\")\n\n![none](/files/abc/e.png)\n", nil)
	got := d.Tabs[0].Blocks
	if len(got) != 3 || got[0].Size != "large" || got[1].Size != "" || got[2].Size != "" {
		t.Fatalf("sizes read from the titles: %+v", got)
	}
	md := ToMarkdown(d)
	if !strings.Contains(md, `![a chart](/files/abc/c.png "large")`) || strings.Contains(md, "Quarterly") {
		t.Errorf("the Markdown does not carry the size alone: %s", md)
	}
	again := fromMD(t, md, &d)
	if again.Tabs[0].Blocks[0].Size != "large" {
		t.Errorf("the size did not survive: %+v", again.Tabs[0].Blocks[0])
	}
}

func TestAPictureWhoseFileWasDeletedIsAPlaceholderAndOthersAreLeftAlone(t *testing.T) {
	t.Parallel()
	d := mustParse(t, one(`
{"id":"a","type":"image","src":"/files/abc/gone.png","alt":"A chart"},
{"id":"b","type":"image","src":"/files/abc/here.png","alt":"Kept","size":"small"},
{"id":"c","type":"image","src":"https://host.example/files/abc/gone.png"},
{"id":"d","type":"image","src":"/files/other/gone.png","alt":"another artifact's"},
{"id":"e","type":"image","src":"https://example.com/far.png","alt":"far"}`))
	page := RenderPage(d, "")
	got := MissingPictures(page, "abc", func(name string) bool { return name == "here.png" })
	for _, want := range []string{
		`<p class="missing" data-block="a">Picture deleted: A chart</p>`,
		`<p class="missing" data-block="c">Picture deleted</p>`,
		`<img src="/files/abc/here.png" alt="Kept" width="224" loading="lazy">`,
		`<img src="/files/other/gone.png"`,
		`<img src="https://example.com/far.png"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	zh := MissingPictures(RenderPage(mustParse(t, one(`{"id":"a","type":"image","src":"/files/abc/gone.png","alt":"圖表"}`)), ""), "abc", func(string) bool { return false })
	if !strings.Contains(zh, `<p class="missing" data-block="a">圖片已刪除: 圖表</p>`) {
		t.Errorf("a Chinese page gets the Chinese note: %s", zh)
	}
	if unchanged := MissingPictures(page, "abc", func(string) bool { return true }); unchanged != page {
		t.Error("a page whose files all exist was changed")
	}
}
