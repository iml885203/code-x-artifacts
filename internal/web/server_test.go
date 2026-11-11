package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/iml885203/code-x-artifacts/internal/store/storetest"
)

type harness struct {
	t   *testing.T
	srv *httptest.Server
	st  *store.Store
	dir *fakeDirectory
	// disk is where the harness's server keeps asset and file bytes.
	disk      *assets.Disk
	assetsDir string
}

// newHarness serves over TLS because the session cookie is always Secure.
func newHarness(t *testing.T, options ...func(*Config)) *harness {
	t.Helper()
	st := storetest.Open(t)
	return newHarnessOn(t, st, options...)
}

// newHarnessOn serves an existing store, so a test can restart the server
// with a different configuration over the same data.
func newHarnessOn(t *testing.T, st *store.Store, options ...func(*Config)) *harness {
	t.Helper()
	dir := newFakeDirectory()
	assetsDir := t.TempDir()
	files, err := assets.OpenDisk(assetsDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		ThumbnailDir:  t.TempDir(),
		Store:         st,
		Assets:        files,
		Authenticator: dir,
		SessionKey:    []byte("test-key"),
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for _, option := range options {
		option(&cfg)
	}
	srv := httptest.NewTLSServer(New(cfg).Handler())
	t.Cleanup(srv.Close)
	return &harness{t: t, srv: srv, st: st, dir: dir, disk: files, assetsDir: assetsDir}
}

// client trusts the test server's certificate and reports redirects instead of following them.
func (h *harness) client(jar http.CookieJar) *http.Client {
	return &http.Client{
		Jar:           jar,
		Transport:     h.srv.Client().Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (h *harness) browser(username, password string) *http.Client {
	h.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		h.t.Fatal(err)
	}
	c := h.client(jar)
	form := url.Values{"username": {username}, "password": {password}}.Encode()
	if res, _ := h.do(c, "POST", "/login", form, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}); res.StatusCode != http.StatusSeeOther {
		h.t.Fatalf("login %s: status %d", username, res.StatusCode)
	}
	return c
}

func (h *harness) token(username string) string {
	h.t.Helper()
	tok, err := h.st.CreateToken(h.t.Context(), username, "test")
	if err != nil {
		h.t.Fatal(err)
	}
	return tok
}

// response is what a test inspects once the body has been read and closed.
type response struct {
	StatusCode int
	Header     http.Header
	cookies    []*http.Cookie
}

func (r response) Cookies() []*http.Cookie { return r.cookies }

func (h *harness) do(c *http.Client, method, path, body string, header map[string]string) (response, string) {
	h.t.Helper()
	req, err := http.NewRequestWithContext(h.t.Context(), method, h.srv.URL+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if c == nil {
		c = h.client(nil)
	}
	res, err := c.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	return response{StatusCode: res.StatusCode, Header: res.Header, cookies: res.Cookies()}, string(b)
}

func decode[T any](t *testing.T, body string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode %.120q: %v", body, err)
	}
	return v
}

// publish creates an artifact the way a client that names no visibility does.
func (h *harness) publish(tok, title, html string) artifactView {
	h.t.Helper()
	return h.publishAs(tok, title, html, "")
}

// publishShared creates an artifact visible to the whole organisation.
func (h *harness) publishShared(tok, title, html string) artifactView {
	h.t.Helper()
	return h.publishAs(tok, title, html, store.VisibilityOrganisation)
}

func (h *harness) publishAs(tok, title, html string, visibility store.Visibility) artifactView {
	h.t.Helper()
	body, err := json.Marshal(artifactInput{Title: title, HTML: html, Visibility: visibility})
	if err != nil {
		h.t.Fatal(err)
	}
	res, out := h.do(nil, "POST", "/api/artifacts", string(body), map[string]string{"Authorization": "Bearer " + tok})
	if res.StatusCode != http.StatusCreated {
		h.t.Fatalf("publish: %d %s", res.StatusCode, out)
	}
	return decode[artifactView](h.t, out)
}

func bridgeHeaders(h *harness) map[string]string {
	return map[string]string{"X-Pictor": "1", "Origin": h.srv.URL, "Content-Type": "application/json"}
}

// galleryList is what the gallery shows its viewer: the React app fills the
// shell with this call, with the viewer's cookie and the bridge header.
func galleryList(h *harness, c *http.Client) string {
	h.t.Helper()
	res, body := h.do(c, "GET", "/api/artifacts", "", bridgeHeaders(h))
	if res.StatusCode != http.StatusOK {
		h.t.Fatalf("the gallery's list: %d %s", res.StatusCode, body)
	}
	return body
}

func TestPagesRequireSignIn(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	res, _ := h.do(nil, "GET", "/raw/anything", "", nil)
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login?next=%2Fraw%2Fanything" {
		t.Fatalf("got %d %q", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	res, body := h.do(nil, "POST", "/login", "username=alice&password=nope", map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if res.StatusCode != http.StatusUnauthorized || !strings.Contains(body, "Wrong username or password") {
		t.Fatalf("got %d", res.StatusCode)
	}
	if len(res.Cookies()) != 0 {
		t.Fatal("a failed login must not set a session")
	}
}

func TestLoginOnlyRedirectsWithinTheSite(t *testing.T) {
	t.Parallel()
	for next, want := range map[string]string{"/a/x": "/a/x", "//evil.example": "/", `/\evil.example`: "/", "https://evil.example": "/"} {
		if got := safeNext(next); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", next, got, want)
		}
	}
}

func TestPublishWithTokenAndViewInBrowser(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Report", "<!doctype html><html><head><title>r</title></head><body>hi</body></html>")
	if a.Author != "alice" || !strings.HasSuffix(a.URL, "/a/"+a.ID) {
		t.Fatalf("got %+v", a)
	}

	bob := h.browser("bob", "pw-b")
	res, body := h.do(bob, "GET", "/a/"+a.ID, "", nil)
	if b := bootOf(t, body); res.StatusCode != http.StatusOK || b.Page != "viewer" || b.Artifact == nil || b.Artifact.ID != a.ID || b.Artifact.Title != "Report" {
		t.Fatalf("viewer: %d %+v", res.StatusCode, b)
	}
}

func TestRawRunsInAnOpaqueOriginWithTheBridge(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Report", "<!DOCTYPE html><html><head><meta charset=utf-8></head><body>hi</body></html>")

	res, body := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil)
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.HasPrefix(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") {
		t.Fatalf("csp %q", csp)
	}
	// A page saves a file only through pictor.download, which the viewer lets the person refuse.
	if strings.Contains(csp, "allow-downloads") {
		t.Fatalf("the sandbox lets a page start a download without asking: %q", csp)
	}
	if !strings.HasPrefix(body, "<!DOCTYPE html><html><head><script>") || !strings.Contains(body, "window.pictor") {
		t.Fatalf("bridge not injected after <head>: %.80s", body)
	}
}

func TestInjectBridgeKeepsDoctypeFirst(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"<!doctype html><p>x":         "<!doctype html><script>",
		"<p>fragment":                 "<script>",
		"<html><HEAD lang=x>y</HEAD>": "<html><HEAD lang=x><script>",
	}
	for in, prefix := range cases {
		if got := injectBridge(in); !strings.HasPrefix(got, prefix) {
			t.Errorf("injectBridge(%q) = %.40q", in, got)
		}
	}
}

