package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// startNew makes a blank artifact of the kind as the caller.
func (h *harness) startNew(tok, kind, title string) (response, string) {
	h.t.Helper()
	return api(h, "POST", "/api/new", `{"kind":"`+kind+`","title":"`+title+`"}`, tok)
}

func TestANewDocIsPrivateToItsAuthorAndStaysAfterAReload(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.token("alice"), h.token("bob")
	res, out := h.startNew(alice, "doc", "Untitled document")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("new doc: %d %s", res.StatusCode, out)
	}
	a := decode[artifactView](t, out)
	if a.Kind != "doc" || a.Format != "2" || a.Visibility != store.VisibilityPrivate || a.Author != "alice" || a.Version != 1 || a.Title != "Untitled document" || a.Role != "author" {
		t.Errorf("the new doc = %+v", a)
	}
	// Reload: it is still there, for alice.
	if res, out := api(h, "GET", "/api/artifacts/"+a.ID, "", alice); res.StatusCode != http.StatusOK || decode[artifactView](t, out).Title != "Untitled document" {
		t.Errorf("alice reads it again: %d %s", res.StatusCode, out)
	}
	// Nobody else can open it, and it does not say it is there.
	for _, path := range []string{"/api/artifacts/" + a.ID, "/api/artifacts/" + a.ID + "/blocks"} {
		if res, _ := api(h, "GET", path, "", bob); res.StatusCode != http.StatusNotFound {
			t.Errorf("bob reads %s: %d, want 404", path, res.StatusCode)
		}
	}
	if _, list := api(h, "GET", "/api/artifacts", "", bob); strings.Contains(list, a.ID) {
		t.Errorf("bob's list shows alice's new doc: %s", list)
	}
}

func TestANewDocIsOneTabWithOneEmptyHeadingOneWithAnId(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.token("alice")
	_, out := h.startNew(alice, "doc", "Untitled document")
	tabs := h.tabsOf(alice, decode[artifactView](t, out).ID)
	if len(tabs) != 1 {
		t.Fatalf("tabs = %v", tabs)
	}
	blocks := tabs[0]["blocks"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("blocks = %v", blocks)
	}
	b := blocks[0].(map[string]any)
	if b["type"] != "heading" || b["level"] != float64(1) || b["id"] == "" || b["content"] != nil {
		t.Errorf("a new doc starts with an empty H1 with an id, got %v", b)
	}
}

func TestANewDesignStartsFromOnePageWithOneFrameAndTakesAnyTitle(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.token("alice"), h.token("bob")
	res, out := h.startNew(alice, "design", "Landing")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("new design: %d %s", res.StatusCode, out)
	}
	a := decode[artifactView](t, out)
	if a.Kind != "design" || a.Visibility != store.VisibilityPrivate || a.Title != "Landing" || a.Version != 1 {
		t.Errorf("the new design = %+v", a)
	}
	_, body := api(h, "GET", "/api/artifacts/"+a.ID+"/design", "", alice)
	design := decode[struct {
		Manifest struct {
			Pages []struct {
				Rows [][]struct {
					File string `json:"file"`
					W, H int
				} `json:"rows"`
			} `json:"pages"`
		} `json:"manifest"`
	}](t, body)
	if len(design.Manifest.Pages) != 1 || len(design.Manifest.Pages[0].Rows) != 1 || len(design.Manifest.Pages[0].Rows[0]) != 1 {
		t.Fatalf("a new design has one page with one frame: %s", body)
	}
	if f := design.Manifest.Pages[0].Rows[0][0]; f.W < 1 || f.H < 1 || !strings.HasPrefix(f.File, "frames/") {
		t.Errorf("the frame = %+v", f)
	}
	if res, body := api(h, "GET", "/api/artifacts/"+a.ID+"/files/frames/frame-1.html", "", alice); res.StatusCode != http.StatusOK || !strings.Contains(body, "<body") {
		t.Errorf("the frame's file: %d %s", res.StatusCode, body)
	}
	if res, _ := api(h, "GET", "/api/artifacts/"+a.ID, "", bob); res.StatusCode != http.StatusNotFound {
		t.Errorf("bob opens alice's new design: %d", res.StatusCode)
	}
}

func TestANewBoardIsAnEmptyBoardPrivateToItsAuthorWithADefaultTitle(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.token("alice"), h.token("bob")
	res, out := h.startNew(alice, "board", "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("new board: %d %s", res.StatusCode, out)
	}
	a := decode[artifactView](t, out)
	if a.Kind != "board" || a.Visibility != store.VisibilityPrivate || a.Title != "Untitled board" || a.Version != 1 {
		t.Errorf("the new board = %+v", a)
	}
	if read := h.agent(alice).ok("read", map[string]any{"id": a.ID}); read["content"] != `{"version":1,"elements":[]}` {
		t.Errorf("a new board holds %v", read["content"])
	}
	if res, _ := api(h, "GET", "/api/artifacts/"+a.ID, "", bob); res.StatusCode != http.StatusNotFound {
		t.Errorf("bob opens alice's new board: %d", res.StatusCode)
	}
}

