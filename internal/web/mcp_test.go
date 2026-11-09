package web

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// bearerTransport sends a personal token with every request, as an agent's
// MCP client does.
type bearerTransport struct {
	token string
	next  http.RoundTripper
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

// agent is a real MCP client connected to the server with someone's token.
type agent struct {
	t       *testing.T
	session *mcp.ClientSession
}

func (h *harness) agent(token string) *agent {
	h.t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "1"}, nil)
	session, err := client.Connect(h.t.Context(), &mcp.StreamableClientTransport{
		Endpoint:   h.srv.URL + "/api/mcp",
		HTTPClient: &http.Client{Transport: bearerTransport{token: token, next: h.srv.Client().Transport}},
	}, nil)
	if err != nil {
		h.t.Fatalf("connect: %v", err)
	}
	h.t.Cleanup(func() { _ = session.Close() })
	return &agent{t: h.t, session: session}
}

// call runs a tool and returns what the agent sees: the structured result, or
// the error text when the tool refused.
func (a *agent) call(tool string, args map[string]any) (map[string]any, string) {
	a.t.Helper()
	res, err := a.session.CallTool(a.t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		a.t.Fatalf("%s: protocol error: %v", tool, err)
	}
	if res.IsError {
		var text strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text.WriteString(tc.Text)
			}
		}
		return nil, text.String()
	}
	out, ok := res.StructuredContent.(map[string]any)
	if !ok {
		a.t.Fatalf("%s: no structured result: %#v", tool, res.StructuredContent)
	}
	return out, ""
}

// ok is call for a tool that must succeed.
func (a *agent) ok(tool string, args map[string]any) map[string]any {
	a.t.Helper()
	out, refusal := a.call(tool, args)
	if refusal != "" {
		a.t.Fatalf("%s refused: %s", tool, refusal)
	}
	return out
}

// refused is call for a tool that must refuse; it returns the refusal text.
func (a *agent) refused(tool string, args map[string]any) string {
	a.t.Helper()
	out, refusal := a.call(tool, args)
	if refusal == "" {
		a.t.Fatalf("%s succeeded with %v, want a refusal", tool, out)
	}
	return refusal
}

const page = "<!doctype html><html><head><title>t</title></head><body>hello</body></html>"

func TestAnAgentLearnsWhoItIsAndHowToWriteAPage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.agent(h.token("alice"))

	if me := alice.ok("whoami", nil); me["username"] != "alice" || me["publicLinks"] != false {
		t.Errorf("whoami = %v", me)
	}

	kinds := alice.ok("kinds", nil)
	guides := map[string]string{}
	for _, k := range kinds["kinds"].([]any) {
		m := k.(map[string]any)
		guides[m["name"].(string)] = m["guide"].(string)
	}
	if kinds["default"] != "html" || len(guides) != 6 || !strings.Contains(guides["board"], "containerId") || !strings.Contains(guides["html"], "window.pictor") || !strings.Contains(guides["doc"], "Markdown") || !strings.Contains(guides["react"], "export default") || !strings.Contains(guides["slides"], `"slides"`) {
		t.Errorf("kinds = %v", kinds)
	}
}

func TestAnAgentPublishesUpdatesSharesAndListsThroughTheMCPProtocol(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.agent(h.token("alice")), h.agent(h.token("bob"))

	made := alice.ok("publish", map[string]any{"title": "Report", "content": page})
	id := made["id"].(string)
	if made["created"] != true || made["visibility"] != "private" || made["author"] != "alice" ||
		!strings.HasSuffix(made["url"].(string), "/a/"+id) || !strings.Contains(made["next"].(string), "set_visibility") {
		t.Fatalf("publish = %v", made)
	}

	if got := bob.refused("read", map[string]any{"id": id}); !strings.Contains(got, "no artifact") {
		t.Errorf("bob read a private artifact: %s", got)
	}
	updateAndShare(t, alice, bob, made)
}