func TestCookieAPICallsNeedTheBridgeHeaderAndSameOrigin(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Poll", "<p>poll")
	bob := h.browser("bob", "pw-b")
	path := "/api/artifacts/" + a.ID + "/me"

	if res, _ := h.do(bob, "GET", path, "", nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("without X-Pictor: %d", res.StatusCode)
	}
	if res, _ := h.do(bob, "GET", path, "", map[string]string{"X-Pictor": "1", "Origin": "null"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("from the artifact's opaque origin: %d", res.StatusCode)
	}
	res, body := h.do(bob, "GET", path, "", bridgeHeaders(h))
	me := decode[store.User](t, body)
	if res.StatusCode != http.StatusOK || me.Username != "bob" || me.DisplayName != "BOB" || me.Groups[0] != "SE" {
		t.Fatalf("whoami: %d %s", res.StatusCode, body)
	}
}

func TestAReaderCanNeitherUpdateNorDeleteAnArtifact(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Mine", "<p>v1")
	update := `{"title":"Mine","html":"<p>v2"}`

	if res, _ := h.do(nil, "PUT", "/api/artifacts/"+a.ID, update, map[string]string{"Authorization": "Bearer " + h.token("bob")}); res.StatusCode != http.StatusForbidden {
		t.Errorf("bob update: %d", res.StatusCode)
	}
	if res, _ := h.do(nil, "DELETE", "/api/artifacts/"+a.ID, "", map[string]string{"Authorization": "Bearer " + h.token("bob")}); res.StatusCode != http.StatusForbidden {
		t.Errorf("bob delete: %d", res.StatusCode)
	}
	if res, _ := h.do(nil, "PUT", "/api/artifacts/"+a.ID, update, map[string]string{"Authorization": "Bearer " + h.token("alice")}); res.StatusCode != http.StatusOK {
		t.Errorf("alice update: %d", res.StatusCode)
	}
	got, _ := h.st.Artifact(t.Context(), a.ID)
	if got.Content != "<p>v2" {
		t.Errorf("html %q", got.Content)
	}
}

func TestSharedStateIsScopedToItsArtifactAndRecordsTheWriter(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	poll := h.publishShared(tok, "Poll", "<p>poll")
	other := h.publishShared(tok, "Other", "<p>other")
	bob := h.browser("bob", "pw-b")

	res, _ := h.do(bob, "PUT", "/api/artifacts/"+poll.ID+"/db/votes/bob", `{"choice":"A"}`, bridgeHeaders(h))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("put: %d", res.StatusCode)
	}
	_, body := h.do(h.browser("alice", "pw-a"), "GET", "/api/artifacts/"+poll.ID+"/db/votes", "", bridgeHeaders(h))
	docs := decode[[]store.Doc](t, body)
	if len(docs) != 1 || docs[0].ID != "bob" || docs[0].UpdatedBy != "bob" || string(docs[0].Data) != `{"choice":"A"}` {
		t.Fatalf("list: %s", body)
	}
	if res, _ := h.do(bob, "GET", "/api/artifacts/"+other.ID+"/db/votes/bob", "", bridgeHeaders(h)); res.StatusCode != http.StatusNotFound {
		t.Errorf("another artifact saw the doc: %d", res.StatusCode)
	}
	if res, _ := h.do(bob, "PUT", "/api/artifacts/missing/db/votes/bob", `{}`, bridgeHeaders(h)); res.StatusCode != http.StatusNotFound {
		t.Errorf("put into a missing artifact: %d", res.StatusCode)
	}
}

