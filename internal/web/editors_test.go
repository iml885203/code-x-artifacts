package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func patchEditors(h *harness, tok, id, body string) (response, string) {
	return api(h, "PATCH", "/api/artifacts/"+id+"/editors", body, tok)
}

// namedEditor is alice's private artifact with bob named as its editor.
func namedEditor(t *testing.T) (h *harness, alice, bob string, a artifactView) {
	t.Helper()
	h = newHarness(t)
	alice, bob = h.token("alice"), h.token("bob")
	a = h.publish(alice, "Shared notes", "<p>v1")
	if res, out := patchEditors(h, alice, a.ID, `{"add":["bob"]}`); res.StatusCode != http.StatusOK {
		t.Fatalf("name an editor: %d %s", res.StatusCode, out)
	}
	return h, alice, bob, a
}

// editorManagesFilesAndTheDB is the files and shared-data part of what an editor may do.
func editorManagesFilesAndTheDB(t *testing.T, h *harness, bob, id string) {
	t.Helper()
	if up := h.mustUpload(bob, id, "pic.png", pngBytes); up.Name != "pic.png" {
		t.Errorf("editor upload: %+v", up)
	}
	if res, out := api(h, "DELETE", "/api/artifacts/"+id+"/assets/pic.png", "", bob); res.StatusCode != http.StatusNoContent {
		t.Errorf("editor deletes a file: %d %s", res.StatusCode, out)
	}
	if res, _ := api(h, "PUT", "/api/artifacts/"+id+"/db/c/d", `{"n":1}`, bob); res.StatusCode != http.StatusOK {
		t.Errorf("editor writes the db: %d", res.StatusCode)
	}
}

func TestNamingAnEditorLetsThemOpenAPrivateArtifactWithoutChangingTheAudience(t *testing.T) {
	t.Parallel()
	h, _, bob, a := namedEditor(t)
	res, out := api(h, "GET", "/api/artifacts/"+a.ID, "", bob)
	got := decode[artifactView](t, out)
	if res.StatusCode != http.StatusOK || got.Role != "editor" || !got.CanEdit || got.CanChange || len(got.Editors) != 1 || got.Editors[0] != "bob" {
		t.Fatalf("what the editor sees: %d %s", res.StatusCode, out)
	}
	if got.Visibility != store.VisibilityPrivate {
		t.Errorf("naming an editor changed the audience to %q", got.Visibility)
	}
}

func TestAnEditorPublishesAndRestoresAndTheVersionsSayWhoSavedThem(t *testing.T) {
	t.Parallel()
	h, alice, bob, a := namedEditor(t)
	res, out := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"By Bob","content":"<p>v2","baseVersion":1}`, bob)
	if v := decode[artifactView](t, out); res.StatusCode != http.StatusOK || v.Version != 2 {
		t.Fatalf("editor publish: %d %s", res.StatusCode, out)
	}
	if list := versionsOf(t, h, bob, a.ID); len(list) != 2 {
		t.Errorf("an editor reads the history: %+v", list)
	}
	res, out = api(h, "POST", "/api/artifacts/"+a.ID+"/versions/1/restore", `{"baseVersion":2}`, bob)
	if v := decode[artifactView](t, out); res.StatusCode != http.StatusOK || v.Version != 3 {
		t.Fatalf("editor restore: %d %s", res.StatusCode, out)
	}
	if list := versionsOf(t, h, alice, a.ID); list[0].Version != 3 || list[1].Title != "By Bob" {
		t.Errorf("versions record who saved them: %+v", list)
	}
	if v, _ := h.st.Version(t.Context(), a.ID, 2); v.Author != "bob" {
		t.Errorf("version 2 was saved by %q", v.Author)
	}
	editorManagesFilesAndTheDB(t, h, bob, a.ID)
}

func TestAnEditorCannotChangeTheAudienceTheEditorsOrDelete(t *testing.T) {
	t.Parallel()
	h, _, bob, a := namedEditor(t)
	for name, c := range map[string]struct{ method, path, body string }{
		"change the audience":      {"PATCH", "/api/artifacts/" + a.ID, `{"visibility":"organisation"}`},
		"change it while updating": {"PUT", "/api/artifacts/" + a.ID, `{"title":"x","content":"<p>x","visibility":"organisation"}`},
		"change the editors":       {"PATCH", "/api/artifacts/" + a.ID + "/editors", `{"add":["carol"]}`},
		"delete":                   {"DELETE", "/api/artifacts/" + a.ID, ""},
	} {
		if res, out := api(h, c.method, c.path, c.body, bob); res.StatusCode != http.StatusForbidden {
			t.Errorf("editor tries to %s: %d %s, want 403", name, res.StatusCode, out)
		}
	}
	if got, _ := h.st.Artifact(t.Context(), a.ID); got.Visibility != store.VisibilityPrivate || len(got.Editors) != 1 {
		t.Errorf("a refused request changed the artifact: %+v", got)
	}
	// Repeating the audience it already has is not a change.
	if res, out := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"x","content":"<p>x","visibility":"private"}`, bob); res.StatusCode != http.StatusOK {
		t.Errorf("editor republishing with the current audience: %d %s", res.StatusCode, out)
	}
}

