package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestFieldsAreEditedAndRefusedTheSameOverHTTPAndMCPAndSurviveSQLiteReopening(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "dates.db")
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := newHarnessOn(t, st)
	token := h.token("alice")
	agent := h.agent(token)
	ids := []string{h.editableDoc(token), h.editableDoc(token)}
	const value = `[{"text":"Due "},{"date":"2026-10-24"},{"text":" / "},{"dropdown":{"options":["Not started","Done"],"value":"Not started"}},{"text":" now"}]`
	for i, id := range ids {
		hash := fieldBlock(t, h, token, id)["hash"].(string)
		ops := `[{"op":"replace","id":"p1","ifHash":"` + hash + `","blocks":[{"type":"paragraph","content":` + value + `}]}]`
		if i == 0 {
			h.editOverHTTP(token, id, ops)
		} else {
			editOverMCP(t, agent, id, ops)
		}
		current := fieldBlock(t, h, token, id)
		if got := string(mustJSON(t, current["content"])); got != value {
			t.Fatalf("stored date: %s", got)
		}
		read := agent.ok("read", map[string]any{"id": id, "view": "blocks"})
		if !strings.Contains(string(mustJSON(t, read)), `"date":"2026-10-24"`) || !strings.Contains(string(mustJSON(t, read)), `"dropdown":{"options":["Not started","Done"],"value":"Not started"}`) {
			t.Fatalf("MCP dropped the field: %v", read)
		}

		assertFieldRefusals(t, h, agent, token, id, hash, ops, current)
	}
	h.srv.Close()
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	h = newHarnessOn(t, reopened)
	for _, id := range ids {
		if got := string(mustJSON(t, fieldBlock(t, h, token, id)["content"])); got != value {
			t.Fatalf("SQLite reload: %s", got)
		}
		res, body := h.do(h.browser("alice", "pw-a"), "GET", "/raw/"+id, "", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `<time datetime="2026-10-24">2026-10-24</time>`) || !strings.Contains(body, `<span class="dropdown">Not started</span>`) || strings.Contains(body, `type="date"`) {
			t.Fatalf("static print page: %d %s", res.StatusCode, body)
		}
	}
}

func fieldBlock(t *testing.T, h *harness, token, id string) map[string]any {
	t.Helper()
	res, body := api(h, "GET", "/api/artifacts/"+id+"/blocks", "", token)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("read: %d %s", res.StatusCode, body)
	}
	read := decode[map[string]any](t, body)
	for _, tab := range read["tabs"].([]any) {
		for _, raw := range tab.(map[string]any)["blocks"].([]any) {
			b := raw.(map[string]any)
			if b["id"] == "p1" {
				return b
			}
		}
	}
	t.Fatal("the edited block lost its id")
	return nil
}

func assertFieldRefusals(t *testing.T, h *harness, agent *agent, token, id, hash, ops string, current map[string]any) {
	t.Helper()
	for _, badField := range []struct{ run, path string }{
		{`{"date":"2026-02-29"}`, "content[0].date"},
		{`{"dropdown":{"options":["A"],"value":"removed"}}`, "content[0].dropdown.value"},
	} {
		bad := `[{"op":"replace","id":"p1","blocks":[{"type":"paragraph","content":[` + badField.run + `]}]}]`
		res, body := api(h, "POST", "/api/artifacts/"+id+"/edit", `{"ops":`+bad+`}`, token)
		refusal := agent.refused("doc_edit", map[string]any{"id": id, "ops": decode[[]any](t, bad)})
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, badField.path) || !strings.Contains(refusal, badField.path) {
			t.Fatalf("refusals differ: %d %s / %s", res.StatusCode, body, refusal)
		}
		if string(mustJSON(t, fieldBlock(t, h, token, id))) != string(mustJSON(t, current)) {
			t.Fatal("refused edit changed stored content or hash")
		}
	}

	stale := `[{"op":"replace","id":"p1","ifHash":"` + hash + `","markdown":"overwrite"}]`
	res, body := api(h, "POST", "/api/artifacts/"+id+"/edit", `{"ops":`+stale+`}`, token)
	if res.StatusCode != http.StatusConflict || !strings.Contains(body, "2026-10-24") {
		t.Fatalf("stale guard: %d %s", res.StatusCode, body)
	}
	if res, _ := api(h, "POST", "/api/artifacts/"+id+"/edit", `{"ops":`+ops+`}`, h.token("mallory")); res.StatusCode != http.StatusNotFound {
		t.Fatalf("stranger edited date: %d", res.StatusCode)
	}
}