func TestFormsRefuseCrossOriginPosts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	bob := h.browser("bob", "pw-b")
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "null"}
	if res, _ := h.do(bob, "POST", "/logout", "", form); res.StatusCode != http.StatusForbidden {
		t.Errorf("sign-out from opaque origin: %d", res.StatusCode)
	}
	form["Origin"] = h.srv.URL
	if res, _ := h.do(bob, "POST", "/logout", "", form); res.StatusCode != http.StatusSeeOther {
		t.Errorf("sign-out from own page: %d", res.StatusCode)
	}
}

func (h *harness) bearer(username string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + h.token(username)}
}

func TestANewArtifactIsVisibleOnlyToItsAuthorUnlessTheAuthorSaysOtherwise(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	cases := map[store.Visibility]store.Visibility{
		"":                           store.VisibilityPrivate,
		store.VisibilityPrivate:      store.VisibilityPrivate,
		store.VisibilityOrganisation: store.VisibilityOrganisation,
	}
	for asked, want := range cases {
		if got := h.publishAs(tok, "t", "<p>x", asked).Visibility; got != want {
			t.Errorf("publish with visibility %q: got %q, want %q", asked, got, want)
		}
	}
	body := `{"title":"t","html":"<p>x","visibility":"everyone"}`
	if res, _ := h.do(nil, "POST", "/api/artifacts", body, map[string]string{"Authorization": "Bearer " + tok}); res.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown visibility on publish: %d", res.StatusCode)
	}
}

