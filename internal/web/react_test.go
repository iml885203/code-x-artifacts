package web

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/jsruntime"
)

const reactSource = `import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Bar, BarChart } from "recharts";

export default function App() {
  const [n, setN] = useState(0);
  return (
    <Tabs defaultValue="a">
      <TabsList><TabsTrigger value="a">A</TabsTrigger></TabsList>
      <TabsContent value="a"><Button onClick={() => setN(n + 1)}>{n}</Button><BarChart width={200} height={100} data={[{ x: n }]}><Bar dataKey="x" /></BarChart></TabsContent>
    </Tabs>
  );
}
`

func (h *harness) publishReact(tok, source string) artifactView {
	h.t.Helper()
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"Chart","kind":"react","visibility":"organisation","content":`+jsonString(source)+`}`, tok)
	if res.StatusCode != http.StatusCreated {
		h.t.Fatalf("publish react: %d %s", res.StatusCode, out)
	}
	return decode[artifactView](h.t, out)
}

func TestPublishingReactOverHTTPAndMCPGivesTheSameResultAndTheSameRefusal(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)

	viaHTTP := h.publishReact(tok, reactSource)
	viaMCP := ag.ok("publish", map[string]any{"title": "Chart", "kind": "react", "content": reactSource})
	if viaHTTP.Kind != "react" || viaHTTP.Format != "v1" || viaMCP["kind"] != "react" || viaMCP["format"] != "v1" {
		t.Errorf("http %q/%q, mcp %v/%v, want react/v1 from both", viaHTTP.Kind, viaHTTP.Format, viaMCP["kind"], viaMCP["format"])
	}

	bad := `import { Fire } from "lucide-react"; import _ from "lodash"; export default () => <Fire n={_.size([])} />`
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"Bad","kind":"react","content":`+jsonString(bad)+`}`, tok)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("http refusal status %d: %s", res.StatusCode, out)
	}
	var httpErr struct{ Error string }
	if err := json.Unmarshal([]byte(out), &httpErr); err != nil {
		t.Fatal(err)
	}
	mcpErr := ag.refused("publish", map[string]any{"title": "Bad", "kind": "react", "content": bad})
	for _, refusal := range []string{httpErr.Error, mcpErr} {
		if !strings.Contains(refusal, `cannot import "lodash"`) || !strings.Contains(refusal, "recharts") {
			t.Errorf("the refusal does not say what to fix:\n%s", refusal)
		}
	}
	if !strings.Contains(mcpErr, httpErr.Error) {
		t.Errorf("the two entrances refuse differently:\nhttp: %s\nmcp:  %s", httpErr.Error, mcpErr)
	}
	if list := ag.ok("list", nil); len(list["artifacts"].([]any)) != 2 {
		t.Errorf("a refused publish stored something: %v", list)
	}
}

func TestAnAgentLearnsTheReactKindFromKinds(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	kinds := h.agent(h.token("alice")).ok("kinds", nil)
	for _, k := range kinds["kinds"].([]any) {
		m := k.(map[string]any)
		if m["name"] != "react" {
			continue
		}
		if m["currentFormat"] != "v1" || !strings.Contains(m["guide"].(string), "export default function App()") {
			t.Errorf("the react entry does not teach the component shape or name format v1: %v", m)
		}
		return
	}
	t.Fatalf("kinds does not list react: %v", kinds)
}

