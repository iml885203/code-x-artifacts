package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// A design of one page with three frames, as the files an agent sends.
const designManifest = `{"version":1,"pages":[{"id":"home","name":"Home","rows":[[` +
	`{"id":"hero","name":"Hero","file":"frames/hero.html","w":1440,"h":900},` +
	`{"id":"mobile","name":"Mobile","file":"frames/mobile.html","w":390,"h":844}],[` +
	`{"id":"footer","name":"Footer","file":"frames/footer.html","w":1440,"h":200}]]}]}`

// twoFrameManifest is the same design without its footer.
const twoFrameManifest = `{"version":1,"pages":[{"id":"home","name":"Home","rows":[[` +
	`{"id":"hero","name":"Hero","file":"frames/hero.html","w":1440,"h":900},` +
	`{"id":"mobile","name":"Mobile","file":"frames/mobile.html","w":390,"h":844}]]}]}`

func designFrames() map[string]string {
	frame := func(text string) string {
		return `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0"><h1 style="font-size:40px">` + text + `</h1></body></html>`
	}
	return map[string]string{
		"frames/hero.html":   frame("Hero"),
		"frames/mobile.html": frame("Mobile"),
		"frames/footer.html": frame("Footer"),
	}
}

// publishDesign creates a design with the three frames.
func (h *harness) publishDesign(tok string, fs map[string]string, visibility store.Visibility) artifactView {
	h.t.Helper()
	res, out := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "Landing", "kind": "design", "content": designManifest, "visibility": visibility, "files": fileMap(fs)})
	if res.StatusCode != http.StatusCreated {
		h.t.Fatalf("publish a design: %d %s", res.StatusCode, out)
	}
	return decode[artifactView](h.t, out)
}

func TestAReaderWithoutAccessGetsTheSame404ForEveryFrameFile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishDesign(tok, designFrames(), "") // private
	bob := h.browser("bob", "pw-b")
	bobTok := h.token("bob")
	_, ghost := h.do(nil, "GET", "/raw/nosuchartifact/1.1.x/frames/hero.html", "", nil)
	_, noFile := api(h, "GET", "/api/artifacts/nosuchartifact/files/frames/hero.html", "", bobTok)
	for p := range designFrames() {
		for _, c := range []*http.Client{bob, nil} {
			res, body := h.do(c, "GET", "/raw/"+a.ID+"/1.9999999999.forged/"+p, "", nil)
			if res.StatusCode != http.StatusNotFound || body != ghost {
				t.Errorf("%s with a forged grant: %d %q, want the answer for a missing artifact %q", p, res.StatusCode, body, ghost)
			}
		}
		if res, body := api(h, "GET", "/api/artifacts/"+a.ID+"/files/"+p, "", bobTok); res.StatusCode != http.StatusNotFound || body != noFile {
			t.Errorf("%s read over the API by a stranger: %d %q, want what a missing artifact gives", p, res.StatusCode, body)
		}
	}
	// A frame that is not there is the same refusal for the author, who holds a real grant.
	address := h.pageAddress(h.browser("alice", "pw-a"), a.ID, "")
	if res, body := h.do(nil, "GET", address+"frames/nothing.html", "", nil); res.StatusCode != http.StatusNotFound || body != ghost {
		t.Errorf("a frame that is not there: %d %q", res.StatusCode, body)
	}
}