func TestWhoCannotSeeAnArtifactCannotTellItExists(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	secret := h.publish(h.token("alice"), "Secret plan", "<p>secret")
	if err := h.st.PutDoc(t.Context(), secret.ID, "c", "d", json.RawMessage(`{}`), "alice"); err != nil {
		t.Fatal(err)
	}
	bob := h.browser("bob", "pw-b")
	bobTok := h.bearer("bob")

	// The same request against an id that does not exist is the yardstick:
	// the status and the body must match, so a 404 never says "it is there".
	type call struct{ method, path, body string }
	routes := func(id string) []call {
		api := "/api/artifacts/" + id
		return []call{
			{"GET", "/a/" + id, ""},
			{"GET", "/raw/" + id, ""},
			{"GET", api, ""},
			{"PUT", api, `{"title":"x","html":"x"}`},
			{"PATCH", api, `{"visibility":"organisation"}`},
			{"DELETE", api, ""},
			{"GET", api + "/me", ""},
			{"GET", api + "/changes?sinceVersion=1", ""},
			{"GET", api + "/db/c", ""},
			{"GET", api + "/db/c/d", ""},
			{"PUT", api + "/db/c/d", `{}`},
			{"DELETE", api + "/db/c/d", ""},
			{"GET", api + "/assets", ""},
			{"PUT", api + "/assets/a.png", "x"},
			{"DELETE", api + "/assets/a.png", ""},
			{"GET", "/files/" + id + "/a.png", ""},
		}
	}
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": h.srv.URL, "X-Pictor": "1"}
	missing, hidden := routes("nosuchartifact0000"), routes(secret.ID)
	for i := range hidden {
		for who, client := range map[string]*http.Client{"browser": bob, "token": nil} {
			header := form
			if client == nil {
				if !strings.HasPrefix(hidden[i].path, "/api/") {
					continue // pages take a browser session, not a token
				}
				header = bobTok
			}
			wantRes, wantBody := h.do(client, hidden[i].method, missing[i].path, missing[i].body, header)
			res, body := h.do(client, hidden[i].method, hidden[i].path, hidden[i].body, header)
			// A page names the address it was asked for (so the language switch can
			// return to it) and nothing else, so the two answers match once that is
			// taken out.
			wantBody = strings.ReplaceAll(wantBody, "nosuchartifact0000", "ID")
			body = strings.ReplaceAll(body, secret.ID, "ID")
			if res.StatusCode != http.StatusNotFound || res.StatusCode != wantRes.StatusCode || body != wantBody {
				t.Errorf("%s %s %s: got %d %q, want %d %q", who, hidden[i].method, hidden[i].path, res.StatusCode, body, wantRes.StatusCode, wantBody)
			}
		}
	}

	if gallery := galleryList(h, bob); strings.Contains(gallery, "Secret plan") {
		t.Error("the gallery lists an artifact its viewer cannot see")
	}
	_, list := h.do(nil, "GET", "/api/artifacts", "", bobTok)
	if strings.Contains(list, secret.ID) {
		t.Error("the API lists an artifact its viewer cannot see")
	}

	alice := h.browser("alice", "pw-a")
	if !strings.Contains(galleryList(h, alice), "Secret plan") {
		t.Error("the author no longer sees their own private artifact")
	}
	if res, _ := h.do(alice, "GET", "/raw/"+secret.ID, "", nil); res.StatusCode != http.StatusOK {
		t.Errorf("author opens a private artifact: %d", res.StatusCode)
	}
}

func TestSwitchingVisibilityDecidesWhoCanSeeTheArtifactAndItsData(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publish(h.token("alice"), "Notes", "<p>notes")
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")
	if res, _ := h.do(alice, "PUT", "/api/artifacts/"+a.ID+"/db/n/1", `{"v":1}`, bridgeHeaders(h)); res.StatusCode != http.StatusOK {
		t.Fatalf("author writes to the db: %d", res.StatusCode)
	}
	status := func(c *http.Client, path string) int {
		res, _ := h.do(c, "GET", path, "", bridgeHeaders(h))
		return res.StatusCode
	}
	paths := []string{"/a/" + a.ID, "/raw/" + a.ID, "/api/artifacts/" + a.ID, "/api/artifacts/" + a.ID + "/me", "/api/artifacts/" + a.ID + "/db/n", "/api/artifacts/" + a.ID + "/db/n/1"}
	for _, p := range paths {
		if got := status(bob, p); got != http.StatusNotFound {
			t.Errorf("private, bob GET %s: %d", p, got)
		}
	}

	res, body := h.do(alice, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"organisation"}`, bridgeHeaders(h))
	if res.StatusCode != http.StatusOK || decode[artifactView](t, body).Visibility != store.VisibilityOrganisation {
		t.Fatalf("author shares: %d %s", res.StatusCode, body)
	}
	for _, p := range paths {
		if got := status(bob, p); got != http.StatusOK {
			t.Errorf("organisation, bob GET %s: %d", p, got)
		}
	}
	if !strings.Contains(galleryList(h, bob), "Notes") {
		t.Error("the gallery does not list a shared artifact")
	}
	if res, _ := h.do(bob, "PUT", "/api/artifacts/"+a.ID+"/db/n/2", `{"v":2}`, bridgeHeaders(h)); res.StatusCode != http.StatusOK {
		t.Errorf("a viewer writes to the shared db: %d", res.StatusCode)
	}

	if res, _ := h.do(alice, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"private"}`, bridgeHeaders(h)); res.StatusCode != http.StatusOK {
		t.Fatalf("author makes it private again: %d", res.StatusCode)
	}
	for _, p := range paths {
		if got := status(bob, p); got != http.StatusNotFound {
			t.Errorf("private again, bob GET %s: %d", p, got)
		}
	}
	if res, body := h.do(alice, "GET", "/api/artifacts/"+a.ID+"/db/n/2", "", bridgeHeaders(h)); res.StatusCode != http.StatusOK || !strings.Contains(body, `"v":2`) {
		t.Errorf("data written while shared survives: %d %s", res.StatusCode, body)
	}
}

