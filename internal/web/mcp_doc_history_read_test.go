package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
)

func writeLiveDocWords(t *testing.T, h *liveHarness, id, words string) {
	t.Helper()
	d := docfmt.Doc{Tabs: []docfmt.Tab{{ID: "tab_live", Title: "Document", Blocks: []docfmt.Block{{ID: "p_live", Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: words}}}}}}}
	err := h.app.live.hub.Edit(t.Context(), id, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) { return docroom.Open(doc).Write(d), nil })
	if err != nil {
		t.Fatal(err)
	}
}

func TestExplicitMCPDocVersionsKeepSavedBodyAndBlocksBeforeAndAfterProtectiveRestore(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	res, out := api(h.harness, "POST", "/api/artifacts", `{"title":"History","kind":"doc","content":"Initial saved."}`, tok)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish: %d %s", res.StatusCode, out)
	}
	a := decode[artifactView](t, out)
	agent := h.agent(tok)
	check := func(number int, words string) {
		checkSavedDocRead(t, h.harness, agent, tok, a.ID, number, words)
	}
	writeLiveDocWords(t, h, a.ID, "Unsaved before snapshot.")
	check(1, "Initial saved.") // Explicit latest is immutable too.
	if live := agent.ok("read", map[string]any{"id": a.ID}); !strings.Contains(live["content"].(string), "Unsaved before snapshot.") {
		t.Errorf("omitted version lost live content: %v", live)
	}
	h.clock.Advance(roomkind.Quiet)
	h.app.live.snap.Sweep(t.Context())
	writeLiveDocWords(t, h, a.ID, "Protected.")
	res, out = api(h.harness, "POST", "/api/artifacts/"+a.ID+"/versions/1/restore", "", tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d %s", res.StatusCode, out)
	}
	restored := decode[artifactView](t, out)
	check(restored.Version-1, "Protected.")
	writeLiveDocWords(t, h, a.ID, "After restore unsaved.")
	check(1, "Initial saved.")
	check(2, "Unsaved before snapshot.")
	check(restored.Version-1, "Protected.")
	check(restored.Version, "Initial saved.")
	for _, view := range []string{"markdown", "blocks"} {
		live := agent.ok("read", map[string]any{"id": a.ID, "view": view})
		data, err := json.Marshal(live)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "After restore unsaved.") {
			t.Errorf("omitted version %s did not read the live room: %s", view, data)
		}
	}
}

func TestMCPExplicitVersionReadsUseTheSameHistoryPermissionsAndRefusalsAsHTTP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	alice, bob := h.agent(aliceTok), h.agent(bobTok)
	made := alice.ok("publish", map[string]any{"title": "Shared doc", "kind": "doc", "content": "Visible saved.", "visibility": "organisation"})
	id := made["id"].(string)
	if read := bob.ok("read", map[string]any{"id": id, "version": 1}); !strings.Contains(read["content"].(string), "Visible saved.") {
		t.Fatal("reader lost explicit newest-version access")
	}
	if read := bob.ok("read", map[string]any{"id": id}); !strings.Contains(read["content"].(string), "Visible saved.") {
		t.Fatal("ordinary reader lost access to current content")
	}
	alice.ok("publish", map[string]any{"id": id, "title": "Shared doc", "kind": "doc", "content": "Visible newest.", "baseVersion": 1})
	if read := bob.ok("read", map[string]any{"id": id, "version": 2}); !strings.Contains(read["content"].(string), "Visible newest.") {
		t.Fatal("reader lost current saved-version access")
	}
	res, out := api(h, "GET", "/api/artifacts/"+id+"/versions/1", "", bobTok)
	refusal := bob.refused("read", map[string]any{"id": id, "version": 1})
	if res.StatusCode != http.StatusForbidden || !strings.Contains(refusal, "only the author and editors") || !strings.Contains(refusal, "older versions") {
		t.Fatalf("reader history permission drift: HTTP %d %s; MCP %s", res.StatusCode, out, refusal)
	}
	res, out = api(h, "GET", "/api/artifacts/"+id+"/versions/99", "", aliceTok)
	if refusal = alice.refused("read", map[string]any{"id": id, "version": 99}); res.StatusCode != http.StatusBadRequest || refusal != decode[map[string]string](t, out)["error"] {
		t.Fatalf("missing version drift: HTTP %d %s; MCP %s", res.StatusCode, out, refusal)
	}
	hidden := alice.ok("publish", map[string]any{"title": "Hidden", "kind": "doc", "content": "Private saved."})["id"].(string)
	checkHiddenVersionMatchesMissing(t, h, bob, bobTok, hidden)
	if res, out = patchEditors(h, aliceTok, id, `{"add":["bob"]}`); res.StatusCode != http.StatusOK {
		t.Fatalf("share: %d %s", res.StatusCode, out)
	}
	res, out = api(h, "GET", "/api/artifacts/"+id+"/versions/1", "", bobTok)
	saved := decode[versionView](t, out)
	if read := bob.ok("read", map[string]any{"id": id, "version": 1}); res.StatusCode != http.StatusOK || saved.Content == nil || read["content"] != *saved.Content {
		t.Errorf("named editor did not read the saved version: HTTP %d %s / MCP %v", res.StatusCode, out, read)
	}
}