func TestPublishingOneChangedFrameKeepsEveryOtherFrameByteForByte(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	v1 := h.publishDesign(tok, designFrames(), "")
	before := map[string]string{}
	for p := range designFrames() {
		_, before[p] = h.fileText(tok, v1.ID, p, 0)
	}
	changed := `<!doctype html><html><head><meta charset="utf-8"></head><body><h1 style="font-size:64px">Bigger hero</h1></body></html>`
	res, out := h.sendFiles("PUT", "/api/artifacts/"+v1.ID, tok, map[string]any{
		"title": "Landing", "content": designManifest, "baseVersion": 1, "files": fileMap(map[string]string{"frames/hero.html": changed}),
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update with one frame: %d %s", res.StatusCode, out)
	}
	v2 := decode[artifactView](t, out)
	if v2.Version != 2 || len(v2.Files) != 3 {
		t.Fatalf("after the update: version %d with %d files, want 2 with 3", v2.Version, len(v2.Files))
	}
	sums1, sums2 := map[string]string{}, map[string]string{}
	for _, f := range v1.Files {
		sums1[f.Path] = f.SHA256
	}
	for _, f := range v2.Files {
		sums2[f.Path] = f.SHA256
	}
	for p := range designFrames() {
		_, after := h.fileText(tok, v1.ID, p, 0)
		same := after == before[p] && sums2[p] == sums1[p]
		if p == "frames/hero.html" && (same || !strings.Contains(after, "Bigger hero")) {
			t.Errorf("the frame that was sent did not change: %q", after)
		}
		if p != "frames/hero.html" && !same {
			t.Errorf("%s changed though it was not sent: hash %s then %s", p, sums1[p], sums2[p])
		}
	}
	// Version 1 still has its own hero.
	if _, body := h.fileText(tok, v1.ID, "frames/hero.html", 1); body != before["frames/hero.html"] {
		t.Errorf("version 1's hero changed: %q", body)
	}
}

var cspSources = regexp.MustCompile(`([a-z-]+)((?: [^;]+)?)(?:;|$)`)

// directives splits a Content-Security-Policy into its directives.
func directives(csp string) map[string][]string {
	out := map[string][]string{}
	for _, m := range cspSources.FindAllStringSubmatch(csp, -1) {
		out[m[1]] = strings.Fields(m[2])
	}
	return out
}

func TestAFrameIsServedInAnOpaqueSandboxWhoseOnlyScriptIsTheInspectorByHash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	frames := designFrames()
	a := h.publishDesign(tok, frames, store.VisibilityOrganisation)
	address := h.pageAddress(h.browser("bob", "pw-b"), a.ID, "")
	for p := range frames {
		res, body := h.do(nil, "GET", address+p, "", nil)
		if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
			t.Fatalf("%s: %d %q", p, res.StatusCode, res.Header.Get("Content-Type"))
		}
		checkFramePolicy(t, p, directives(res.Header.Get("Content-Security-Policy")), h.srv.URL+"/files/"+a.ID+"/")
		checkFrameDocument(t, p, directives(res.Header.Get("Content-Security-Policy")), body)
		if res.Header.Get("Cache-Control") != "no-store" || res.Header.Get("Referrer-Policy") != "no-referrer" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s headers: %v", p, res.Header)
		}
	}
}

// checkFramePolicy holds a frame's policy to what a frame may have.
func checkFramePolicy(t *testing.T, p string, d map[string][]string, ownFiles string) {
	t.Helper()
	if len(d["sandbox"]) != 1 || d["sandbox"][0] != "allow-scripts" {
		t.Errorf("%s: sandbox %v, want an opaque origin that may run scripts and nothing else", p, d["sandbox"])
	}
	for name, sources := range d {
		for _, loose := range []string{"'unsafe-eval'", "allow-same-origin", "*"} {
			if slicesContain(sources, loose) {
				t.Errorf("%s: %s allows %s, which is looser than a frame may have", p, name, loose)
			}
		}
		if slicesContain(sources, "'self'") {
			t.Errorf("%s: %s allows 'self', which is every file of this site, not this artifact's", p, name)
		}
	}
	if got := d["default-src"]; len(got) != 1 || got[0] != "'none'" {
		t.Errorf("%s: default-src %v, want 'none'", p, got)
	}
	if got := d["script-src"]; len(got) != 1 || !strings.HasPrefix(got[0], "'sha256-") {
		t.Errorf("%s: script-src %v, want exactly one hash, the inspector's", p, got)
	}
	if !slicesContain(d["img-src"], "data:") || !slicesContain(d["img-src"], ownFiles) {
		t.Errorf("%s: img-src %v, want data: and this artifact's own files", p, d["img-src"])
	}
}

// checkFrameDocument checks that the one script the policy allows is in the
// document exactly once, and that the bridge is not.
func checkFrameDocument(t *testing.T, p string, d map[string][]string, body string) {
	t.Helper()
	matching := 0
	for _, sc := range regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindAllStringSubmatch(body, -1) {
		if len(d["script-src"]) == 1 && scriptHash(sc[1]) == d["script-src"][0] {
			matching++
		}
	}
	if matching != 1 {
		t.Errorf("%s: %d scripts in the document match the policy's hash, want the inspector once", p, matching)
	}
	if strings.Contains(body, "window.pictor") {
		t.Errorf("%s carries the bridge: a frame has no pictor to talk to", p)
	}
}

