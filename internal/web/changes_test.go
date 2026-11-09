package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// changesView is what changes_since answers, as JSON.
type changesView struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
	Versions  []struct {
		Version      int    `json:"version"`
		Author       string `json:"author"`
		Title        string `json:"title"`
		RestoredFrom int    `json:"restoredFrom"`
		CreatedAt    string `json:"createdAt"`
	} `json:"versions"`
	Comments []commentView `json:"comments"`
	Access   []struct {
		By               string   `json:"by"`
		Visibility       string   `json:"visibility"`
		Editors          []string `json:"editors"`
		OrganisationEdit bool     `json:"organisationEdit"`
		At               string   `json:"at"`
	} `json:"access"`
}

func changesOver(t *testing.T, h *harness, tok, id, query string) (response, string) {
	t.Helper()
	return api(h, "GET", "/api/artifacts/"+id+"/changes?"+query, "", tok)
}

func TestChangesSinceReturnsWhatOthersDidSinceAVersionAndNothingElse(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	bob := h.agent(bobTok)
	a := h.publishShared(aliceTok, "Plan", "<p>one")
	if res, out := patchEditors(h, aliceTok, a.ID, `{"add":["bob"]}`); res.StatusCode != http.StatusOK {
		t.Fatalf("name an editor: %d %s", res.StatusCode, out)
	}

	// Alice's agent is up to date at version 1. Bob then publishes and comments.
	if out := bob.ok("publish", map[string]any{"id": a.ID, "title": "Plan, revised", "content": "<p>two</p>", "baseVersion": 1}); out["version"] != float64(2) {
		t.Fatalf("bob publishes: %v", out)
	}
	if res, out := h.comment(h.browser("bob", "pw-b"), a.ID, "Please check the dates.", ""); res.StatusCode != http.StatusCreated {
		t.Fatalf("bob comments: %d %s", res.StatusCode, out)
	}
	// A comment from long before is not news.
	if _, err := h.st.AddComment(t.Context(), store.Comment{ArtifactID: a.ID, Author: "bob", Body: "ancient", CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}

	res, out := changesOver(t, h, aliceTok, a.ID, "sinceVersion=1")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("changes: %d %s", res.StatusCode, out)
	}
	got := decode[changesView](t, out)
	if got.ID != a.ID || got.Version != 2 || got.Title != "Plan, revised" {
		t.Errorf("where the artifact is now: %+v", got)
	}
	expectNewVersionFromBob(t, got)
	expectBobsComment(t, got)
	expectAlicesAccessChange(t, got)

	// Asked from the newest version, no version is new.
	_, out = changesOver(t, h, aliceTok, a.ID, "sinceVersion=2")
	if got := decode[changesView](t, out); len(got.Versions) != 0 || got.Version != 2 {
		t.Errorf("nothing new after the newest version: %+v", got)
	}
}

