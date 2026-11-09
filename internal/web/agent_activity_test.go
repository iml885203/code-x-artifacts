package web

import (
	"encoding/json"
	"maps"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/service"
)

func activityPage(t *testing.T, h *harness, token, query string) service.AgentActivityPage {
	t.Helper()
	res, body := h.do(nil, "GET", "/api/agent-activity"+query, "", map[string]string{"Authorization": "Bearer " + token})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("activity %d: %s", res.StatusCode, body)
	}
	var out service.AgentActivityPage
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func rawActivityMCP(t *testing.T, h *harness, token, body string) string {
	t.Helper()
	res, out := h.do(nil, "POST", "/api/mcp", body, map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json", "Accept": "application/json, text/event-stream"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("MCP %d: %s", res.StatusCode, out)
	}
	return out
}

func TestAgentActivityKeepsPerRequestClientsAndDoesNotInventStatelessIdentity(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token := h.token("alice")
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"claude-code","version":"2"}}}`
	rawActivityMCP(t, h, token, init)
	var calls sync.WaitGroup
	for _, client := range []string{"CodeXArtifacts Acceptance A", "CodeXArtifacts Acceptance B", ""} {
		calls.Add(1)
		go func() {
			defer calls.Done()
			meta := ""
			if client != "" {
				rawActivityMCP(t, h, token, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"`+client+`","version":"3"}}}`)
				meta = `"_meta":{"io.modelcontextprotocol/clientInfo":{"name":"` + client + `","version":"3"}},`
			}
			rawActivityMCP(t, h, token, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{`+meta+`"name":"publish","arguments":{"title":"`+client+` report","content":"`+page+`"}}}`)
		}()
	}
	calls.Wait()
	rawActivityMCP(t, h, token, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"_meta":{"io.modelcontextprotocol/clientInfo":{"name":"\t\u0000","version":"3"}},"name":"publish","arguments":{"title":"Sanitized unknown","content":"`+page+`"}}}`)
	if res, body := h.do(nil, "POST", "/api/artifacts", `{"title":"HTTP after initialize","html":"`+page+`"}`, map[string]string{"Authorization": "Bearer " + token}); res.StatusCode != http.StatusCreated {
		t.Fatalf("HTTP publish: %d %s", res.StatusCode, body)
	}
	got := activityPage(t, h, token, "")
	assertActivityClients(t, got.Items)
	if len(got.Tokens) != 1 || got.Tokens[0].LastConnected == nil || got.Tokens[0].LastUsed == nil {
		t.Fatalf("summaries: %+v", got.Tokens)
	}
	before, _ := json.Marshal(got)
	rawActivityMCP(t, h, token, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"agent_activity","arguments":{}}}`)
	after, _ := json.Marshal(activityPage(t, h, token, ""))
	if string(before) != string(after) {
		t.Fatalf("reading activity changed itself: %s -> %s", before, after)
	}
}

func TestAgentActivityReadsAgreeAndHideTargetsAfterAccessLossOrDeletion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.token("alice"), h.token("bob")
	res, body := h.do(nil, "POST", "/api/artifacts", `{"title":"Secret title","html":"`+page+`","visibility":"organisation"}`, map[string]string{"Authorization": "Bearer " + alice})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", res.StatusCode, body)
	}
	a := decode[struct {
		ID string `json:"id"`
	}](t, body)
	// Bob writes only while explicitly an editor.
	if res, body := h.do(nil, "PATCH", "/api/artifacts/"+a.ID+"/editors", `{"organisationEdit":true}`, map[string]string{"Authorization": "Bearer " + alice}); res.StatusCode != 200 {
		t.Fatalf("share: %d %s", res.StatusCode, body)
	}
	update := `{"title":"Secret title","html":"<!doctype html><p>bob edit</p>"}`
	res, body = h.do(nil, "PUT", "/api/artifacts/"+a.ID, update, map[string]string{"Authorization": "Bearer " + bob})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("edit: %d %s", res.StatusCode, body)
	}
	agent := h.agent(bob)
	mcpPage := agent.ok("agent_activity", nil)
	httpPage := activityPage(t, h, bob, "")
	encoded, _ := json.Marshal(httpPage)
	decoded := map[string]any{}
	_ = json.Unmarshal(encoded, &decoded)
	want, _ := json.Marshal(decoded)
	actual, _ := json.Marshal(mcpPage)
	if string(actual) != string(want) {
		t.Fatalf("HTTP/MCP drift: %s / %s", actual, want)
	}
	if len(httpPage.Items) != 1 || httpPage.Items[0].Transport != "http" || httpPage.Tokens[0].LastConnected != nil {
		t.Fatalf("stateless MCP use and HTTP edit: %+v", httpPage)
	}
	if err := h.st.SetSharing(t.Context(), a.ID, "private"); err != nil {
		t.Fatal(err)
	}
	hidden := activityPage(t, h, bob, "")
	data, _ := json.Marshal(hidden)
	if strings.Contains(string(data), a.ID) || strings.Contains(string(data), "Secret title") {
		t.Fatalf("hidden target leaked: %s", data)
	}
	own := activityPage(t, h, alice, "")
	foreign := own.Tokens[0].ID
	res, _ = h.do(nil, "GET", "/api/agent-activity?tokenID="+foreign, "", map[string]string{"Authorization": "Bearer " + bob})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign filter: %d", res.StatusCode)
	}
	if err := h.st.DeleteArtifact(t.Context(), a.ID); err != nil {
		t.Fatal(err)
	}
	deleted := activityPage(t, h, alice, "")
	data, _ = json.Marshal(deleted)
	if strings.Contains(string(data), a.ID) || strings.Contains(string(data), "Secret title") {
		t.Fatalf("deleted target leaked: %s", data)
	}
	assertRevokedActivity(t, h, foreign)
}

func TestFailedAndNoopAgentOperationsDoNotCreateActivity(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token := h.token("alice")
	headers := map[string]string{"Authorization": "Bearer " + token}
	rawActivityMCP(t, h, token, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"publish","arguments":{"title":"","html":"bad"}}}`)
	got := activityPage(t, h, token, "")
	if len(got.Items) != 0 || len(got.Tokens) != 1 || got.Tokens[0].LastUsed != nil || got.Tokens[0].LastConnected != nil {
		t.Fatalf("failed write counted: %+v", got)
	}
	res, body := h.do(nil, "POST", "/api/artifacts", `{"title":"same","html":"`+page+`"}`, headers)
	if res.StatusCode != http.StatusCreated {
		t.Fatal(body)
	}
	var a struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &a)
	h.do(nil, "PUT", "/api/artifacts/"+a.ID, `{"title":"same","html":"`+page+`"}`, headers)
	got = activityPage(t, h, token, "")
	if len(got.Items) != 1 {
		t.Fatalf("no-op creates activity: %+v", got.Items)
	}
	// A failed authenticated read must not advance the successful-use summary.
	previous := *got.Tokens[0].LastUsed
	_ = previous
	for _, q := range []string{"?limit=-1", "?limit=101", "?cursor=broken"} {
		res, _ = h.do(nil, "GET", "/api/agent-activity"+q, "", headers)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("bad query %s: %d", q, res.StatusCode)
		}
	}
	if time.Since(previous) > time.Minute {
		t.Fatal("unexpected use timestamp")
	}
}

