package docfmt

import (
	"reflect"
	"strings"
	"testing"
)

// sample has every construct Markdown can say.
const sample = "# Plan 季度\n\n" +
	"Some **bold**, *italic*, ***both***, ~~old~~ and `code` with a [link](https://example.com/a?b=1&c=2), a [mail](mailto:a@b.co) and an [anchor](#plan).\n\n" +
	"Line one\\\nline two\n\n" +
	"## Lists\n\n" +
	"- one\n  - nested\n    - deeper\n- two\n\n" +
	"Between.\n\n" +
	"3. three\n4. four\n\n" +
	"Between again.\n\n" +
	"- [x] done\n- [ ] todo\n\n" +
	"> quoted **text**\n>\n> - in the quote\n>\n> ```\n> code in a quote\n> ```\n\n" +
	"```go\nfmt.Println(\"<hi>\")\n\n```\n\n" +
	"---\n\n" +
	"| Name | Qty | Mid |\n|:-----|----:|:---:|\n| a | 1 | x |\n| b \\| c | 2 | y |\n\n" +
	"![alt text](https://example.com/x.png)\n\n" +
	"<!-- pictor:pending Goals: the three outcomes -->\n\n" +
	"Literal \\*stars\\* and a_b_c and 1\\. not a list, https\\://example.com and www\\.example.com and \\<b\\> and \\&amp;.\n"

func fromMD(t *testing.T, md string, prev *Doc) Doc {
	t.Helper()
	d, err := FromMarkdown(md, prev, counting())
	if err != nil {
		t.Fatalf("FromMarkdown: %v\n%s", err, md)
	}
	return d
}

func TestMarkdownToBlocksKnowsEveryConstruct(t *testing.T) {
	t.Parallel()
	d := fromMD(t, sample, nil)
	if err := mustValid(d); err != nil {
		t.Fatalf("the converted doc does not validate: %v", err)
	}
	var types []string
	for _, b := range d.Tabs[0].Blocks {
		types = append(types, b.Type)
	}
	want := []string{Heading, Paragraph, Paragraph, Heading, BulletList, Paragraph, NumberedList, Paragraph, TaskList, Quote, Code, Rule, Table, Image, Pending, Paragraph}
	if !reflect.DeepEqual(types, want) {
		t.Fatalf("block types\n got %v\nwant %v", types, want)
	}
	checkTextBlocks(t, d.Tabs[0].Blocks)
	checkOtherBlocks(t, d.Tabs[0].Blocks)
}

func checkTextBlocks(t *testing.T, blocks []Block) {
	t.Helper()
	if h := blocks[0]; h.Level != 1 || plainText(h.Content) != "Plan 季度" {
		t.Errorf("heading = %+v", h)
	}
	marks := map[string][]string{}
	for _, r := range blocks[1].Content {
		marks[r.Text] = r.Marks
	}
	for text, want := range map[string][]string{"bold": {MarkBold}, "italic": {MarkItalic}, "both": {MarkBold, MarkItalic}, "old": {MarkStrike}, "code": {MarkCode}, "link": {MarkLink}} {
		if !reflect.DeepEqual(marks[text], want) {
			t.Errorf("marks of %q = %v, want %v", text, marks[text], want)
		}
	}
	if !strings.Contains(plainText(blocks[2].Content), "Line one\nline two") {
		t.Errorf("a hard break is a newline: %q", plainText(blocks[2].Content))
	}
	if text := plainText(blocks[15].Content); !strings.Contains(text, "*stars* and a_b_c and 1. not a list, https://example.com and www.example.com and <b> and &amp;.") {
		t.Errorf("escapes were not resolved: %q", text)
	}
}

