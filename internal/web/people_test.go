package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type personView struct {
	User    string `json:"user"`
	Name    string `json:"name"`
	CanOpen bool   `json:"canOpen"`
}

// peopleOf searches the people a doc's editor can mention.
func (h *harness) peopleOf(c *http.Client, id, q string) (response, []personView) {
	h.t.Helper()
	res, body := h.do(c, "GET", "/api/artifacts/"+id+"/people?q="+url.QueryEscape(q), "", bridgeHeaders(h))
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	return res, decode[struct {
		People []personView `json:"people"`
	}](h.t, body).People
}

func accountsOf(people []personView) string {
	var names []string
	for _, p := range people {
		names = append(names, p.User)
	}
	return strings.Join(names, ",")
}

// privateDocWithEditor publishes a doc of alice's, shared with bob as an editor and with nobody else.
func (h *harness) privateDocWithEditor(aliceToken string) string {
	h.t.Helper()
	id := h.editableDoc(aliceToken)
	if res, out := patchEditors(h, aliceToken, id, `{"add":["bob"]}`); res.StatusCode != http.StatusOK {
		h.t.Fatalf("name an editor: %d %s", res.StatusCode, out)
	}
	return id
}

func TestPeopleSearchIsForSignedInPeopleWhoCanEditTheDoc(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceToken := h.token("alice")
	alice, bob, carol := h.browser("alice", "pw-a"), h.browser("bob", "pw-b"), h.browser("carol", "pw-c")
	id := h.privateDocWithEditor(aliceToken)

	for who, c := range map[string]*http.Client{"the author": alice, "a named editor": bob} {
		if res, _ := h.peopleOf(c, id, ""); res.StatusCode != http.StatusOK {
			t.Errorf("%s searches: %d", who, res.StatusCode)
		}
	}
	if res, _ := h.peopleOf(nil, id, ""); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous: %d, want 401", res.StatusCode)
	}
	if res, _ := h.peopleOf(carol, id, ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("someone who cannot even see the doc: %d, want the answer of a missing doc", res.StatusCode)
	}
	if res, _ := api(h, "PATCH", "/api/artifacts/"+id, `{"visibility":"organisation"}`, aliceToken); res.StatusCode != http.StatusOK {
		t.Fatalf("share with the organisation: %d", res.StatusCode)
	}
	if res, _ := h.peopleOf(carol, id, ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("a reader who cannot edit: %d, want 403", res.StatusCode)
	}
	if res, _ := h.peopleOf(alice, "nope", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("a doc that does not exist: %d", res.StatusCode)
	}
}

func TestPeopleSearchFindsDirectoryColleaguesByAccountOrNameAndSaysWhoCanOpenTheDoc(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceToken := h.token("alice")
	alice := h.browser("alice", "pw-a")
	h.browser("bob", "pw-b")
	h.browser("carol", "pw-c")
	id := h.privateDocWithEditor(aliceToken)

	_, all := h.peopleOf(alice, id, "")
	if len(all) != 3 {
		t.Fatalf("with nothing typed the organisation directory is listed: %+v", all)
	}
	open := map[string]bool{}
	for _, p := range all {
		open[p.User] = p.CanOpen
	}
	if !open["alice"] || !open["bob"] || open["carol"] {
		t.Errorf("who can open the private doc = %v, want its author and its editor, not carol", open)
	}
	for q, want := range map[string]string{"bo": "bob", "CAROL": "carol", "zzz": "", "%": "", "_": ""} {
		if _, got := h.peopleOf(alice, id, q); accountsOf(got) != want {
			t.Errorf("search %q = %q, want %q", q, accountsOf(got), want)
		}
	}
	if _, got := h.peopleOf(alice, id, " ali "); accountsOf(got) != "alice" {
		t.Errorf("surrounding query whitespace: %v", got)
	}
	if _, got := h.peopleOf(alice, id, "bob"); len(got) != 1 || got[0].Name != "BOB" {
		t.Errorf("a person is shown by the name the directory gave: %+v", got)
	}
}

