package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// dbHTTP is the shared db as the bridge reaches it: the viewer's own calls to
// /api/artifacts/{id}/db, which the bridge forwards one for one.
type dbHTTP struct {
	h    *harness
	c    *http.Client
	base string
}

func (h *harness) dbAs(user, pw, id string) dbHTTP {
	return dbHTTP{h: h, c: h.browser(user, pw), base: "/api/artifacts/" + id + "/db"}
}

func (d dbHTTP) do(method, path, body string) (int, string) {
	d.h.t.Helper()
	res, out := d.h.do(d.c, method, d.base+path, body, bridgeHeaders(d.h))
	return res.StatusCode, out
}

// ok runs a call that must succeed and returns the document, or for a batch the list.
func (d dbHTTP) ok(method, path, body string) map[string]any {
	d.h.t.Helper()
	status, out := d.do(method, path, body)
	if status != http.StatusOK {
		d.h.t.Fatalf("%s %s: %d %s", method, path, status, out)
	}
	return decode[map[string]any](d.h.t, out)
}

func (d dbHTTP) list(path, body string) []map[string]any {
	d.h.t.Helper()
	method := "POST"
	if body == "" {
		method = "GET"
	}
	status, out := d.do(method, path, body)
	if status != http.StatusOK {
		d.h.t.Fatalf("%s %s: %d %s", method, path, status, out)
	}
	return decode[[]map[string]any](d.h.t, out)
}

func versionOf(t *testing.T, doc map[string]any) int {
	t.Helper()
	v, ok := doc["version"].(float64)
	if !ok {
		t.Fatalf("no version in %v", doc)
	}
	return int(v)
}

func ids(docs []map[string]any) []string {
	out := []string{}
	for _, d := range docs {
		out = append(out, d["id"].(string))
	}
	return out
}

func TestEveryDocumentCarriesAVersionThatEachWriteAdvances(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Tracker", "<p>x")
	db := h.dbAs("alice", "pw-a", a.ID)

	first := db.ok("PUT", "/t/1", `{"n":1}`)
	second := db.ok("PUT", "/t/1", `{"n":2}`)
	third := db.ok("PATCH", "/t/1", `{"m":3}`)
	if versionOf(t, first) != 1 || versionOf(t, second) != 2 || versionOf(t, third) != 3 {
		t.Errorf("versions %v %v %v, want 1 2 3", first["version"], second["version"], third["version"])
	}
	if got := db.ok("GET", "/t/1", ""); versionOf(t, got) != 3 || string(mustJSON(t, got["data"])) != `{"m":3,"n":2}` {
		t.Errorf("get = %v", got)
	}
	if got := db.list("/t", ""); len(got) != 1 || versionOf(t, got[0]) != 3 {
		t.Errorf("list = %v", got)
	}
}

func TestADocumentOlderThanVersionsStartsAtVersionOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Old", "<p>x")
	if err := h.st.PutDoc(t.Context(), a.ID, "c", "d", json.RawMessage(`{"v":1}`), "alice"); err != nil {
		t.Fatal(err)
	}
	if got := h.dbAs("alice", "pw-a", a.ID).ok("GET", "/c/d", ""); versionOf(t, got) != 1 {
		t.Errorf("version = %v", got["version"])
	}
}