func TestChangesSinceByTimeIsEverythingFromLongAgoAndNothingFromTheFuture(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok := h.token("alice")
	a := h.publishShared(aliceTok, "Plan", "<p>one")
	api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Plan","content":"<p>two"}`, aliceTok)
	h.comment(h.browser("alice", "pw-a"), a.ID, "now", "")
	if _, err := h.st.AddComment(t.Context(), store.Comment{ArtifactID: a.ID, Author: "bob", Body: "ancient", CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	_, out := changesOver(t, h, aliceTok, a.ID, "since="+url.QueryEscape("2019-01-01T00:00:00Z"))
	if got := decode[changesView](t, out); len(got.Versions) != 2 || len(got.Comments) != 2 {
		t.Errorf("since the distant past: %d versions, %d comments", len(got.Versions), len(got.Comments))
	}
	_, out = changesOver(t, h, aliceTok, a.ID, "since="+url.QueryEscape(time.Now().Add(time.Hour).UTC().Format(time.RFC3339)))
	if got := decode[changesView](t, out); len(got.Versions) != 0 || len(got.Comments) != 0 || len(got.Access) != 0 {
		t.Errorf("since the future: %+v", got)
	}
}

func TestChangesSinceRecordsAudienceAndEditorChangesOnlyWhenSomethingChanged(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	a := h.publish(aliceTok, "Plan", "<p>one")
	since := "since=" + url.QueryEscape("2019-01-01T00:00:00Z")
	access := func() []string {
		t.Helper()
		_, out := changesOver(t, h, aliceTok, a.ID, since)
		var rows []string
		for _, c := range decode[changesView](t, out).Access {
			rows = append(rows, c.By+":"+c.Visibility+":"+strings.Join(c.Editors, ",")+":"+map[bool]string{true: "all", false: "-"}[c.OrganisationEdit])
		}
		return rows
	}
	want := func(rows ...string) {
		t.Helper()
		if got := access(); strings.Join(got, "|") != strings.Join(rows, "|") {
			t.Errorf("access changes = %v, want %v", got, rows)
		}
	}
	want() // creating is not a change

	api(h, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"organisation"}`, aliceTok)
	want("alice:organisation::-")
	api(h, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"organisation"}`, aliceTok) // the same again
	want("alice:organisation::-")
	api(h, "PATCH", "/api/artifacts/"+a.ID+"/editors", `{"add":["bob"],"organisationEdit":true}`, aliceTok)
	want("alice:organisation::-", "alice:organisation:bob:all")
	api(h, "PATCH", "/api/artifacts/"+a.ID+"/editors", `{"add":["bob"]}`, aliceTok) // already an editor
	want("alice:organisation::-", "alice:organisation:bob:all")
	// An editor publishing, or repeating the audience, changes none of it.
	if res, out := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Plan","content":"<p>two","visibility":"organisation"}`, bobTok); res.StatusCode != http.StatusOK {
		t.Fatalf("editor publish: %d %s", res.StatusCode, out)
	}
	want("alice:organisation::-", "alice:organisation:bob:all")
	// The author narrowing the audience while publishing is one, and switches organisation editing off.
	if res, out := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Plan","content":"<p>three","visibility":"private"}`, aliceTok); res.StatusCode != http.StatusOK {
		t.Fatalf("author publish: %d %s", res.StatusCode, out)
	}
	want("alice:organisation::-", "alice:organisation:bob:all", "alice:private:bob:-")
}

func TestChangesSinceGivesNothingAwayToWhoCannotSeeTheArtifact(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	secret := h.publish(h.token("alice"), "Secret plan", "<p>secret")
	bobTok := h.token("bob")
	bob := h.agent(bobTok)

	wantRes, wantBody := changesOver(t, h, bobTok, "nosuchartifact0000", "sinceVersion=1")
	res, body := changesOver(t, h, bobTok, secret.ID, "sinceVersion=1")
	wantBody = strings.ReplaceAll(wantBody, "nosuchartifact0000", "ID")
	body = strings.ReplaceAll(body, secret.ID, "ID")
	if res.StatusCode != http.StatusNotFound || res.StatusCode != wantRes.StatusCode || body != wantBody {
		t.Errorf("a stranger asking over HTTP: %d %q, want %d %q", res.StatusCode, body, wantRes.StatusCode, wantBody)
	}
	// A malformed query does not tell a stranger the artifact is there either.
	if res, _ := changesOver(t, h, bobTok, secret.ID, "sinceVersion=banana"); res.StatusCode != http.StatusNotFound {
		t.Errorf("a stranger with a malformed query: %d, want 404", res.StatusCode)
	}
	if got := bob.refused("changes_since", map[string]any{"id": secret.ID, "since_version": 1}); !strings.Contains(got, "no artifact with that id is visible to you") {
		t.Errorf("a stranger asking over MCP: %q", got)
	}
	if res, _ := h.do(nil, "GET", "/api/artifacts/"+secret.ID+"/changes?sinceVersion=1", "", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("without credentials: %d, want 401", res.StatusCode)
	}
}

