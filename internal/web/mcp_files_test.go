package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestHTTPAndMCPPublishTheSameFilesTheSameWay(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	files := map[string]any{"app.js": appJS, "styles/site.css": "p{}", "data/rows.json": "[1]"}

	viaMCP := agent.ok("publish", map[string]any{"title": "Multi", "content": multiPage, "files": files})
	viaHTTP := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS, "styles/site.css": "p{}", "data/rows.json": "[1]"}, "")
	// Compared as data: a struct and a map put their keys in a different order.
	listed := func(files any) string { return string(mustJSON(t, decode[any](t, string(mustJSON(t, files))))) }
	if got, want := listed(viaMCP["files"]), listed(viaHTTP.Files); got != want {
		t.Errorf("the files after a publish: tool %s, http %s", got, want)
	}

	// An update by difference: the same on both, including null to remove.
	upd := map[string]any{"id": viaMCP["id"], "title": "Multi", "content": multiPage, "baseVersion": 1, "files": map[string]any{"app.js": "two", "data/rows.json": nil}}
	updated := agent.ok("publish", upd)
	if v := updated["version"]; v != float64(2) || len(updated["files"].([]any)) != 2 {
		t.Fatalf("update through the tool = %v", updated)
	}
	res, out := h.update(tok, viaHTTP.ID, 1, multiPage, map[string]string{"app.js": "two", "data/rows.json": remove})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update over http: %d %s", res.StatusCode, out)
	}
	if got, want := listed(updated["files"]), listed(decode[artifactView](t, out).Files); got != want {
		t.Errorf("the files after an update: tool %s, http %s", got, want)
	}

	// The same refusal, word for word, for the same mistake.
	refusal := agent.refused("publish", map[string]any{"title": "x", "content": "<p>x", "files": map[string]any{"../x.js": "1"}})
	res, out = h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "x", "content": "<p>x", "files": map[string]any{"../x.js": "1"}})
	if res.StatusCode != http.StatusBadRequest || decode[map[string]string](t, out)["error"] != refusal {
		t.Errorf("one mistake, two answers: tool %q, http %d %s", refusal, res.StatusCode, out)
	}
	if got := agent.refused("publish", map[string]any{"title": "x", "kind": "doc", "content": "# x", "files": map[string]any{"a.js": "1"}}); !strings.Contains(got, "takes no files") {
		t.Errorf("a doc with files: %q", got)
	}
	// A stale update with files is the same conflict.
	stale := agent.refused("publish", map[string]any{"id": viaMCP["id"], "title": "Multi", "content": multiPage, "baseVersion": 1, "files": map[string]any{"a.js": "x"}})
	if !strings.Contains(stale, "version 2") || !strings.Contains(stale, "baseVersion 2") {
		t.Errorf("stale update through the tool: %q", stale)
	}
}

func TestMCPReadReturnsTheFileListAndOneFilesText(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	a := agent.ok("publish", map[string]any{"title": "Multi", "content": multiPage, "visibility": "organisation", "files": map[string]any{"app.js": appJS, "styles/site.css": "p{}"}})
	id := a["id"].(string)
	agent.ok("publish", map[string]any{"id": id, "title": "Multi", "content": multiPage, "baseVersion": 1, "files": map[string]any{"app.js": "two"}})

	read := agent.ok("read", map[string]any{"id": id})
	files, _ := read["files"].([]any)
	if len(files) != 2 || files[0].(map[string]any)["path"] != "app.js" || files[0].(map[string]any)["contentType"] != "text/javascript; charset=utf-8" || files[1].(map[string]any)["size"] != float64(3) {
		t.Errorf("read lists files = %v", read["files"])
	}
	if read["content"] != multiPage {
		t.Error("read without file lost the page")
	}
	one := agent.ok("read", map[string]any{"id": id, "file": "app.js"})
	if f, _ := one["file"].(map[string]any); f["text"] != "two" || f["path"] != "app.js" || one["content"] != nil {
		t.Errorf("read of one file = %v", one)
	}
	old := agent.ok("read", map[string]any{"id": id, "file": "app.js", "version": 1})
	if f, _ := old["file"].(map[string]any); f["text"] != appJS || old["latestVersion"] != float64(2) {
		t.Errorf("read of one file of version 1 = %v", old)
	}
	oldPage := agent.ok("read", map[string]any{"id": id, "version": 1})
	if got := oldPage["files"].([]any); len(got) != 2 {
		t.Errorf("version 1's files = %v", got)
	}
	if got := agent.refused("read", map[string]any{"id": id, "file": "nope.js"}); !strings.Contains(got, "nope.js") || !strings.Contains(got, "app.js") {
		t.Errorf("a file that is not there: %q, want the files that are", got)
	}
	readersReadFiles(t, h.agent(h.token("bob")), id)
	versions := agent.ok("versions", map[string]any{"id": id})["versions"].([]any)
	if versions[0].(map[string]any)["fileCount"] != float64(2) {
		t.Errorf("versions = %v, want each to say how many files it has", versions)
	}
}

func TestMCPPublishDescribesFilesWellEnoughToUse(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.agent(h.token("alice"))
	tools, err := a.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name != "publish" {
			continue
		}
		for _, want := range []string{
			"RELATIVE", `<script src="app.js">`, "styles/site.css", `fetch("data/rows.json")`, "null removes", "carried over", "index.html",
			".mjs", "16 MB", "255", "511", "256 MB", "upload_asset", "window.pictor.db", "restore", "doc, slides) take no files",
		} {
			if !strings.Contains(tool.Description, want) {
				t.Errorf("the publish description never mentions %q", want)
			}
		}
		props := tool.InputSchema.(map[string]any)["properties"].(map[string]any)
		if d, _ := props["files"].(map[string]any)["description"].(string); !strings.Contains(d, "relative path") || !strings.Contains(d, "null") {
			t.Errorf("the files argument is described as %q", d)
		}
	}
}

func TestTheDeletePlanCountsTheFilesThatWouldGo(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	agent := h.agent(h.token("alice"))
	a := agent.ok("publish", map[string]any{"title": "Multi", "content": multiPage, "files": map[string]any{"app.js": appJS, "x.css": "a{}"}})
	plan := agent.ok("delete", map[string]any{"id": a["id"]})["plan"].(map[string]any)
	if plan["files"] != float64(2) {
		t.Errorf("delete plan = %v, want 2 files", plan)
	}
}

// readersReadFiles: a reader sees the newest files; the older versions stay with
// the people who may edit.
func readersReadFiles(t *testing.T, bob *agent, id string) {
	t.Helper()
	if got := bob.ok("read", map[string]any{"id": id, "file": "app.js"}); got["file"].(map[string]any)["text"] != "two" {
		t.Errorf("a reader's read of a file = %v", got)
	}
	if got := bob.refused("read", map[string]any{"id": id, "file": "app.js", "version": 1}); !strings.Contains(got, "editors") {
		t.Errorf("a reader reading an older version's file: %q", got)
	}
}