func TestAStaleConditionalWriteIsRefusedWithTheCurrentDocumentAndChangesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Sign-up", "<p>x")
	alice, bob := h.dbAs("alice", "pw-a", a.ID), h.dbAs("bob", "pw-b", a.ID)
	alice.ok("PUT", "/slots/mon", `{"who":null}`)

	// Both read version 1; bob's claim lands first.
	bob.ok("PATCH", "/slots/mon?ifVersion=1", `{"who":"bob"}`)

	cases := []struct{ name, method, path, body string }{
		{"update", "PATCH", "/slots/mon?ifVersion=1", `{"who":"alice"}`},
		{"set", "PUT", "/slots/mon?ifVersion=1", `{"who":"alice"}`},
		{"delete", "DELETE", "/slots/mon?ifVersion=1", ``},
	}
	for _, c := range cases {
		status, body := alice.do(c.method, c.path, c.body)
		got := decode[struct {
			Error    string `json:"error"`
			Conflict struct {
				Index      int            `json:"index"`
				Collection string         `json:"collection"`
				ID         string         `json:"id"`
				Current    map[string]any `json:"current"`
			} `json:"conflict"`
		}](t, body)
		if status != http.StatusConflict || !strings.Contains(got.Error, "slots/mon was written against version 1 but now is at version 2") ||
			got.Conflict.ID != "mon" || got.Conflict.Collection != "slots" || versionOf(t, got.Conflict.Current) != 2 ||
			string(mustJSON(t, got.Conflict.Current["data"])) != `{"who":"bob"}` {
			t.Errorf("%s with a stale version: %d %s", c.name, status, body)
		}
	}
	if got := alice.ok("GET", "/slots/mon", ""); string(mustJSON(t, got["data"])) != `{"who":"bob"}` || versionOf(t, got) != 2 {
		t.Errorf("a refused write changed the document: %v", got)
	}

	// With the version it read back, alice's write goes through.
	if got := alice.ok("PATCH", "/slots/mon?ifVersion=2", `{"who":"alice"}`); versionOf(t, got) != 3 {
		t.Errorf("version after the retry: %v", got["version"])
	}
	if status, body := alice.do("DELETE", "/slots/mon?ifVersion=3", ""); status != http.StatusNoContent {
		t.Errorf("delete with the current version: %d %s", status, body)
	}
}

func TestIfVersionZeroCreatesOnlyWhereThereIsNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Claim", "<p>x")
	alice, bob := h.dbAs("alice", "pw-a", a.ID), h.dbAs("bob", "pw-b", a.ID)

	if got := alice.ok("PUT", "/names/ada?ifVersion=0", `{"by":"alice"}`); versionOf(t, got) != 1 {
		t.Errorf("first claim: %v", got)
	}
	status, body := bob.do("PUT", "/names/ada?ifVersion=0", `{"by":"bob"}`)
	if status != http.StatusConflict || !strings.Contains(body, "no existing document") || !strings.Contains(body, "is at version 1") {
		t.Errorf("second claim: %d %s", status, body)
	}
	// A version for a document that is not there is stale too, with nothing to show.
	status, body = bob.do("PUT", "/names/zed?ifVersion=4", `{}`)
	if status != http.StatusConflict || !strings.Contains(body, "does not exist") || !strings.Contains(body, `"current":null`) {
		t.Errorf("a version of nothing: %d %s", status, body)
	}
	if got := alice.ok("GET", "/names/ada", ""); string(mustJSON(t, got["data"])) != `{"by":"alice"}` {
		t.Errorf("document = %v", got)
	}
}

func TestUpdateNeedsAnObjectDocumentAndANameForIfVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Edge", "<p>x")
	db := h.dbAs("alice", "pw-a", a.ID)
	db.ok("PUT", "/c/list", `[1,2]`)
	db.ok("PUT", "/c/obj", `{"a":1}`)
	for _, c := range []struct {
		name, method, path, body, says string
		want                           int
	}{
		{"updating a document that is not an object", "PATCH", "/c/list", `{"a":1}`, "not a JSON object", http.StatusBadRequest},
		{"updating with a list", "PATCH", "/c/obj", `[1]`, "must be a JSON object", http.StatusBadRequest},
		{"updating a document that does not exist", "PATCH", "/c/none", `{"a":1}`, "not found", http.StatusNotFound},
		{"an ifVersion that is not a number", "PUT", "/c/obj?ifVersion=latest", `{}`, "ifVersion is a version number", http.StatusBadRequest},
		{"a negative ifVersion", "DELETE", "/c/obj?ifVersion=-1", ``, "ifVersion is a version number", http.StatusBadRequest},
	} {
		if status, body := db.do(c.method, c.path, c.body); status != c.want || !strings.Contains(body, c.says) {
			t.Errorf("%s: %d %s", c.name, status, body)
		}
	}
}

// batchFixture is a collection c holding a (version 1) and b (version 2).
func batchFixture(t *testing.T) dbHTTP {
	t.Helper()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Batch", "<p>x")
	db := h.dbAs("alice", "pw-a", a.ID)
	db.ok("PUT", "/c/a", `{"n":1}`)
	db.ok("PUT", "/c/b", `{"n":1}`)
	db.ok("PUT", "/c/b", `{"n":2}`)
	return db
}

