package web

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/files"
)

// sessionAnswer is what begin answers, over HTTP or MCP.
type sessionAnswer struct {
	Session          string    `json:"session"`
	ExpiresAt        time.Time `json:"expiresAt"`
	ExpiresInSeconds int       `json:"expiresInSeconds"`
	Missing          []struct {
		Paths  []string `json:"paths"`
		SHA256 string   `json:"sha256"`
		Size   int64    `json:"size"`
		URL    string   `json:"url"`
	} `json:"missing"`
	Stored []string `json:"stored"`
	Limits struct {
		FileBytes  int   `json:"fileBytes"`
		Files      int   `json:"files"`
		TotalBytes int64 `json:"totalBytes"`
	} `json:"limits"`
}

// declared is how a publisher announces a file: its path, hash and size.
func declared(path, text string) map[string]any {
	return map[string]any{"path": path, "sha256": sha(text), "size": len(text)}
}

// bigPage is a page of about 1 MB, the size no model types into a call.
func bigPage() string {
	return "<!doctype html><html><head><meta charset=\"utf-8\"><title>Big</title></head><body>" +
		strings.Repeat("<p>0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef</p>\n", 15000) + "</body></html>"
}

func (h *harness) begin(tok string, body map[string]any) (response, string) {
	h.t.Helper()
	return h.sendFiles("POST", "/api/publish-sessions", tok, body)
}

func (h *harness) mustBegin(tok string, body map[string]any) sessionAnswer {
	h.t.Helper()
	res, out := h.begin(tok, body)
	if res.StatusCode != http.StatusCreated {
		h.t.Fatalf("begin: %d %s", res.StatusCode, out)
	}
	return decode[sessionAnswer](h.t, out)
}

// put sends bytes to an upload URL the way curl -X PUT --data-binary does: no
// token, no cookie.
func (h *harness) put(url, data string) (response, string) {
	h.t.Helper()
	return h.do(nil, "PUT", strings.TrimPrefix(url, h.srv.URL), data, map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
}

// uploadAll sends the bytes of every missing blob; texts maps a path to its bytes.
func (h *harness) uploadAll(a sessionAnswer, texts map[string]string) {
	h.t.Helper()
	for _, m := range a.Missing {
		if res, out := h.put(m.URL, texts[m.Paths[0]]); res.StatusCode != http.StatusOK {
			h.t.Fatalf("upload %v: %d %s", m.Paths, res.StatusCode, out)
		}
	}
}

func (h *harness) commit(tok, session string) (response, string) {
	h.t.Helper()
	return api(h, "POST", "/api/publish-sessions/"+session+"/commit", "", tok)
}

func (h *harness) storedContent(id string) string {
	h.t.Helper()
	a, err := h.st.Artifact(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	return a.Content
}

func TestAPageOfOneMegabyteUploadedThroughASessionIsStoredByteForByte(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	page := bigPage()

	a := h.mustBegin(tok, map[string]any{"title": "Big report", "files": []any{declared("content", page)}})
	assertBeganForOnePage(t, a, page)
	if res, out := h.commit(tok, a.Session); res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "content") {
		t.Fatalf("a commit before the upload: %d %s", res.StatusCode, out)
	}
	h.uploadAll(a, map[string]string{"content": page})
	res, out := h.commit(tok, a.Session)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("commit: %d %s", res.StatusCode, out)
	}
	got := decode[artifactView](t, out)
	if got.Version != 1 || got.Author != "alice" || got.Kind != "html" || got.Visibility != "private" || got.Title != "Big report" || got.Role != "author" {
		t.Errorf("the artifact after commit = %+v", got.Artifact)
	}
	if h.storedContent(got.ID) != page {
		t.Error("the stored page is not the bytes that were uploaded")
	}
	h.assertOpens(got.ID)
}

func assertBeganForOnePage(t *testing.T, a sessionAnswer, page string) {
	t.Helper()
	if len(page) < 1<<20 {
		t.Fatalf("the test page is %d bytes, want at least 1 MB", len(page))
	}
	if len(a.Missing) != 1 || a.Missing[0].Paths[0] != "content" || a.Missing[0].Size != int64(len(page)) || a.ExpiresInSeconds != 600 || a.Limits.FileBytes != 16<<20 || a.Limits.Files != 255 {
		t.Fatalf("begin answered %+v", a)
	}
}