func TestChangesSinceAndItsToolAnswerTheSameAndRefuseUnclearQuestions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	bob := h.agent(bobTok)
	a := h.publishShared(aliceTok, "Plan", "<p>one")
	api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Plan 2","content":"<p>two"}`, aliceTok)
	h.comment(h.browser("alice", "pw-a"), a.ID, "Done.", "")

	_, body := changesOver(t, h, bobTok, a.ID, "sinceVersion=1")
	viaTool := bob.ok("changes_since", map[string]any{"id": a.ID, "since_version": 1})
	if string(mustJSON(t, viaTool)) != string(mustJSON(t, decode[map[string]any](t, body))) {
		t.Errorf("HTTP and MCP differ:\n%s\n%s", body, mustJSON(t, viaTool))
	}
	_, body = changesOver(t, h, bobTok, a.ID, "since=2019-01-01T00:00:00Z")
	viaTool = bob.ok("changes_since", map[string]any{"id": a.ID, "since": "2019-01-01T00:00:00Z"})
	if string(mustJSON(t, viaTool)) != string(mustJSON(t, decode[map[string]any](t, body))) {
		t.Errorf("by time, HTTP and MCP differ:\n%s\n%s", body, mustJSON(t, viaTool))
	}

	for name, c := range map[string]struct {
		query string
		args  map[string]any
		say   string
	}{
		"neither":       {"", map[string]any{}, "say where to start"},
		"both":          {"sinceVersion=1&since=2019-01-01T00:00:00Z", map[string]any{"since_version": 1, "since": "2019-01-01T00:00:00Z"}, "not both"},
		"beyond newest": {"sinceVersion=9", map[string]any{"since_version": 9}, "newest is 2"},
		"zero":          {"sinceVersion=0", map[string]any{"since_version": 0, "since": ""}, "say where to start"},
		"bad time":      {"since=yesterday", map[string]any{"since": "yesterday"}, "RFC 3339"},
		"not a number":  {"sinceVersion=two", nil, "number"},
	} {
		res, out := changesOver(t, h, bobTok, a.ID, c.query)
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, c.say) {
			t.Errorf("%s over HTTP: %d %s, want 400 mentioning %q", name, res.StatusCode, out, c.say)
		}
		if c.args != nil {
			c.args["id"] = a.ID
			if got := bob.refused("changes_since", c.args); !strings.Contains(got, c.say) {
				t.Errorf("%s over MCP: %q, want it to mention %q", name, got, c.say)
			}
		}
	}
}

func TestAnAnonymousViewerOfAPublicArtifactLearnsOnlyAboutNewVersions(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	aliceTok := h.token("alice")
	a := h.publishAs(aliceTok, "Open page", "<p>one", store.VisibilityPublic)
	api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Open page","content":"<p>two"}`, aliceTok)
	h.comment(h.browser("bob", "pw-b"), a.ID, "hello", "")
	api(h, "PATCH", "/api/artifacts/"+a.ID+"/editors", `{"add":["bob"]}`, aliceTok)

	res, out := h.do(nil, "GET", "/api/artifacts/"+a.ID+"/changes?sinceVersion=1", "", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("anonymous changes: %d %s", res.StatusCode, out)
	}
	got := decode[changesView](t, out)
	if got.Version != 2 || len(got.Versions) != 1 || got.Versions[0].Author != "" {
		t.Errorf("an anonymous viewer is told the new version and not who saved it: %+v", got)
	}
	if len(got.Comments) != 0 || len(got.Access) != 0 {
		t.Errorf("an anonymous viewer is told about comments or editors: %s", out)
	}
	if strings.Contains(out, "bob") || strings.Contains(out, "alice") {
		t.Errorf("an anonymous viewer reads account names: %s", out)
	}
}

func expectNewVersionFromBob(t *testing.T, got changesView) {
	t.Helper()
	if len(got.Versions) != 1 || got.Versions[0].Version != 2 || got.Versions[0].Author != "bob" || got.Versions[0].CreatedAt == "" {
		t.Errorf("new versions, who and when: %+v", got.Versions)
	}
	if len(got.Versions) > 0 && got.Versions[0].Title != "" {
		t.Errorf("a version in the changes carries its title %q: titles are for those who may read history", got.Versions[0].Title)
	}
}

func expectBobsComment(t *testing.T, got changesView) {
	t.Helper()
	if len(got.Comments) != 1 || got.Comments[0].Author != "bob" || got.Comments[0].Body != "Please check the dates." {
		t.Errorf("new comments: %+v", got.Comments)
	}
}

func expectAlicesAccessChange(t *testing.T, got changesView) {
	t.Helper()
	if len(got.Access) != 1 || got.Access[0].By != "alice" || got.Access[0].Visibility != "organisation" || len(got.Access[0].Editors) != 1 || got.Access[0].Editors[0] != "bob" {
		t.Errorf("audience and editor changes: %+v", got.Access)
	}
}