func TestOnlyTheAuthorChangesVisibility(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Shared", "<p>x")
	bob := h.browser("bob", "pw-b")
	patch := func(c *http.Client, header map[string]string, visibility string) int {
		res, _ := h.do(c, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"`+visibility+`"}`, header)
		return res.StatusCode
	}

	if got := patch(nil, h.bearer("bob"), "private"); got != http.StatusForbidden {
		t.Errorf("bob PATCH with a token: %d", got)
	}
	if res, _ := h.do(nil, "PUT", "/api/artifacts/"+a.ID, `{"title":"Shared","html":"<p>x","visibility":"private"}`, h.bearer("bob")); res.StatusCode != http.StatusForbidden {
		t.Errorf("bob PUT with visibility: %d", res.StatusCode)
	}
	if got := patch(bob, bridgeHeaders(h), "private"); got != http.StatusForbidden {
		t.Errorf("bob PATCH from the viewer: %d", got)
	}
	if got, _ := h.st.Artifact(t.Context(), a.ID); got.Visibility != store.VisibilityOrganisation {
		t.Fatalf("visibility changed to %q", got.Visibility)
	}

	alice := h.browser("alice", "pw-a")
	if got := patch(alice, bridgeHeaders(h), "private"); got != http.StatusOK {
		t.Errorf("alice PATCH from the viewer: %d", got)
	}
	if got, _ := h.st.Artifact(t.Context(), a.ID); got.Visibility != store.VisibilityPrivate {
		t.Errorf("visibility is %q after the author's change", got.Visibility)
	}
	if got := patch(nil, h.bearer("alice"), "everyone"); got != http.StatusBadRequest {
		t.Errorf("unknown visibility on PATCH: %d", got)
	}
	if res, _ := h.do(nil, "PATCH", "/api/artifacts/"+a.ID, `{}`, h.bearer("alice")); res.StatusCode != http.StatusBadRequest {
		t.Errorf("PATCH without visibility: %d", res.StatusCode)
	}
}

func TestRepublishingKeepsVisibilityUnlessItIsNamed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.bearer("alice")
	a := h.publishShared(h.token("alice"), "T", "<p>v1")

	if res, _ := h.do(nil, "PUT", "/api/artifacts/"+a.ID, `{"title":"T","html":"<p>v2"}`, alice); res.StatusCode != http.StatusOK {
		t.Fatalf("republish: %d", res.StatusCode)
	}
	if got, _ := h.st.Artifact(t.Context(), a.ID); got.Visibility != store.VisibilityOrganisation || got.Content != "<p>v2" {
		t.Errorf("republish without visibility changed it: %+v", got)
	}
	if res, _ := h.do(nil, "PUT", "/api/artifacts/"+a.ID, `{"title":"T","html":"<p>v3","visibility":"private"}`, alice); res.StatusCode != http.StatusOK {
		t.Fatalf("republish private: %d", res.StatusCode)
	}
	if got, _ := h.st.Artifact(t.Context(), a.ID); got.Visibility != store.VisibilityPrivate {
		t.Errorf("republish naming private left %q", got.Visibility)
	}
}

func withPublicLinks(cfg *Config) { cfg.PublicLinks = true }

func TestPublicSharingIsOffUnlessTheDeployerTurnsItOn(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.bearer("alice")
	a := h.publishShared(h.token("alice"), "Demo", "<p>demo")

	refused := func(what string, res response, body string) {
		t.Helper()
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "public links are turned off") {
			t.Errorf("%s: %d %s", what, res.StatusCode, body)
		}
	}
	res, body := h.do(nil, "POST", "/api/artifacts", `{"title":"T","html":"<p>","visibility":"public"}`, alice)
	refused("publish as public", res, body)
	res, body = h.do(nil, "PUT", "/api/artifacts/"+a.ID, `{"title":"Demo","html":"<p>demo","visibility":"public"}`, alice)
	refused("republish as public", res, body)
	res, body = h.do(nil, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"public"}`, alice)
	refused("PATCH to public", res, body)
	res, body = h.do(h.browser("alice", "pw-a"), "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"public"}`, bridgeHeaders(h))
	refused("the viewer's switch to public", res, body)
	if got, _ := h.st.Artifact(t.Context(), a.ID); got.Visibility != store.VisibilityOrganisation {
		t.Errorf("visibility is %q after refused attempts", got.Visibility)
	}
	if list, _ := h.st.Artifacts(t.Context()); len(list) != 1 {
		t.Errorf("a refused publish still stored an artifact: %d artifacts", len(list))
	}
}

func TestAPublicArtifactOpensWithoutSigningIn(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	alice := h.bearer("alice")
	pub := h.publishAs(h.token("alice"), "Demo", "<p>demo", store.VisibilityPublic)
	org := h.publishShared(h.token("alice"), "Internal", "<p>internal")
	if err := h.st.PutDoc(t.Context(), pub.ID, "c", "d", json.RawMessage(`{"v":1}`), "alice"); err != nil {
		t.Fatal(err)
	}
	if res, body := h.do(nil, "PATCH", "/api/artifacts/"+org.ID, `{"visibility":"public"}`, alice); res.StatusCode != http.StatusOK || !strings.Contains(body, `"public"`) {
		t.Fatalf("author switches to public: %d %s", res.StatusCode, body)
	}

	// Anonymous: the page, its content and the bridge's reads.
	for _, p := range []string{"/a/" + pub.ID, "/raw/" + pub.ID, "/api/artifacts/" + pub.ID + "/me", "/api/artifacts/" + pub.ID + "/db/c", "/api/artifacts/" + pub.ID + "/db/c/d"} {
		if res, _ := h.do(nil, "GET", p, "", nil); res.StatusCode != http.StatusOK {
			t.Errorf("anonymous GET %s: %d", p, res.StatusCode)
		}
	}
	_, body := h.do(nil, "GET", "/api/artifacts/"+pub.ID+"/me", "", nil)
	if me := decode[service.Profile](t, body); !me.Anonymous || me.Username != "" || len(me.Groups) != 0 {
		t.Errorf("whoami for an anonymous viewer: %+v", me)
	}
	_, page := h.do(nil, "GET", "/a/"+pub.ID, "", nil)
	if b := bootOf(t, page); b.Page != "viewer" || b.User != "" || b.Initials != "" || b.Artifact == nil || b.Artifact.CanChange {
		t.Errorf("the anonymous viewer page is handed signed-in controls: %+v", b)
	}
}

func TestSignedInPeopleSeeAPublicArtifactLikeAnOrganisationOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	pub := h.publishAs(h.token("alice"), "Demo", "<p>demo", store.VisibilityPublic)
	priv := h.publish(h.token("alice"), "Mine", "<p>mine")
	// Signed-in people see a public artifact like an organisation one, in the gallery too.
	bob := h.browser("bob", "pw-b")
	if res, _ := h.do(bob, "GET", "/raw/"+pub.ID, "", nil); res.StatusCode != http.StatusOK {
		t.Errorf("bob opens a public artifact: %d", res.StatusCode)
	}
	if !strings.Contains(galleryList(h, bob), "Demo") {
		t.Error("the gallery does not list a public artifact to a signed-in person")
	}
	if _, body := h.do(bob, "GET", "/api/artifacts/"+pub.ID+"/me", "", bridgeHeaders(h)); decode[service.Profile](t, body).Anonymous {
		t.Error("whoami calls a signed-in person anonymous")
	}
	aliceBrowser := h.browser("alice", "pw-a")
	if _, body := h.do(aliceBrowser, "GET", "/a/"+priv.ID, "", nil); !bootOf(t, body).PublicLinks || !bootOf(t, body).Artifact.CanChange {
		t.Error("the author is not told public links are on")
	}
}

