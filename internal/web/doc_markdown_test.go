package web

import (
	"encoding/json"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

const skeleton = "# Quarterly review\n\nBy the platform team.\n\n" +
	"<!-- pictor:pending Goals: the three outcomes this quarter commits to -->\n\n" +
	"<!-- pictor:pending Risks and what we do about them -->\n"

var blockIDs = regexp.MustCompile(`data-block="([^"]+)"`)

// pageIDs are the ids of the blocks in the order the page draws them.
func (h *harness) pageIDs(viewer, id string) []string {
	h.t.Helper()
	_, body := h.do(h.browser(viewer, "pw-"+viewer[:1]), "GET", "/raw/"+id, "", nil)
	var ids []string
	for _, m := range blockIDs.FindAllStringSubmatch(body, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

func TestAnAgentPublishesAMarkdownSkeletonAndPeopleSeePlaceholdersAndReadsItBackAsMarkdown(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ag := h.agent(h.token("alice"))

	out := ag.ok("publish", map[string]any{"title": "Review", "kind": "doc", "visibility": "organisation", "content": skeleton})
	if out["format"] != "2" {
		t.Fatalf("a Markdown doc is stored as blocks, format %v", out["format"])
	}
	_, page := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+out["id"].(string), "", nil)
	for _, want := range []string{
		`<div class="pending"`, "Goals: the three outcomes this quarter commits to", "Risks and what we do about them", "<h1", "Quarterly review",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if _, main, _ := strings.Cut(page, `<main class="doc">`); strings.Contains(main, "pictor:pending") || strings.Contains(main, "&lt;!--") {
		t.Errorf("a placeholder is not shown as the Markdown line:\n%s", main)
	}
	read := ag.ok("read", map[string]any{"id": out["id"]})
	if read["content"] != skeleton {
		t.Errorf("read returns\n%q\nwant what was published\n%q", read["content"], skeleton)
	}
}

func TestRepublishingMarkdownKeepsTheIdsOfTheBlocksThatDidNotChange(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishDoc(tok, skeleton)
	before := h.pageIDs("alice", a.ID)
	if len(before) != 4 {
		t.Fatalf("ids %v, want 4 blocks", before)
	}

	filled := strings.Replace(skeleton, "<!-- pictor:pending Goals: the three outcomes this quarter commits to -->", "## Goals\n\nShip the editor.", 1)
	res, out := putJSON(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Doc","baseVersion":1,"content":`+jsonString(filled)+`}`, tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("republish: %d %s", res.StatusCode, out)
	}
	after := h.pageIDs("alice", a.ID)
	if len(after) != 5 {
		t.Fatalf("ids %v, want 5 blocks", after)
	}
	if after[0] != before[0] || after[1] != before[1] || after[4] != before[3] {
		t.Errorf("unchanged blocks changed id: before %v, after %v", before, after)
	}
}

func TestAMarkdownLinkThatCouldRunScriptRefusesThePublishTheSameOverHTTPAndMCPAndNamesTheLine(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)
	md := "# Report\n\nfine\n\n[x](javascript:alert(1))\n"

	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"D","kind":"doc","content":`+jsonString(md)+`}`, tok)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d: %s", res.StatusCode, out)
	}
	var httpErr struct{ Error string }
	if err := json.Unmarshal([]byte(out), &httpErr); err != nil {
		t.Fatal(err)
	}
	mcpErr := ag.refused("publish", map[string]any{"title": "D", "kind": "doc", "content": md})
	if !strings.Contains(httpErr.Error, "line 5") || !strings.Contains(httpErr.Error, "javascript:") || !strings.Contains(mcpErr, httpErr.Error) {
		t.Errorf("http %q\nmcp %q", httpErr.Error, mcpErr)
	}
	if list := ag.ok("list", nil); len(list["artifacts"].([]any)) != 0 {
		t.Errorf("a refused publish stored something: %v", list)
	}
}

func TestAFormat2DocDownloadsAsTheMarkdownOfItsBlocks(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	const md = "# Notes\n\nSome **bold** text and a [link](https://example.com).\n\n| a | b |\n| :-- | --: |\n| 1 | 2 |\n"
	a := h.publishDoc(tok, md)

	res, body := putJSON(h, "GET", "/api/artifacts/"+a.ID+"/export/md", "", tok)
	if res.StatusCode != http.StatusOK || body != md {
		t.Fatalf("export: %d %q", res.StatusCode, body)
	}
	_, version := putJSON(h, "GET", "/api/artifacts/"+a.ID+"/versions/1", "", tok)
	var v struct{ Content string }
	if err := json.Unmarshal([]byte(version), &v); err != nil || v.Content != md {
		t.Errorf("the version API gives the content as it is written: %q (%v)", v.Content, err)
	}
}

func TestAStaleUpdateOfAFormat2DocHandsBackTheNewestAsMarkdown(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishDoc(tok, "# One\n")
	if res, out := putJSON(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Doc","baseVersion":1,"content":"# Two\n"}`, tok); res.StatusCode != http.StatusOK {
		t.Fatalf("%d %s", res.StatusCode, out)
	}
	res, out := putJSON(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Doc","baseVersion":1,"content":"# Three\n"}`, tok)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("%d %s", res.StatusCode, out)
	}
	var body struct {
		Current struct{ Content, Format string }
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil || body.Current.Content != "# Two\n" || body.Current.Format != "2" {
		t.Errorf("current = %+v (%v), want the Markdown of version 2", body.Current, err)
	}
}

func TestPublishingFormat1IsRefusedWithAMessageThatNamesFormat2(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"D","kind":"doc","format":"1","content":"# Hi\n"}`, tok)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "the versions are: 2") {
		t.Errorf("http: %d %s", res.StatusCode, out)
	}
	mcp := h.agent(tok).refused("publish", map[string]any{"title": "D", "kind": "doc", "format": "1", "content": "# Hi\n"})
	if !strings.Contains(mcp, "the versions are: 2") {
		t.Errorf("mcp: %s", mcp)
	}
	a := h.publishDoc(tok, "# Hi\n")
	if res, out := putJSON(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"D","baseVersion":1,"format":"1","content":"# Again\n"}`, tok); res.StatusCode != http.StatusBadRequest {
		t.Errorf("an update that names format 1: %d %s", res.StatusCode, out)
	}
}

