package web

import (
	"encoding/base64"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// twoVersionDoc is a doc the organisation can read, saved twice by alice.
func twoVersionDoc(t *testing.T, h *harness, tok string) string {
	t.Helper()
	res, out := api(h, "POST", "/api/artifacts", `{"title":"Plan one","kind":"doc","visibility":"organisation","content":"Version one words."}`, tok)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish: %d %s", res.StatusCode, out)
	}
	id := decode[artifactView](t, out).ID
	if res, out := api(h, "PUT", "/api/artifacts/"+id, `{"title":"Plan two","content":"Version two words.","baseVersion":1}`, tok); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	return id
}

func downloadName(t *testing.T, res response) string {
	t.Helper()
	_, params, err := mime.ParseMediaType(res.Header.Get("Content-Disposition"))
	if err != nil {
		t.Fatalf("Content-Disposition %q: %v", res.Header.Get("Content-Disposition"), err)
	}
	return params["filename"]
}

func TestExportingASavedVersionDownloadsThatVersionAndOnlyThoseWhoReadHistoryGetAnOlderOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok, malloryTok := h.token("alice"), h.token("bob"), h.token("mallory")
	id := twoVersionDoc(t, h, aliceTok)
	export := func(tok, query string) (response, string) {
		return api(h, "GET", "/api/artifacts/"+id+"/export/md"+query, "", tok)
	}

	res, body := export(aliceTok, "?version=1")
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "Version one words.") || strings.Contains(body, "Version two") {
		t.Fatalf("version 1: %d %q", res.StatusCode, body)
	}
	if got := downloadName(t, res); got != "Plan one.md" {
		t.Errorf("version 1 is saved as %q, want the title it had then", got)
	}
	if res, body := export(aliceTok, ""); res.StatusCode != http.StatusOK || !strings.Contains(body, "Version two words.") || downloadName(t, res) != "Plan two.md" {
		t.Errorf("no version: %d %q, want the newest", res.StatusCode, body)
	}

	// A reader is refused an older version as when reading it, and keeps the newest.
	res, body = export(bobTok, "?version=1")
	history, historyBody := api(h, "GET", "/api/artifacts/"+id+"/versions/1", "", bobTok)
	if res.StatusCode != http.StatusForbidden || res.StatusCode != history.StatusCode || !strings.Contains(body, "only the author and editors can read this artifact's older versions") {
		t.Errorf("a reader's older version: %d %q, want what reading it says: %d %q", res.StatusCode, body, history.StatusCode, historyBody)
	}
	if res, body := export(bobTok, "?version=2"); res.StatusCode != http.StatusOK || !strings.Contains(body, "Version two words.") {
		t.Errorf("a reader's newest version by number: %d %q", res.StatusCode, body)
	}

	checkExportVersionRefusals(t, h, aliceTok, malloryTok, id)
}

// checkExportVersionRefusals: someone who cannot see an artifact learns nothing,
// whatever version they ask for; its author is told what is wrong with a version.
func checkExportVersionRefusals(t *testing.T, h *harness, aliceTok, malloryTok, id string) {
	t.Helper()
	missing, missingBody := api(h, "GET", "/api/artifacts/nosuchartifact0000/export/md?version=1", "", malloryTok)
	_, out := api(h, "POST", "/api/artifacts", `{"title":"Hidden","kind":"doc","content":"Private."}`, aliceTok)
	private := decode[artifactView](t, out)
	if private.Visibility != store.VisibilityPrivate {
		t.Fatalf("the hidden doc is %q", private.Visibility)
	}
	for _, query := range []string{"?version=1", "?version=x"} {
		res, body := api(h, "GET", "/api/artifacts/"+private.ID+"/export/md"+query, "", malloryTok)
		if res.StatusCode != http.StatusNotFound || strings.ReplaceAll(body, private.ID, "ID") != strings.ReplaceAll(missingBody, "nosuchartifact0000", "ID") {
			t.Errorf("a hidden doc %s: %d %q, want %d %q", query, res.StatusCode, body, missing.StatusCode, missingBody)
		}
	}

	for query, want := range map[string]string{"?version=99": "no version 99", "?version=0": "number from 1", "?version=x": "number from 1"} {
		if res, body := api(h, "GET", "/api/artifacts/"+id+"/export/md"+query, "", aliceTok); res.StatusCode != http.StatusBadRequest || !strings.Contains(body, want) {
			t.Errorf("%s: %d %q, want a 400 saying %q", query, res.StatusCode, body, want)
		}
	}
}

func TestADesignsOlderVersionExportsTheFramesItHadThen(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	frames := designFrames()
	frames["frames/hero.html"] = `<body><h1>Old hero.</h1></body>`
	a := h.publishDesign(tok, frames, "")
	if res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, map[string]any{"title": "Landing", "content": designManifest, "baseVersion": 1, "files": fileMap(map[string]string{"frames/hero.html": `<body><h1>New hero.</h1></body>`})}); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	for version, want := range map[string]string{"1": "Old hero.", "2": "New hero."} {
		res, body := api(h, "GET", "/api/artifacts/"+a.ID+"/export/zip?version="+version, "", tok)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("version %s: %d %s", version, res.StatusCode, body)
		}
		if got := zipOf(t, body)["frames/hero.html"]; !strings.Contains(got, want) {
			t.Errorf("version %s's hero frame: %s", version, got)
		}
	}
}

