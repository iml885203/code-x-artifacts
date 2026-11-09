package kind

import (
	"strings"
	"testing"
)

func TestDocRefusesEmptyBinaryAndOversizedMarkdownWithAReadableReason(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		want    string // empty: accepted
	}{
		"plain markdown":          {"# Hi\n\ntext", ""},
		"over the limit":          {strings.Repeat("a", MaxDocBytes+1), "at most"},
		"empty":                   {"", "needs content"},
		"only whitespace":         {" \n\t ", "needs content"},
		"not utf-8":               {"ok \xff\xfe", "UTF-8"},
		"nul byte":                {"a\x00b", "NUL"},
		"an html page by mistake": {"<!doctype html><html><body><p>hi</p></body></html>", "Markdown"},
	} {
		_, err := (Doc{}).Compose("2", tc.content, nil, "")
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: refused: %v", name, err)
		case tc.want != "" && err == nil:
			t.Errorf("%s: accepted, want a refusal mentioning %q", name, tc.want)
		case tc.want != "" && !strings.Contains(err.Error(), tc.want):
			t.Errorf("%s: error %q does not mention %q", name, err, tc.want)
		}
	}
	for _, format := range []string{"1", "9", ""} {
		if err := (Doc{}).Validate(`{"tabs":[]}`, format); err == nil || !strings.Contains(err.Error(), `"2"`) {
			t.Errorf("format %q: %v, want a refusal that names format 2", format, err)
		}
	}
}

func TestDocHasOnlyTheBlocksFormat(t *testing.T) {
	d := Doc{}
	if got := d.FormatVersions(); len(got) != 1 || got[0] != "2" {
		t.Fatalf("FormatVersions = %v, want [2]: Markdown docs were migrated and format 1 is gone", got)
	}
	if CurrentFormat(d) != "2" || HasFormat(d, "1") || HasFormat(d, "") {
		t.Error("a doc is format 2 (blocks) and nothing else")
	}
	if _, err := Lookup("doc"); err != nil {
		t.Errorf("doc is not registered: %v", err)
	}
}

// pageOf is what a publisher's Markdown becomes as a page.
func pageOf(md string) (string, error) {
	content, err := (Doc{}).Compose("2", md, nil, "")
	if err != nil {
		return "", err
	}
	return (Doc{}).Render(content, "2")
}

func TestDocRendersMarkdownAsAReadingPage(t *testing.T) {
	page, err := pageOf("# Quarterly report\n\nSome **bold** text.\n\n- one\n- two\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n```go\nfmt.Println(1)\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<!doctype html>", `<meta charset="utf-8">`, `name="viewport"`, `<html lang="`,
		"<title>Quarterly report</title>", "<h1", "<strong>bold</strong>", "<li>one</li>", "<table", "<pre",
		"prefers-color-scheme: light", "color-scheme: dark",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("rendered page lacks %q", want)
		}
	}
}

func TestDocTitleFallsBackWhenThereIsNoHeading(t *testing.T) {
	page, err := pageOf("just a paragraph")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "<title>Document</title>") {
		t.Errorf("no fallback title: %.300s", page)
	}
	page, _ = pageOf("# <script>alert(1)</script> & co\n")
	if strings.Contains(page, "<title><script>") || !strings.Contains(page, "<title>") {
		t.Errorf("the title must be escaped text: %.400s", page)
	}
}