func TestAnExportIsSavedUnderTheTitleOfTheArtifactWhateverItHolds(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	for title, want := range map[string]string{
		"Q3 計畫":              "Q3 計畫.md",
		`a/b\c:d*e?"f<g>h|i`: "a b c d e f g h i.md",
		"  ..hidden  name  ": "hidden name.md",
	} {
		res, out := putJSON(h, "POST", "/api/artifacts", `{"title":`+jsonString(title)+`,"kind":"doc","content":"# Hi\n","visibility":"organisation"}`, tok)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("publish %q: %d %s", title, res.StatusCode, out)
		}
		a := decode[artifactView](t, out)
		res, _ = putJSON(h, "GET", "/api/artifacts/"+a.ID+"/export/md", "", tok)
		_, params, err := mime.ParseMediaType(res.Header.Get("Content-Disposition"))
		if err != nil {
			t.Fatalf("title %q: %v", title, err)
		}
		if params["filename"] != want {
			t.Errorf("title %q was saved as %q, want %q", title, params["filename"], want)
		}
	}
}

func TestExportFileNameFallsBackToTheIdWhenNothingOfTheTitleIsLeft(t *testing.T) {
	t.Parallel()
	if got := exportFileName(" /\\ ", "abc123", "md"); got != "abc123.md" {
		t.Errorf("got %q", got)
	}
	if got := exportFileName(strings.Repeat("長", 150), "id", "md"); len([]rune(got)) != 100+len(".md") {
		t.Errorf("a long title is cut to 100 characters: %d", len([]rune(got)))
	}
}

func TestADocExportWritesItsUploadedPicturesByTheAddressPeopleReachTheServerAt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishDoc(tok, "# Pics\n\n![Chart](/files/x/chart.png)\n")
	res, body := h.do(nil, "GET", "/api/artifacts/"+a.ID+"/export/md", "", map[string]string{"Authorization": "Bearer " + tok, "X-Forwarded-Proto": "https"})
	// The test server's own address, which the request names as its host.
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "![Chart](https://") || !strings.Contains(body, "/files/x/chart.png)") {
		t.Errorf("export: %d %q", res.StatusCode, body)
	}
}