func updateAndShare(t *testing.T, alice, bob *agent, made map[string]any) {
	t.Helper()
	id := made["id"].(string)
	updated := alice.ok("publish", map[string]any{"id": id, "title": "Report v2", "content": page + "<!-- 2 -->"})
	if updated["created"] != false || updated["id"] != id || updated["url"] != made["url"] || updated["visibility"] != "private" || updated["title"] != "Report v2" {
		t.Errorf("update = %v", updated)
	}

	if shared := alice.ok("set_visibility", map[string]any{"id": id, "visibility": "organisation"}); shared["visibility"] != "organisation" {
		t.Errorf("set_visibility = %v", shared)
	}

	read := bob.ok("read", map[string]any{"id": id})
	if read["content"] != page+"<!-- 2 -->" || read["title"] != "Report v2" || read["kind"] != "html" {
		t.Errorf("read = %v", read)
	}
}

func TestAnAgentListsAndSearchesWhatItMaySee(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.agent(h.token("alice")), h.agent(h.token("bob"))
	alice.ok("publish", map[string]any{"title": "Report v2", "content": page, "visibility": "organisation"})
	alice.ok("publish", map[string]any{"title": "Private", "content": page})

	for scope, want := range map[string]int{"mine": 0, "shared": 1, "all": 1} {
		got := bob.ok("list", map[string]any{"scope": scope})
		if int(got["total"].(float64)) != want || len(got["artifacts"].([]any)) != want {
			t.Errorf("bob list %s = %v, want %d", scope, got, want)
		}
	}
	if got := bob.ok("list", map[string]any{"query": "REPORT V2"}); got["total"].(float64) != 1 {
		t.Errorf("search by title: %v", got)
	}
	if got := bob.ok("list", map[string]any{"query": "nothing like it"}); got["total"].(float64) != 0 {
		t.Errorf("search found %v", got)
	}
}

