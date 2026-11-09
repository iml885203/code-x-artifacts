package web

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestAnAgentPublishesTokensAndEveryRawFrameUsesTheGrantedVersionsVariables(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	manifest := strings.Replace(twoFrameManifest, `"version":1`, `"version":1,"tokens":{"--accent":"#336699"}`, 1)
	source := `<!doctype html><html><head><link rel="stylesheet" href="/kit/v5/pictor.css"></head><body><div style="color:var(--accent)">Bound</div></body></html>`
	made := agent.ok("publish", map[string]any{"title": "Tokens", "kind": "design", "content": manifest, "files": map[string]any{"frames/hero.html": source, "frames/mobile.html": source}})
	id := made["id"].(string)
	first := agent.ok("read", map[string]any{"id": id})
	if !strings.Contains(first["content"].(string), `"--accent":"#336699"`) {
		t.Fatal(first)
	}
	for _, version := range []int{1, 2} {
		if version == 2 {
			agent.ok("publish", map[string]any{"id": id, "title": "Tokens", "content": strings.Replace(manifest, "#336699", "#112233", 1), "baseVersion": 1})
		}
		address := h.pageAddress(h.browser("alice", "pw-a"), id, fmt.Sprintf("?version=%d", version))
		for _, file := range []string{"frames/hero.html", "frames/mobile.html"} {
			res, raw := h.do(nil, "GET", address+file, "", nil)
			value := "#336699"
			if version == 2 {
				value = "#112233"
			}
			if res.StatusCode != http.StatusOK || !strings.Contains(raw, "--accent:"+value+";") || !strings.Contains(raw, "color:var(--accent)") {
				t.Fatalf("v%d %s: %d %s", version, file, res.StatusCode, raw)
			}
			if !strings.Contains(raw, `<style media="">`+string(kitCSS)+`</style>`) {
				t.Fatal("kit was not inlined")
			}
			_, stored := h.fileText(tok, id, file, version)
			if strings.Contains(stored, "data-pictor-tokens") || !strings.Contains(stored, "color:var(--accent)") {
				t.Fatalf("stored source: %s", stored)
			}
		}
	}
	// An old grant remains bound to its version after a later token publish.
	_, raw := h.do(nil, "GET", h.pageAddress(h.browser("alice", "pw-a"), id, "?version=1")+"frames/hero.html", "", nil)
	if !strings.Contains(raw, "--accent:#336699;") {
		t.Fatal("older frame got latest theme")
	}
	bad := strings.Replace(manifest, "#336699", "red; color:blue", 1)
	_, refused := agent.call("publish", map[string]any{"title": "Unsafe", "kind": "design", "content": bad, "files": map[string]any{"frames/hero.html": source, "frames/mobile.html": source}})
	if !strings.Contains(refused, "tokens") {
		t.Fatal("unsafe token was not refused:", refused)
	}
}
