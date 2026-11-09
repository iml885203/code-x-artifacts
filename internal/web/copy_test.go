package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// copyOf copies an artifact over the HTTP API with a token; version 0 is the newest.
func (h *harness) copyOf(tok, id string, version int) (response, string) {
	h.t.Helper()
	body := ""
	if version > 0 {
		body = `{"version":` + string(mustJSON(h.t, version)) + `}`
	}
	return h.do(nil, "POST", "/api/artifacts/"+id+"/copy", body, map[string]string{"Authorization": "Bearer " + tok})
}

// copiedFixture is alice's organisation artifact with a picture, a second
// version that uses it, shared data, a comment and an editor, and bob's copy of it.
type copiedFixture struct {
	h                *harness
	aliceTok, bobTok string
	bob              *http.Client
	src, copy        artifactView
}

func copiedByBob(t *testing.T) copiedFixture {
	t.Helper()
	h := newHarness(t)
	f := copiedFixture{h: h, aliceTok: h.token("alice"), bobTok: h.token("bob"), bob: h.browser("bob", "pw-b")}
	alice := h.browser("alice", "pw-a")
	f.src = h.publishShared(f.aliceTok, "Quarterly plan", "<p>v1</p>")
	h.mustUpload(f.aliceTok, f.src.ID, "chart.png", pngBytes)
	res, out := api(h, "PUT", "/api/artifacts/"+f.src.ID, `{"title":"Quarterly plan","content":"<img src=\"/files/`+f.src.ID+`/chart.png\"><a href=\"https://pictor.test/files/`+f.src.ID+`/chart.png\">x</a>","baseVersion":1}`, f.aliceTok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	if res, out := h.do(alice, "PUT", "/api/artifacts/"+f.src.ID+"/db/votes/alice", `{"choice":"A"}`, bridgeHeaders(h)); res.StatusCode != http.StatusOK {
		t.Fatalf("db write: %d %s", res.StatusCode, out)
	}
	if res, out := h.comment(alice, f.src.ID, "note", ""); res.StatusCode != http.StatusCreated {
		t.Fatalf("comment: %d %s", res.StatusCode, out)
	}
	api(h, "PATCH", "/api/artifacts/"+f.src.ID+"/editors", `{"add":["carol"]}`, f.aliceTok)

	res, out = h.copyOf(f.bobTok, f.src.ID, 0)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("copy: %d %s", res.StatusCode, out)
	}
	f.copy = decode[artifactView](t, out)
	return f
}

func TestACopyIsANewPrivateArtifactOfTheCallerThatSaysWhereItCameFrom(t *testing.T) {
	t.Parallel()
	f := copiedByBob(t)
	c := f.copy
	if c.ID == "" || c.ID == f.src.ID || c.Author != "bob" || c.Visibility != store.VisibilityPrivate || c.Version != 1 || c.Title != "Quarterly plan (Copy)" || c.Kind != f.src.Kind || c.Role != "author" {
		t.Errorf("the copy = %+v", c)
	}
	if len(c.Editors) != 0 || c.OrganisationEdit {
		t.Errorf("the copy took the editors: %v %v", c.Editors, c.OrganisationEdit)
	}
	if c.CopiedFrom == nil || c.CopiedFrom.ID != f.src.ID || c.CopiedFrom.Title != "Quarterly plan" {
		t.Errorf("the copy does not say where it came from: %+v", c.CopiedFrom)
	}
}