// Raw HTML in Markdown must never reach the page as markup. The sandbox is the
// second line of defence; this is the first.
func TestDocNeverEmitsRawHTMLOrScriptableURLs(t *testing.T) {
	for name, md := range map[string]string{
		"script block":      "<script>alert(1)</script>\n",
		"inline script":     "hello <script>alert(1)</script> world\n",
		"onerror image":     "<img src=x onerror=alert(1)>\n",
		"html in a table":   "| a |\n|---|\n| <b onclick=alert(1)>x</b> |\n",
		"iframe":            "<iframe src=\"https://evil.example\"></iframe>\n",
		"style block":       "<style>body{display:none}</style>\n",
		"form":              "<form action=\"https://evil.example\"><input name=p></form>\n",
		"comment trick":     "<!-- --><script>alert(1)</script>\n",
		"javascript link":   "[x](javascript:alert(1))\n",
		"mixed case link":   "[x](JaVaScRiPt:alert(1))\n",
		"entity-hidden":     "[x](&#106;avascript:alert(1))\n",
		"vbscript link":     "[x](vbscript:msgbox(1))\n",
		"data html link":    "[x](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)\n",
		"javascript image":  "![x](javascript:alert(1))\n",
		"data html image":   "![x](data:text/html,<script>alert(1)</script>)\n",
		"autolink":          "<javascript:alert(1)>\n",
		"reference link":    "[x][r]\n\n[r]: javascript:alert(1)\n",
		"heading with html": "# <img src=x onerror=alert(1)>\n",
		"link title":        "[x](https://a.example \"\\\" onmouseover=\\\"alert(1)\")\n",
	} {
		page, err := pageOf(md)
		if err != nil {
			continue // refused: nothing is stored, nothing can be drawn
		}
		_, body, found := strings.Cut(page, "<body")
		if !found {
			t.Fatalf("%s: no body in the page", name)
		}
		for _, bad := range []string{"<script", "<img src=x", "<iframe", "<form", "<style", "<input name", "onerror=", "onclick=", `href="javascript`, `src="javascript`, `href="vbscript`, "data:text/html"} {
			if strings.Contains(strings.ToLower(body), bad) {
				t.Errorf("%s: rendered body contains %q:\n%s", name, bad, body)
			}
		}
		if strings.Contains(body, `" onmouseover=`) {
			t.Errorf("%s: attribute injection survived:\n%s", name, body)
		}
	}
}

func TestDocKeepsSafeLinksAndImagesAndOpensExternalOnesInANewTab(t *testing.T) {
	page, err := pageOf("[site](https://example.com/a) [top](#intro) [mail](mailto:a@b.example)\n\n![chart](https://example.com/c.png)\n\n## Intro\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href="https://example.com/a"`, `target="_blank"`, `rel="noopener noreferrer"`,
		`href="#intro"`, `href="mailto:a@b.example"`, `src="https://example.com/c.png"`, `id="intro"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, `href="#intro" target=`) {
		t.Error("an in-page anchor must not open a new tab")
	}
}

func TestDocRendersGitHubFlavouredExtras(t *testing.T) {
	page, err := pageOf("~~gone~~\n\n- [x] done\n- [ ] todo\n\nhttps://example.com/auto\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<s>gone</s>", `type="checkbox"`, `href="https://example.com/auto"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

func TestDocExportsTheMarkdownOfItsBlocks(t *testing.T) {
	const md = "# T\n\nSome **bold** text.\n"
	content, err := (Doc{}).Compose("2", md, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	d, err := (Doc{}).Export(content, "2", "md", "")
	if err != nil {
		t.Fatal(err)
	}
	if string(d.Body) != md || d.Extension != "md" || !strings.HasPrefix(d.ContentType, "text/markdown") {
		t.Errorf("Export = %+v", d)
	}
	if got := (Doc{}).Formats(); len(got) != 2 || got[0] != "md" || got[1] != "docx" {
		t.Errorf("Formats = %v", got)
	}
	if _, err := (Doc{}).Export(content, "2", "pdf", ""); err == nil {
		t.Error("exported a format the kind does not list")
	}
	if _, err := (Doc{}).Export(md, "1", "md", ""); err == nil {
		t.Error("exported content of the removed format 1")
	}
}

func TestTheDocGuideTeachesTheMarkdownRules(t *testing.T) {
	g := (Doc{}).Guide()
	for _, want := range []string{"Markdown", "raw HTML", "tables", "https", "MB", "download"} {
		if !strings.Contains(g, want) {
			t.Errorf("the doc guide never mentions %q", want)
		}
	}
}

func TestTheDocGuideSaysWhatACommentOnWordsPointsAt(t *testing.T) {
	g := (Doc{}).Guide()
	for _, want := range []string{"anchor", "tab", "block", "quote", "original text changed"} {
		if !strings.Contains(g, want) {
			t.Errorf("the doc guide never mentions %q when it explains comments on words", want)
		}
	}
}