func TestEverythingElseStillNeedsSigningInWhenAnArtifactIsPublic(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	pub := h.publishAs(h.token("alice"), "Demo", "<p>demo", store.VisibilityPublic)
	priv := h.publish(h.token("alice"), "Mine", "<p>mine")
	for _, p := range []string{"/", "/connect", "/api/artifacts", "/api/artifacts/" + pub.ID} {
		if res, _ := h.do(nil, "GET", p, "", nil); res.StatusCode != http.StatusSeeOther && res.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous GET %s: %d, want a sign-in demand", p, res.StatusCode)
		}
	}
	for _, id := range []string{priv.ID, "nosuchartifact0000"} {
		for _, p := range []string{"/a/" + id, "/raw/" + id, "/api/artifacts/" + id + "/me", "/api/artifacts/" + id + "/db/c", "/api/artifacts/" + id + "/db/c/d"} {
			res, _ := h.do(nil, "GET", p, "", nil)
			want := http.StatusUnauthorized
			if !strings.HasPrefix(p, "/api/") {
				want = http.StatusSeeOther
			}
			if res.StatusCode != want {
				t.Errorf("anonymous GET %s: %d, want %d", p, res.StatusCode, want)
			}
		}
	}
}

func TestAnAnonymousViewerCannotWriteTheDatabase(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	pub := h.publishAs(h.token("alice"), "Poll", "<p>poll", store.VisibilityPublic)
	doc := "/api/artifacts/" + pub.ID + "/db/votes/1"
	if err := h.st.PutDoc(t.Context(), pub.ID, "votes", "1", json.RawMessage(`{"v":1}`), "alice"); err != nil {
		t.Fatal(err)
	}
	for _, header := range []map[string]string{nil, bridgeHeaders(h), {"X-Pictor": "1", "Content-Type": "application/json"}} {
		if res, _ := h.do(nil, "PUT", doc, `{"v":2}`, header); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous PUT: %d", res.StatusCode)
		}
		if res, _ := h.do(nil, "DELETE", doc, "", header); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous DELETE: %d", res.StatusCode)
		}
	}
	if res, _ := h.do(nil, "PUT", "/api/artifacts/"+pub.ID, `{"title":"x","html":"x"}`, nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous PUT of the artifact: %d", res.StatusCode)
	}
	if d, err := h.st.Doc(t.Context(), pub.ID, "votes", "1"); err != nil || string(d.Data) != `{"v":1}` || d.UpdatedBy != "alice" {
		t.Errorf("the document changed: %+v %v", d, err)
	}
	// A signed-in viewer still writes, as before.
	if res, _ := h.do(h.browser("bob", "pw-b"), "PUT", doc, `{"v":3}`, bridgeHeaders(h)); res.StatusCode != http.StatusOK {
		t.Errorf("signed-in PUT on a public artifact: %d", res.StatusCode)
	}
}