func TestAReactPageKeepsTheSandboxAndTheBridgeAndPinsItsScriptsByHash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishReact(h.token("alice"), reactSource)

	res, body := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil)
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.HasPrefix(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") {
		t.Fatalf("csp %q: the sandbox is the boundary and must be the same as every other kind's", csp)
	}
	if !strings.HasPrefix(body, "<!doctype html>") || !strings.Contains(body, "<head><script>") || !strings.Contains(body, "window.pictor") {
		t.Fatalf("the bridge is not first in the head, after the doctype: %.100s", body)
	}

	// Every inline script of the page, the bridge's included, is allowed by hash,
	// and nothing else is: a script someone else wrote into the page would not run.
	inline := regexp.MustCompile(`(?s)<script(?: type="(?:importmap|module)")?>(.*?)</script>`).FindAllStringSubmatch(body, -1)
	if len(inline) != 4 {
		t.Fatalf("the page has %d inline scripts, want the bridge, the import map, the error script and the component", len(inline))
	}
	m := regexp.MustCompile(`script-src ([^;]*)`).FindStringSubmatch(csp)
	if m == nil {
		t.Fatalf("csp %q has no script-src", csp)
	}
	allowed := strings.Fields(m[1])
	if len(allowed) != 1+len(inline) || allowed[0] != h.srv.URL+"/vendor/" {
		t.Errorf("script-src %q: want the runtime libraries' path (%s/vendor/) and one hash per inline script, the bridge included", m[1], h.srv.URL)
	}
	refuseSelf(t, m[1])
	for _, s := range inline {
		if !strings.Contains(m[1], scriptHash(s[1])) {
			t.Errorf("an inline script is not allowed by hash: %.60s", s[1])
		}
	}
	if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Errorf("csp %q loosens script-src", csp)
	}
	if !strings.HasSuffix(csp, "frame-ancestors 'self'") {
		t.Errorf("csp %q lost frame-ancestors", csp)
	}
}

