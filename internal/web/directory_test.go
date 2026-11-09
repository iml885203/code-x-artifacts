package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestPeopleSearchIncludesADirectoryColleagueWhoNeverSignedInAndKeepsDepartedMentionNames(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.dir.mu.Lock()
	h.dir.accounts["dana"] = &fakeAccount{password: "test-only", groups: []string{"SE"}}
	h.dir.mu.Unlock()
	alice := h.browser("alice", "pw-a")
	token := h.token("alice")
	id := h.privateDocWithEditor(token)
	if _, err := h.st.User(t.Context(), "dana"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("roster colleague has a sign-in profile: %v", err)
	}
	_, found := h.peopleOf(alice, id, "dana")
	if len(found) != 1 || found[0].Name != "DANA" || found[0].CanOpen {
		t.Fatalf("never-signed-in directory colleague: %+v", found)
	}
	// Lookup metadata never turns into an authenticated profile or a sharing grant.
	if _, err := h.st.User(t.Context(), "dana"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("search created a profile")
	}
	mention := h.agent(token).ok("publish", map[string]any{"title": "Directory mention", "kind": "doc", "blocks": []any{map[string]any{"id": "p", "type": "paragraph", "content": []any{map[string]any{"user": "dana", "name": "DANA <script>"}}}}})
	mentionID := mention["id"].(string)
	h.dir.mu.Lock()
	delete(h.dir.accounts, "dana")
	h.dir.mu.Unlock()
	h.browser("alice", "pw-a")
	_, found = h.peopleOf(alice, id, "dana")
	if len(found) != 0 {
		t.Fatalf("departed colleague suggested: %+v", found)
	}
	res, raw := h.do(alice, "GET", "/raw/"+mentionID, "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(raw, `<span class="mention">@DANA &lt;script&gt;</span>`) || !strings.Contains(raw, `.mention { color: var(--muted); }`) {
		t.Fatalf("departed name is not stored escaped muted text: %d %s", res.StatusCode, raw)
	}
	read := h.agent(token).ok("read", map[string]any{"id": mentionID, "view": "blocks"})
	if !strings.Contains(string(mustJSON(t, read)), `DANA \u003cscript\u003e`) {
		t.Fatalf("agent lost stored name: %v", read)
	}
}

func TestAnIncompleteDirectoryRefreshKeepsSignInAndThePreviousRosterButReportsLimitedSearch(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.browser("alice", "pw-a")
	id := h.privateDocWithEditor(h.token("alice"))
	h.dir.mu.Lock()
	h.dir.incomplete = true
	delete(h.dir.accounts, "bob")
	h.dir.mu.Unlock()
	alice := h.browser("alice", "pw-a")
	res, body := h.do(alice, "GET", "/api/artifacts/"+id+"/people?q=bob", "", bridgeHeaders(h))
	var got struct {
		People    []personView          `json:"people"`
		Directory store.DirectoryStatus `json:"directory"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || got.Directory.Complete || len(got.People) != 1 {
		t.Fatalf("limited search after successful sign-in: %d %s", res.StatusCode, body)
	}
	if !strings.Contains(body, "BOB") {
		t.Fatal("previous roster was lost")
	}
}
