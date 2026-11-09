package web

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// curl does what an agent's shell does with a begin_publish answer: PUT the
// bytes of each missing blob to its URL, which carries the only credential.
func (h *harness) curl(begun map[string]any, texts map[string]string) {
	h.t.Helper()
	for _, m := range begun["missing"].([]any) {
		m := m.(map[string]any)
		path := m["paths"].([]any)[0].(string)
		if res, out := h.put(m["url"].(string), texts[path]); res.StatusCode != http.StatusOK {
			h.t.Fatalf("curl %s: %d %s", path, res.StatusCode, out)
		}
	}
}

// asDataOf writes a value as JSON that does not depend on the order a struct or a map put its keys in.
func asDataOf(t *testing.T, v any) string {
	t.Helper()
	return string(mustJSON(t, decode[any](t, string(mustJSON(t, v)))))
}

func fileList(files ...map[string]any) []any {
	list := make([]any, len(files))
	for i, f := range files {
		list[i] = f
	}
	return list
}

func TestAnAgentPublishesAMegabytePageFromAFileWithBeginUploadAndCommit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	agent := h.agent(h.token("alice"))
	page := bigPage()

	begun := agent.ok("begin_publish", map[string]any{"title": "Big report", "files": fileList(declared("content", page))})
	if begun["session"] == "" || len(begun["missing"].([]any)) != 1 || begun["expiresInSeconds"] != float64(600) {
		t.Fatalf("begin_publish = %v", begun)
	}
	url := begun["missing"].([]any)[0].(map[string]any)["url"].(string)
	if next := begun["next"].(string); strings.Contains(next, url) || strings.Contains(next, "pup_") {
		t.Errorf("the next-step text repeats the secret URL: %s", next)
	}
	h.curl(begun, map[string]string{"content": page})

	done := agent.ok("commit_publish", map[string]any{"session": begun["session"]})
	if done["created"] != true || done["version"] != float64(1) || done["author"] != "alice" || done["role"] != "author" || !strings.Contains(done["next"].(string), "Give the person the url") {
		t.Fatalf("commit_publish = %v", done)
	}
	// Large-byte integrity belongs to the published state; MCP read serialization
	// is covered by TestAnAgentPublishesUpdatesSharesAndListsThroughTheMCPProtocol.
	if h.storedContent(done["id"].(string)) != page {
		t.Error("the stored page is not the bytes that were uploaded")
	}

	// The next version of the same page: send baseVersion, as with publish.
	page2 := page + "<!-- 2 -->"
	again := agent.ok("begin_publish", map[string]any{"id": done["id"], "title": "Big report", "baseVersion": 1, "files": fileList(declared("content", page2))})
	h.curl(again, map[string]string{"content": page2})
	if updated := agent.ok("commit_publish", map[string]any{"session": again["session"]}); updated["created"] != false || updated["version"] != float64(2) {
		t.Errorf("update through a session = %v", updated)
	}
	if h.storedContent(done["id"].(string)) != page2 {
		t.Error("the stored update is not the bytes that were uploaded")
	}
}

func TestASessionPublishAndADirectPublishMakeTheSameArtifactOverMCPAndHTTP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	site := map[string]string{"app.js": appJS, "styles/site.css": "p{}", "data/rows.json": "[1,2]"}

	direct := agent.ok("publish", map[string]any{"title": "Multi", "content": multiPage, "visibility": "organisation", "files": map[string]any{"app.js": appJS, "styles/site.css": "p{}", "data/rows.json": "[1,2]"}})
	declare := []any{declared("content", multiPage)}
	texts := map[string]string{"content": multiPage}
	for p, text := range site {
		declare = append(declare, declared(p, text))
		texts[p] = text
	}
	begun := agent.ok("begin_publish", map[string]any{"title": "Multi", "visibility": "organisation", "files": declare})
	h.curl(begun, texts)
	viaMCP := agent.ok("commit_publish", map[string]any{"session": begun["session"]})

	httpBegun := h.mustBegin(tok, map[string]any{"title": "Multi", "visibility": "organisation", "files": declare})
	h.uploadAll(httpBegun, texts)
	res, out := h.commit(tok, httpBegun.Session)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("commit over http: %d %s", res.StatusCode, out)
	}
	viaHTTP := decode[artifactView](t, out)

	// The same artifact however it got there: same kind, format, audience, version, author, files and page.
	for _, key := range []string{"kind", "format", "visibility", "version", "author", "title", "files", "role"} {
		if got, want := asDataOf(t, viaMCP[key]), asDataOf(t, direct[key]); got != want {
			t.Errorf("session over MCP: %s = %s, a direct publish has %s", key, got, want)
		}
	}
	if asDataOf(t, viaHTTP.Files) != asDataOf(t, direct["files"]) || viaHTTP.Kind != direct["kind"] || string(viaHTTP.Visibility) != direct["visibility"] || viaHTTP.Version != 1 {
		t.Errorf("session over HTTP = %+v, a direct publish has %v", viaHTTP.Artifact, direct)
	}
	for _, id := range []string{direct["id"].(string), viaMCP["id"].(string), viaHTTP.ID} {
		if h.storedContent(id) != multiPage {
			t.Errorf("artifact %s has other content", id)
		}
		if _, text := h.fileText(tok, id, "data/rows.json", 0); text != "[1,2]" {
			t.Errorf("artifact %s has other file bytes: %q", id, text)
		}
	}

	// begin answers in the same shape over both.
	keys := func(m map[string]any) []string {
		var ks []string
		for k := range m {
			if k != "next" {
				ks = append(ks, k)
			}
		}
		slices.Sort(ks)
		return ks
	}
	viaHTTPKeys := decode[map[string]any](t, string(mustJSON(t, httpBegun)))
	if !slices.Equal(keys(begun), keys(viaHTTPKeys)) {
		t.Errorf("begin answers with %v over MCP and %v over HTTP", keys(begun), keys(viaHTTPKeys))
	}
}

