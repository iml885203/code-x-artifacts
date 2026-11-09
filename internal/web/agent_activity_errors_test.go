package web

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// Reading through both transports also proves that observing a failure does not change its evidence.
func errorActivitySnapshot(t *testing.T, h *harness, token string) string {
	t.Helper()
	httpPage := activityPage(t, h, token, "")
	body := rawActivityMCP(t, h, token, `{"jsonrpc":"2.0","id":99,"method":"tools/call","params":{"name":"agent_activity","arguments":{}}}`)
	var reply struct {
		Result struct {
			StructuredContent json.RawMessage `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &reply); err != nil {
		t.Fatal(err)
	}
	var mcpPage any
	if err := json.Unmarshal(reply.Result.StructuredContent, &mcpPage); err != nil {
		t.Fatalf("MCP activity: %s: %v", body, err)
	}
	encoded, err := json.Marshal(httpPage)
	if err != nil {
		t.Fatal(err)
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(normalized)
	got, _ := json.Marshal(mcpPage)
	if string(got) != string(want) {
		t.Fatalf("HTTP/MCP activity differ: %s / %s", want, got)
	}
	return string(want)
}

func TestFailedProtocolAndToolRequestsLeaveAgentHistoryAndSummariesUnchanged(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, request, credential, want string
		status                          int
	}{
		{"initialize missing params", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":null}`, "", "missing required", 200},
		{"initialize malformed client", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":17}}`, "", "error", 200},
		{"unknown method", `{"jsonrpc":"2.0","id":1,"method":"no_such_method","params":{}}`, "", "unsupported", 400},
		{"tool missing params", `{"jsonrpc":"2.0","id":1,"method":"tools/call"}`, "", "missing required", 400},
		{"unknown tool", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"no_such_tool","arguments":{}}}`, "", "error", 200},
		{"invalid publish", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"publish","arguments":{"title":"","content":"bad"}}}`, "", `"isError":true`, 200},
		{"invalid credential", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, "invalid-local-fixture", "", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			token := h.token("alice")
			before := errorActivitySnapshot(t, h, token)
			credential := token
			if tc.credential != "" {
				credential = tc.credential
			}
			res, body := h.do(nil, "POST", "/api/mcp", tc.request, map[string]string{"Authorization": "Bearer " + credential, "Content-Type": "application/json", "Accept": "application/json, text/event-stream"})
			if res.StatusCode != tc.status || !strings.Contains(body, tc.want) {
				t.Fatalf("failure response: %d %s", res.StatusCode, body)
			}
			if after := errorActivitySnapshot(t, h, token); after != before {
				t.Fatalf("failed request changed history/summaries: %s -> %s", before, after)
			}
		})
	}
}