func TestABatchWithAStaleOperationChangesNothingAndNamesIt(t *testing.T) {
	t.Parallel()
	db := batchFixture(t)

	// The third operation is stale: the first two must not have happened.
	status, body := db.do("POST", "/batch", `{"ops":[
		{"op":"set","collection":"c","id":"new","data":{"n":9}},
		{"op":"update","collection":"c","id":"a","fields":{"n":5},"ifVersion":1},
		{"op":"update","collection":"c","id":"b","fields":{"n":5},"ifVersion":1}]}`)
	got := decode[struct {
		Error    string `json:"error"`
		Conflict struct {
			Index   int            `json:"index"`
			Current map[string]any `json:"current"`
		} `json:"conflict"`
	}](t, body)
	if status != http.StatusConflict || got.Conflict.Index != 2 || versionOf(t, got.Conflict.Current) != 2 || !strings.Contains(got.Error, "operation 3: c/b") || !strings.Contains(got.Error, "nothing was changed") {
		t.Errorf("stale batch: %d %s", status, body)
	}
	if all := db.list("/c", ""); strings.Join(ids(all), ",") != "a,b" || versionOf(t, all[0]) != 1 || string(mustJSON(t, all[0]["data"])) != `{"n":1}` {
		t.Errorf("the refused batch changed the collection: %v", all)
	}
}

func TestEveryOtherRefusalOfABatchIsWholeToo(t *testing.T) {
	t.Parallel()
	db := batchFixture(t)
	for name, ops := range map[string]string{
		"an update of a document that is not there": `[{"op":"set","collection":"c","id":"x","data":1},{"op":"update","collection":"c","id":"ghost","fields":{"a":1}}]`,
		"a delete of a document that is not there":  `[{"op":"set","collection":"c","id":"x","data":1},{"op":"delete","collection":"c","id":"ghost"}]`,
		"an unknown operation":                      `[{"op":"set","collection":"c","id":"x","data":1},{"op":"upsert","collection":"c","id":"y","data":1}]`,
		"a missing id":                              `[{"op":"set","collection":"c","id":"x","data":1},{"op":"set","collection":"c","data":1}]`,
		"fields that are not an object":             `[{"op":"set","collection":"c","id":"x","data":1},{"op":"update","collection":"c","id":"a","fields":[1]}]`,
		"an update of a document that is a list":    `[{"op":"set","collection":"c","id":"l","data":[1]},{"op":"update","collection":"c","id":"l","fields":{"a":1}}]`,
		"an empty batch":                            `[]`,
	} {
		status, body := db.do("POST", "/batch", `{"ops":`+ops+`}`)
		if status != http.StatusBadRequest || !strings.Contains(body, "operation 2") && name != "an empty batch" && name != "an update of a document that is a list" {
			t.Errorf("%s: %d %s", name, status, body)
		}
		if all := db.list("/c", ""); strings.Join(ids(all), ",") != "a,b" {
			t.Errorf("%s changed the collection: %v", name, ids(all))
		}
	}
}

func TestAGoodBatchAppliesInOrderEachOperationSeeingTheOnesBefore(t *testing.T) {
	t.Parallel()
	db := batchFixture(t)
	out := db.ok("POST", "/batch", `{"ops":[
		{"op":"set","collection":"c","id":"new","data":{"n":9}},
		{"op":"update","collection":"c","id":"new","fields":{"m":1},"ifVersion":1},
		{"op":"update","collection":"c","id":"a","fields":{"n":null,"k":"v"},"ifVersion":1},
		{"op":"delete","collection":"c","id":"b","ifVersion":2},
		{"op":"set","collection":"other","id":"z","data":"x","ifVersion":0}]}`)
	results := out["results"].([]any)
	if len(results) != 5 || versionOf(t, results[1].(map[string]any)) != 2 || results[3].(map[string]any)["id"] != "b" || results[3].(map[string]any)["version"].(float64) != 0 {
		t.Fatalf("results = %v", out)
	}
	all := db.list("/c", "")
	if strings.Join(ids(all), ",") != "a,new" || string(mustJSON(t, all[0]["data"])) != `{"k":"v"}` || string(mustJSON(t, all[1]["data"])) != `{"m":1,"n":9}` {
		t.Errorf("after the batch: %v", all)
	}
	if got := db.ok("GET", "/other/z", ""); got["data"] != "x" {
		t.Errorf("a batch reaches other collections: %v", got)
	}
	if status, body := db.do("POST", "/batch", `{"ops":[`+strings.Repeat(`{"op":"set","collection":"c","id":"q","data":1},`, 100)+`{"op":"set","collection":"c","id":"q","data":1}]}`); status != http.StatusBadRequest || !strings.Contains(body, "1 to 100") {
		t.Errorf("a batch of 101: %d %s", status, body)
	}
}