func checkOtherBlocks(t *testing.T, blocks []Block) {
	t.Helper()
	if l := blocks[4]; len(l.Items) != 2 || len(l.Items[0].Items) != 1 || len(l.Items[0].Items[0].Items) != 1 {
		t.Errorf("nested list = %+v", l)
	}
	if n := blocks[6]; n.Start != 3 || len(n.Items) != 2 {
		t.Errorf("numbered list = %+v", n)
	}
	if k := blocks[8]; k.Items[0].Checked == nil || !*k.Items[0].Checked || *k.Items[1].Checked {
		t.Errorf("task list = %+v", k)
	}
	if c := blocks[10]; c.Lang != "go" || c.Text != "fmt.Println(\"<hi>\")\n" {
		t.Errorf("code = %q %q", c.Lang, c.Text)
	}
	if im := blocks[13]; im.Src != "https://example.com/x.png" || im.Alt != "alt text" {
		t.Errorf("image = %+v", im)
	}
	if p := blocks[14]; p.Intent != "Goals: the three outcomes" {
		t.Errorf("pending = %+v", p)
	}
}

func mustValid(d Doc) error {
	s, err := Canonical(d)
	if err != nil {
		return err
	}
	return Validate(s)
}

func TestMarkdownBlocksMarkdownBlocksIsTheSameBlocks(t *testing.T) {
	t.Parallel()
	first := fromMD(t, sample, nil)
	md := ToMarkdown(first)
	second := fromMD(t, md, &first)
	if !reflect.DeepEqual(first, second) {
		a, _ := Canonical(first)
		b, _ := Canonical(second)
		t.Fatalf("round trip changed the blocks\nmarkdown:\n%s\nfirst:  %s\nsecond: %s", md, a, b)
	}
	if again := ToMarkdown(second); again != md {
		t.Errorf("the Markdown is not stable:\n%s\n---\n%s", md, again)
	}
}

func TestATableKeepsItsAlignmentAndHeaderRowThroughTheRoundTrip(t *testing.T) {
	t.Parallel()
	d := fromMD(t, "| A | B | C | D |\n|:--|--:|:-:|---|\n| 1 | 2 | 3 | 4 |\n", nil)
	tb := d.Tabs[0].Blocks[0]
	if !tb.Header || len(tb.Rows) != 2 {
		t.Fatalf("table = %+v", tb)
	}
	var aligns []string
	for _, c := range tb.Rows[0].Cells {
		aligns = append(aligns, c.Align)
	}
	if !reflect.DeepEqual(aligns, []string{"left", "right", "center", ""}) {
		t.Errorf("alignment = %v", aligns)
	}
	md := ToMarkdown(d)
	if !strings.Contains(md, "| :-- | --: | :-: | --- |") {
		t.Errorf("the delimiter row lost the alignment:\n%s", md)
	}
	if back := fromMD(t, md, &d); !reflect.DeepEqual(back, d) {
		t.Errorf("table changed: %+v", back.Tabs[0].Blocks[0])
	}
}

func TestRepublishingWithOneParagraphChangedKeepsEveryOtherBlocksId(t *testing.T) {
	t.Parallel()
	first := fromMD(t, "# Title\n\nOne.\n\nTwo.\n\n- a\n- b\n\nThree.\n\n---\n\nFour.\n", nil)
	for name, edit := range map[string]string{
		"a paragraph changed": "# Title\n\nOne.\n\nTwo, edited.\n\n- a\n- b\n\nThree.\n\n---\n\nFour.\n",
		"a block inserted":    "# Title\n\nOne.\n\nNew.\n\nTwo.\n\n- a\n- b\n\nThree.\n\n---\n\nFour.\n",
		"a block removed":     "# Title\n\nOne.\n\n- a\n- b\n\nThree.\n\n---\n\nFour.\n",
		"a paragraph moved":   "# Title\n\nTwo.\n\nOne.\n\n- a\n- b\n\nThree.\n\n---\n\nFour.\n",
	} {
		next := fromMD(t, edit, &first)
		ids := map[string]string{} // content -> id
		for _, b := range first.Tabs[0].Blocks {
			ids[contentKey(b)] = b.ID
		}
		kept := 0
		for _, b := range next.Tabs[0].Blocks {
			if id, ok := ids[contentKey(b)]; ok {
				kept++
				if b.ID != id {
					t.Errorf("%s: unchanged block %s has id %s, want %s", name, contentKey(b), b.ID, id)
				}
			}
		}
		if next.Tabs[0].ID != first.Tabs[0].ID {
			t.Errorf("%s: the tab changed id", name)
		}
		if name == "a paragraph changed" {
			if next.Tabs[0].Blocks[2].ID != first.Tabs[0].Blocks[2].ID {
				t.Errorf("%s: the edited paragraph lost its id", name)
			}
			if kept != len(first.Tabs[0].Blocks)-1 {
				t.Errorf("%s: %d blocks kept their ids, want %d", name, kept, len(first.Tabs[0].Blocks)-1)
			}
		}
		seen := map[string]bool{}
		for _, b := range next.Tabs[0].Blocks {
			if seen[b.ID] {
				t.Errorf("%s: id %s is used twice", name, b.ID)
			}
			seen[b.ID] = true
		}
	}
}