func TestDeniedAndStaleWritesLeaveAgentHistorySummariesAndContentUnchanged(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	owner, alice, bob := h.token("alice"), h.token("alice"), h.token("bob")
	a := h.publish(owner, "Original", page)
	if res, body := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Private","content":"`+page+`","baseVersion":1}`, owner); res.StatusCode != http.StatusOK {
		t.Fatalf("seed second version: %d %s", res.StatusCode, body)
	}
	beforeAlice, beforeBob := errorActivitySnapshot(t, h, alice), errorActivitySnapshot(t, h, bob)
	for _, tc := range []struct {
		token        string
		base, status int
	}{{bob, 1, 404}, {alice, 1, 409}} {
		input := map[string]any{"id": a.ID, "title": "Must not save", "content": "<p>refused</p>", "baseVersion": tc.base}
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		res, body := api(h, "PUT", "/api/artifacts/"+a.ID, string(data), tc.token)
		if res.StatusCode != tc.status {
			t.Fatalf("HTTP refusal: %d %s", res.StatusCode, body)
		}
		request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"publish","arguments":` + string(data) + `}}`
		body = rawActivityMCP(t, h, tc.token, request)
		if !strings.Contains(body, `"isError":true`) {
			t.Fatalf("MCP accepted refused write: %s", body)
		}
	}
	if after := errorActivitySnapshot(t, h, alice); after != beforeAlice {
		t.Fatalf("owner metadata changed: %s -> %s", beforeAlice, after)
	}
	if after := errorActivitySnapshot(t, h, bob); after != beforeBob {
		t.Fatalf("denied caller metadata changed: %s -> %s", beforeBob, after)
	}
	saved, err := h.st.Artifact(t.Context(), a.ID)
	if err != nil || saved.Content != page || saved.Version != 2 || saved.Title != "Private" {
		t.Fatalf("refusal changed content: %+v %v", saved, err)
	}
}

func activityFailureHarness(t *testing.T) (*harness, *sql.DB, *syncBuffer) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "failure.db")
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	logs := new(syncBuffer)
	h := newHarnessOn(t, st, func(c *Config) { c.Logger = slog.New(slog.NewTextHandler(logs, nil)) })
	return h, db, logs
}

func activityFailureSQL(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), statement); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicActivityFailureReturnsAnErrorWithoutSavingTheHTTPOrMCPWrite(t *testing.T) {
	t.Parallel()
	h, db, _ := activityFailureHarness(t)
	token := h.token("alice")
	a := h.publish(token, "Before", page)
	before := errorActivitySnapshot(t, h, token)
	activityFailureSQL(t, db, `CREATE TRIGGER fail_activity BEFORE INSERT ON agent_activity BEGIN SELECT RAISE(ABORT,'activity storage unavailable'); END`)
	res, body := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"After","content":"<p>after</p>","baseVersion":1}`, token)
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("HTTP claimed success: %d %s", res.StatusCode, body)
	}
	body = rawActivityMCP(t, h, token, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"publish","arguments":{"id":"`+a.ID+`","title":"After","content":"<p>after</p>","baseVersion":1}}}`)
	if !strings.Contains(body, `"isError":true`) {
		t.Fatalf("MCP claimed success: %s", body)
	}
	saved, err := h.st.Artifact(t.Context(), a.ID)
	if err != nil || saved.Version != 1 || saved.Content != page || saved.Title != "Before" {
		t.Fatalf("partial write: %+v %v", saved, err)
	}
	if after := errorActivitySnapshot(t, h, token); after != before {
		t.Fatalf("atomic failure changed metadata: %s -> %s", before, after)
	}
}

func TestPostOperationUsageFailureKeepsCommittedContentAndLogsTheMetadataError(t *testing.T) {
	t.Parallel()
	for _, transport := range []string{"http", "mcp"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()
			h, db, logs := activityFailureHarness(t)
			token := h.token("alice")
			activityFailureSQL(t, db, `CREATE TRIGGER fail_post_use BEFORE INSERT ON agent_token_usage WHEN EXISTS(SELECT 1 FROM agent_token_usage WHERE token_id=NEW.token_id) BEGIN SELECT RAISE(ABORT,'post-operation usage unavailable'); END`)
			// Expire the initial summary's throttle without waiting; the next post-operation
			// summary insert fails after the content/activity transaction has committed.
			activityFailureSQL(t, db, `CREATE TRIGGER expire_usage AFTER INSERT ON agent_token_usage BEGIN UPDATE agent_token_usage SET last_used=last_used-120000 WHERE token_id=NEW.token_id; END`)
			id := committedActivityPublish(t, h, token, transport)
			saved, err := h.st.Artifact(t.Context(), id)
			if err != nil || saved.Content != page || saved.Version != 1 {
				t.Fatalf("committed content: %+v %v", saved, err)
			}
			got := activityPage(t, h, token, "")
			if len(got.Items) != 1 || got.Items[0].Operation != "publish" || len(got.Tokens) != 1 || got.Tokens[0].LastUsed == nil || got.Tokens[0].LastConnected != nil {
				t.Fatalf("committed metadata: %+v", got)
			}
			message := `level=ERROR msg="record token use"`
			if transport == "mcp" {
				message = `level=ERROR msg="record agent activity"`
			}
			if !strings.Contains(logs.String(), message) || !strings.Contains(logs.String(), "post-operation usage unavailable") {
				t.Fatalf("metadata failure was not observable: %s", logs.String())
			}
		})
	}
}

func committedActivityPublish(t *testing.T, h *harness, token, transport string) string {
	t.Helper()
	var id string
	if transport == "http" {
		res, body := api(h, "POST", "/api/artifacts", `{"title":"Committed","content":"`+page+`"}`, token)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("committed write reported failure: %d %s", res.StatusCode, body)
		}
		id = decode[struct {
			ID string `json:"id"`
		}](t, body).ID
	} else {
		body := rawActivityMCP(t, h, token, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"publish","arguments":{"title":"Committed","content":"`+page+`"}}}`)
		if strings.Contains(body, `"isError":true`) {
			t.Fatalf("committed MCP write reported failure: %s", body)
		}
		var reply struct {
			Result struct {
				StructuredContent struct {
					ID string `json:"id"`
				} `json:"structuredContent"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(body), &reply); err != nil {
			t.Fatal(err)
		}
		id = reply.Result.StructuredContent.ID
	}
	return id
}
