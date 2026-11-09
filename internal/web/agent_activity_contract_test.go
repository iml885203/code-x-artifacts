package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/iml885203/code-x-artifacts/internal/service"
)

func activityContractParity(t *testing.T, h *harness, a *agent, token string, args map[string]any, query string) service.AgentActivityPage {
	t.Helper()
	got := activityPage(t, h, token, query)
	data, err := json.Marshal(a.ok("agent_activity", args))
	if err != nil {
		t.Fatal(err)
	}
	var other service.AgentActivityPage
	if err := json.Unmarshal(data, &other); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, other) {
		t.Fatalf("HTTP/MCP activity drift: %+v / %+v", got, other)
	}
	return got
}

func TestActivityFiltersDistinguishSameLabelTokensAndDoNotDiscloseForeignSummaries(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	first, second, foreign := h.token("alice"), h.token("alice"), h.token("bob")
	a := h.agent(first)
	b := h.agent(foreign)
	for _, token := range []string{first, second, foreign} {
		h.publish(token, "Owned edit", page)
	}
	overview := activityContractParity(t, h, a, first, nil, "")
	if len(overview.Tokens) != 2 {
		t.Fatalf("owned summaries: %+v", overview.Tokens)
	}
	ids := map[string]bool{}
	for _, token := range overview.Tokens {
		if token.Name != "test" || ids[token.ID] {
			t.Fatalf("same-label identities: %+v", overview.Tokens)
		}
		ids[token.ID] = true
		filtered := activityContractParity(t, h, a, first, map[string]any{"tokenID": token.ID}, "?tokenID="+token.ID)
		edits := 0
		for _, item := range filtered.Items {
			if item.TokenID != token.ID || item.TokenLabel != "test" {
				t.Fatalf("filter crossed token identity: %+v", filtered)
			}
			if item.Operation == "publish" {
				edits++
			}
		}
		if edits != 1 || !reflect.DeepEqual(filtered.Tokens, overview.Tokens) {
			t.Fatalf("filter lost edit or summaries: %+v", filtered)
		}
	}
	bob := activityContractParity(t, h, b, foreign, nil, "")
	if len(bob.Tokens) != 1 || ids[bob.Tokens[0].ID] {
		t.Fatalf("foreign ownership: %+v", bob)
	}
	assertActivityForeignFilter(t, h, a, first, bob.Tokens[0].ID, overview)
}

func assertActivityForeignFilter(t *testing.T, h *harness, a *agent, first, foreignID string, overview service.AgentActivityPage) {
	t.Helper()
	var httpError, mcpError string
	for _, id := range []string{foreignID, "nonexistent-token"} {
		res, body := h.do(nil, "GET", "/api/agent-activity?tokenID="+id, "", map[string]string{"Authorization": "Bearer " + first})
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("foreign filter: %d %s", res.StatusCode, body)
		}
		out, refusal := a.call("agent_activity", map[string]any{"tokenID": id})
		if out != nil || refusal == "" {
			t.Fatalf("foreign filter returned data: %v %s", out, refusal)
		}
		if httpError == "" {
			httpError, mcpError = body, refusal
		} else if body != httpError || refusal != mcpError {
			t.Fatalf("foreign and missing errors differ: %s / %s", body, refusal)
		}
		for _, secret := range []string{foreignID, overview.Tokens[0].ID, overview.Tokens[1].ID, "Owned edit", `"tokens"`} {
			if strings.Contains(body+refusal, secret) {
				t.Fatalf("filter error disclosed %q: %s %s", secret, body, refusal)
			}
		}
	}
}