func (h *harness) assertOpens(id string) {
	h.t.Helper()
	t := h.t
	if res, seen := h.do(h.browser("alice", "pw-a"), "GET", "/raw/"+id, "", nil); res.StatusCode != http.StatusOK || !strings.Contains(seen, "<title>Big</title>") {
		t.Errorf("the page cannot be opened: %d", res.StatusCode)
	}
}

func TestASessionUpdatesAnArtifactAndItsFilesLikeAnInlinePublish(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	first := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS, "styles/site.css": "p{}", "data/rows.json": "[1]"}, "")

	// Change one file, add one, remove one; the page is sent again, the others are carried.
	page2 := multiPage + "<!-- v2 -->"
	a := h.mustBegin(tok, map[string]any{
		"id": first.ID, "title": "Multi", "baseVersion": 1,
		"files":  []any{declared("content", page2), declared("app.js", "two"), declared("extra.json", "{}")},
		"remove": []string{"data/rows.json"},
	})
	h.uploadAll(a, map[string]string{"content": page2, "app.js": "two", "extra.json": "{}"})
	res, out := h.commit(tok, a.Session)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("commit: %d %s", res.StatusCode, out)
	}
	got := decode[artifactView](t, out)
	var paths []string
	for _, f := range got.Files {
		paths = append(paths, f.Path)
	}
	if got.Version != 2 || strings.Join(paths, ",") != "app.js,extra.json,styles/site.css" {
		t.Fatalf("after the update: version %d files %v", got.Version, paths)
	}
	if _, text := h.fileText(tok, first.ID, "app.js", 0); text != "two" {
		t.Errorf("app.js = %q", text)
	}
	if _, text := h.fileText(tok, first.ID, "styles/site.css", 0); text != "p{}" {
		t.Errorf("a file that was not sent was not carried: %q", text)
	}
	if h.storedContent(first.ID) != page2 {
		t.Error("the page was not replaced")
	}
}

func TestBlobsTheArtifactAlreadyHoldsAreNotSentAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	data := `{"rows": "` + strings.Repeat("x", 100000) + `"}`
	first := h.mustBegin(tok, map[string]any{"title": "Report", "files": []any{declared("content", multiPage), declared("data/rows.json", data)}})
	if len(first.Missing) != 2 {
		t.Fatalf("a first publish needs both blobs: %+v", first.Missing)
	}
	h.uploadAll(first, map[string]string{"content": multiPage, "data/rows.json": data})
	res, out := h.commit(tok, first.Session)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("commit: %d %s", res.StatusCode, out)
	}
	id := decode[artifactView](t, out).ID

	// The same big data again, a new page: only the page travels.
	page2 := multiPage + "<!-- again -->"
	second := h.mustBegin(tok, map[string]any{"id": id, "title": "Report", "baseVersion": 1, "files": []any{declared("content", page2), declared("data/rows.json", data)}})
	if len(second.Missing) != 1 || second.Missing[0].Paths[0] != "content" || len(second.Stored) != 1 || second.Stored[0] != "data/rows.json" {
		t.Fatalf("second begin: missing %+v stored %v", second.Missing, second.Stored)
	}
	h.uploadAll(second, map[string]string{"content": page2})
	if res, out := h.commit(tok, second.Session); res.StatusCode != http.StatusOK {
		t.Fatalf("commit: %d %s", res.StatusCode, out)
	}
	if _, text := h.fileText(tok, id, "data/rows.json", 0); text != data {
		t.Error("the file that was not sent again lost its bytes")
	}

	// Two paths with the same bytes are one upload.
	twice := h.mustBegin(tok, map[string]any{"id": id, "title": "Report", "baseVersion": 2, "files": []any{declared("content", page2), declared("a.json", "[1]"), declared("b.json", "[1]")}})
	if len(twice.Missing) != 2 || strings.Join(twice.Missing[0].Paths, ",") != "a.json,b.json" {
		t.Errorf("one blob under two paths: %+v", twice.Missing)
	}
}

