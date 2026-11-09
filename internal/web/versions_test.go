package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/kind/kindtest"
)

// api calls the HTTP API with a token that already exists.
func api(h *harness, method, path, body, tok string) (response, string) {
	return h.do(nil, method, path, body, map[string]string{"Authorization": "Bearer " + tok})
}

type versionRow struct {
	Version      int    `json:"version"`
	Title        string `json:"title"`
	Format       string `json:"format"`
	Content      string `json:"content"`
	RestoredFrom int    `json:"restoredFrom"`
}

func versionsOf(t *testing.T, h *harness, tok, id string) []versionRow {
	t.Helper()
	res, out := api(h, "GET", "/api/artifacts/"+id+"/versions", "", tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("versions: %d %s", res.StatusCode, out)
	}
	return decode[[]versionRow](t, out)
}

func TestEveryPublishIsAVersionAndAFreshUpdateSucceeds(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publish(tok, "One", "<p>one")
	if a.Version != 1 {
		t.Fatalf("a new artifact is version %d, want 1", a.Version)
	}
	res, out := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Two","content":"<p>two","baseVersion":1}`, tok)
	if got := decode[artifactView](t, out); res.StatusCode != http.StatusOK || got.Version != 2 || got.Title != "Two" {
		t.Fatalf("fresh update: %d %s", res.StatusCode, out)
	}
	list := versionsOf(t, h, tok, a.ID)
	if len(list) != 2 || list[0].Version != 2 || list[0].Title != "Two" || list[1].Version != 1 || list[1].Title != "One" {
		t.Errorf("history, newest first = %+v", list)
	}
	if _, raw := api(h, "GET", "/api/artifacts/"+a.ID+"/versions", "", tok); strings.Contains(raw, "content") {
		t.Errorf("the history list carries content: %s", raw)
	}
}

func TestAStaleUpdateIsRefusedAndNamesTheNewerVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publish(tok, "One", "<p>one")
	api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Other agent","content":"<p>theirs","baseVersion":1}`, tok)

	res, out := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Mine","content":"<p>mine","baseVersion":1}`, tok)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("stale update: %d %s, want 409", res.StatusCode, out)
	}
	body := decode[struct {
		Error          string `json:"error"`
		CurrentVersion int    `json:"currentVersion"`
	}](t, out)
	if body.CurrentVersion != 2 || !strings.Contains(body.Error, "version 2") || !strings.Contains(body.Error, "merge") || !strings.Contains(body.Error, "baseVersion 2") {
		t.Errorf("the refusal does not name the newer version and what to do: %s", out)
	}
	if got := versionsOf(t, h, tok, a.ID); len(got) != 2 || got[0].Title != "Other agent" {
		t.Errorf("a refused update changed the history: %+v", got)
	}
	// After merging, the retry on top of the newer version goes through.
	res, out = api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Merged","content":"<p>theirs and mine","baseVersion":2}`, tok)
	if got := decode[artifactView](t, out); res.StatusCode != http.StatusOK || got.Version != 3 {
		t.Errorf("retry: %d %s", res.StatusCode, out)
	}
}

func TestAnUpdateWithoutABaseVersionIsStillAccepted(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publish(tok, "One", "<p>one")
	for i, want := range []int{2, 3} {
		res, out := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"T","content":"<p>x"}`, tok)
		if got := decode[artifactView](t, out); res.StatusCode != http.StatusOK || got.Version != want {
			t.Errorf("update %d without a base: %d %s", i, res.StatusCode, out)
		}
	}
}

func TestABaseVersionThatNeverExistedIsRefusedAsInvalid(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publish(tok, "One", "<p>one")
	for _, base := range []string{"9", "-1"} {
		res, out := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"T","content":"<p>x","baseVersion":`+base+`}`, tok)
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "version") {
			t.Errorf("baseVersion %s: %d %s", base, res.StatusCode, out)
		}
	}
	if got := versionsOf(t, h, tok, a.ID); len(got) != 1 {
		t.Errorf("a refused update saved a version: %+v", got)
	}
}