func TestACommitThatCannotPublishTellsTheAgentWhatToDo(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	agent := h.agent(h.token("alice"))

	begun := agent.ok("begin_publish", map[string]any{"title": "x", "files": fileList(declared("content", multiPage), declared("app.js", appJS))})
	got := agent.refused("commit_publish", map[string]any{"session": begun["session"]})
	if !strings.Contains(got, "nothing was published") || !strings.Contains(got, "app.js") || !strings.Contains(got, "content") || !strings.Contains(got, "begin_publish") {
		t.Errorf("a commit before the uploads: %s", got)
	}
	h.curl(begun, map[string]string{"content": multiPage, "app.js": appJS})
	agent.ok("commit_publish", map[string]any{"session": begun["session"]})
	if got := agent.refused("commit_publish", map[string]any{"session": begun["session"]}); !strings.Contains(got, "already committed") || !strings.Contains(got, "begin_publish") {
		t.Errorf("a second commit: %s", got)
	}
	if got := agent.refused("commit_publish", map[string]any{"session": "nope"}); !strings.Contains(got, "no such publish session") {
		t.Errorf("an unknown session: %s", got)
	}
	if got := agent.refused("commit_publish", map[string]any{"session": ""}); !strings.Contains(got, "session is required") {
		t.Errorf("no session: %s", got)
	}

	// A stale base is the conflict of publish, with the same way out.
	art := agent.ok("publish", map[string]any{"title": "Doc", "content": "<p>one</p>"})
	agent.ok("publish", map[string]any{"id": art["id"], "title": "Doc", "content": "<p>two</p>", "baseVersion": 1})
	stale := agent.refused("begin_publish", map[string]any{"id": art["id"], "title": "Doc", "baseVersion": 1, "files": fileList(declared("content", multiPage))})
	want := agent.refused("publish", map[string]any{"id": art["id"], "title": "Doc", "content": multiPage, "baseVersion": 1})
	if stale != want || !strings.Contains(stale, "Do not retry unchanged") {
		t.Errorf("a stale begin:\n%s\nthe same publish:\n%s", stale, want)
	}
}

func TestTheToolsAndGuidesTeachPublishingFromAFile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	agent := h.agent(h.token("alice"))
	tools, err := agent.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	desc := map[string]string{}
	for _, tool := range tools.Tools {
		desc[tool.Name] = tool.Description
	}
	for _, want := range []string{
		"about 50 KB", "shasum -a 256", "wc -c", `"content"`, "curl -sS --fail-with-body -X PUT --data-binary @", "Never print it", "10 minutes",
		"16 MB", "255 files", "commit_publish", "baseVersion", "remove", "stored", "missing",
	} {
		if !strings.Contains(desc["begin_publish"], want) {
			t.Errorf("the begin_publish description never says %q:\n%s", want, desc["begin_publish"])
		}
	}
	for _, want := range []string{"begin_publish", "same checks", "same result", "once"} {
		if !strings.Contains(desc["commit_publish"], want) {
			t.Errorf("the commit_publish description never says %q:\n%s", want, desc["commit_publish"])
		}
	}
	if !strings.Contains(desc["publish"], "begin_publish") || !strings.Contains(desc["publish"], "50 KB") {
		t.Errorf("the publish description does not send big content to begin_publish:\n%s", desc["publish"])
	}
	kinds := agent.ok("kinds", nil)
	for _, k := range kinds["kinds"].([]any) {
		m := k.(map[string]any)
		if g := m["guide"].(string); !strings.Contains(g, "begin_publish") {
			t.Errorf("the %s guide never mentions begin_publish", m["name"])
		}
	}
}