func TestAnUploadURLWorksOnceAndOnlyForTheBytesItWasIssuedFor(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	page := multiPage
	a := h.mustBegin(tok, map[string]any{"title": "One", "files": []any{declared("content", page)}})
	url := a.Missing[0].URL

	// Same size, other bytes; a shorter body; a longer body: refused, nothing is spent.
	other := strings.Repeat("z", len(page))
	for name, body := range map[string]string{"other bytes": other, "short": page[:len(page)-1], "long": page + "x", "empty": ""} {
		res, out := h.put(url, body)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, res.StatusCode, out)
		}
	}
	if res, _ := h.commit(tok, a.Session); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("a commit after only refused uploads: %d", res.StatusCode)
	}
	// The right bytes still work, and only once.
	if res, out := h.put(url, page); res.StatusCode != http.StatusOK {
		t.Fatalf("the right bytes: %d %s", res.StatusCode, out)
	}
	if res, out := h.put(url, page); res.StatusCode != http.StatusNotFound || !strings.Contains(out, "already used") {
		t.Errorf("a second use: %d %s", res.StatusCode, out)
	}
	// Anyone holding the URL is who it is for: a token or cookie is neither needed nor looked at.
	if res, out := h.do(nil, "PUT", "/api/uploads/pup_"+strings.Repeat("0", 64), page, h.bearer("bob")); res.StatusCode != http.StatusNotFound {
		t.Errorf("a made-up URL with a token: %d %s", res.StatusCode, out)
	}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		res, out := h.do(nil, method, strings.TrimPrefix(url, h.srv.URL), "", nil)
		if res.StatusCode != http.StatusMethodNotAllowed || res.Header.Get("Allow") != "PUT" || !strings.Contains(out, "curl") {
			t.Errorf("%s on an upload URL: %d %s", method, res.StatusCode, out)
		}
	}
	if res, out := h.commit(tok, a.Session); res.StatusCode != http.StatusCreated {
		t.Fatalf("commit: %d %s", res.StatusCode, out)
	}
}

func TestASessionAndItsUploadURLsExpireAfterTenMinutes(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	h := newHarness(t, func(c *Config) { c.Clock = clock })
	tok := h.token("alice")
	a := h.mustBegin(tok, map[string]any{"title": "Slow", "files": []any{declared("content", multiPage), declared("app.js", appJS)}})
	if res, out := h.put(a.Missing[0].URL, appJS); res.StatusCode != http.StatusOK {
		t.Fatalf("an upload inside the window: %d %s", res.StatusCode, out)
	}

	clock.Advance(10*time.Minute - time.Second)
	if res, out := h.put(a.Missing[1].URL, multiPage); res.StatusCode != http.StatusOK {
		t.Fatalf("an upload just inside the window: %d %s", res.StatusCode, out)
	}
	clock.Advance(2 * time.Second)
	fresh := h.mustBegin(tok, map[string]any{"title": "Slow", "files": []any{declared("content", multiPage)}})
	if res, out := h.put(fresh.Missing[0].URL, multiPage); res.StatusCode != http.StatusOK {
		t.Fatalf("a new session is a new window: %d %s", res.StatusCode, out)
	}
	if res, out := h.commit(tok, a.Session); res.StatusCode != http.StatusNotFound || !strings.Contains(out, "expired") {
		t.Errorf("a commit after the session expired: %d %s", res.StatusCode, out)
	}
	late := h.mustBegin(tok, map[string]any{"title": "Slow", "files": []any{declared("content", multiPage)}})
	clock.Advance(11 * time.Minute)
	if res, out := h.put(late.Missing[0].URL, multiPage); res.StatusCode != http.StatusNotFound || !strings.Contains(out, "expired") {
		t.Errorf("an upload after the URL expired: %d %s", res.StatusCode, out)
	}
	if res, out := api(h, "GET", "/api/artifacts", "", tok); res.StatusCode != http.StatusOK || out != "[]\n" {
		t.Errorf("nothing was published by sessions that were never committed: %s", out)
	}
}