func TestRemovingAnEditorOrSwitchingOrganisationEditingOffTakesEffectOnTheNextRequest(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob, carol := h.token("alice"), h.token("bob"), h.token("carol")
	a := h.publishShared(alice, "Notes", "<p>v1")
	edit := func(tok string) int {
		res, _ := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"T","content":"<p>x"}`, tok)
		return res.StatusCode
	}

	if got := edit(bob); got != http.StatusForbidden {
		t.Fatalf("a reader edits: %d", got)
	}
	patchEditors(h, alice, a.ID, `{"add":["RAVI\\Bob"]}`)
	if got := edit(bob); got != http.StatusOK {
		t.Errorf("a named editor (typed with the domain) edits: %d", got)
	}
	patchEditors(h, alice, a.ID, `{"remove":["bob"]}`)
	if got := edit(bob); got != http.StatusForbidden {
		t.Errorf("a removed editor edits: %d", got)
	}

	if res, out := patchEditors(h, alice, a.ID, `{"organisationEdit":true}`); res.StatusCode != http.StatusOK {
		t.Fatalf("switch on: %d %s", res.StatusCode, out)
	}
	if edit(bob) != http.StatusOK || edit(carol) != http.StatusOK {
		t.Error("with organisation editing on, everyone signed in edits")
	}
	patchEditors(h, alice, a.ID, `{"organisationEdit":false}`)
	if got := edit(carol); got != http.StatusForbidden {
		t.Errorf("after switching it off: %d", got)
	}

	// Organisation editing follows the audience: it cannot be on for a private
	// artifact, and making the artifact private switches it off for good.
	patchEditors(h, alice, a.ID, `{"organisationEdit":true}`)
	api(h, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"private"}`, alice)
	api(h, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"organisation"}`, alice)
	if got := edit(carol); got != http.StatusForbidden {
		t.Errorf("organisation editing came back with the audience: %d", got)
	}
	api(h, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"private"}`, alice)
	if res, out := patchEditors(h, alice, a.ID, `{"organisationEdit":true}`); res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "organisation") {
		t.Errorf("organisation editing on a private artifact: %d %s", res.StatusCode, out)
	}
}

func TestAnEditorsPublishWithAStaleBaseVersionIsRefusedLikeTheAuthorsAndCarriesTheNewestVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.token("alice"), h.token("bob")
	a := h.publishShared(alice, "One", "<p>one")
	patchEditors(h, alice, a.ID, `{"add":["bob"]}`)
	api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Alice's","content":"<p>alice","baseVersion":1}`, alice)

	res, out := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Bob's","content":"<p>bob","baseVersion":1}`, bob)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("stale editor publish: %d %s, want 409", res.StatusCode, out)
	}
	body := decode[struct {
		Error          string `json:"error"`
		CurrentVersion int    `json:"currentVersion"`
		Current        struct {
			Version int    `json:"version"`
			Title   string `json:"title"`
			Content string `json:"content"`
		} `json:"current"`
	}](t, out)
	if body.CurrentVersion != 2 || body.Current.Version != 2 || body.Current.Title != "Alice's" || body.Current.Content != "<p>alice" {
		t.Errorf("the HTTP refusal does not carry the newest version in full: %s", out)
	}
	if got := versionsOf(t, h, alice, a.ID); len(got) != 2 {
		t.Errorf("a refused editor publish saved something: %+v", got)
	}

	// An agent gets the same refusal, with the call that fetches the newest version.
	refusal := h.agent(bob).refused("publish", map[string]any{"id": a.ID, "title": "Bob's", "content": "<p>bob", "baseVersion": 1})
	if !strings.Contains(refusal, "version 2") || !strings.Contains(refusal, "Call read with") || !strings.Contains(refusal, a.ID) {
		t.Errorf("the MCP refusal does not say how to get the newest version: %s", refusal)
	}
	read := h.agent(bob).ok("read", map[string]any{"id": a.ID})
	if read["content"] != "<p>alice" || read["version"] != float64(2) || read["role"] != "editor" {
		t.Errorf("read as an editor: %v", read)
	}
	// Merged on top of the newest, the retry goes through.
	if res, out := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Merged","content":"<p>both","baseVersion":2}`, bob); res.StatusCode != http.StatusOK {
		t.Errorf("retry: %d %s", res.StatusCode, out)
	}
}