func TestQueryFiltersACollectionAndRefusesAFilterItDoesNotUnderstand(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Tasks", "<p>x")
	db := h.dbAs("alice", "pw-a", a.ID)
	// Written in order: a query answers least recently updated first, then by id,
	// so the expected lists below hold whether or not the writes share a second.
	for _, d := range []struct{ id, body string }{
		{"2026-10-01", `{"status":"open","score":2,"owner":{"name":"ada"}}`},
		{"2026-10-02", `{"status":"done","score":5,"owner":{"name":"bob"}}`},
		{"2026-11-01", `{"status":"open","score":8,"owner":{"name":"ada"}}`},
	} {
		db.ok("PUT", "/tasks/"+d.id, d.body)
	}
	for _, c := range []struct{ filter, want string }{
		{`{}`, "2026-10-01,2026-10-02,2026-11-01"},
		{`{"status":"open"}`, "2026-10-01,2026-11-01"},
		{`{"status":"open","owner.name":"ada","score":{"gte":3}}`, "2026-11-01"},
		{`{"score":{"gt":2,"lt":8}}`, "2026-10-02"},
		{`{"id":{"prefix":"2026-10-"}}`, "2026-10-01,2026-10-02"},
		{`{"owner.name":{"in":["bob","cy"]}}`, "2026-10-02"},
		{`{"status":"archived"}`, ""},
	} {
		got := db.list("/tasks/query", `{"filter":`+c.filter+`}`)
		if strings.Join(ids(got), ",") != c.want {
			t.Errorf("filter %s = %v, want %s", c.filter, ids(got), c.want)
		}
	}
	if got := db.list("/tasks/query", `{"filter":{"status":"open"},"limit":1}`); len(got) != 1 {
		t.Errorf("limit 1 gave %d", len(got))
	}
	if got := db.list("/tasks/query", `{}`); len(got) != 3 || versionOf(t, got[0]) != 1 {
		t.Errorf("no filter = %v", got)
	}
	status, body := db.do("POST", "/tasks/query", `{"filter":{"score":{"between":[1,2]}}}`)
	if status != http.StatusBadRequest || !strings.Contains(body, `unknown operator \"between\"`) {
		t.Errorf("a filter it does not understand: %d %s", status, body)
	}
	if status, _ := db.do("POST", "/tasks/query", `{"limit":-1}`); status != http.StatusBadRequest {
		t.Errorf("a negative limit: %d", status)
	}
}

func TestAccessToTheNewDbOperationsFollowsTheExistingRules(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	pub := h.publishAs(h.token("alice"), "Poll", "<p>poll", store.VisibilityPublic)
	priv := h.publish(h.token("alice"), "Secret", "<p>secret")
	for _, id := range []string{pub.ID, priv.ID} {
		if err := h.st.PutDoc(t.Context(), id, "votes", "1", json.RawMessage(`{"v":1}`), "alice"); err != nil {
			t.Fatal(err)
		}
	}
	query := "/api/artifacts/" + pub.ID + "/db/votes/query"

	// Anonymous readers of a public artifact query, without learning who wrote.
	if res, body := h.do(nil, "POST", query, `{"filter":{"v":1}}`, bridgeHeaders(h)); res.StatusCode != http.StatusOK || !strings.Contains(body, `"v":1`) || strings.Contains(body, "alice") {
		t.Errorf("anonymous query: %d %s", res.StatusCode, body)
	}
	// ...and cannot write any other way.
	for _, c := range []struct{ method, path, body string }{
		{"PATCH", "/api/artifacts/" + pub.ID + "/db/votes/1", `{"v":2}`},
		{"POST", "/api/artifacts/" + pub.ID + "/db/batch", `{"ops":[{"op":"delete","collection":"votes","id":"1"}]}`},
		{"PUT", "/api/artifacts/" + pub.ID + "/db/votes/1?ifVersion=1", `{"v":2}`},
	} {
		if res, _ := h.do(nil, c.method, c.path, c.body, bridgeHeaders(h)); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s: %d", c.method, c.path, res.StatusCode)
		}
	}
	// Someone who cannot see the artifact is told what they would be told for an id that is not there.
	bob := h.browser("bob", "pw-b")
	for _, c := range []struct{ method, suffix, body string }{
		{"POST", "/db/votes/query", `{}`},
		{"PATCH", "/db/votes/1", `{"v":2}`},
		{"POST", "/db/batch", `{"ops":[{"op":"delete","collection":"votes","id":"1"}]}`},
	} {
		seen, seenBody := h.do(bob, c.method, "/api/artifacts/"+priv.ID+c.suffix, c.body, bridgeHeaders(h))
		none, noneBody := h.do(bob, c.method, "/api/artifacts/nosuchid"+c.suffix, c.body, bridgeHeaders(h))
		if seen.StatusCode != http.StatusNotFound || none.StatusCode != http.StatusNotFound || seenBody != noneBody {
			t.Errorf("%s %s on a private artifact: %d %q, on a missing one: %d %q", c.method, c.suffix, seen.StatusCode, seenBody, none.StatusCode, noneBody)
		}
	}
	// A signed-in person who can see it writes, as with db_set, and the writer is recorded.
	if res, body := h.do(bob, "PATCH", "/api/artifacts/"+pub.ID+"/db/votes/1", `{"v":2}`, bridgeHeaders(h)); res.StatusCode != http.StatusOK || !strings.Contains(body, `"updatedBy":"bob"`) {
		t.Errorf("signed-in update: %d %s", res.StatusCode, body)
	}
}