func TestStartingSomethingNewNeedsASignedInPersonAKindThatCanBeBlankAndHasADefaultTitle(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.token("alice")
	if res, _ := h.do(nil, "POST", "/api/new", `{"kind":"doc"}`, nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("without signing in: %d, want 401", res.StatusCode)
	}
	for _, body := range []string{`{"kind":"html"}`, `{"kind":"slides"}`, `{}`, `nope`} {
		if res, out := api(h, "POST", "/api/new", body, alice); res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "doc") || !strings.Contains(out, "design") || !strings.Contains(out, "board") {
			t.Errorf("%s: %d %s, want a 400 that names doc, design and board", body, res.StatusCode, out)
		}
	}
	res, out := api(h, "POST", "/api/new", `{"kind":"doc"}`, alice)
	if got := decode[artifactView](t, out); res.StatusCode != http.StatusCreated || got.Title != "Untitled document" {
		t.Errorf("a doc without a title: %d %s", res.StatusCode, out)
	}
	res, out = api(h, "POST", "/api/new", `{"kind":"design","title":"  "}`, alice)
	if got := decode[artifactView](t, out); res.StatusCode != http.StatusCreated || got.Title != "Untitled design" {
		t.Errorf("a design with a blank title: %d %s", res.StatusCode, out)
	}
}

// sharedDoc is alice's new doc, open to the organisation, with carol its editor.
func sharedDoc(t *testing.T) (h *harness, alice, carol, bob string, id string) {
	t.Helper()
	h = newHarness(t)
	alice, carol, bob = h.token("alice"), h.token("carol"), h.token("bob")
	_, out := h.startNew(alice, "doc", "Notes")
	id = decode[artifactView](t, out).ID
	if res, out := api(h, "PATCH", "/api/artifacts/"+id, `{"visibility":"organisation"}`, alice); res.StatusCode != http.StatusOK {
		t.Fatalf("share: %d %s", res.StatusCode, out)
	}
	if res, out := patchEditors(h, alice, id, `{"add":["carol"]}`); res.StatusCode != http.StatusOK {
		t.Fatalf("editor: %d %s", res.StatusCode, out)
	}
	return h, alice, carol, bob, id
}

func TestTheAuthorAndAnEditorRenameAndTheContentStaysAsItWas(t *testing.T) {
	t.Parallel()
	h, alice, carol, bob, id := sharedDoc(t)
	_, before := api(h, "GET", "/api/artifacts/"+id+"/blocks", "", alice)

	res, out := api(h, "PUT", "/api/artifacts/"+id+"/title", `{"title":"  Plan for Q4 "}`, carol)
	if got := decode[artifactView](t, out); res.StatusCode != http.StatusOK || got.Title != "Plan for Q4" || got.Version != 2 {
		t.Fatalf("an editor renames: %d %s", res.StatusCode, out)
	}
	res, out = api(h, "PUT", "/api/artifacts/"+id+"/title", `{"title":"Final"}`, alice)
	if got := decode[artifactView](t, out); res.StatusCode != http.StatusOK || got.Title != "Final" || got.Version != 3 {
		t.Fatalf("the author renames: %d %s", res.StatusCode, out)
	}
	_, got := api(h, "GET", "/api/artifacts/"+id, "", bob)
	if a := decode[artifactView](t, got); a.Title != "Final" || a.Version != 3 {
		t.Errorf("what a reader sees: %+v", a)
	}
	// The H1 and every block are exactly what they were; the new versions are the same content with the new title.
	_, after := api(h, "GET", "/api/artifacts/"+id+"/blocks", "", alice)
	if strip(before) != strip(after) {
		t.Errorf("a rename changed the content:\n%s\n%s", before, after)
	}
	rows := versionsOf(t, h, alice, id)
	titles := map[string]bool{}
	for _, r := range rows {
		titles[r.Title] = true
	}
	if len(rows) != 3 || !titles["Notes"] || !titles["Plan for Q4"] || !titles["Final"] {
		t.Errorf("versions keep every title: %+v", rows)
	}
}