func TestMentioningSomeoneWhoCannotSeeADocGivesThemNoAccessAndTellsThemNothingAboutIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceToken := h.token("alice")
	h.browser("alice", "pw-a")
	carol := h.browser("carol", "pw-c")
	carolAgent := h.agent(h.token("carol"))

	body := `{"title":"Quarterly secrets","kind":"doc","blocks":[{"id":"p","type":"paragraph","content":[{"text":"Carol to confirm: "},{"user":"carol","name":"CAROL"}]}]}`
	res, out := api(h, "POST", "/api/artifacts", body, aliceToken)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish a doc that mentions carol: %d %s", res.StatusCode, out)
	}
	id := decode[artifactView](t, out).ID

	if res, _ := h.do(carol, "GET", "/api/artifacts/"+id, "", bridgeHeaders(h)); res.StatusCode != http.StatusNotFound {
		t.Errorf("the doc itself for the person it mentions: %d, want the answer of a missing doc", res.StatusCode)
	}
	for _, query := range []string{"", "?query=carol", "?query=Quarterly", "?query=alice"} {
		_, list := h.do(carol, "GET", "/api/artifacts"+query, "", bridgeHeaders(h))
		if strings.Contains(list, id) || strings.Contains(list, "Quarterly secrets") {
			t.Errorf("the gallery list%s shows the doc to the person it mentions: %s", query, list)
		}
	}
	if got := carolAgent.refused("changes_since", map[string]any{"id": id, "since_version": 1}); !strings.Contains(got, "no artifact with that id is visible to you") {
		t.Errorf("changes_since for the person it mentions: %s", got)
	}
	listed := carolAgent.ok("list", map[string]any{"query": "Quarterly"})
	if strings.Contains(string(mustJSON(t, listed)), id) {
		t.Errorf("the agent's list shows the doc: %v", listed)
	}
	_, all := api(h, "GET", "/api/artifacts/"+id, "", aliceToken)
	if got := decode[artifactView](t, all); len(got.Editors) != 0 || got.Visibility != "private" {
		t.Errorf("the mention changed who the doc is shared with: editors %v, audience %q", got.Editors, got.Visibility)
	}
}

func TestAnAgentMentionsAPersonByAccountInMarkdownOrBlocksAndReadsTheSameMentionBack(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	viaHTTP, viaMCP := h.editableDoc(tok), h.editableDoc(tok)

	ops := `[{"op":"replace","id":"p1","markdown":"Ask [@Anna Lee](mention:anna.lee) to confirm"},{"op":"insert","after":"p1","blocks":[{"id":"p3","type":"paragraph","content":[{"user":"bob","name":"BOB"}]}]}]`
	res, body := api(h, "POST", "/api/artifacts/"+viaHTTP+"/edit", `{"ops":`+ops+`}`, tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("edit over HTTP: %d %s", res.StatusCode, body)
	}
	var opsValue []any
	if err := jsonUnmarshal(ops, &opsValue); err != nil {
		t.Fatal(err)
	}
	agent.ok("doc_edit", map[string]any{"id": viaMCP, "ops": opsValue})

	for name, id := range map[string]string{"over HTTP": viaHTTP, "over MCP": viaMCP} {
		read := agent.ok("read", map[string]any{"id": id, "view": "blocks"})
		raw := string(mustJSON(t, read))
		for _, want := range []string{`"user":"anna.lee"`, `"name":"Anna Lee"`, `"user":"bob"`} {
			if !strings.Contains(raw, want) {
				t.Errorf("the blocks read back %s lack %s: %s", name, want, raw)
			}
		}
		markdown := agent.ok("read", map[string]any{"id": id})
		if !strings.Contains(string(mustJSON(t, markdown)), "[@Anna Lee](mention:anna.lee)") {
			t.Errorf("the Markdown read back %s lacks the mention: %v", name, markdown)
		}
	}
	if refusal := agent.refused("doc_edit", map[string]any{"id": viaMCP, "ops": []any{map[string]any{"op": "insert", "after": "p1", "blocks": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"user": "a b", "name": "x"}}}}}}}); !strings.Contains(refusal, "content[0].user") {
		t.Errorf("a bad account is refused with its path: %s", refusal)
	}
}