func TestSharingOverMCPShowsOverHTTPAndAReaderIsRefusedTheSameOnBoth(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	alice, bob := h.agent(aliceTok), h.agent(bobTok)
	a := h.publishShared(aliceTok, "Notes", "<p>v1")

	// A reader is refused the same way on both.
	httpRes, _ := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"x","content":"<p>x"}`, bobTok)
	toolRefusal := bob.refused("publish", map[string]any{"id": a.ID, "title": "x", "content": "<p>x"})
	if httpRes.StatusCode != http.StatusForbidden || !strings.Contains(toolRefusal, "made an editor") {
		t.Errorf("reader publish: http %d, tool %q", httpRes.StatusCode, toolRefusal)
	}
	if r := bob.refused("share", map[string]any{"id": a.ID, "add": []string{"bob"}}); !strings.Contains(r, "only the author") {
		t.Errorf("a reader shares: %s", r)
	}

	// The author names bob over MCP; HTTP sees it.
	shared := alice.ok("share", map[string]any{"id": a.ID, "add": []string{"bob", "Carol"}})
	if eds, _ := shared["editors"].([]any); len(eds) != 2 || shared["role"] != "author" {
		t.Fatalf("share = %v", shared)
	}
	_, body := api(h, "GET", "/api/artifacts/"+a.ID, "", bobTok)
	if got := decode[artifactView](t, body); got.Role != "editor" || len(got.Editors) != 2 {
		t.Errorf("HTTP after share: %s", body)
	}
}

func TestAnEditorWorksThroughMCPAndIsRefusedTheAuthorsThingsTheSameWayAsOverHTTP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	alice, bob := h.agent(aliceTok), h.agent(bobTok)
	a := h.publishShared(aliceTok, "Notes", "<p>v1")
	alice.ok("share", map[string]any{"id": a.ID, "add": []string{"bob"}})
	if out := bob.ok("publish", map[string]any{"id": a.ID, "title": "By Bob", "content": "<p>v2", "baseVersion": 1}); out["version"] != float64(2) {
		t.Errorf("editor publish = %v", out)
	}
	if out := bob.ok("restore", map[string]any{"id": a.ID, "version": 1, "baseVersion": 2}); out["version"] != float64(3) {
		t.Errorf("editor restore = %v", out)
	}
	if out := bob.ok("versions", map[string]any{"id": a.ID}); out["latest"] != float64(3) {
		t.Errorf("editor versions = %v", out)
	}
	for tool, args := range map[string]map[string]any{
		"set_visibility": {"id": a.ID, "visibility": "private"},
		"share":          {"id": a.ID, "remove": []string{"carol"}},
		"delete":         {"id": a.ID},
	} {
		if r := bob.refused(tool, args); !strings.Contains(r, "only the author") {
			t.Errorf("editor calls %s: %s", tool, r)
		}
	}
	if res, _ := api(h, "DELETE", "/api/artifacts/"+a.ID, "", bobTok); res.StatusCode != http.StatusForbidden {
		t.Errorf("editor DELETE over HTTP: %d", res.StatusCode)
	}
}

func TestTheRefusalsOfShareReadTheSameOnHTTPAndMCP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok := h.token("alice")
	alice := h.agent(aliceTok)
	a := h.publishShared(aliceTok, "Notes", "<p>v1")
	httpRes, out := patchEditors(h, aliceTok, a.ID, `{"add":["alice"]}`)
	r := alice.refused("share", map[string]any{"id": a.ID, "add": []string{"alice"}})
	if httpRes.StatusCode != http.StatusBadRequest || !strings.Contains(out, "is the author") || !strings.Contains(r, "is the author") {
		t.Errorf("naming the author: http %d %s, tool %s", httpRes.StatusCode, out, r)
	}
	if r := alice.refused("share", map[string]any{"id": a.ID}); !strings.Contains(r, "nothing to change") {
		t.Errorf("empty share: %s", r)
	}
	out2 := alice.ok("share", map[string]any{"id": a.ID, "organisation_edit": true})
	if out2["organisationEdit"] != true {
		t.Errorf("organisation_edit = %v", out2)
	}
	if listed := h.agent(h.token("dave")).ok("list", nil)["artifacts"].([]any); listed[0].(map[string]any)["role"] != "editor" {
		t.Errorf("list does not report the caller's role: %v", listed[0])
	}
}