func TestAPublicArtifactBehavesAsOrganisationWhenPublicLinksAreOff(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	pub := h.publishAs(h.token("alice"), "Demo", "<p>demo", store.VisibilityPublic)
	if res, _ := h.do(nil, "GET", "/raw/"+pub.ID, "", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("public links on, anonymous: %d", res.StatusCode)
	}

	// Same database, public links now off.
	off := newHarnessOn(t, h.st)
	if res, _ := off.do(nil, "GET", "/raw/"+pub.ID, "", nil); res.StatusCode != http.StatusSeeOther {
		t.Errorf("public links off, anonymous raw: %d", res.StatusCode)
	}
	if res, _ := off.do(nil, "GET", "/api/artifacts/"+pub.ID+"/me", "", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("public links off, anonymous whoami: %d", res.StatusCode)
	}
	if res, _ := off.do(off.browser("bob", "pw-b"), "GET", "/raw/"+pub.ID, "", nil); res.StatusCode != http.StatusOK {
		t.Errorf("public links off, signed in: %d", res.StatusCode)
	}
	other := h.publish(h.token("alice"), "Mine", "<p>mine")
	if _, body := off.do(off.browser("alice", "pw-a"), "GET", "/a/"+other.ID, "", nil); bootOf(t, body).PublicLinks {
		t.Error("the author is told public links are on while they are off")
	}
	if got, _ := h.st.Artifact(t.Context(), pub.ID); got.Visibility != store.VisibilityPublic {
		t.Errorf("turning the switch off rewrote the stored visibility to %q", got.Visibility)
	}
}