func slicesContain(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestTheEntryPageOfADesignIsADocumentWithTheBridgeAndNotAFrame(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishDesign(tok, designFrames(), store.VisibilityOrganisation)
	address := h.pageAddress(h.browser("bob", "pw-b"), a.ID, "")
	res, body := h.do(nil, "GET", address, "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "window.pictor") {
		t.Fatalf("the entry page of a design: %d, want a document with the bridge: %.200s", res.StatusCode, body)
	}
	if got := directives(res.Header.Get("Content-Security-Policy"))["sandbox"]; len(got) < 2 {
		t.Errorf("the entry page's sandbox %v is the frame's, it should be the page's", got)
	}
}

func TestADesignTakesFrameFilesOnlyAndSaysWhichTypes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	res, out := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "x", "kind": "design", "content": designManifest, "files": map[string]any{"frames/hero.js": "alert(1)"}})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "frames/hero.js") || !strings.Contains(out, ".html") {
		t.Errorf("a script file in a design: %d %s", res.StatusCode, out)
	}
}

func TestAnAgentPublishesTwoFramesThroughMCPAndReadsThemBack(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	agent := h.agent(h.token("alice"))
	frames := designFrames()
	made := agent.ok("publish", map[string]any{
		"title": "Landing", "kind": "design", "content": twoFrameManifest,
		"files": map[string]any{"frames/hero.html": frames["frames/hero.html"], "frames/mobile.html": frames["frames/mobile.html"]},
	})
	id := made["id"].(string)
	if made["kind"] != "design" || len(made["files"].([]any)) != 2 {
		t.Fatalf("publish = %v", made)
	}
	rewritten, _ := made["rewritten"].(map[string]any)
	one := agent.ok("read", map[string]any{"id": id, "file": "frames/mobile.html"})
	text := one["file"].(map[string]any)["text"]
	if text != rewritten["frames/mobile.html"] || !strings.Contains(text.(string), `data-pid="e`) {
		t.Errorf("read of one frame is %v, the publish said it stored %v", text, rewritten["frames/mobile.html"])
	}
	// One frame changed: the other is carried over untouched.
	updated := agent.ok("publish", map[string]any{"id": id, "title": "Landing", "content": twoFrameManifest, "baseVersion": 1, "files": map[string]any{"frames/hero.html": "<p>new"}})
	if len(updated["files"].([]any)) != 2 {
		t.Fatalf("update = %v", updated)
	}
	other := agent.ok("read", map[string]any{"id": id, "file": "frames/mobile.html"})
	if f, _ := other["file"].(map[string]any); f["text"] != text {
		t.Errorf("the frame that was not sent changed: %v", other)
	}
}

func TestADesignPublishIsRefusedWholeWithEveryProblemAndLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishDesign(tok, designFrames(), "")
	bad := map[string]any{"frames/hero.html": `<script>1</script><p onclick="x()">a</p>`}
	res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, map[string]any{"title": "Landing", "content": designManifest, "baseVersion": 1, "files": bad})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("a frame with a script: %d %s", res.StatusCode, out)
	}
	for _, want := range []string{`Page \"Home\" frame \"Hero\" element 1: tag script is not allowed`, "attribute onclick is not allowed", "nothing was stored"} {
		if !strings.Contains(out, want) {
			t.Errorf("the refusal never says %q: %s", want, out)
		}
	}
	if got := versionsOf(t, h, tok, a.ID); len(got) != 1 {
		t.Errorf("a refused publish saved a version: %+v", got)
	}
	// A manifest naming a frame that has no file is refused too.
	res, out = h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "x", "kind": "design", "content": designManifest, "files": fileMap(map[string]string{"frames/hero.html": "<p>x"})})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "file frames/mobile.html is not among this design's files") {
		t.Errorf("a missing frame file: %d %s", res.StatusCode, out)
	}
}

func TestHTTPAndMCPGiveTheSameDesignResultAndTheSameRefusal(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	frames := map[string]any{"frames/hero.html": designFrames()["frames/hero.html"], "frames/mobile.html": designFrames()["frames/mobile.html"]}
	viaMCP := agent.ok("publish", map[string]any{"title": "Landing", "kind": "design", "content": twoFrameManifest, "files": frames})
	res, out := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "Landing", "kind": "design", "content": twoFrameManifest, "files": frames})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish over http: %d %s", res.StatusCode, out)
	}
	viaHTTP := decode[map[string]any](t, out)
	same := func(key string) {
		t.Helper()
		if got, want := string(mustJSON(t, viaMCP[key])), string(mustJSON(t, viaHTTP[key])); got != want {
			t.Errorf("%s: tool %s, http %s", key, got, want)
		}
	}
	same("rewritten")
	same("review")
	same("files")
	for _, p := range []string{"frames/hero.html", "frames/mobile.html"} {
		if viaMCP["rewritten"].(map[string]any)[p] == nil {
			t.Errorf("the publish did not return %s as stored", p)
		}
	}
	bad := map[string]any{"title": "x", "kind": "design", "content": twoFrameManifest, "files": map[string]any{"frames/hero.html": `<iframe src="#a"></iframe>`, "frames/mobile.html": "<p>x"}}
	refusal := agent.refused("publish", bad)
	res, out = h.sendFiles("POST", "/api/artifacts", tok, bad)
	if res.StatusCode != http.StatusBadRequest || decode[map[string]string](t, out)["error"] != refusal {
		t.Errorf("one mistake, two answers: tool %q, http %d %s", refusal, res.StatusCode, out)
	}
}

