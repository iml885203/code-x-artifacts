package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const blocksDoc = `[{"type":"heading","level":1,"content":[{"text":"Plan"}]},` +
	`{"type":"paragraph","content":[{"text":"Ship "},{"text":"beta","marks":["bold"]}]},` +
	`{"type":"pending","intent":"Goals: the three outcomes"}]`

func (h *harness) publishBlocks(tok, blocks string) artifactView {
	h.t.Helper()
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"Doc","kind":"doc","visibility":"organisation","blocks":`+blocks+`}`, tok)
	if res.StatusCode != http.StatusCreated {
		h.t.Fatalf("publish blocks: %d %s", res.StatusCode, out)
	}
	return decode[artifactView](h.t, out)
}

func TestPublishingBlocksOverHTTPAndMCPGivesTheSameResultAndTheSameRefusal(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)

	viaHTTP := h.publishBlocks(tok, blocksDoc)
	var blocks any
	if err := json.Unmarshal([]byte(blocksDoc), &blocks); err != nil {
		t.Fatal(err)
	}
	viaMCP := ag.ok("publish", map[string]any{"title": "Doc", "kind": "doc", "blocks": blocks})
	if viaHTTP.Kind != "doc" || viaHTTP.Format != "2" || viaMCP["kind"] != "doc" || viaMCP["format"] != "2" {
		t.Errorf("http %q/%q, mcp %v/%v, want doc/2 from both", viaHTTP.Kind, viaHTTP.Format, viaMCP["kind"], viaMCP["format"])
	}
	for name, id := range map[string]string{"http": viaHTTP.ID, "mcp": viaMCP["id"].(string)} {
		_, page := h.do(h.browser("alice", "pw-a"), "GET", "/raw/"+id, "", nil)
		for _, want := range []string{`data-block="b_`, "Goals: the three outcomes", "<strong>beta</strong>"} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: the page lacks %q", name, want)
			}
		}
	}
	read := ag.ok("read", map[string]any{"id": viaMCP["id"]})
	if got := read["content"].(string); got != "# Plan\n\nShip **beta**\n\n<!-- pictor:pending Goals: the three outcomes -->\n" {
		t.Errorf("read returns %q, want the Markdown of the blocks", got)
	}
}

func TestABadBlockIsRefusedTheSameOverHTTPAndMCPAndNothingIsStored(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)
	bad := `[{"type":"paragraph"},{"type":"heading","level":9}]`
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"Bad","kind":"doc","blocks":`+bad+`}`, tok)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("http refusal status %d: %s", res.StatusCode, out)
	}
	var httpErr struct{ Error string }
	if err := json.Unmarshal([]byte(out), &httpErr); err != nil {
		t.Fatal(err)
	}
	var badBlocks any
	if err := json.Unmarshal([]byte(bad), &badBlocks); err != nil {
		t.Fatal(err)
	}
	mcpErr := ag.refused("publish", map[string]any{"title": "Bad", "kind": "doc", "blocks": badBlocks})
	for _, refusal := range []string{httpErr.Error, mcpErr} {
		if !strings.Contains(refusal, "tabs[0].blocks[1].level") || !strings.Contains(refusal, "1 to 6") {
			t.Errorf("the refusal does not name the path and the rule:\n%s", refusal)
		}
	}
	if !strings.Contains(mcpErr, httpErr.Error) {
		t.Errorf("the two entrances refuse differently:\nhttp: %s\nmcp:  %s", httpErr.Error, mcpErr)
	}
	if list := ag.ok("list", nil); len(list["artifacts"].([]any)) != 0 {
		t.Errorf("a refused publish stored something: %v", list)
	}
}

func TestBlocksAreRefusedWhereTheyDoNotBelong(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	for name, tc := range map[string]struct{ body, want string }{
		"both content and blocks": {`{"title":"D","kind":"doc","content":"# Hi","blocks":[{"type":"rule"}]}`, "not both"},
		"an html artifact":        {`{"title":"D","kind":"html","blocks":[{"type":"rule"}]}`, "takes content, not blocks"},
		"format 1":                {`{"title":"D","kind":"doc","format":"1","blocks":[{"type":"rule"}]}`, "the versions are: 2"},
		"not blocks":              {`{"title":"D","kind":"doc","blocks":"# Hi"}`, "list of blocks"},
		"nothing at format 2":     {`{"title":"D","kind":"doc","format":"2","content":" "}`, "needs content"},
	} {
		res, out := putJSON(h, "POST", "/api/artifacts", tc.body, tok)
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, tc.want) {
			t.Errorf("%s: %d %.200s, want 400 mentioning %q", name, res.StatusCode, out, tc.want)
		}
	}
}

func TestBlocksWithoutAKindMakeADoc(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"D","blocks":[{"type":"rule"}]}`, h.token("alice"))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("%d %s", res.StatusCode, out)
	}
	if a := decode[artifactView](t, out); a.Kind != "doc" || a.Format != "2" {
		t.Errorf("kind %q format %q, want doc 2", a.Kind, a.Format)
	}
}

func TestABlockDocShowsInertWhateverItsStringsContainInsideTheSandbox(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	const evil = `<script>alert(1)</script><img src=x onerror=alert(2)>`
	blocks, err := json.Marshal([]map[string]any{
		{"type": "heading", "level": 1, "content": []map[string]any{{"text": evil}}},
		{"type": "paragraph", "content": []map[string]any{{"text": evil}, {"text": "go", "marks": []string{"link"}, "href": "https://example.com/?a=1&b=2"}}},
		{"type": "code", "text": evil},
		{"type": "pending", "intent": evil},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := h.publishBlocks(h.token("alice"), string(blocks))

	res, body := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil)
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.HasPrefix(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") {
		t.Fatalf("csp %q", csp)
	}
	if !strings.HasPrefix(body, "<!doctype html>") || !strings.Contains(body, "window.pictor") {
		t.Fatalf("bridge not injected after the doctype: %.80s", body)
	}
	_, main, found := strings.Cut(body, `<main class="doc">`)
	if !found {
		t.Fatalf("no document in the page: %s", body)
	}
	for _, bad := range []string{"<script", "<img", "onerror=alert(2)>"} {
		if strings.Contains(main, bad) {
			t.Errorf("the document body contains %q:\n%s", bad, main)
		}
	}
	if !strings.Contains(main, "&lt;script&gt;") || !strings.Contains(main, `href="https://example.com/?a=1&amp;b=2"`) {
		t.Errorf("the strings were not shown as text:\n%s", main)
	}
}