func TestACommittedPublishAndRestoreCreateOneActivityEachWithoutStagingOrRetryNoise(t *testing.T) {
	t.Parallel()
	h, restart := activityRestartHarness(t)
	token := h.token("alice")
	started := time.Now().Truncate(time.Millisecond)
	session := h.mustBegin(token, map[string]any{"title": "Large publish", "files": []any{declared("content", multiPage)}})
	h.uploadAll(session, map[string]string{"content": multiPage})
	if got := activityPage(t, h, token, ""); len(got.Items) != 0 {
		t.Fatalf("staging became edits: %+v", got.Items)
	}
	checkTime := activityRequestTime(t, h)
	res, body := h.commit(token, session.Session)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("commit: %d %s", res.StatusCode, body)
	}
	var made struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &made); err != nil {
		t.Fatal(err)
	}
	checkTime("publish", made.ID, 1)
	if res, _ = h.commit(token, session.Session); res.StatusCode != http.StatusNotFound {
		t.Fatalf("retry commit: %d", res.StatusCode)
	}
	if got := activityPage(t, h, token, ""); len(got.Items) != 1 {
		t.Fatalf("nested/retry publish: %+v", got.Items)
	}
	checkTime = activityRequestTime(t, h)
	if res, body := api(h, "PUT", "/api/artifacts/"+made.ID, `{"title":"Updated","html":"<!doctype html><p>new</p>"}`, token); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, body)
	}
	checkTime("publish", made.ID, 2)
	checkTime = activityRequestTime(t, h)
	if res, body := api(h, "POST", "/api/artifacts/"+made.ID+"/versions/1/restore", "", token); res.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d %s", res.StatusCode, body)
	}
	checkTime("restore", made.ID, 3)
	got := activityPage(t, h, token, "")
	if len(got.Items) != 3 || got.Items[0].Operation != "restore" {
		t.Fatalf("restore activity: %+v", got.Items)
	}
	finished := time.Now()
	before := positiveActivityRows(t, h.st, token, started, finished, map[string]int{"publish": 2, "restore": 1})
	h = restart()
	after := positiveActivityRows(t, h.st, token, started, finished, map[string]int{"publish": 2, "restore": 1})
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("commit and restore metadata changed across restart: %+v -> %+v", before, after)
	}
}