func TestAFrameThatCameBackNormalisedIsNotRewrittenWhenSentAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	agent := h.agent(h.token("alice"))
	made := agent.ok("publish", map[string]any{"title": "Landing", "kind": "design", "content": twoFrameManifest, "files": map[string]any{"frames/hero.html": "<h1>Hero</h1>", "frames/mobile.html": "<h1>Mobile</h1>"}})
	stored := made["rewritten"].(map[string]any)["frames/hero.html"].(string)
	again := agent.ok("publish", map[string]any{"id": made["id"], "title": "Landing", "content": twoFrameManifest, "baseVersion": 1, "files": map[string]any{"frames/hero.html": stored}})
	if again["rewritten"] != nil {
		t.Errorf("sending the stored form back rewrote it: %v", again["rewritten"])
	}
	if f := again["files"].([]any); f[0].(map[string]any)["sha256"] != sha(stored) {
		t.Errorf("the stored bytes are not the ones that were sent")
	}
}

func TestPublishingADesignReturnsReviewNotesForWhatTheAgentCannotSee(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	agent := h.agent(h.token("alice"))
	made := agent.ok("publish", map[string]any{"title": "Landing", "kind": "design", "content": twoFrameManifest, "files": map[string]any{
		"frames/hero.html":   `<img src="data:image/png;base64,AA"><p>text</p>`,
		"frames/mobile.html": `<div style="width:10px"></div>`,
		"frames/extra.html":  `<p>unused</p>`,
	}})
	notes := strings.Join(anyStrings(made["review"]), "\n")
	for _, want := range []string{`Page "Home" frame "Hero": img data-pid`, "has no alt text", `frame "Mobile": the frame has no text`, "frames/extra.html is not in design.json"} {
		if !strings.Contains(notes, want) {
			t.Errorf("review lacks %q: %s", want, notes)
		}
	}
	if !strings.Contains(made["next"].(string), "Review before you tell the person it is done") {
		t.Errorf("next does not point at the review: %v", made["next"])
	}
}

func anyStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestADesignRestoresAndCopiesWithItsFramesAndTheOverviewDrawsThem(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishDesign(tok, designFrames(), store.VisibilityOrganisation)
	res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, map[string]any{"title": "Landing", "content": designManifest, "baseVersion": 1, "files": fileMap(map[string]string{"frames/hero.html": "<h1>Two</h1>"})})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	if res, out := api(h, "POST", "/api/artifacts/"+a.ID+"/versions/1/restore", "", tok); res.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d %s", res.StatusCode, out)
	}
	_, hero := h.fileText(tok, a.ID, "frames/hero.html", 0)
	_, first := h.fileText(tok, a.ID, "frames/hero.html", 1)
	if hero != first {
		t.Errorf("the restored hero is %q, version 1 had %q", hero, first)
	}
	bobTok := h.token("bob")
	res, out = h.copyOf(bobTok, a.ID, 0)
	c := decode[artifactView](t, out)
	if res.StatusCode != http.StatusCreated || c.Kind != "design" || len(c.Files) != 3 {
		t.Fatalf("copy of a design: %d %s", res.StatusCode, out)
	}
	address := h.pageAddress(h.browser("bob", "pw-b"), c.ID, "")
	_, page := h.do(nil, "GET", address, "", nil)
	for _, want := range []string{`src="frames/hero.html"`, `src="frames/mobile.html"`, `src="frames/footer.html"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the overview of the copy lacks %s", want)
		}
	}
	if res, _ := h.do(nil, "GET", address+"frames/hero.html", "", nil); res.StatusCode != http.StatusOK {
		t.Errorf("a frame of the copy: %d", res.StatusCode)
	}
}