func TestAPendingLineIsAPlaceholderThatStoresItsIntent(t *testing.T) {
	t.Parallel()
	d := fromMD(t, "# Report\n\n<!-- pictor:pending Goals: the three outcomes -->\n\n<!--pictor:pending  Risks -->\n\n<!-- just a comment -->\n", nil)
	blocks := d.Tabs[0].Blocks
	if len(blocks) != 3 || blocks[1].Type != Pending || blocks[1].Intent != "Goals: the three outcomes" || blocks[2].Intent != "Risks" {
		t.Fatalf("blocks = %+v", blocks)
	}
	page := RenderBody(d)
	if !strings.Contains(page, `<div class="pending"`) || !strings.Contains(page, "Goals: the three outcomes") || strings.Contains(page, "just a comment") {
		t.Errorf("page = %s", page)
	}
	if md := ToMarkdown(d); !strings.Contains(md, "<!-- pictor:pending Goals: the three outcomes -->") {
		t.Errorf("the skeleton does not read back as Markdown:\n%s", md)
	}
	if _, err := FromMarkdown("<!-- pictor:pending -->\n", nil, counting()); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Errorf("a pending line without an intent: %v", err)
	}
	if _, err := FromMarkdown("> <!-- pictor:pending x -->\n", nil, counting()); err == nil || !strings.Contains(err.Error(), "inside a quote") {
		t.Errorf("a pending line in a quote: %v", err)
	}
}

func TestAnUnsafeLinkOrImageRefusesTheWholeConversionAndNamesTheLine(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ md, want string }{
		"javascript":        {"# T\n\nfine\n\n[x](javascript:alert(1))\n", "line 5: link \"javascript:alert(1)\": the scheme javascript: is not allowed"},
		"in a list":         {"- a\n- see [x](javascript:alert(1))\n", "line 2"},
		"in a table":        {"| a |\n|---|\n| [x](data:text/html,x) |\n", "line 3"},
		"in a heading":      {"# [x](vbscript:x)\n", "line 1"},
		"mid paragraph":     {"one\ntwo [x](file:///etc/passwd) three\n", "line 2"},
		"a relative link":   {"[x](other.html)\n", "line 1: link \"other.html\": is a relative address"},
		"an image":          {"x\n\n![a](javascript:alert(1))\n", "line 3: image"},
		"a relative image":  {"![a](pic.png)\n", "line 1: image \"pic.png\""},
		"a reference style": {"[x][r]\n\n[r]: javascript:alert(1)\n", "line 1"},
	} {
		_, err := FromMarkdown(tc.md, nil, counting())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}

func TestLinksOfEveryAllowedKindConvert(t *testing.T) {
	t.Parallel()
	d := fromMD(t, "[a](https://x.co) [b](http://x.co) [c](mailto:a@b.co) [d](#top) <https://auto.co/p> <me@x.co> www.bare.co https://bare.co/x [e](https://x.co \"title\")\n", nil)
	var hrefs []string
	for _, r := range d.Tabs[0].Blocks[0].Content {
		if r.Href != "" {
			hrefs = append(hrefs, r.Href)
		}
	}
	want := []string{"https://x.co", "http://x.co", "mailto:a@b.co", "#top", "https://auto.co/p", "mailto:me@x.co", "http://www.bare.co", "https://bare.co/x", "https://x.co"}
	if !reflect.DeepEqual(hrefs, want) {
		t.Errorf("hrefs\n got %v\nwant %v", hrefs, want)
	}
}

