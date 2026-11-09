package service

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestActivityQueriesRequireBoundedOwnerScopedCursors(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	cursor := func(owner, token, id string) string {
		data, err := json.Marshal(activityCursor{Owner: owner, Token: token, ID: id, At: at})
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(data)
	}
	for _, tc := range []struct {
		name  string
		q     ActivityQuery
		ok    bool
		limit int
	}{
		{name: "default", ok: true, limit: 51},
		{name: "minimum", q: ActivityQuery{Limit: 1}, ok: true, limit: 2},
		{name: "maximum", q: ActivityQuery{Limit: 100}, ok: true, limit: 101},
		{name: "negative", q: ActivityQuery{Limit: -1}},
		{name: "too many", q: ActivityQuery{Limit: 101}},
		{name: "broken cursor", q: ActivityQuery{Cursor: "broken"}},
		{name: "foreign cursor", q: ActivityQuery{Cursor: cursor("bob", "", "event")}},
		{name: "filter changed", q: ActivityQuery{TokenID: "one", Cursor: cursor("alice", "two", "event")}},
		{name: "missing event", q: ActivityQuery{Cursor: cursor("alice", "", "")}},
		{name: "own cursor", q: ActivityQuery{TokenID: "one", Cursor: cursor("alice", "one", "event")}, ok: true, limit: 51},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.q.storeFilter("alice")
			if (err == nil) != tc.ok {
				t.Fatalf("accepted=%v, error %v", tc.ok, err)
			}
			if tc.ok && f.Limit != tc.limit {
				t.Fatalf("fetch limit %d, want %d", f.Limit, tc.limit)
			}
		})
	}
	if (Viewer{}).CanReadAgentActivity() {
		t.Fatal("anonymous history is open")
	}
}