func TestExpiredSessionsAreSweptWithTheirBytes(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	h := newHarness(t, func(c *Config) { c.Clock = clock })
	tok := h.token("alice")
	a := h.mustBegin(tok, map[string]any{"title": "Gone", "files": []any{declared("content", multiPage)}})
	h.uploadAll(a, map[string]string{"content": multiPage})
	staged := filepath.Join(h.assetsDir, "~"+a.Session)
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("the uploaded bytes are not waiting on the disk: %v", err)
	}

	clock.Advance(11 * time.Minute)
	h.mustBegin(tok, map[string]any{"title": "Next", "files": []any{declared("content", multiPage)}})
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Errorf("the bytes of an expired session are still on the disk: %v", err)
	}
	ids, err := h.st.PublishSessionIDs(context.Background())
	if err != nil || len(ids) != 1 || ids[a.Session] {
		t.Errorf("sessions after the sweep = %v, %v; want only the new one", ids, err)
	}
}

func TestACommittedSessionLeavesNothingBehindAndCommitsOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.mustBegin(tok, map[string]any{"title": "Once", "files": []any{declared("content", multiPage)}})
	h.uploadAll(a, map[string]string{"content": multiPage})
	if res, out := h.commit(tok, a.Session); res.StatusCode != http.StatusCreated {
		t.Fatalf("commit: %d %s", res.StatusCode, out)
	}
	if res, out := h.commit(tok, a.Session); res.StatusCode != http.StatusNotFound || !strings.Contains(out, "already committed") {
		t.Errorf("a second commit: %d %s", res.StatusCode, out)
	}
	if _, err := os.Stat(filepath.Join(h.assetsDir, "~"+a.Session)); !os.IsNotExist(err) {
		t.Errorf("the staged bytes outlived the commit: %v", err)
	}
	_, list := api(h, "GET", "/api/artifacts", "", tok)
	if strings.Count(list, `"id"`) != 1 {
		t.Errorf("a repeated commit made another artifact: %s", list)
	}
}

func TestASessionIsOnlyEverTheCallersOwn(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.token("alice"), h.token("bob")
	a := h.mustBegin(alice, map[string]any{"title": "Mine", "files": []any{declared("content", multiPage)}})
	h.uploadAll(a, map[string]string{"content": multiPage})

	if res, out := h.commit(bob, a.Session); res.StatusCode != http.StatusNotFound {
		t.Errorf("another person's commit: %d %s", res.StatusCode, out)
	}
	if res, out := h.do(nil, "POST", "/api/publish-sessions/"+a.Session+"/commit", "", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("a commit without a token: %d %s", res.StatusCode, out)
	}
	if res, out := h.do(nil, "POST", "/api/publish-sessions", `{"title":"x"}`, nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("a begin without a token: %d %s", res.StatusCode, out)
	}
	if res, out := h.commit(alice, a.Session); res.StatusCode != http.StatusCreated {
		t.Fatalf("the owner's commit: %d %s", res.StatusCode, out)
	}

	// Updating needs the right to edit, as publish does; the begin says so before any upload.
	art := h.publishShared(alice, "Shared", "<p>x</p>")
	res, out := h.begin(bob, map[string]any{"id": art.ID, "title": "Shared", "files": []any{declared("content", multiPage)}})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("begin on a page bob may not edit: %d %s", res.StatusCode, out)
	}
	hidden := h.publish(alice, "Private", "<p>x</p>")
	if res, out := h.begin(bob, map[string]any{"id": hidden.ID, "title": "Private", "files": []any{declared("content", multiPage)}}); res.StatusCode != http.StatusNotFound {
		t.Errorf("begin on a page bob cannot see: %d %s", res.StatusCode, out)
	}
}

func TestAnEditorWhoLosesTheRightBeforeTheCommitCannotPublish(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.token("alice"), h.token("bob")
	art := h.publishShared(alice, "Shared", "<p>x</p>")
	if res, out := api(h, "PATCH", "/api/artifacts/"+art.ID+"/editors", `{"add":["bob"]}`, alice); res.StatusCode != http.StatusOK {
		t.Fatalf("make bob an editor: %d %s", res.StatusCode, out)
	}
	a := h.mustBegin(bob, map[string]any{"id": art.ID, "title": "Shared", "baseVersion": 1, "files": []any{declared("content", multiPage)}})
	h.uploadAll(a, map[string]string{"content": multiPage})
	if res, out := api(h, "PATCH", "/api/artifacts/"+art.ID+"/editors", `{"remove":["bob"]}`, alice); res.StatusCode != http.StatusOK {
		t.Fatalf("remove bob: %d %s", res.StatusCode, out)
	}
	if res, out := h.commit(bob, a.Session); res.StatusCode != http.StatusForbidden {
		t.Errorf("a commit after the right was taken away: %d %s", res.StatusCode, out)
	}
	if h.storedContent(art.ID) != "<p>x</p>" {
		t.Error("the page changed")
	}
}