func TestOnlyPagesThePlatformGeneratesGetAScriptSrc(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	bob := h.browser("bob", "pw-b")

	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"P","visibility":"organisation","content":`+jsonString(page)+`}`, tok)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish html: %d %s", res.StatusCode, out)
	}
	if csp := rawCSP(h, bob, decode[artifactView](t, out).ID); strings.Contains(csp, "script-src") {
		t.Errorf("an html page got %q: its inline scripts and handlers are the author's, so it would break", csp)
	}
	doc := h.publishDoc(tok, "# Hi\n")
	if csp := rawCSP(h, bob, doc.ID); !strings.Contains(csp, "script-src "+scriptHash(bridgeJS)+";") {
		t.Errorf("a doc page, which the platform draws, got %q", csp)
	}
}

func rawCSP(h *harness, c *http.Client, id string) string {
	h.t.Helper()
	res, _ := h.do(c, "GET", "/raw/"+id, "", nil)
	return res.Header.Get("Content-Security-Policy")
}

func TestARuntimeFileIsServedToTheSandboxWithoutASessionAndCachedForever(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	rt, err := jsruntime.Load(jsruntime.V1)
	if err != nil {
		t.Fatal(err)
	}
	for spec, m := range rt.Manifest.Specifiers {
		res, body := h.do(nil, "GET", "/vendor/v1/"+m.File, "", fromTheSandbox)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d without a session", spec, res.StatusCode)
		}
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("%s: Access-Control-Allow-Origin %q: a module script from an opaque origin is a CORS request, and its origin is null", spec, got)
		}
		if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=31536000") {
			t.Errorf("%s: Cache-Control %q", spec, cc)
		}
		if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
			t.Errorf("%s: Content-Type %q: a module with another type does not load", spec, ct)
		}
		if body == "" {
			t.Errorf("%s: empty body", spec)
		}
	}
	tw := rt.Manifest.Scripts["tailwind"]
	if res, _ := h.do(nil, "GET", "/vendor/v1/"+tw, "", fromTheSandbox); res.StatusCode != http.StatusOK || res.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("the Tailwind script: status %d", res.StatusCode)
	}
	if res, body := h.do(nil, "GET", "/vendor/v1/"+rt.Manifest.Notices, "", nil); res.StatusCode != http.StatusOK || !strings.Contains(body, "react@") {
		t.Errorf("the licence notices are not served: %d", res.StatusCode)
	}
}

func TestOnlyTheFilesOfAnEmbeddedRuntimeAreServed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, path := range []string{
		"/vendor/v2/react-FCA2SRJW.js", // a runtime this binary lacks
		"/vendor/v1/manifest.json",     // not a library
		"/vendor/v1/nothing.js",
		"/vendor/v1/..%2F..%2Fgo.mod",
		"/vendor/v1/",
		"/vendor/v1",
	} {
		if res, _ := h.do(nil, "GET", path, "", nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, res.StatusCode)
		}
	}
}

// A react component carries its file urls inside the inline module script, so
// the grant has to be in the script before its hash is taken; a hash of the
// script without the grant would block the whole page.
func TestAReactPageThatShowsAnUploadedFileStillRunsAndLoadsIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishReact(tok, reactSource)
	up := h.mustUpload(tok, a.ID, "chart.png", pngBytes)
	source := "export default function App() { return <img src=\"" + up.URL + "\" alt=\"Revenue\" />; }\n"
	if res, out := h.do(nil, "PUT", "/api/artifacts/"+a.ID, input(t, artifactInput{Title: "Chart", Content: source}), h.bearer("alice")); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}

	res, body := h.do(h.browser("alice", "pw-a"), "GET", "/raw/"+a.ID, "", nil)
	m := regexp.MustCompile(`script-src ([^;]*)`).FindStringSubmatch(res.Header.Get("Content-Security-Policy"))
	if m == nil {
		t.Fatalf("csp %q has no script-src", res.Header.Get("Content-Security-Policy"))
	}
	for _, s := range regexp.MustCompile(`(?s)<script(?: type="(?:importmap|module)")?>(.*?)</script>`).FindAllStringSubmatch(body, -1) {
		if !strings.Contains(m[1], scriptHash(s[1])) {
			t.Errorf("an inline script the page carries is not allowed by hash, so it would not run: %.80s", s[1])
		}
	}
	file := regexp.MustCompile(`(/files/` + a.ID + `/[^/"]+/chart\.png)`).FindStringSubmatch(body)
	if file == nil {
		t.Fatalf("the component's file url was not given a grant:\n%s", body)
	}
	if got, _ := h.fileStatus(nil, file[1]); got != http.StatusOK {
		t.Errorf("the component's file: %d", got)
	}
}

func TestAReactPageIsNamedByTheArtifactTitleAndTheTitleStaysText(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	title := `Q3 </title><script>alert(1)</script> & co`
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":`+jsonString(title)+`,"kind":"react","visibility":"organisation","content":`+jsonString(reactSource)+`}`, tok)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish: %d %s", res.StatusCode, out)
	}
	a := decode[artifactView](t, out)
	bob := h.browser("bob", "pw-b")

	res, body := h.do(bob, "GET", "/raw/"+a.ID, "", nil)
	want := "<title>Q3 &lt;/title&gt;&lt;script&gt;alert(1)&lt;/script&gt; &amp; co</title>"
	if !strings.Contains(body, want) {
		t.Errorf("page has no %s", want)
	}
	if strings.Contains(body, "<script>alert(1)") {
		t.Error("the title became markup")
	}

	// The title is not part of the scripts the CSP pins: two titles, one set of hashes.
	other := decode[artifactView](t, mustPublishReactTitled(h, tok, "Another name"))
	if got, want := rawCSP(h, bob, other.ID), res.Header.Get("Content-Security-Policy"); got != want {
		t.Errorf("csp depends on the title:\n%s\n%s", got, want)
	}
}

func mustPublishReactTitled(h *harness, tok, title string) string {
	h.t.Helper()
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":`+jsonString(title)+`,"kind":"react","visibility":"organisation","content":`+jsonString(reactSource)+`}`, tok)
	if res.StatusCode != http.StatusCreated {
		h.t.Fatalf("publish: %d %s", res.StatusCode, out)
	}
	return out
}

// refuseSelf fails when a script-src allows 'self': that is every script on this
// site, and an artifact's files are scripts now, so another artifact's file
// could be loaded into the page by whoever can write markup into it.
func refuseSelf(t *testing.T, scriptSrc string) {
	t.Helper()
	if strings.Contains(scriptSrc, "'self'") {
		t.Errorf("script-src %q allows 'self', which is every script on this site, other artifacts' files included", scriptSrc)
	}
}