func TestAnAgentReadsAndWritesTheSharedDataThroughTheMCPProtocol(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.agent(h.token("alice")), h.agent(h.token("bob"))
	id := alice.ok("publish", map[string]any{"title": "Poll", "content": page, "visibility": "organisation"})["id"].(string)

	bob.ok("db_set", map[string]any{"id": id, "collection": "votes", "doc_id": "bob", "data": map[string]any{"choice": "A"}})
	got := alice.ok("db_get", map[string]any{"id": id, "collection": "votes", "doc_id": "bob"})
	if got["updatedBy"] != "bob" || string(mustJSON(t, got["data"])) != `{"choice":"A"}` {
		t.Errorf("db_get = %v", got)
	}
	docs := alice.ok("db_list", map[string]any{"id": id, "collection": "votes"})["documents"].([]any)
	if len(docs) != 1 || docs[0].(map[string]any)["id"] != "bob" {
		t.Errorf("db_list = %v", docs)
	}
	if out := bob.ok("db_delete", map[string]any{"id": id, "collection": "votes", "doc_id": "bob"}); out["deleted"] != true {
		t.Errorf("db_delete = %v", out)
	}
	if docs := alice.ok("db_list", map[string]any{"id": id, "collection": "votes"})["documents"].([]any); len(docs) != 0 {
		t.Errorf("document survived db_delete: %v", docs)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTheEndpointNeedsAPersonalTokenAndIgnoresTheBrowserSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"x","version":"1"}}}`
	headers := func(extra map[string]string) map[string]string {
		m := map[string]string{"Content-Type": "application/json", "Accept": "application/json, text/event-stream"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	for name, tc := range map[string]struct {
		client *http.Client
		header map[string]string
	}{
		"no credentials":     {nil, headers(nil)},
		"a wrong token":      {nil, headers(map[string]string{"Authorization": "Bearer pct_nope"})},
		"a signed-in cookie": {h.browser("alice", "pw-a"), headers(map[string]string{"X-Pictor": "1", "Origin": h.srv.URL})},
	} {
		res, body := h.do(tc.client, "POST", "/api/mcp", initialize, tc.header)
		if res.StatusCode != http.StatusUnauthorized || !strings.Contains(res.Header.Get("WWW-Authenticate"), "Bearer") {
			t.Errorf("%s: %d %s, want 401 with a Bearer challenge", name, res.StatusCode, body)
		}
	}
	res, body := h.do(nil, "POST", "/api/mcp", initialize, headers(map[string]string{"Authorization": "Bearer " + h.token("alice")}))
	if res.StatusCode != http.StatusOK || !strings.Contains(body, `"serverInfo"`) {
		t.Errorf("a valid token: %d %s", res.StatusCode, body)
	}
}

func TestRefusalsTellTheAgentWhatWasWrongAndWhatToSendInstead(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.agent(h.token("alice")), h.agent(h.token("bob"))
	id := alice.ok("publish", map[string]any{"title": "Mine", "content": page, "visibility": "organisation"})["id"].(string)
	private := alice.ok("publish", map[string]any{"title": "Secret", "content": page})["id"].(string)

	for _, tc := range []struct {
		name  string
		who   *agent
		tool  string
		args  map[string]any
		wants []string
	}{
		{"not visible", bob, "read", map[string]any{"id": private}, []string{"no artifact", "list"}},
		{"not there", bob, "read", map[string]any{"id": "nonexistent"}, []string{"no artifact", "list"}},
		{"not the author, update", bob, "publish", map[string]any{"id": id, "title": "x", "content": page}, []string{"only the author", "publish a new artifact"}},
		{"not the author, visibility", bob, "set_visibility", map[string]any{"id": id, "visibility": "private"}, []string{"only the author"}},
		{"not the author, delete", bob, "delete", map[string]any{"id": id}, []string{"only the author"}},
		{"unknown kind", alice, "publish", map[string]any{"title": "x", "content": page, "kind": "video"}, []string{`"video"`, "html"}},
		{"kind cannot change", alice, "publish", map[string]any{"id": id, "title": "x", "content": page, "kind": "video"}, []string{"video"}},
		{"missing title", alice, "publish", map[string]any{"title": "  ", "content": page}, []string{"title is required"}},
		{"missing content", alice, "publish", map[string]any{"title": "x", "content": ""}, []string{"needs content"}},
		{"unknown visibility", alice, "set_visibility", map[string]any{"id": id, "visibility": "specific"}, []string{`"private"`, `"organisation"`, `"public"`}},
		{"public links off", alice, "set_visibility", map[string]any{"id": id, "visibility": "public"}, []string{"turned off", "organisation"}},
		{"unknown list scope", alice, "list", map[string]any{"scope": "everything"}, []string{"scope must be"}},
		{"list limit", alice, "list", map[string]any{"limit": 5000}, []string{"limit"}},
		{"no document", bob, "db_get", map[string]any{"id": id, "collection": "c", "doc_id": "d"}, []string{"no such document", "db_list"}},
		{"delete no document", bob, "db_delete", map[string]any{"id": id, "collection": "c", "doc_id": "d"}, []string{"no such document"}},
		{"db on an invisible artifact", bob, "db_list", map[string]any{"id": private, "collection": "c"}, []string{"no artifact"}},
		{"db write to an invisible artifact", bob, "db_set", map[string]any{"id": private, "collection": "c", "doc_id": "d", "data": 1}, []string{"no artifact"}},
		{"db_set without data", bob, "db_set", map[string]any{"id": id, "collection": "c", "doc_id": "d", "data": nil}, []string{"data is required"}},
	} {
		// Each refusal changes nothing, so the cases share one server and run side by side.
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			who := *tc.who
			who.t = t // a failure is reported on this subtest's goroutine
			got := who.refused(tc.tool, tc.args)
			for _, want := range tc.wants {
				if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) {
					t.Errorf("refusal %q does not mention %q", got, want)
				}
			}
		})
	}

	t.Run("hidden reads as absent", func(t *testing.T) {
		t.Parallel()
		b := *bob
		b.t = t
		if hidden, absent := b.refused("read", map[string]any{"id": private}), b.refused("read", map[string]any{"id": "nonexistent"}); hidden != absent {
			t.Errorf("a private artifact answers differently from a missing one:\n%s\n%s", hidden, absent)
		}
	})
}

func TestPublicIsAvailableWhenTheDeploymentAllowsIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *Config) { c.PublicLinks = true })
	alice := h.agent(h.token("alice"))
	if alice.ok("whoami", nil)["publicLinks"] != true {
		t.Error("whoami does not say public links are on")
	}
	id := alice.ok("publish", map[string]any{"title": "Open", "content": page})["id"].(string)
	if got := alice.ok("set_visibility", map[string]any{"id": id, "visibility": "public"}); got["visibility"] != "public" {
		t.Errorf("set_visibility = %v", got)
	}
}

func TestAToolAndItsHTTPCounterpartGiveTheSameResult(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withUsageReader)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	alice, bob := h.agent(aliceTok), h.agent(bobTok)
	bearer := func(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }

	viaHTTP := h.publishShared(aliceTok, "Via HTTP", page)
	viaMCP := alice.ok("publish", map[string]any{"title": "Via MCP", "content": page, "visibility": "organisation"})
	h.publishAs(aliceTok, "Hidden", page, store.VisibilityPrivate)
	id := viaMCP["id"].(string)

	// The same artifact, read both ways, by the same viewer.
	res, body := h.do(nil, "GET", "/api/artifacts/"+id, "", bearer(bobTok))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET: %d %s", res.StatusCode, body)
	}
	read := bob.ok("read", map[string]any{"id": id})
	delete(read, "content")
	delete(read, "viewers") // who is looking now is not part of the artifact the two share
	got := decode[map[string]any](t, body)
	// canChange is for the browser's controls; an agent learns the same from the refusal it gets.
	delete(got, "canChange")
	delete(got, "canEdit")
	delete(got, "canComment")
	if string(mustJSON(t, read)) != string(mustJSON(t, got)) {
		t.Errorf("read and GET differ:\n%s\n%s", mustJSON(t, read), body)
	}

	// The same listing: what bob may see, in the same order.
	_, body = h.do(nil, "GET", "/api/artifacts", "", bearer(bobTok))
	var httpIDs, toolIDs []string
	for _, a := range decode[[]artifactView](t, body) {
		httpIDs = append(httpIDs, a.ID)
	}
	for _, a := range bob.ok("list", nil)["artifacts"].([]any) {
		toolIDs = append(toolIDs, a.(map[string]any)["id"].(string))
	}
	if !slices.Equal(httpIDs, toolIDs) || len(toolIDs) != 2 || !slices.Contains(toolIDs, viaHTTP.ID) {
		t.Errorf("list: tool %v, http %v", toolIDs, httpIDs)
	}

	// The same refusals, for what bob may not change.
	res, _ = h.do(nil, "PUT", "/api/artifacts/"+id, `{"title":"x","content":"<p>x"}`, bearer(bobTok))
	refusal := bob.refused("publish", map[string]any{"id": id, "title": "x", "content": "<p>x"})
	if res.StatusCode != http.StatusForbidden || !strings.Contains(refusal, "can edit this artifact") {
		t.Errorf("update by a non-author: http %d, tool %q", res.StatusCode, refusal)
	}

	// Shared data written one way is read the other, with the same writer.
	bob.ok("db_set", map[string]any{"id": id, "collection": "c", "doc_id": "d", "data": map[string]any{"n": 1}})
	_, body = h.do(nil, "GET", "/api/artifacts/"+id+"/db/c/d", "", bearer(aliceTok))
	viaHTTPDoc := decode[map[string]any](t, body)
	viaTool := alice.ok("db_get", map[string]any{"id": id, "collection": "c", "doc_id": "d"})
	if string(mustJSON(t, viaHTTPDoc)) != string(mustJSON(t, viaTool)) {
		t.Errorf("db_get and GET differ:\n%s\n%s", mustJSON(t, viaTool), mustJSON(t, viaHTTPDoc))
	}

	sameUsageBothWays(t, h, alice, bob, aliceTok, bobTok)

	// Something published over HTTP is the same shape when read by a tool.
	if got := bob.ok("read", map[string]any{"id": viaHTTP.ID}); got["author"] != viaHTTP.Author || got["visibility"] != string(viaHTTP.Visibility) || got["url"] != viaHTTP.URL {
		t.Errorf("read of an HTTP-published artifact = %v, want %+v", got, viaHTTP)
	}
}

// sameUsageBothWays checks that the usage summary reads the same for a reader
// over HTTP and MCP, and that anyone else gets the same refusal from both.
func sameUsageBothWays(t *testing.T, h *harness, reader, other *agent, readerTok, otherTok string) {
	t.Helper()
	_, body := h.do(nil, "GET", "/api/usage?by=department,kind&period=day", "", map[string]string{"Authorization": "Bearer " + readerTok})
	usage := reader.ok("usage_summary", map[string]any{"by": []string{"department", "kind"}, "period": "day"})
	if got := decode[map[string]any](t, body); string(mustJSON(t, got)) != string(mustJSON(t, usage)) || len(usage["rows"].([]any)) == 0 {
		t.Errorf("usage_summary and GET /api/usage differ:\n%s\n%s", mustJSON(t, usage), body)
	}
	res, body := h.do(nil, "GET", "/api/usage", "", map[string]string{"Authorization": "Bearer " + otherTok})
	refusal := other.refused("usage_summary", nil)
	if res.StatusCode != http.StatusForbidden || !strings.Contains(refusal, decode[map[string]string](t, body)["error"]) {
		t.Errorf("usage by a non-reader: http %d %s, tool %q", res.StatusCode, body, refusal)
	}
}

func TestDeleteWithoutAValidConfirmTokenDeletesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.agent(h.token("alice")), h.agent(h.token("bob"))
	victim := alice.ok("publish", map[string]any{"title": "Keep me", "content": page, "visibility": "organisation"})["id"].(string)
	other := alice.ok("publish", map[string]any{"title": "Other", "content": page})["id"].(string)
	alice.ok("db_set", map[string]any{"id": victim, "collection": "c", "doc_id": "d", "data": 1})
	exists := func(id string) bool {
		_, refusal := alice.call("read", map[string]any{"id": id})
		return refusal == ""
	}

	plan := alice.ok("delete", map[string]any{"id": victim})
	token, _ := plan["confirm"].(string)
	p := plan["plan"].(map[string]any)
	if plan["deleted"] != false || token == "" || p["title"] != "Keep me" || p["sharedDocuments"] != float64(1) || !exists(victim) {
		t.Fatalf("step one must only plan: %v", plan)
	}

	for name, args := range map[string]map[string]any{
		"a made-up token":          {"id": victim, "confirm": "cf_madeup"},
		"a token for another page": {"id": other, "confirm": token},
	} {
		if refusal := alice.refused("delete", args); !strings.Contains(refusal, "Nothing was deleted") {
			t.Errorf("%s: %q", name, refusal)
		}
		if !exists(victim) || !exists(other) {
			t.Fatalf("%s deleted something", name)
		}
	}
	confirmAsTheWrongPersonThenTheRightOne(t, alice, bob, victim, other, token, exists)
}

func confirmAsTheWrongPersonThenTheRightOne(t *testing.T, alice, bob *agent, victim, other, token string, exists func(string) bool) {
	t.Helper()
	if refusal := bob.refused("delete", map[string]any{"id": victim, "confirm": token}); !strings.Contains(refusal, "only the author") {
		t.Errorf("someone else with the token: %q", refusal)
	}
	if !exists(victim) {
		t.Fatal("someone else's call deleted the page")
	}

	done := alice.ok("delete", map[string]any{"id": victim, "confirm": token})
	if done["deleted"] != true || exists(victim) {
		t.Fatalf("the right token must delete: %v", done)
	}
	if !exists(other) {
		t.Fatal("deleted the wrong page")
	}
	if refusal := alice.refused("delete", map[string]any{"id": victim, "confirm": token}); !strings.Contains(refusal, "no artifact") {
		t.Errorf("a spent token on a deleted page: %q", refusal)
	}
}

var registeredTool = regexp.MustCompile(`mcp\.AddTool\(s, &mcp\.Tool\{Name: "([a-z_]+)"`)

// registeredToolNames is every tool the mcptools package registers, read from
// its mcp.AddTool calls, so a new tool needs no edit here: the test only asks
// that what the server lists is what the code registers.
func registeredToolNames(t *testing.T) []string {
	t.Helper()
	pkg := os.DirFS("../mcptools")
	files, err := fs.Glob(pkg, "*.go")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := fs.ReadFile(pkg, file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range registeredTool.FindAllStringSubmatch(string(src), -1) {
			names = append(names, m[1])
		}
	}
	if len(names) < 10 {
		t.Fatalf("found only %d registered tools: the scan is broken", len(names))
	}
	slices.Sort(names)
	return names
}

// The tool descriptions are the only manual an agent gets, so each tool and
// each of its arguments has to explain itself.
func TestEveryToolAndArgumentIsDescribedForTheAgent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.agent(h.token("alice"))
	tools, err := a.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := registeredToolNames(t)
	var got []string
	for _, tool := range tools.Tools {
		got = append(got, tool.Name)
		if len(tool.Description) < 80 {
			t.Errorf("tool %s: description %q is too thin to teach an agent", tool.Name, tool.Description)
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("tool %s: input schema %T", tool.Name, tool.InputSchema)
		}
		props, _ := schema["properties"].(map[string]any)
		for name, p := range props {
			if d, _ := p.(map[string]any)["description"].(string); d == "" {
				t.Errorf("tool %s: argument %s has no description", tool.Name, name)
			}
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
}

func TestTheConnectPageHandsTheAppThisServersAddressAndTheTextOfTheSetupInBothLanguages(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	for lang, heading := range map[string]string{"en": "Connect your agent", "zh-TW": "連接你的 agent"} {
		res, body := h.do(alice, "GET", "/connect", "", map[string]string{"Accept-Language": lang})
		b := bootOf(t, body)
		if res.StatusCode != http.StatusOK || b.Page != "connect" || b.Strings["connect.title"] != heading {
			t.Fatalf("%s: %d, boot %+v", lang, res.StatusCode, b.Page)
		}
		// The app writes the commands from this address (web/src/pages.test.tsx
		// checks them); a wrong address here would put wrong commands on the page.
		if b.Connect == nil || b.Connect.BaseURL != h.srv.URL {
			t.Errorf("%s: connect boot %+v", lang, b.Connect)
		}
	}
	if res, _ := h.do(nil, "GET", "/connect", "", nil); res.StatusCode != http.StatusSeeOther {
		t.Errorf("the connect page without a session: %d", res.StatusCode)
	}
}

func TestATokenMadeFromTheConnectPageComesBackOnceAndWorksOnMCPButOnlyABrowserSessionCanMakeOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	res, body := h.do(alice, "POST", "/api/tokens", `{"name":"laptop"}`, bridgeHeaders(h))
	made := decode[struct{ Token, Name string }](t, body)
	if res.StatusCode != http.StatusCreated || !strings.HasPrefix(made.Token, "pct_") || made.Name != "laptop" {
		t.Fatalf("create: %d %s", res.StatusCode, body)
	}
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("a response carrying a secret must not be cached: %q", res.Header.Get("Cache-Control"))
	}
	// The token is the one the MCP endpoint takes, and it is listed, without its secret, on the Connect page.
	if res, _ := h.do(nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + made.Token, "Content-Type": "application/json", "Accept": "application/json, text/event-stream"}); res.StatusCode == http.StatusUnauthorized {
		t.Errorf("the new token is refused by MCP: %d", res.StatusCode)
	}
	refused := map[string]struct {
		c      *http.Client
		header map[string]string
		want   int
	}{
		"no session":             {nil, map[string]string{"X-Pictor": "1"}, http.StatusUnauthorized},
		"no bridge header":       {alice, map[string]string{"Origin": h.srv.URL}, http.StatusForbidden},
		"another origin":         {alice, map[string]string{"X-Pictor": "1", "Origin": "https://evil.example"}, http.StatusForbidden},
		"a token, not a session": {nil, map[string]string{"X-Pictor": "1", "Authorization": "Bearer " + made.Token}, http.StatusUnauthorized},
	}
	for name, c := range refused {
		if res, body := h.do(c.c, "POST", "/api/tokens", `{"name":"x"}`, c.header); res.StatusCode != c.want || strings.Contains(body, "pct_") {
			t.Errorf("%s: %d %s", name, res.StatusCode, body)
		}
	}
}

func TestRevokingATokenFromTheConnectPageStopsItAtOnceAndOnlyTheOwnerCanDoIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	bob := h.browser("bob", "pw-b")
	_, body := h.do(alice, "POST", "/api/tokens", `{"name":"laptop"}`, bridgeHeaders(h))
	made := decode[struct{ Token string }](t, body)
	_, page := h.do(alice, "GET", "/connect", "", nil)
	id := bootOf(t, page).Connect.Tokens[0].ID
	mcp := func() int {
		res, _ := h.do(nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + made.Token, "Content-Type": "application/json", "Accept": "application/json, text/event-stream"})
		return res.StatusCode
	}
	if mcp() == http.StatusUnauthorized {
		t.Fatal("the token does not work before it is revoked")
	}
	if res, _ := h.do(bob, "DELETE", "/api/tokens/"+id, "", bridgeHeaders(h)); res.StatusCode != http.StatusNotFound {
		t.Errorf("another person revoking it: %d", res.StatusCode)
	}
	if res, _ := h.do(nil, "DELETE", "/api/tokens/"+id, "", map[string]string{"X-Pictor": "1", "Authorization": "Bearer " + made.Token}); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("the token revoking itself: %d", res.StatusCode)
	}
	if res, _ := h.do(alice, "DELETE", "/api/tokens/"+id, "", map[string]string{"Origin": h.srv.URL}); res.StatusCode != http.StatusForbidden {
		t.Errorf("revoking without the bridge header: %d", res.StatusCode)
	}
	if mcp() == http.StatusUnauthorized {
		t.Fatal("a refused revoke still revoked it")
	}
	if res, _ := h.do(alice, "DELETE", "/api/tokens/"+id, "", bridgeHeaders(h)); res.StatusCode != http.StatusNoContent {
		t.Fatalf("revoking: %d", res.StatusCode)
	}
	if got := mcp(); got != http.StatusUnauthorized {
		t.Errorf("a revoked token on /api/mcp: %d", got)
	}
	if _, after := h.do(alice, "GET", "/connect", "", nil); len(bootOf(t, after).Connect.Tokens) != 0 {
		t.Error("the revoked token is still listed")
	}
	if res, _ := h.do(alice, "DELETE", "/api/tokens/"+id, "", bridgeHeaders(h)); res.StatusCode != http.StatusNotFound {
		t.Errorf("revoking twice: %d", res.StatusCode)
	}
}

func TestTheConnectPageListsTheCallersTokensWithoutTheirSecretsAndTheListCanBeAskedAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	_, body := h.do(alice, "POST", "/api/tokens", `{"name":"laptop"}`, bridgeHeaders(h))
	made := decode[struct{ Token string }](t, body)
	h.token("bob") // another person's token is not on alice's page
	_, page := h.do(alice, "GET", "/connect", "", nil)
	if list := bootOf(t, page).Connect.Tokens; len(list) != 1 || list[0].Name != "laptop" || strings.Contains(page, made.Token) {
		t.Errorf("the connect page's tokens: %+v", list)
	}
	// The same list is there for the page to ask again, newest first, uncached and without the secret.
	res, listed := h.do(alice, "GET", "/api/tokens", "", bridgeHeaders(h))
	if got := decode[[]store.Token](t, listed); res.StatusCode != http.StatusOK || len(got) != 1 || got[0].Name != "laptop" || strings.Contains(listed, made.Token) || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("GET /api/tokens: %d %s", res.StatusCode, listed)
	}
}