func assertRevokedActivity(t *testing.T, h *harness, tokenID string) {
	t.Helper()
	second := h.token("alice")
	if err := h.st.DeleteToken(t.Context(), "alice", tokenID); err != nil {
		t.Fatal(err)
	}
	kept := activityPage(t, h, second, "?tokenID="+tokenID)
	if len(kept.Items) != 1 || !kept.Items[0].Revoked {
		t.Fatalf("revoked history: %+v", kept)
	}
}

func assertActivityClients(t *testing.T, items []service.AgentActivityItem) {
	t.Helper()
	clients := map[string]int{}
	for _, e := range items {
		clients[e.Client]++
		if strings.HasPrefix(e.Client, "CodeXArtifacts Acceptance ") && e.ClientVersion != "3" {
			t.Fatalf("named request lost its version: %+v", e)
		}
		if e.Client == "" && e.ClientVersion != "" {
			t.Fatalf("unknown client exposes a guessed version: %+v", e)
		}
	}
	if !maps.Equal(clients, map[string]int{"Claude Code": 1, "CodeXArtifacts Acceptance A": 2, "CodeXArtifacts Acceptance B": 2, "": 3}) {
		t.Fatalf("request clients: %+v", items)
	}
}

// The SDK hands a missing or null params object to middleware as a typed nil, which reads no client metadata.
func TestAnMCPRequestWithoutParamsGetsAnAnswerAndTheServerKeepsServing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token := h.token("alice")
	headers := map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json", "Accept": "application/json, text/event-stream"}
	for _, c := range []struct {
		name, request string
		status        int
		want          string
	}{
		{"tools/list omitted", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, http.StatusOK, `"name":"publish"`},
		{"tools/list null", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":null}`, http.StatusOK, `"name":"publish"`},
		{"tools/list empty", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, http.StatusOK, `"name":"publish"`},
		{"tools/list null meta", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":null}}`, http.StatusOK, `"name":"publish"`},
		{"tools/call omitted", `{"jsonrpc":"2.0","id":1,"method":"tools/call"}`, http.StatusBadRequest, `missing required`},
		{"tools/call null", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":null}`, http.StatusOK, `missing required`},
		{"initialize null", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":null}`, http.StatusOK, `missing required`},
	} {
		res, got := h.do(nil, "POST", "/api/mcp", c.request, headers)
		if res.StatusCode != c.status || !strings.Contains(got, c.want) {
			t.Errorf("%s: want %d with %s, got %d %s", c.name, c.status, c.want, res.StatusCode, got)
		}
	}
	got := rawActivityMCP(t, h, token, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"_meta":{"io.modelcontextprotocol/clientInfo":{"name":"claude-code","version":"2"}},"name":"whoami","arguments":{}}}`)
	if !strings.Contains(got, `"result"`) || strings.Contains(got, `"isError":true`) {
		t.Fatalf("a valid call after requests without params: %s", got)
	}
	if items := activityPage(t, h, token, "").Tokens; len(items) != 1 || items[0].LastUsed == nil {
		t.Fatalf("a valid call after requests without params records its use: %+v", items)
	}
}
