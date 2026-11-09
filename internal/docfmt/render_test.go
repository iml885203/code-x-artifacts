package docfmt

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, content string) Doc {
	t.Helper()
	d, err := Parse(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return d
}

func TestRenderDrawsEveryBlockTypeAsReadingMarkup(t *testing.T) {
	t.Parallel()
	d := mustParse(t, one(`
{"id":"h","type":"heading","level":2,"content":[{"text":"Goals & plans"}]},
{"id":"p","type":"paragraph","content":[{"text":"a","marks":["bold","italic"]},{"text":" "},{"text":"b","marks":["underline","strike","highlight"]},{"text":"c","marks":["code"]},{"text":"l","marks":["link"],"href":"https://example.com/?a=1&b=2"},{"text":"top","marks":["link"],"href":"#goals-plans"},{"text":"x\ny"}]},
{"id":"u","type":"bullet_list","items":[{"content":[{"text":"one"}],"items":[{"content":[{"text":"inner"}]}]}]},
{"id":"o","type":"numbered_list","start":3,"items":[{"content":[{"text":"three"}]}]},
{"id":"k","type":"task_list","items":[{"checked":true,"content":[{"text":"done"}]},{"checked":false,"content":[{"text":"todo"}]}]},
{"id":"q","type":"quote","blocks":[{"id":"q1","type":"paragraph","content":[{"text":"said"}]}]},
{"id":"c","type":"code","lang":"go","text":"if a < b {}\n"},
{"id":"r","type":"rule"},
{"id":"t","type":"table","header":true,"rows":[{"cells":[{"align":"left","content":[{"text":"H"}]},{"align":"right","content":[{"text":"V"}]}]},{"cells":[{"content":[{"text":"1"}]},{"align":"center"}]}]},
{"id":"i","type":"image","src":"https://example.com/a.png","alt":"A \"pic\""},
{"id":"n","type":"pending","intent":"Goals: the three outcomes"}`))
	page := RenderBody(d)
	for _, want := range []string{
		`<h2 id="goals-plans" data-block="h">Goals &amp; plans</h2>`,
		`<strong><em>a</em></strong>`,
		`<u><s><mark>b</mark></s></u>`,
		`<code>c</code>`,
		`<a href="https://example.com/?a=1&amp;b=2" target="_blank" rel="noopener noreferrer">l</a>`,
		`<a href="#goals-plans">top</a>`,
		`x<br>y`,
		`<ul data-block="u"><li>one<ul><li>inner</li></ul></li></ul>`,
		`<ol start="3" data-block="o"><li>three</li></ol>`,
		`<input type="checkbox" disabled checked>`,
		`<input type="checkbox" disabled>`,
		"<blockquote data-block=\"q\"><p data-block=\"q1\">said</p>\n</blockquote>",
		`<pre data-block="c"><code class="language-go">if a &lt; b {}`,
		`<hr data-block="r">`,
		`<thead><tr><th class="align-left">H</th><th class="align-right">V</th></tr></thead>`,
		`<td>1</td><td class="align-center"></td>`,
		`<img src="https://example.com/a.png" alt="A &#34;pic&#34;" loading="lazy">`,
		`<div class="pending" data-block="n"><span class="pending-intent">Goals: the three outcomes</span></div>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("body lacks %s\n%s", want, page)
		}
	}
}

func TestRenderEscapesEveryStringSoScriptsAndHandlersStayInert(t *testing.T) {
	t.Parallel()
	const evil = `<script>alert(1)</script><img src=x onerror=alert(2)>" onmouseover="alert(3)`
	d := Doc{Tabs: []Tab{{ID: "t", Title: evil, Blocks: []Block{
		{ID: "a", Type: Paragraph, Content: []Inline{{Text: evil}, {Text: evil, Marks: []string{MarkBold}}}},
		{ID: "b", Type: Heading, Level: 1, Content: []Inline{{Text: evil}}},
		{ID: "c", Type: Code, Lang: "go", Text: evil},
		{ID: "d", Type: Pending, Intent: evil},
		{ID: "e", Type: Image, Src: "https://example.com/x.png", Alt: evil},
		{ID: "f", Type: BulletList, Items: []Item{{Content: []Inline{{Text: evil}}}}},
		{ID: "g", Type: Table, Rows: []Row{{Cells: []Cell{{Content: []Inline{{Text: evil}}}}}}},
	}}, {ID: "u", Title: evil, Blocks: nil}}}
	page := RenderPage(d, "/*shared*/")
	for _, bad := range []string{"<script", "<img src=x", `onerror=alert(2)>`, `" onmouseover="`} {
		if strings.Contains(page, bad) {
			t.Errorf("rendered page contains live %q", bad)
		}
	}
	if strings.Count(page, "<script") != 0 || strings.Contains(page, "<style>alert") {
		t.Error("a script tag survived")
	}
}

func TestRenderNeverEmitsAnAddressThatCouldRunScriptEvenFromUnvalidatedData(t *testing.T) {
	t.Parallel()
	// Validate refuses these; the renderer is a second line, because a stored
	// doc is data from a time before any rule change.
	d := Doc{Tabs: []Tab{{ID: "t", Title: "t", Blocks: []Block{
		{ID: "a", Type: Paragraph, Content: []Inline{{Text: "x", Marks: []string{MarkLink}, Href: "javascript:alert(1)"}}},
		{ID: "b", Type: Image, Src: "javascript:alert(1)", Alt: "x"},
	}}}}
	page := RenderBody(d)
	if strings.Contains(page, "javascript:") {
		t.Errorf("an unsafe address was written: %s", page)
	}
}

func TestRenderUsesTheFirstHeadingAsTheTitleAndTheLanguageOfTheText(t *testing.T) {
	t.Parallel()
	d := mustParse(t, one(`{"id":"h","type":"heading","level":1,"content":[{"text":"季度報告"}]}`))
	page := RenderPage(d, "/*shared*/")
	for _, want := range []string{"<!doctype html>", `<html lang="zh-Hant">`, "<title>季度報告</title>", `<main class="doc">`, "/*shared*/", ".pending"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if got := RenderPage(mustParse(t, one(`{"id":"r","type":"rule"}`)), ""); !strings.Contains(got, "<title>Document</title>") || !strings.Contains(got, `lang="en"`) {
		t.Error("a doc without a heading is titled Document, in English")
	}
}

func TestRenderGivesEveryHeadingAnAnchorAndKeepsThemUnique(t *testing.T) {
	t.Parallel()
	d := mustParse(t, one(`
{"id":"a","type":"heading","level":2,"content":[{"text":"Plan"}]},
{"id":"b","type":"heading","level":2,"content":[{"text":"Plan"}]},
{"id":"c","type":"heading","level":2,"content":[{"text":"??"}]}`))
	page := RenderBody(d)
	for _, want := range []string{`id="plan"`, `id="plan-1"`, `id="heading"`} {
		if !strings.Contains(page, want) {
			t.Errorf("no %s in %s", want, page)
		}
	}
}

func TestRenderShowsTabsOnlyWhenThereIsMoreThanOne(t *testing.T) {
	t.Parallel()
	single := RenderBody(mustParse(t, one(`{"id":"r","type":"rule"}`)))
	if strings.Contains(single, "<section") {
		t.Errorf("a single tab is not wrapped: %s", single)
	}
	two := RenderBody(mustParse(t, `{"tabs":[{"id":"t1","title":"One <b>","blocks":[]},{"id":"t2","title":"Two","blocks":[{"id":"r","type":"rule"}]}]}`))
	for _, want := range []string{`<section class="tab" data-tab="t1">`, `<p class="tab-title">One &lt;b&gt;</p>`, `data-tab="t2"`} {
		if !strings.Contains(two, want) {
			t.Errorf("tabs: no %s in %s", want, two)
		}
	}
}