func TestRawHTMLIsLeftOutAndNeverReachesTheBlocks(t *testing.T) {
	t.Parallel()
	d := fromMD(t, "# T\n\n<script>alert(1)</script>\n\n<img src=x onerror=alert(2)>\n\nSome <b>bold</b> text<i>.\n", nil)
	s, _ := Canonical(d)
	for _, bad := range []string{"script", "onerror", "<b>", "<i>"} {
		if strings.Contains(s, bad) {
			t.Errorf("the blocks contain %q: %s", bad, s)
		}
	}
	if text := plainText(d.Tabs[0].Blocks[len(d.Tabs[0].Blocks)-1].Content); text != "Some bold text." {
		t.Errorf("text = %q", text)
	}
}

func TestTabsBecomeMarkerLinesAndBack(t *testing.T) {
	t.Parallel()
	d := fromMD(t, "<!-- pictor:tab Overview -->\n\n# One\n\n<!-- pictor:tab Details -->\n\nTwo.\n", nil)
	if len(d.Tabs) != 2 || d.Tabs[0].Title != "Overview" || d.Tabs[1].Title != "Details" || d.Tabs[1].Blocks[0].Type != Paragraph {
		t.Fatalf("tabs = %+v", d.Tabs)
	}
	md := ToMarkdown(d)
	if !strings.HasPrefix(md, "<!-- pictor:tab Overview -->") {
		t.Errorf("markdown = %s", md)
	}
	if back := fromMD(t, md, &d); !reflect.DeepEqual(back, d) {
		t.Errorf("tabs changed")
	}
	plain := fromMD(t, "# Only\n", nil)
	if md := ToMarkdown(plain); strings.Contains(md, "pictor:tab") {
		t.Errorf("a single default tab has no marker: %s", md)
	}
}

func TestWhatMarkdownCannotHoldDegradesWithANoteAtTheEnd(t *testing.T) {
	t.Parallel()
	d := Doc{Tabs: []Tab{{ID: "t", Title: DefaultTabTitle, Blocks: []Block{
		{ID: "a", Type: Paragraph, Content: []Inline{{Text: "u", Marks: []string{MarkUnderline}}, {Text: " "}, {Text: "h", Marks: []string{MarkHighlight}}}},
		{ID: "b", Type: Paragraph},
		{ID: "c", Type: Table, Rows: []Row{{Cells: []Cell{{Content: []Inline{{Text: "x"}}}}}}},
	}}}}
	md := ToMarkdown(d)
	if !strings.Contains(md, "u h") || !strings.HasSuffix(md, "<!-- pictor: Markdown cannot hold underline, highlight, empty paragraphs, tables without a header row; they are plain text or left out here -->\n") {
		t.Errorf("markdown:\n%s", md)
	}
	if clean := ToMarkdown(fromMD(t, "# Fine\n", nil)); strings.Contains(clean, "cannot hold") {
		t.Errorf("a doc Markdown can hold has no note: %s", clean)
	}
}

func TestTextThatLooksLikeMarkdownComesBackAsTheSameText(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"# not a heading", "- not a list", "+ plus", "1. one", "2) two", "> not a quote", "=== not a setext", "--- not a rule",
		"*stars* _under_ `ticks` ~tilde~ [brackets](x) ![bang](x)", "<b>tags</b> &amp; &copy;", "https://not.a/link www.not.a/link a@b.co",
		"back\\slash \\*", "tail #", "| pipes |",
	} {
		d := Doc{Tabs: []Tab{{ID: "t", Title: DefaultTabTitle, Blocks: []Block{{ID: "p", Type: Paragraph, Content: []Inline{{Text: text}}}}}}}
		back := fromMD(t, ToMarkdown(d), &d)
		if got := plainText(back.Tabs[0].Blocks[0].Content); got != text || len(back.Tabs[0].Blocks) != 1 || back.Tabs[0].Blocks[0].Type != Paragraph {
			t.Errorf("%q came back as %q (%d blocks, first %s)\nmarkdown: %s", text, got, len(back.Tabs[0].Blocks), back.Tabs[0].Blocks[0].Type, ToMarkdown(d))
		}
	}
}