// bothSides is a page (the HTTP calls the bridge forwards) and an agent (MCP)
// working on one artifact.
type bothSides struct {
	t     *testing.T
	id    string
	page  dbHTTP
	agent *agent
}

func newBothSides(t *testing.T) bothSides {
	t.Helper()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Both", "<p>x")
	return bothSides{t: t, id: a.ID, page: h.dbAs("alice", "pw-a", a.ID), agent: h.agent(h.token("bob"))}
}

func (b bothSides) tool(name string, args map[string]any) map[string]any {
	b.t.Helper()
	args["id"] = b.id
	return b.agent.ok(name, args)
}

func (b bothSides) refusal(name string, args map[string]any) string {
	b.t.Helper()
	args["id"] = b.id
	return b.agent.refused(name, args)
}

// The bridge only forwards the page's call to the HTTP API; an agent reaches
// the same service through MCP. Whatever one writes the other reads the same,
// and both are refused the same way.
func TestAWriteByTheAgentIsReadByThePageAndAnUpdateMergesTheSameFromBothSides(t *testing.T) {
	t.Parallel()
	b := newBothSides(t)
	set := b.tool("db_set", map[string]any{"collection": "t", "doc_id": "x", "data": map[string]any{"n": 1, "tag": "a"}})
	if versionOf(t, set) != 1 {
		t.Fatalf("db_set = %v", set)
	}
	if got := b.page.ok("GET", "/t/x", ""); string(mustJSON(t, got)) != string(mustJSON(t, set)) {
		t.Errorf("page read %s, agent wrote %s", mustJSON(t, got), mustJSON(t, set))
	}
	viaAgent := b.tool("db_update", map[string]any{"collection": "t", "doc_id": "x", "fields": map[string]any{"n": 2}, "ifVersion": 1})
	viaPage := b.page.ok("PATCH", "/t/x?ifVersion=2", `{"tag":null,"k":true}`)
	if versionOf(t, viaAgent) != 2 || versionOf(t, viaPage) != 3 || string(mustJSON(t, viaPage["data"])) != `{"k":true,"n":2}` {
		t.Errorf("update: agent %v, page %v", viaAgent, viaPage)
	}
}