func TestRestoreMakesANewVersionAndKeepsTheHistory(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishShared(tok, "One", "<p>one")
	api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Two","content":"<p>two"}`, tok)
	api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Three","content":"<p>three"}`, tok)

	res, out := api(h, "POST", "/api/artifacts/"+a.ID+"/versions/1/restore", "", tok)
	got := decode[artifactView](t, out)
	if res.StatusCode != http.StatusOK || got.Version != 4 || got.Title != "One" || got.Visibility != "organisation" {
		t.Fatalf("restore: %d %s", res.StatusCode, out)
	}
	list := versionsOf(t, h, tok, a.ID)
	if len(list) != 4 || list[0].RestoredFrom != 1 || list[1].Title != "Three" || list[2].Title != "Two" || list[3].Title != "One" {
		t.Errorf("history after restore = %+v", list)
	}
	if _, body := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil); !strings.Contains(body, "<p>one") {
		t.Errorf("the page does not show the restored content: %s", body)
	}
	_, out = api(h, "GET", "/api/artifacts/"+a.ID+"/versions/3", "", tok)
	if v := decode[versionRow](t, out); v.Content != "<p>three" {
		t.Errorf("version 3 after a restore = %s", out)
	}
}

func TestRestoreRefusesWhenTheArtifactMovedOnOrTheVersionIsUnknown(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publish(tok, "One", "<p>one")
	api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Two","content":"<p>two"}`, tok)

	if res, out := api(h, "POST", "/api/artifacts/"+a.ID+"/versions/1/restore", `{"baseVersion":1}`, tok); res.StatusCode != http.StatusConflict {
		t.Errorf("stale restore: %d %s", res.StatusCode, out)
	}
	for _, path := range []string{"/versions/7/restore", "/versions/0/restore", "/versions/x/restore"} {
		if res, out := api(h, "POST", "/api/artifacts/"+a.ID+path, "", tok); res.StatusCode != http.StatusBadRequest {
			t.Errorf("restore %s: %d %s", path, res.StatusCode, out)
		}
	}
	if got := versionsOf(t, h, tok, a.ID); len(got) != 2 {
		t.Errorf("a refused restore saved a version: %+v", got)
	}
}

func TestOnlyTheAuthorAndEditorsReadOrRestoreOlderVersions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.token("alice"), h.token("bob")
	shared := h.publishShared(alice, "Shared", "<p>secret removed later")
	private := h.publish(alice, "Private", "<p>x")
	api(h, "PUT", "/api/artifacts/"+shared.ID, `{"title":"Shared","content":"<p>clean"}`, alice)

	for _, c := range []struct{ method, path, body string }{
		{"GET", "/versions", ""}, {"GET", "/versions/1", ""}, {"POST", "/versions/1/restore", ""},
	} {
		if res, out := api(h, c.method, "/api/artifacts/"+shared.ID+c.path, c.body, bob); res.StatusCode != http.StatusForbidden || strings.Contains(out, "secret") {
			t.Errorf("bob %s %s on a shared artifact: %d %s", c.method, c.path, res.StatusCode, out)
		}
		// An artifact bob may not see answers like one that does not exist.
		hidden, _ := api(h, c.method, "/api/artifacts/"+private.ID+c.path, c.body, bob)
		missing, _ := api(h, c.method, "/api/artifacts/nonexistent"+c.path, c.body, bob)
		if hidden.StatusCode != http.StatusNotFound || missing.StatusCode != http.StatusNotFound {
			t.Errorf("bob %s %s: hidden %d, missing %d, want 404 for both", c.method, c.path, hidden.StatusCode, missing.StatusCode)
		}
	}
	if got := versionsOf(t, h, alice, shared.ID); len(got) != 2 {
		t.Errorf("bob changed the history: %+v", got)
	}
}

func TestDeletingAnArtifactDeletesItsVersions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publish(tok, "One", "<p>one")
	api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Two","content":"<p>two"}`, tok)
	api(h, "DELETE", "/api/artifacts/"+a.ID, "", tok)
	if got, err := h.st.Versions(t.Context(), a.ID); err != nil || len(got) != 0 {
		t.Errorf("versions of a deleted artifact: %+v %v", got, err)
	}
}