func TestTheExportToolDownloadsTheSameFileAsTheExportRouteAndRefusesTheSameViewers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok, malloryTok := h.token("alice"), h.token("bob"), h.token("mallory")
	alice, bob, mallory := h.agent(aliceTok), h.agent(bobTok), h.agent(malloryTok)
	id := twoVersionDoc(t, h, aliceTok)
	checkExportToolFiles(t, h, alice, aliceTok, id)

	refusalOf := func(tok string, query string) string {
		_, out := api(h, "GET", "/api/artifacts/"+id+"/export/"+query, "", tok)
		return decode[map[string]string](t, out)["error"]
	}
	if refusal := bob.refused("export", map[string]any{"id": id, "format": "md", "version": 1}); !strings.HasPrefix(refusal, refusalOf(bobTok, "md?version=1")) || !strings.Contains(refusal, "only the author and editors") {
		t.Errorf("a reader's older version: %q", refusal)
	}
	if got := bob.ok("export", map[string]any{"id": id, "format": "md"}); !strings.Contains(got["text"].(string), "Version two words.") {
		t.Errorf("a reader's newest: %v", got)
	}
	if refusal := alice.refused("export", map[string]any{"id": id, "format": "pdf"}); refusal != refusalOf(aliceTok, "pdf") {
		t.Errorf("pdf: tool %q, HTTP %q", refusal, refusalOf(aliceTok, "pdf"))
	}
	if refusal := alice.refused("export", map[string]any{"id": id, "format": "md", "version": 99}); refusal != refusalOf(aliceTok, "md?version=99") {
		t.Errorf("a missing version: tool %q, HTTP %q", refusal, refusalOf(aliceTok, "md?version=99"))
	}
	private := alice.ok("publish", map[string]any{"title": "Hidden", "kind": "doc", "content": "Private."})["id"].(string)
	if got, want := mallory.refused("export", map[string]any{"id": private, "format": "md", "version": 1}), mallory.refused("export", map[string]any{"id": "nosuchartifact0000", "format": "md", "version": 1}); strings.ReplaceAll(got, private, "ID") != strings.ReplaceAll(want, "nosuchartifact0000", "ID") {
		t.Errorf("a hidden doc: %q, want what a missing one says: %q", got, want)
	}
}

// checkExportToolFiles: the tool hands over the bytes, name and type the route
// does, as text for a text format and base64 for any other.
func checkExportToolFiles(t *testing.T, h *harness, alice *agent, aliceTok, id string) {
	t.Helper()
	res, body := api(h, "GET", "/api/artifacts/"+id+"/export/md?version=1", "", aliceTok)
	got := alice.ok("export", map[string]any{"id": id, "format": "md", "version": 1})
	if res.StatusCode != http.StatusOK || got["text"] != body || got["name"] != downloadName(t, res) || got["contentType"] != res.Header.Get("Content-Type") ||
		got["version"] != float64(1) || got["size"] != float64(len(body)) || !strings.HasSuffix(got["url"].(string), "/api/artifacts/"+id+"/export/md?version=1") {
		t.Errorf("export md v1: tool %v, HTTP %d %q", got, res.StatusCode, body)
	}

	res, body = api(h, "GET", "/api/artifacts/"+id+"/export/docx", "", aliceTok)
	got = alice.ok("export", map[string]any{"id": id, "format": "docx"})
	decoded, err := base64.StdEncoding.DecodeString(got["base64"].(string))
	if err != nil || res.StatusCode != http.StatusOK || string(decoded) != body || got["version"] != float64(2) || got["text"] != nil {
		t.Errorf("export docx: tool %v (%v), HTTP %d", got["name"], err, res.StatusCode)
	}
}

func TestTheExportToolHandsOverAFileOverTheMCPLimitOnlyAsItsURLAndOneUnderItInline(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withSmallUploadLimit)
	tok := h.token("alice")
	alice := h.agent(tok)
	words := strings.Repeat("Many words in a long paragraph. ", smallUploadLimit/32+64)
	big := alice.ok("publish", map[string]any{"title": "Long", "kind": "doc", "content": words})["id"].(string)
	small := alice.ok("publish", map[string]any{"title": "Short", "kind": "doc", "content": "Few words."})["id"].(string)

	res, body := api(h, "GET", "/api/artifacts/"+big+"/export/md", "", tok)
	got := alice.ok("export", map[string]any{"id": big, "format": "md"})
	if res.StatusCode != http.StatusOK || len(body) <= smallUploadLimit || got["size"] != float64(len(body)) || got["name"] != "Long.md" || got["version"] != float64(1) {
		t.Fatalf("over the limit: tool %v, HTTP %d with %d bytes", got, res.StatusCode, len(body))
	}
	checkNotInline(t, got, "/api/artifacts/"+big+"/export/md?version=1")

	_, body = api(h, "GET", "/api/artifacts/"+small+"/export/md", "", tok)
	if got := alice.ok("export", map[string]any{"id": small, "format": "md"}); got["text"] != body || got["note"] != nil {
		t.Errorf("under the limit: %v, want the file inline", got)
	}
}

// checkNotInline: a file over the limit is named and addressed, never inline
// or cut short, and the note says how to get it.
func checkNotInline(t *testing.T, got map[string]any, path string) {
	t.Helper()
	for _, field := range []string{"text", "base64"} {
		if _, ok := got[field]; ok {
			t.Errorf("a file over the limit came back inline as %s", field)
		}
	}
	if url, _ := got["url"].(string); !strings.HasSuffix(url, path) {
		t.Errorf("url = %v, want it to end in %s", got["url"], path)
	}
	if note, _ := got["note"].(string); !strings.Contains(note, "64 KB") || !strings.Contains(note, "url") || !strings.Contains(note, "token") {
		t.Errorf("note = %q, want it to say the limit and to download url with the token", note)
	}
}