func TestAnonymousReadersDoNotLearnWhoWroteTheSharedData(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	pub := h.publishAs(h.token("alice"), "Poll", "<p>poll", store.VisibilityPublic)
	if err := h.st.PutDoc(t.Context(), pub.ID, "votes", "1", json.RawMessage(`{"v":1}`), "alice"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/api/artifacts/" + pub.ID + "/db/votes", "/api/artifacts/" + pub.ID + "/db/votes/1"} {
		if res, body := h.do(nil, "GET", p, "", nil); res.StatusCode != http.StatusOK || strings.Contains(body, "alice") || !strings.Contains(body, `"v":1`) {
			t.Errorf("anonymous GET %s: %d %s", p, res.StatusCode, body)
		}
		if _, body := h.do(h.browser("bob", "pw-b"), "GET", p, "", bridgeHeaders(h)); !strings.Contains(body, `"updatedBy":"alice"`) {
			t.Errorf("signed-in GET %s lost the writer: %s", p, body)
		}
	}
}

func TestAnArtifactTellsItsViewerWhetherTheyMayChangeIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Notes", "<p>x")
	for who, want := range map[string]bool{"alice": true, "bob": false} {
		c := h.browser(who, "pw-"+map[string]string{"alice": "a", "bob": "b"}[who])
		_, one := h.do(c, "GET", "/api/artifacts/"+a.ID, "", bridgeHeaders(h))
		_, list := h.do(c, "GET", "/api/artifacts", "", bridgeHeaders(h))
		_, page := h.do(c, "GET", "/a/"+a.ID, "", nil)
		if got := decode[artifactView](t, one).CanChange; got != want {
			t.Errorf("%s: GET artifact says canChange %v", who, got)
		}
		if got := decode[[]artifactView](t, list)[0].CanChange; got != want {
			t.Errorf("%s: the list says canChange %v", who, got)
		}
		if got := bootOf(t, page).Artifact.CanChange; got != want {
			t.Errorf("%s: the viewer page says canChange %v", who, got)
		}
	}
}

func TestTheViewersFrameIsSandboxedWithTheListTheServerSendsAsCSP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	a := h.publish(h.token("alice"), "Notes", "<p>x")
	res, _ := h.do(alice, "GET", "/raw/"+a.ID, "", nil)
	policy, _, _ := strings.Cut(res.Header.Get("Content-Security-Policy"), ";")
	_, page := h.do(alice, "GET", "/a/"+a.ID, "", nil)
	frame := bootOf(t, page).FrameSandbox
	if want := strings.TrimPrefix(policy, "sandbox "); frame != want || frame == "" {
		t.Errorf("the viewer's iframe is told sandbox=%q, /raw sends %q", frame, want)
	}
	if strings.Contains(frame, "allow-same-origin") {
		t.Errorf("the frame gets the artifact's own origin: %q", frame)
	}
}

func TestSignedOutPeopleAndMissingArtifactsGetThePagesThatSayWhatIsTrueOfBoth(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	res, body := h.do(h.browser("alice", "pw-a"), "GET", "/a/nosuchartifact0000", "", nil)
	if b := bootOf(t, body); res.StatusCode != http.StatusNotFound || b.Page != "error" || b.Status != http.StatusNotFound || b.Artifact != nil {
		t.Errorf("a missing artifact: %d %+v", res.StatusCode, b)
	}
	if res.Header.Get("X-Robots-Tag") != "" && res.Header.Get("Content-Security-Policy") == "" {
		t.Errorf("the error page lost its headers: %v", res.Header)
	}
}

func TestInitialsComeFromThePartsOfTheAccountName(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{"ada.lovelace": "AL", "alice": "AL", "a": "A", "bob_the.builder": "BT", "": ""} {
		if got := initials(name); got != want {
			t.Errorf("initials(%q) = %q, want %q", name, got, want)
		}
	}
}