func TestAQueryAnswersTheSameListToThePageAndTheAgent(t *testing.T) {
	t.Parallel()
	b := newBothSides(t)
	b.page.ok("PUT", "/t/x", `{"n":2}`)
	b.page.ok("PUT", "/t/y", `{"n":7}`)
	b.tool("db_set", map[string]any{"collection": "t", "doc_id": "z", "data": map[string]any{"n": 9}})
	for _, filter := range []string{`{"n":{"gte":2}}`, `{"id":{"prefix":"y"}}`, `{}`, `{"n":{"in":[7,9]}}`} {
		var f map[string]any
		if err := json.Unmarshal([]byte(filter), &f); err != nil {
			t.Fatal(err)
		}
		tool := b.tool("db_query", map[string]any{"collection": "t", "filter": f})
		web := b.page.list("/t/query", `{"filter":`+filter+`}`)
		if string(mustJSON(t, tool["documents"])) != string(mustJSON(t, web)) {
			t.Errorf("filter %s: agent %s, page %s", filter, mustJSON(t, tool["documents"]), mustJSON(t, web))
		}
	}
	bad := b.refusal("db_query", map[string]any{"collection": "t", "filter": map[string]any{"n": map[string]any{"near": 1}}})
	status, body := b.page.do("POST", "/t/query", `{"filter":{"n":{"near":1}}}`)
	if status != http.StatusBadRequest || bad != decode[map[string]string](t, body)["error"] {
		t.Errorf("bad filter: agent %q, page %d %s", bad, status, body)
	}
}

func TestAStaleWriteIsRefusedTheSameWayOnBothSidesAndSaysWhatTheDocumentIsNow(t *testing.T) {
	t.Parallel()
	b := newBothSides(t)
	b.page.ok("PUT", "/t/x", `{"n":1}`)
	b.page.ok("PUT", "/t/x", `{"n":2}`)
	b.page.ok("PUT", "/t/x", `{"n":3}`)

	stale := b.refusal("db_update", map[string]any{"collection": "t", "doc_id": "x", "fields": map[string]any{"n": 0}, "ifVersion": 1})
	status, body := b.page.do("PATCH", "/t/x?ifVersion=1", `{"n":0}`)
	pageSays := decode[map[string]any](t, body)["error"].(string)
	if status != http.StatusConflict || !strings.Contains(stale, pageSays) || !strings.Contains(stale, `"version":3`) || !strings.Contains(stale, "ifVersion") {
		t.Errorf("stale: agent %q, page %d %s", stale, status, body)
	}
	if refusal := b.refusal("db_delete", map[string]any{"collection": "t", "doc_id": "x", "ifVersion": 1}); !strings.Contains(refusal, "nothing was changed") {
		t.Errorf("stale delete by the agent: %q", refusal)
	}
	if status, _ := b.page.do("DELETE", "/t/x?ifVersion=1", ""); status != http.StatusConflict {
		t.Errorf("stale delete by the page: %d", status)
	}
	if got := b.page.ok("GET", "/t/x", ""); versionOf(t, got) != 3 {
		t.Errorf("a refused write changed the document: %v", got)
	}
}

func TestABatchIsRefusedWholeByBothSidesWithTheSameMessageAndAppliedTheSame(t *testing.T) {
	t.Parallel()
	b := newBothSides(t)
	b.page.ok("PUT", "/t/x", `{"n":1}`)
	b.page.ok("PUT", "/t/x", `{"n":2}`)
	ops := []map[string]any{
		{"op": "set", "collection": "b", "id": "one", "data": map[string]any{"n": 1}},
		{"op": "update", "collection": "t", "id": "x", "fields": map[string]any{"n": 1}, "ifVersion": 1},
	}
	agentRefusal := b.refusal("db_batch", map[string]any{"ops": ops})
	status, body := b.page.do("POST", "/batch", `{"ops":`+string(mustJSON(t, ops))+`}`)
	pageMsg := decode[map[string]any](t, body)["error"].(string)
	if status != http.StatusConflict || !strings.Contains(agentRefusal, pageMsg) || !strings.Contains(pageMsg, "operation 2: t/x") {
		t.Errorf("stale batch: agent %q, page %d %s", agentRefusal, status, body)
	}
	if got := b.page.list("/b", ""); len(got) != 0 {
		t.Errorf("a refused batch wrote %v", got)
	}

	ops[1]["ifVersion"] = 2
	byAgent := b.tool("db_batch", map[string]any{"ops": ops})["results"].([]any)
	ops[0]["id"], ops[1]["ifVersion"] = "two", 3
	byPage := b.page.ok("POST", "/batch", `{"ops":`+string(mustJSON(t, ops))+`}`)["results"].([]any)
	if len(byAgent) != 2 || len(byPage) != 2 || versionOf(t, byAgent[1].(map[string]any)) != 3 || versionOf(t, byPage[1].(map[string]any)) != 4 {
		t.Errorf("batch results: agent %v, page %v", byAgent, byPage)
	}
}