func TestACopyHasTheContentAndItsOwnFilesAndTheSourceKeepsItsOwn(t *testing.T) {
	t.Parallel()
	f := copiedByBob(t)
	h, c := f.h, f.copy
	_, page := h.do(f.bob, "GET", "/raw/"+c.ID, "", nil)
	if !strings.Contains(page, "/files/"+c.ID+"/") || strings.Contains(page, "/files/"+f.src.ID+"/") {
		t.Errorf("file URLs were not rewritten to the copy: %s", page)
	}
	files := decode[[]assetView](t, mustGet(h, f.bobTok, "/api/artifacts/"+c.ID+"/assets"))
	if len(files) != 1 || files[0].Name != "chart.png" || !strings.HasSuffix(files[0].URL, "/files/"+c.ID+"/chart.png") {
		t.Errorf("the copy's files = %+v", files)
	}
	if status, body := h.fileStatus(f.bob, "/files/"+c.ID+"/chart.png"); status != http.StatusOK || body != pngBytes {
		t.Errorf("the copied file: %d, %d bytes", status, len(body))
	}
	// Deleting the copy's file does not take the source's with it.
	if res, out := api(h, "DELETE", "/api/artifacts/"+c.ID+"/assets/chart.png", "", f.bobTok); res.StatusCode >= 300 {
		t.Fatalf("delete the copy's file: %d %s", res.StatusCode, out)
	}
	if status, body := h.fileStatus(h.browser("alice", "pw-a"), "/files/"+f.src.ID+"/chart.png"); status != http.StatusOK || body != pngBytes {
		t.Errorf("the source's file after the copy's was deleted: %d, %d bytes", status, len(body))
	}
}

func TestACopyTakesNoSharedDataCommentsOrHistoryAndLeavesTheSourceAlone(t *testing.T) {
	t.Parallel()
	f := copiedByBob(t)
	h, c := f.h, f.copy
	if _, docs := h.do(f.bob, "GET", "/api/artifacts/"+c.ID+"/db/votes", "", bridgeHeaders(h)); strings.TrimSpace(docs) != "[]" {
		t.Errorf("the copy has the source's db rows: %s", docs)
	}
	if got := h.comments(f.bob, c.ID); len(got) != 0 {
		t.Errorf("the copy has comments: %+v", got)
	}
	if got := versionsOf(t, h, f.bobTok, c.ID); len(got) != 1 {
		t.Errorf("the copy has history: %+v", got)
	}
	if got := versionsOf(t, h, f.aliceTok, f.src.ID); len(got) != 2 {
		t.Errorf("the source changed: %+v", got)
	}
	if files := decode[[]assetView](t, mustGet(h, f.aliceTok, "/api/artifacts/"+f.src.ID+"/assets")); len(files) != 1 {
		t.Errorf("the source lost its files: %+v", files)
	}
}

func mustGet(h *harness, tok, path string) string {
	h.t.Helper()
	res, out := api(h, "GET", path, "", tok)
	if res.StatusCode != http.StatusOK {
		h.t.Fatalf("GET %s: %d %s", path, res.StatusCode, out)
	}
	return out
}

func TestWhoCannotSeeAnArtifactCannotCopyIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	private := h.publish(aliceTok, "Private", "<p>secret</p>")
	res, out := h.copyOf(bobTok, private.ID, 0)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("copying what you cannot see: %d %s, want 404", res.StatusCode, out)
	}
	_, missing := h.copyOf(bobTok, "nosuchid", 0)
	if out != missing {
		t.Errorf("a refusal tells a hidden artifact from a missing one: %q vs %q", out, missing)
	}
	if res, _ := h.do(nil, "POST", "/api/artifacts/"+private.ID+"/copy", "", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("an anonymous copy: %d, want 401", res.StatusCode)
	}
	if list := decode[[]artifactView](t, mustGet(h, bobTok, "/api/artifacts")); len(list) != 0 {
		t.Errorf("a refused copy left an artifact behind: %+v", list)
	}
}