func TestMCPHistoricalDesignFilesStaySavedWhileOmittedVersionReadsTheLiveFrame(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	frames := designFrames()
	frames["frames/hero.html"] = `<body><h1 data-pid="title">Old.</h1></body>`
	a := h.publishDesign(tok, frames, "")
	_, old := h.fileText(tok, a.ID, "frames/hero.html", 1)
	next := strings.Replace(old, ">Old.</h1>", ">New.</h1>", 1)
	res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, map[string]any{"title": "Landing", "content": designManifest, "baseVersion": 1, "files": map[string]any{"frames/hero.html": next}})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	appendLiveDesignText(t, h, a.ID, "alice", "title", " Alice.")
	agent := h.agent(tok)
	historical := agent.ok("read", map[string]any{"id": a.ID, "version": 1, "file": "frames/hero.html"})
	if historical["version"] != float64(1) || historical["latestVersion"] != float64(2) || historical["file"].(map[string]any)["text"] != old {
		t.Errorf("historical Design file changed: %v", historical)
	}
	current := agent.ok("read", map[string]any{"id": a.ID, "file": "frames/hero.html"})
	if got := current["file"].(map[string]any)["text"].(string); !strings.Contains(got, ">New. Alice.</h1>") {
		t.Errorf("omitted Design file version lost current live work: %s", got)
	}
}

func checkSavedDocRead(t *testing.T, h *harness, reader *agent, tok, id string, number int, words string) {
	t.Helper()
	res, raw := api(h, "GET", "/api/artifacts/"+id+"/versions/"+strconv.Itoa(number), "", tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("HTTP history: %d %s", res.StatusCode, raw)
	}
	saved := decode[versionView](t, raw)
	if saved.Content == nil || !strings.Contains(*saved.Content, words) {
		t.Fatalf("saved version %d lacks %q: %s", number, words, raw)
	}
	read := reader.ok("read", map[string]any{"id": id, "version": number})
	if read["content"] != *saved.Content || read["version"] != float64(number) || read["title"] != saved.Title || read["format"] != saved.Format {
		t.Errorf("MCP version %d must match immutable HTTP version %q: %v", number, *saved.Content, read)
	}
	blocks := reader.ok("read", map[string]any{"id": id, "version": number, "view": "blocks"})
	data, err := json.Marshal(blocks["tabs"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"text":"`+words+`"`) || blocks["version"] != float64(number) {
		t.Errorf("MCP blocks version %d must contain saved %q: %s", number, words, data)
	}
}

func checkHiddenVersionMatchesMissing(t *testing.T, h *harness, reader *agent, tok, hidden string) {
	t.Helper()
	if got, want := reader.refused("read", map[string]any{"id": hidden, "version": 1}), reader.refused("read", map[string]any{"id": "missing", "version": 1}); got != want {
		t.Errorf("hidden version differs from nonexistent artifact: %q != %q", got, want)
	}
	hiddenRes, hiddenBody := api(h, "GET", "/api/artifacts/"+hidden+"/versions/1", "", tok)
	missingRes, missingBody := api(h, "GET", "/api/artifacts/missing/versions/1", "", tok)
	if hiddenRes.StatusCode != http.StatusNotFound || hiddenRes.StatusCode != missingRes.StatusCode || hiddenBody != missingBody {
		t.Errorf("HTTP hidden history differs from missing: %d %s / %d %s", hiddenRes.StatusCode, hiddenBody, missingRes.StatusCode, missingBody)
	}
}