func TestEachVersionKeepsTheFormatItWasPublishedWith(t *testing.T) { //nolint:paralleltest // kindtest.Register changes the global kind registry
	h := newHarness(t)
	tok := h.token("alice")
	removeV1 := kindtest.Register(t, "v1")
	_, out := api(h, "POST", "/api/artifacts", `{"title":"T","kind":"versioned","content":"`+kindtest.RejectedInV2+`"}`, tok)
	a := decode[artifactView](t, out)
	removeV1()
	kindtest.Register(t, "v1", "v2")
	api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"T","format":"v2","content":"rewritten"}`, tok)

	list := versionsOf(t, h, tok, a.ID)
	if len(list) != 2 || list[0].Format != "v2" || list[1].Format != "v1" {
		t.Fatalf("formats per version = %+v", list)
	}
	// Restoring the v1 version brings back its content with its own format,
	// even though v2 would refuse that content.
	res, out := api(h, "POST", "/api/artifacts/"+a.ID+"/versions/1/restore", "", tok)
	if got := decode[artifactView](t, out); res.StatusCode != http.StatusOK || got.Format != "v1" || got.Version != 3 {
		t.Fatalf("restore: %d %s", res.StatusCode, out)
	}
	if _, body := h.do(h.browser("alice", "pw-a"), "GET", "/raw/"+a.ID, "", nil); !strings.Contains(body, `data-runtime="v1"`) {
		t.Errorf("the restored artifact is not drawn with its own format: %s", body)
	}
}

func TestAnAgentHitsAConflictReadsTheNewerVersionAndRetriesThroughMCP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	first, second := h.agent(tok), h.agent(tok) // two agents working for the same person

	made := first.ok("publish", map[string]any{"title": "Plan", "content": "<p>a"})
	id := made["id"].(string)
	if made["version"] != float64(1) || !strings.Contains(made["next"].(string), "baseVersion 1") {
		t.Fatalf("publish = %v", made)
	}
	// Both read version 1; the second one saves first.
	if got := first.ok("read", map[string]any{"id": id}); got["version"] != float64(1) {
		t.Fatalf("read = %v", got)
	}
	second.ok("publish", map[string]any{"id": id, "title": "Plan", "content": "<p>a b", "baseVersion": 1})

	refusal := first.refused("publish", map[string]any{"id": id, "title": "Plan", "content": "<p>a c", "baseVersion": 1})
	for _, want := range []string{"nothing was saved", "version 2", "merge", "baseVersion 2"} {
		if !strings.Contains(refusal, want) {
			t.Errorf("conflict message %q does not mention %q", refusal, want)
		}
	}
	// The agent reads what won, merges, and retries on top of it.
	latest := first.ok("read", map[string]any{"id": id})
	if latest["version"] != float64(2) || latest["content"] != "<p>a b" {
		t.Fatalf("read after the conflict = %v", latest)
	}
	done := first.ok("publish", map[string]any{"id": id, "title": "Plan", "content": "<p>a b c", "baseVersion": 2})
	if done["version"] != float64(3) {
		t.Errorf("retry = %v", done)
	}

	versionsAndRestoreThroughMCP(t, first, id)
}

func versionsAndRestoreThroughMCP(t *testing.T, first *agent, id string) {
	t.Helper()
	vs := first.ok("versions", map[string]any{"id": id})
	if vs["latest"] != float64(3) || len(vs["versions"].([]any)) != 3 {
		t.Fatalf("versions = %v", vs)
	}
	old := first.ok("read", map[string]any{"id": id, "version": 1})
	if old["content"] != "<p>a" || old["version"] != float64(1) || old["latestVersion"] != float64(3) {
		t.Errorf("read of version 1 = %v", old)
	}
	if got := first.refused("read", map[string]any{"id": id, "version": 8}); !strings.Contains(got, "no version 8") {
		t.Errorf("read of a missing version: %s", got)
	}
	restored := first.ok("restore", map[string]any{"id": id, "version": 1, "baseVersion": 3})
	if restored["version"] != float64(4) || restored["title"] != "Plan" {
		t.Errorf("restore = %v", restored)
	}
	if got := first.refused("restore", map[string]any{"id": id, "version": 1, "baseVersion": 3}); !strings.Contains(got, "version 4") {
		t.Errorf("a stale restore: %s", got)
	}
	if got := first.ok("read", map[string]any{"id": id}); got["content"] != "<p>a" || got["version"] != float64(4) {
		t.Errorf("read after the restore = %v", got)
	}
}

func TestOnlyAuthorAndEditorAgentsSeeVersionsAndMayRestore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.agent(h.token("alice")), h.agent(h.token("bob"))
	id := alice.ok("publish", map[string]any{"title": "T", "content": "<p>1", "visibility": "organisation"})["id"].(string)
	alice.ok("publish", map[string]any{"id": id, "title": "T", "content": "<p>2", "baseVersion": 1})

	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"versions", map[string]any{"id": id}},
		{"read", map[string]any{"id": id, "version": 1}},
		{"restore", map[string]any{"id": id, "version": 1}},
	} {
		if got := bob.refused(c.tool, c.args); !strings.Contains(got, "only the author") {
			t.Errorf("bob %s: %s", c.tool, got)
		}
	}
	// Reading the newest version needs no more than seeing the artifact.
	if got := bob.ok("read", map[string]any{"id": id, "version": 2}); got["content"] != "<p>2" {
		t.Errorf("bob reads the newest = %v", got)
	}
}

func TestAToolAndItsHTTPCounterpartGiveTheSameVersions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	alice := h.agent(tok)
	id := alice.ok("publish", map[string]any{"title": "T", "content": "<p>1"})["id"].(string)
	alice.ok("publish", map[string]any{"id": id, "title": "T2", "content": "<p>2", "baseVersion": 1})

	_, body := api(h, "GET", "/api/artifacts/"+id+"/versions", "", tok)
	viaTool := alice.ok("versions", map[string]any{"id": id})["versions"]
	if string(mustJSON(t, viaTool)) != string(mustJSON(t, decode[[]any](t, body))) {
		t.Errorf("versions and GET differ:\n%s\n%s", mustJSON(t, viaTool), body)
	}
	res, _ := api(h, "PUT", "/api/artifacts/"+id, `{"title":"x","content":"<p>x","baseVersion":1}`, tok)
	refusal := alice.refused("publish", map[string]any{"id": id, "title": "x", "content": "<p>x", "baseVersion": 1})
	if res.StatusCode != http.StatusConflict || !strings.Contains(refusal, "baseVersion 2") {
		t.Errorf("a stale update: http %d, tool %q", res.StatusCode, refusal)
	}
}

func TestAnOlderVersionIsShownInTheFrameToItsAuthorOnlyInTheSameSandbox(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishShared(tok, "One", "<p>secret removed later")
	api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Two","content":"<p>clean"}`, tok)
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")

	res, body := h.do(alice, "GET", "/raw/"+a.ID+"?version=1", "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "secret removed later") || strings.Contains(body, "clean") {
		t.Fatalf("version 1 for its author: %d %s", res.StatusCode, body)
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") || strings.Contains(csp, "allow-same-origin") || !strings.Contains(body, "pictor") {
		t.Errorf("an older version lost the sandbox or the bridge: %q", csp)
	}
	if res, body := h.do(alice, "GET", "/raw/"+a.ID, "", nil); !strings.Contains(body, "clean") || res.StatusCode != http.StatusOK {
		t.Errorf("without a version the newest is shown: %s", body)
	}
	if res, body := h.do(bob, "GET", "/raw/"+a.ID+"?version=1", "", nil); res.StatusCode != http.StatusForbidden || strings.Contains(body, "secret") {
		t.Errorf("bob reading an old version of a shared artifact: %d %s", res.StatusCode, body)
	}
	for _, v := range []string{"0", "x", "9"} {
		if res, body := h.do(alice, "GET", "/raw/"+a.ID+"?version="+v, "", nil); res.StatusCode != http.StatusBadRequest {
			t.Errorf("version=%s: %d %s", v, res.StatusCode, body)
		}
	}
}

func TestAVersionListNamesTheAuthorsOfEachVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publish(tok, "One", "<p>one")
	_, raw := api(h, "GET", "/api/artifacts/"+a.ID+"/versions", "", tok)
	rows := decode[[]struct {
		Author  string   `json:"author"`
		Authors []string `json:"authors"`
	}](t, raw)
	if len(rows) != 1 || rows[0].Author != "alice" || len(rows[0].Authors) != 1 || rows[0].Authors[0] != "alice" {
		t.Errorf("a published version is by its author alone: %s", raw)
	}
}