func TestTheSourceLinkOfACopyGoesWhenTheSourceIsNoLongerVisibleToTheViewer(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	src := h.publishShared(aliceTok, "Plan", "<p>x</p>")
	_, out := h.copyOf(bobTok, src.ID, 0)
	c := decode[artifactView](t, out)

	if got := decode[artifactView](t, mustGet(h, bobTok, "/api/artifacts/"+c.ID)); got.CopiedFrom == nil || got.CopiedFrom.ID != src.ID {
		t.Fatalf("while the source is visible: %+v", got.CopiedFrom)
	}
	// Alice withdraws the source. Bob's copy still says where it came from, without the link.
	api(h, "PATCH", "/api/artifacts/"+src.ID, `{"visibility":"private"}`, aliceTok)
	got := decode[artifactView](t, mustGet(h, bobTok, "/api/artifacts/"+c.ID))
	if got.CopiedFrom == nil || got.CopiedFrom.ID != "" || got.CopiedFrom.Title != "Plan" {
		t.Errorf("after the source went private: %+v", got.CopiedFrom)
	}
	// Deleted sources too.
	if res, out := api(h, "DELETE", "/api/artifacts/"+src.ID, "", aliceTok); res.StatusCode >= 300 {
		t.Fatalf("delete: %d %s", res.StatusCode, out)
	}
	got = decode[artifactView](t, mustGet(h, bobTok, "/api/artifacts/"+c.ID))
	if got.CopiedFrom == nil || got.CopiedFrom.ID != "" || got.CopiedFrom.Title != "Plan" {
		t.Errorf("after the source was deleted: %+v", got.CopiedFrom)
	}
	// An artifact that is not a copy says nothing.
	if plain := h.publish(bobTok, "Plain", "<p>"); plain.CopiedFrom != nil {
		t.Errorf("a new artifact claims a source: %+v", plain.CopiedFrom)
	}
}

func TestACopyOfAnOlderVersionIsThatVersionAndOnlyEditorsMayAskForOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	src := h.publishShared(aliceTok, "Plan", "<p>first</p>")
	api(h, "PUT", "/api/artifacts/"+src.ID, `{"title":"Plan 2","content":"<p>second</p>","baseVersion":1}`, aliceTok)

	res, out := h.copyOf(aliceTok, src.ID, 1)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("copy of version 1: %d %s", res.StatusCode, out)
	}
	c := decode[artifactView](t, out)
	_, page := h.do(h.browser("alice", "pw-a"), "GET", "/raw/"+c.ID, "", nil)
	if c.Title != "Plan (Copy)" || !strings.Contains(page, "first") || strings.Contains(page, "second") {
		t.Errorf("copy of version 1: title %q, page %q", c.Title, page)
	}
	// A reader cannot read history, so cannot copy from it; the newest is still theirs to copy.
	if res, out := h.copyOf(bobTok, src.ID, 1); res.StatusCode != http.StatusForbidden {
		t.Errorf("a reader copying version 1: %d %s, want 403", res.StatusCode, out)
	}
	if res, out := h.copyOf(aliceTok, src.ID, 9); res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "no version 9") {
		t.Errorf("copying a version that does not exist: %d %s", res.StatusCode, out)
	}
}

func TestACopyKeepsTheKindAndFormatOfTheSource(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	doc := h.publishDoc(tok, "# Notes\n\nwords\n")
	res, out := h.copyOf(tok, doc.ID, 0)
	c := decode[artifactView](t, out)
	if res.StatusCode != http.StatusCreated || c.Kind != "doc" || c.Format != doc.Format {
		t.Errorf("copy of a doc: %d %+v", res.StatusCode, c)
	}
}

func TestAnAgentCopiesAnArtifactAndIsToldWhatItGot(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.agent(h.token("alice"))
	bob := h.agent(h.token("bob"))
	src := alice.ok("publish", map[string]any{"title": "Plan", "content": page, "visibility": "organisation"})

	alice.ok("publish", map[string]any{"id": src["id"], "title": "Plan", "content": page + "<!-- 2 -->", "baseVersion": 1})
	got := bob.ok("copy", map[string]any{"id": src["id"]})
	if got["id"] == src["id"] || got["author"] != "bob" || got["visibility"] != "private" || got["title"] != "Plan (Copy)" || got["role"] != "author" {
		t.Errorf("copy = %v", got)
	}
	from, _ := got["copiedFrom"].(map[string]any)
	if from["id"] != src["id"] || !strings.Contains(got["url"].(string), got["id"].(string)) {
		t.Errorf("copy = %v", got)
	}
	if next, _ := got["next"].(string); !strings.Contains(next, got["url"].(string)) {
		t.Errorf("the result does not say what to do next: %v", got["next"])
	}
	if refusal := bob.refused("copy", map[string]any{"id": src["id"], "version": 1}); !strings.Contains(refusal, "author") {
		t.Errorf("a reader asking for a version: %s", refusal)
	}
	carol := h.agent(h.token("carol"))
	priv := alice.ok("publish", map[string]any{"title": "Mine", "content": page})
	if refusal := carol.refused("copy", map[string]any{"id": priv["id"]}); !strings.Contains(refusal, "no artifact with that id is visible") {
		t.Errorf("copying what is hidden: %s", refusal)
	}
}