func TestActivityCursorPagesRecheckUnavailableTargetsThroughHTTPAndMCP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.token("alice"), h.token("bob")
	a := h.agent(bob)
	var ids []string
	for _, title := range []string{"Cursor hidden title one", "Cursor hidden title two"} {
		made := h.publishShared(alice, title, page)
		ids = append(ids, made.ID)
		if res, body := api(h, "PATCH", "/api/artifacts/"+made.ID+"/editors", `{"organisationEdit":true}`, alice); res.StatusCode != 200 {
			t.Fatal(body)
		}
		if res, body := api(h, "PUT", "/api/artifacts/"+made.ID, `{"title":"`+title+`","html":"<!doctype html><p>edited</p>"}`, bob); res.StatusCode != 200 {
			t.Fatal(body)
		}
	}
	first := activityContractParity(t, h, a, bob, map[string]any{"limit": 1}, "?limit=1")
	if len(first.Items) != 1 || first.Items[0].Target == nil || first.NextCursor == "" {
		t.Fatalf("first page: %+v", first)
	}
	args := map[string]any{"limit": 1, "cursor": first.NextCursor}
	query := "?limit=1&cursor=" + url.QueryEscape(first.NextCursor)
	before := activityContractParity(t, h, a, bob, args, query)
	if len(before.Items) != 1 || before.Items[0].Target == nil || before.Items[0].ID == first.Items[0].ID {
		t.Fatalf("second page positive control: %+v", before)
	}
	for _, state := range []string{"access loss", "deletion"} {
		changeActivityTargetAvailability(t, h, ids, state)
		after := activityContractParity(t, h, a, bob, args, query)
		if len(after.Items) != 1 || after.Items[0].ID != before.Items[0].ID || after.Items[0].Target != nil {
			t.Fatalf("%s changed history or retained target: %+v", state, after)
		}
		assertActivityExcludes(t, after, append(ids, "Cursor hidden title", "/a/")...)
	}
}

func TestActivityReturnedObjectsExcludeCredentialsHeadersAndContentAndBoundClientNames(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token := h.token("alice")
	name, version := strings.Repeat("界", 90)+"\t", strings.Repeat("版", 80)+"\n"
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": name, "version": version}}})
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + token, "Cookie": "activity-cookie-sentinel", "X-Activity-Sentinel": "activity-header-sentinel", "Content-Type": "application/json", "Accept": "application/json, text/event-stream"}
	if res, body := h.do(nil, "POST", "/api/mcp", string(payload), headers); res.StatusCode != http.StatusOK || strings.Contains(body, `"error"`) {
		t.Fatalf("initialize: %d %s", res.StatusCode, body)
	}
	h.publish(token, "Visible title control", "<!doctype html><p>activity-content-sentinel</p>")
	a := h.agent(token)
	got := activityContractParity(t, h, a, token, nil, "")
	assertActivityBoundedClients(t, got.Items)
	sum := sha256.Sum256([]byte(token))
	res, httpBody := h.do(nil, "GET", "/api/agent-activity", "", map[string]string{"Authorization": "Bearer " + token})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("activity: %d %s", res.StatusCode, httpBody)
	}
	for _, obj := range []any{json.RawMessage(httpBody), a.ok("agent_activity", nil)} {
		data, err := json.Marshal(obj)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "Visible title control") || !strings.Contains(text, got.Tokens[0].ID) {
			t.Fatalf("positive response controls missing: %s", text)
		}
		for _, excluded := range []string{token, hex.EncodeToString(sum[:]), "activity-cookie-sentinel", "activity-header-sentinel", "activity-content-sentinel", "Authorization", "Cookie", "X-Activity-Sentinel", name, version} {
			if strings.Contains(text, excluded) {
				t.Fatalf("returned activity disclosed %q: %s", excluded, text)
			}
		}
	}
}

func changeActivityTargetAvailability(t *testing.T, h *harness, ids []string, state string) {
	t.Helper()
	for _, id := range ids {
		var err error
		if state == "access loss" {
			err = h.st.SetSharing(t.Context(), id, "private")
		} else {
			err = h.st.DeleteArtifact(t.Context(), id)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func assertActivityBoundedClients(t *testing.T, items []service.AgentActivityItem) {
	t.Helper()
	bounded := false
	for _, item := range items {
		if utf8.RuneCountInString(item.Client) > 60 || utf8.RuneCountInString(item.ClientVersion) > 40 || strings.IndexFunc(item.Client+item.ClientVersion, unicode.IsControl) >= 0 {
			t.Fatalf("unbounded returned client: %+v", item)
		}
		if item.Client == strings.Repeat("界", 60) && item.ClientVersion == strings.Repeat("版", 40) {
			bounded = true
		}
	}
	if !bounded {
		t.Fatalf("long client request missing: %+v", items)
	}
}

func assertActivityExcludes(t *testing.T, obj any, excluded ...string) {
	t.Helper()
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range excluded {
		if strings.Contains(string(data), hidden) {
			t.Fatalf("activity disclosed %q: %s", hidden, data)
		}
	}
}
