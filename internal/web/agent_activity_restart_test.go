package web

import (
	"maps"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func activityRestartHarness(t *testing.T) (*harness, func() *harness) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "activity.db")
	open := func() *harness {
		st, err := store.Open(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		return newHarnessOn(t, st)
	}
	h := open()
	return h, func() *harness {
		h.srv.Close()
		if err := h.st.Close(); err != nil {
			t.Fatal(err)
		}
		h = open()
		return h
	}
}

func positiveActivityRows(t *testing.T, st *store.Store, token string, started, finished time.Time, want map[string]int) []store.ActivityEntry {
	t.Helper()
	p, err := st.AuthenticateToken(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := st.Activity(t.Context(), "alice", store.ActivityFilter{Limit: 100}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, row := range rows {
		if p.Owner != "alice" || row.TokenID != p.TokenID || row.OccurredAt.Before(started) || row.OccurredAt.After(finished) {
			t.Fatalf("wrong owner/token/occurrence time: %+v", row)
		}
		counts[row.Operation]++
	}
	if !maps.Equal(counts, want) {
		t.Fatalf("operations: %v, want %v", counts, want)
	}
	foreign, err := st.Activity(t.Context(), "bob", store.ActivityFilter{Limit: 100}, time.Now())
	if err != nil || len(foreign) != 0 {
		t.Fatalf("owner-scoped history: %+v %v", foreign, err)
	}
	return rows
}

func activityRequestTime(t *testing.T, h *harness) func(string, string, int) {
	t.Helper()
	started := time.Now().Truncate(time.Millisecond)
	return func(operation, artifactID string, version int) {
		t.Helper()
		finished := time.Now()
		rows, err := h.st.Activity(t.Context(), "alice", store.ActivityFilter{Limit: 100}, finished)
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, row := range rows {
			if row.Operation == operation && row.ArtifactID == artifactID && row.Version == version {
				found++
				if row.OccurredAt.Before(started) || row.OccurredAt.After(finished) {
					t.Fatalf("occurrence outside request: %+v, want %v..%v", row, started, finished)
				}
			}
		}
		if found != 1 {
			t.Fatalf("want one %s event for %s version %d, got %d", operation, artifactID, version, found)
		}
	}
}

func TestSuccessfulConnectionsAndWebWritesKeepTheirMetadataAfterStoreAndServerRestart(t *testing.T) {
	t.Parallel()
	h, restart := activityRestartHarness(t)
	token := h.token("alice")
	started := time.Now().Truncate(time.Millisecond)
	checkTime := activityRequestTime(t, h)
	initialized := rawActivityMCP(t, h, token, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"positive-restart","version":"1"}}}`)
	initResult := decode[struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}](t, initialized)
	if initResult.Result.ProtocolVersion != "2025-06-18" {
		t.Fatalf("initialize did not succeed: %s", initialized)
	}
	checkTime("connect", "", 0)
	checkTime = activityRequestTime(t, h)
	a := h.publish(token, "Created", page)
	checkTime("publish", a.ID, 1)
	checkTime = activityRequestTime(t, h)
	if res, body := api(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Updated","html":"<!doctype html><p>updated</p>"}`, token); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, body)
	}
	checkTime("publish", a.ID, 2)
	checkTime = activityRequestTime(t, h)
	out := rawActivityMCP(t, h, token, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"publish","arguments":{"title":"MCP published","content":"`+page+`"}}}`)
	result := decode[struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				ID string `json:"id"`
			} `json:"structuredContent"`
		} `json:"result"`
	}](t, out)
	if result.Result.IsError || result.Result.StructuredContent.ID == "" {
		t.Fatalf("publish did not succeed: %s", out)
	}
	checkTime("publish", result.Result.StructuredContent.ID, 1)
	finished := time.Now()
	before := positiveActivityRows(t, h.st, token, started, finished, map[string]int{"connect": 1, "publish": 3})
	h = restart()
	after := positiveActivityRows(t, h.st, token, started, finished, map[string]int{"connect": 1, "publish": 3})
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("metadata changed across restart: %+v -> %+v", before, after)
	}
	if got := activityPage(t, h, token, ""); len(got.Items) != len(after) {
		t.Fatalf("restarted service history: %+v", got)
	}
}