func TestACopyCarriesTheFilesAndRewritesAssetURLsInThemAndInThePage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	src := h.publishWith(aliceTok, multiPage, map[string]string{"app.js": appJS, "data.json": "[1]"}, store.VisibilityOrganisation)
	h.mustUpload(aliceTok, src.ID, "chart.png", pngBytes)
	assetURL := "/files/" + src.ID + "/chart.png"
	page := multiPage + `<img src="` + assetURL + `">`
	if res, out := h.update(aliceTok, src.ID, 0, page, map[string]string{"app.js": `const u = "` + assetURL + `";`, "site.css": `a{background:url(` + assetURL + `)}`, "data.json": `"` + assetURL + `"`}); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}

	res, out := h.copyOf(bobTok, src.ID, 0)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("copy: %d %s", res.StatusCode, out)
	}
	c := decode[artifactView](t, out)
	if len(c.Files) != 3 {
		t.Fatalf("the copy has %d files, want 3", len(c.Files))
	}
	checkCopiedFiles(t, h, bobTok, c, src.ID, assetURL)

	// A copy of an older version has that version's files, for those who may read history.
	res, out = h.copyOf(aliceTok, src.ID, 1)
	if old := decode[artifactView](t, out); res.StatusCode != http.StatusCreated || len(old.Files) != 2 {
		t.Fatalf("copy of version 1: %d %s", res.StatusCode, out)
	} else if _, text := h.fileText(aliceTok, old.ID, "app.js", 0); text != appJS {
		t.Errorf("the copy of version 1 has app.js %q", text)
	}
	// Deleting the source cannot reach the copy's files: they are its own.
	if res, _ := api(h, "DELETE", "/api/artifacts/"+src.ID, "", aliceTok); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete source: %d", res.StatusCode)
	}
	if _, text := h.fileText(bobTok, c.ID, "app.js", 0); !strings.Contains(text, "/files/"+c.ID+"/chart.png") {
		t.Errorf("the copy's app.js after the source was deleted: %q", text)
	}
}

// checkCopiedFiles checks what a copy of the multi-file page holds: asset URLs
// rewritten where a file can hold one, data left alone, and a manifest that
// describes the bytes the copy really has.
func checkCopiedFiles(t *testing.T, h *harness, tok string, c artifactView, sourceID, assetURL string) {
	t.Helper()
	own := "/files/" + c.ID + "/chart.png"
	for path, rewritten := range map[string]bool{"app.js": true, "site.css": true, "data.json": false} {
		_, text := h.fileText(tok, c.ID, path, 0)
		if rewritten && (!strings.Contains(text, own) || strings.Contains(text, sourceID)) {
			t.Errorf("%s in the copy still points at the source: %q", path, text)
		}
		if !rewritten && !strings.Contains(text, assetURL) {
			t.Errorf("%s is data and must be copied as it is: %q", path, text)
		}
	}
	for _, f := range c.Files {
		_, text := h.fileText(tok, c.ID, f.Path, 0)
		if f.SHA256 != sha(text) || f.Size != int64(len(text)) {
			t.Errorf("%s: manifest says %s/%d, the bytes are %s/%d", f.Path, f.SHA256, f.Size, sha(text), len(text))
		}
	}
}