func TestAReaderCannotRenameAnInvisibleArtifactIsNotFoundAndNobodyRenamesWithoutSigningIn(t *testing.T) {
	t.Parallel()
	h, alice, _, bob, id := sharedDoc(t)
	if res, out := api(h, "PUT", "/api/artifacts/"+id+"/title", `{"title":"Mine now"}`, bob); res.StatusCode != http.StatusForbidden {
		t.Errorf("a reader renames: %d %s, want 403", res.StatusCode, out)
	}
	_, hidden := h.startNew(alice, "doc", "Private")
	hiddenID := decode[artifactView](t, hidden).ID
	if res, out := api(h, "PUT", "/api/artifacts/"+hiddenID+"/title", `{"title":"x"}`, bob); res.StatusCode != http.StatusNotFound {
		t.Errorf("an invisible artifact: %d %s, want 404", res.StatusCode, out)
	}
	if res, out := h.do(nil, "PUT", "/api/artifacts/"+id+"/title", `{"title":"x"}`, nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("not signed in: %d %s, want 401", res.StatusCode, out)
	}
	if _, out := api(h, "GET", "/api/artifacts/"+id, "", alice); decode[artifactView](t, out).Title != "Notes" || decode[artifactView](t, out).Version != 1 {
		t.Errorf("a refused rename changed the artifact: %s", out)
	}
}

// strip removes the version number from a blocks read, which is the one thing a rename moves.
func strip(blocks string) string {
	i := strings.Index(blocks, `"version":`)
	j := strings.Index(blocks[i:], ",")
	return blocks[:i] + blocks[i+j:]
}

func TestARenameNeedsATitleAndIsNotAVersionWhenTheTitleIsTheSame(t *testing.T) {
	t.Parallel()
	h, alice, _, _, id := sharedDoc(t)
	for _, body := range []string{`{"title":""}`, `{"title":"   "}`, `{"title":"a\nb"}`, `{}`, `nope`, `{"title":"` + strings.Repeat("x", 300) + `"}`} {
		if res, out := api(h, "PUT", "/api/artifacts/"+id+"/title", body, alice); res.StatusCode != http.StatusBadRequest {
			t.Errorf("%.30s: %d %s, want 400", body, res.StatusCode, out)
		}
	}
	res, out := api(h, "PUT", "/api/artifacts/"+id+"/title", `{"title":"Notes"}`, alice)
	if got := decode[artifactView](t, out); res.StatusCode != http.StatusOK || got.Version != 1 {
		t.Errorf("the same title: %d %s, want 200 and no new version", res.StatusCode, out)
	}
}

func TestADesignRenamesWithItsFramesAndAnAgentRenamesWithPublish(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.token("alice")
	_, out := h.startNew(alice, "design", "Landing")
	id := decode[artifactView](t, out).ID
	if res, out := api(h, "PUT", "/api/artifacts/"+id+"/title", `{"title":"Landing v2"}`, alice); res.StatusCode != http.StatusOK {
		t.Fatalf("rename a design: %d %s", res.StatusCode, out)
	}
	if res, body := api(h, "GET", "/api/artifacts/"+id+"/files/frames/frame-1.html", "", alice); res.StatusCode != http.StatusOK || !strings.Contains(body, "<body") {
		t.Errorf("the renamed design lost its frame: %d %s", res.StatusCode, body)
	}

	// An agent renames a doc with publish: the same content with a new title.
	agent := h.agent(alice)
	docID := h.editableDoc(alice)
	read := agent.ok("read", map[string]any{"id": docID})
	agent.ok("publish", map[string]any{"id": docID, "title": "Renamed by an agent", "content": read["content"], "baseVersion": read["version"]})
	if got := agent.ok("read", map[string]any{"id": docID}); got["title"] != "Renamed by an agent" {
		t.Errorf("after the agent's publish: %v", got["title"])
	}
}

func TestACopyOfADocKeepsTheBlockIdsAndTheFormatAndHasNoCommentsAndSaysWhereItCameFrom(t *testing.T) {
	t.Parallel()
	h, alice, _, bob, id := sharedDoc(t)
	aliceBrowser := h.browser("alice", "pw-a")
	if res, out := h.comment(aliceBrowser, id, "a note", ""); res.StatusCode != http.StatusCreated {
		t.Fatalf("comment: %d %s", res.StatusCode, out)
	}
	res, out := h.copyOf(bob, id, 0)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("copy: %d %s", res.StatusCode, out)
	}
	c := decode[artifactView](t, out)
	if c.Kind != "doc" || c.Format != "2" || c.Visibility != store.VisibilityPrivate || c.Author != "bob" || c.CopiedFrom == nil || c.CopiedFrom.ID != id {
		t.Errorf("the copy = %+v", c)
	}
	_, src := api(h, "GET", "/api/artifacts/"+id+"/blocks", "", alice)
	_, dup := api(h, "GET", "/api/artifacts/"+c.ID+"/blocks", "", bob)
	if strip(src) != strip(dup) {
		t.Errorf("a copy changed the blocks or their ids:\n%s\n%s", src, dup)
	}
	if got := h.comments(h.browser("bob", "pw-b"), c.ID); len(got) != 0 {
		t.Errorf("the copy has comments: %+v", got)
	}
}