func TestAStaleBaseVersionIsRefusedAtBeginAndAtCommitLikeAPublish(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	art := h.publish(tok, "Doc", "<p>one</p>")
	if res, out := h.update(tok, art.ID, 1, "<p>two</p>", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}

	inline, inlineOut := h.update(tok, art.ID, 1, "<p>three</p>", nil)
	res, out := h.begin(tok, map[string]any{"id": art.ID, "title": "Multi", "baseVersion": 1, "files": []any{declared("content", multiPage)}})
	if res.StatusCode != http.StatusConflict || inline.StatusCode != http.StatusConflict || out != inlineOut {
		t.Errorf("a stale begin: %d %s\nthe same publish: %d %s", res.StatusCode, out, inline.StatusCode, inlineOut)
	}

	// Someone saves between begin and commit: the commit is the same conflict, nothing is lost.
	a := h.mustBegin(tok, map[string]any{"id": art.ID, "title": "Multi", "baseVersion": 2, "files": []any{declared("content", multiPage)}})
	h.uploadAll(a, map[string]string{"content": multiPage})
	if res, out := h.update(tok, art.ID, 2, "<p>newer</p>", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	res, out = h.commit(tok, a.Session)
	if current := decode[struct {
		Error   string `json:"error"`
		Current struct {
			Content string `json:"content"`
		} `json:"current"`
	}](t, out); res.StatusCode != http.StatusConflict || !strings.Contains(current.Error, "version 3") || current.Current.Content != "<p>newer</p>" {
		t.Errorf("a commit on a stale base: %d %s", res.StatusCode, out)
	}
	if h.storedContent(art.ID) != "<p>newer</p>" {
		t.Error("a refused commit changed the page")
	}
}

func TestASessionRefusesWhatAPublishWouldRefuseBeforeAnythingIsUploaded(t *testing.T) {
	t.Parallel()
	small := files.Limits{FileBytes: 1000, PerPublish: 255, PerVersion: 511, TotalBytes: 256 << 20}
	h := newHarness(t, func(c *Config) { c.FileLimits = &small })
	tok := h.token("alice")
	big := strings.Repeat("a", 1001)

	// The file limit: word for word what publish says.
	res, out := h.begin(tok, map[string]any{"title": "x", "files": []any{declared("content", "<p>x</p>"), declared("big.js", big)}})
	inline, inlineOut := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "x", "content": "<p>x</p>", "files": map[string]string{"big.js": big}})
	if res.StatusCode != http.StatusBadRequest || inline.StatusCode != http.StatusBadRequest {
		t.Fatalf("the oversized file: begin %d %s, publish %d %s", res.StatusCode, out, inline.StatusCode, inlineOut)
	}
	if want := `file \"big.js\" is 1001 bytes; a file can be at most 1000 bytes`; !strings.Contains(out, want) || !strings.Contains(inlineOut, want) {
		t.Errorf("the two refusals differ:\nbegin   %s\npublish %s", out, inlineOut)
	}

	for name, c := range map[string]struct {
		body map[string]any
		want string
	}{
		"no page":       {map[string]any{"title": "x", "files": []any{declared("app.js", "1")}}, `path \"content\"`},
		"no title":      {map[string]any{"files": []any{declared("content", "x")}}, "title is required"},
		"bad hash":      {map[string]any{"title": "x", "files": []any{map[string]any{"path": "content", "sha256": "abc", "size": 1}}}, "64 lower-case hex"},
		"a bad path":    {map[string]any{"title": "x", "files": []any{declared("content", "x"), declared("../a.js", "1")}}, "uses .."},
		"a bad type":    {map[string]any{"title": "x", "files": []any{declared("content", "x"), declared("a.png", "1")}}, "not an accepted type"},
		"twice":         {map[string]any{"title": "x", "files": []any{declared("content", "x"), declared("a.js", "1"), declared("A.js", "2")}}, "listed twice"},
		"an empty page": {map[string]any{"title": "x", "files": []any{declared("content", "")}}, "is empty"},
		"a doc's files": {map[string]any{"title": "x", "kind": "doc", "files": []any{declared("content", "# x"), declared("a.js", "1")}}, "takes no files"},
		"a bad kind":    {map[string]any{"title": "x", "kind": "nope", "files": []any{declared("content", "x")}}, "nope"},
		"public":        {map[string]any{"title": "x", "visibility": "public", "files": []any{declared("content", "x")}}, "public links are turned off"},
		"sent+removed":  {map[string]any{"title": "x", "files": []any{declared("content", "x"), declared("a.js", "1")}, "remove": []string{"a.js"}}, "both sent and removed"},
	} {
		res, out := h.begin(tok, c.body)
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, c.want) {
			t.Errorf("%s: %d %s, want 400 with %q", name, res.StatusCode, out, c.want)
		}
	}
	_, list := api(h, "GET", "/api/artifacts", "", tok)
	if list != "[]\n" {
		t.Errorf("a refused begin published something: %s", list)
	}
}

func TestAPageOverItsKindsSizeLimitIsRefusedAtCommitWithTheSameMessageAsAnInlinePublish(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	huge := "# Big\n\n" + strings.Repeat("word ", 2<<20/5+10)

	a := h.mustBegin(tok, map[string]any{"title": "Doc", "kind": "doc", "files": []any{declared("content", huge)}})
	h.uploadAll(a, map[string]string{"content": huge})
	res, out := h.commit(tok, a.Session)
	inline, inlineOut := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "Doc", "kind": "doc", "content": huge})
	if res.StatusCode != http.StatusBadRequest || inline.StatusCode != http.StatusBadRequest {
		t.Fatalf("an oversized doc: commit %d %s, publish %d %s", res.StatusCode, out, inline.StatusCode, inlineOut)
	}
	msg := decode[map[string]string](t, out)["error"]
	inlineMsg := decode[map[string]string](t, inlineOut)["error"]
	if !strings.HasPrefix(msg, "a doc artifact can be at most 2 MB of Markdown") || !strings.HasPrefix(inlineMsg, msg) {
		t.Errorf("the refusals differ:\ncommit  %s\npublish %s", msg, inlineMsg)
	}
	// Only the inline refusal points at the way to send a large file; the one who used it is told nothing it knows.
	if strings.Contains(msg, "begin_publish") || !strings.Contains(inlineMsg, "begin_publish") || !strings.Contains(inlineMsg, "POST /api/publish-sessions") {
		t.Errorf("pointers to the session flow:\ncommit  %s\npublish %s", msg, inlineMsg)
	}
	_, list := api(h, "GET", "/api/artifacts", "", tok)
	if list != "[]\n" {
		t.Errorf("a refused commit published something: %s", list)
	}
}

func TestAPublishTooBigForTheRequestPointsToTheSessionFlow(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	res, out := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "x", "content": strings.Repeat("a", 11<<20)})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "POST /api/publish-sessions") {
		t.Errorf("an 11 MB inline publish: %d %.200s", res.StatusCode, out)
	}
}

func TestOnePersonHoldsAtMostTenSessionsAtOnce(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	h := newHarness(t, func(c *Config) { c.Clock = clock })
	tok := h.token("alice")
	body := map[string]any{"title": "x", "files": []any{declared("content", multiPage)}}
	for range 10 {
		h.mustBegin(tok, body)
	}
	res, out := h.begin(tok, body)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "10 publish sessions") {
		t.Errorf("the eleventh session: %d %s", res.StatusCode, out)
	}
	if bob := h.token("bob"); h.mustBegin(bob, body).Session == "" {
		t.Error("another person is not held back by alice's sessions")
	}
	clock.Advance(11 * time.Minute)
	h.mustBegin(tok, body)
}
