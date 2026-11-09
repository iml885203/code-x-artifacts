package web

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

const threeSlides = `{"version":1,"slides":[
 {"layout":"title","title":"Q3","notes":"hi","elements":[{"type":"text","text":"review"}]},
 {"title":"Two","notes":"","elements":[{"type":"bullets","items":["a","b"]}]},
 {"title":"Three","notes":"bye","elements":[{"type":"quote","text":"q"}]}]}`

const twoSlides = `{"version":1,"slides":[
 {"layout":"title","title":"Q3","notes":"hi","elements":[{"type":"text","text":"review"}]},
 {"title":"Two","notes":"x","elements":[{"type":"bullets","items":["a","b"]}]}]}`

func (h *harness) publishSlides(tok, deck string) artifactView {
	h.t.Helper()
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"Deck","kind":"slides","visibility":"organisation","content":`+jsonString(deck)+`}`, tok)
	if res.StatusCode != http.StatusCreated {
		h.t.Fatalf("publish slides: %d %s", res.StatusCode, out)
	}
	return decode[artifactView](h.t, out)
}

func TestASlidesPageKeepsTheSandboxAndTheBridgeAndPinsItsOneScriptByHash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishSlides(h.token("alice"), threeSlides)
	if a.Kind != "slides" || a.Format != "1" {
		t.Fatalf("kind %q format %q", a.Kind, a.Format)
	}
	res, body := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil)
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.HasPrefix(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") || strings.Contains(csp, "unsafe-") {
		t.Fatalf("csp %q: the sandbox must be the same as every other kind's and script-src must not be loosened", csp)
	}
	if !strings.HasPrefix(body, "<!doctype html>") || !strings.Contains(body, "<head><script>") {
		t.Fatalf("the bridge is not first in the head, after the doctype: %.100s", body)
	}
	inline := regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindAllStringSubmatch(body, -1)
	if len(inline) != 2 { // the bridge and the player
		t.Fatalf("the page has %d inline scripts, want the bridge and the player", len(inline))
	}
	m := regexp.MustCompile(`script-src ([^;]*)`).FindStringSubmatch(csp)
	if m == nil || len(strings.Fields(m[1])) != len(inline) || strings.Contains(m[1], "://") {
		t.Fatalf("csp %q: want one hash per inline script, the bridge's included, and no host: a deck loads no script of the site's", csp)
	}
	refuseSelf(t, m[1])
	for _, s := range inline {
		if !strings.Contains(m[1], scriptHash(s[1])) {
			t.Errorf("an inline script is not allowed by hash: %.60s", s[1])
		}
	}
	if !strings.Contains(body, "Slide 3 of 3") {
		t.Error("the deck page does not hold every slide")
	}
}

func TestAHostileDeckStaysInertAndIsNotGivenAScriptItCouldRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	evil := `</script><script>window.pwned=1</script><img src=x onerror=alert(1)>`
	deck := `{"version":1,"slides":[{"title":` + jsonString(evil) + `,"notes":` + jsonString(evil) + `,"elements":[{"type":"text","text":` + jsonString(evil) + `}]}]}`
	a := h.publishSlides(h.token("alice"), deck)
	_, body := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil)
	if n := strings.Count(body, "<script"); n != 2 {
		t.Errorf("%d script elements, want the bridge and the player only", n)
	}
	if strings.Contains(body, "<img") || strings.Contains(body, "window.pwned=1</script>") {
		t.Error("deck text reached the page as markup")
	}
}

func TestPublishingADeckOverHTTPAndMCPGivesTheSameRefusalNamingTheSlideAndElement(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	bad := `{"version":1,"slides":[{"title":"ok","elements":[{"type":"text","text":"x"}]},{"title":"b","elements":[{"type":"text","text":"x"},{"type":"table","columns":["a"]}]}]}`
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"Bad","kind":"slides","content":`+jsonString(bad)+`}`, tok)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d: %s", res.StatusCode, out)
	}
	var httpErr struct{ Error string }
	if err := json.Unmarshal([]byte(out), &httpErr); err != nil {
		t.Fatal(err)
	}
	mcpErr := h.agent(tok).refused("publish", map[string]any{"title": "Bad", "kind": "slides", "content": bad})
	for _, refusal := range []string{httpErr.Error, mcpErr} {
		if !strings.Contains(refusal, "Slide 2 element 2 (table)") || !strings.Contains(refusal, `"rows"`) {
			t.Errorf("the refusal does not say what to fix:\n%s", refusal)
		}
	}
	if !strings.Contains(mcpErr, httpErr.Error) {
		t.Errorf("the two entrances refuse differently:\nhttp: %s\nmcp:  %s", httpErr.Error, mcpErr)
	}
}

func TestARepublishedDeckReportsTheSlideCountItChangedAndWhatTheAgentCannotSee(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)

	made := ag.ok("publish", map[string]any{"title": "Deck", "kind": "slides", "content": threeSlides})
	id := made["id"].(string)
	review, _ := made["review"].([]any)
	if len(review) != 1 || !strings.Contains(review[0].(string), "without speaker notes: 2") {
		t.Errorf("a new deck's review = %v, want the slide without notes", review)
	}
	if !strings.Contains(made["next"].(string), "Review before you tell the person it is done") {
		t.Errorf("next does not send the agent to the review: %v", made["next"])
	}

	updated := ag.ok("publish", map[string]any{"id": id, "title": "Deck", "content": twoSlides, "baseVersion": 1})
	if got := strings.Join(toStrings(updated["review"]), "\n"); !strings.Contains(got, "from 3 slides to 2 slides") {
		t.Errorf("the MCP result does not report the count change: %q", got)
	}

	res, out := putJSON(h, "PUT", "/api/artifacts/"+id, `{"title":"Deck","content":`+jsonString(threeSlides)+`,"baseVersion":2}`, tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	if v := decode[artifactView](t, out); !strings.Contains(strings.Join(v.Review, "\n"), "from 2 slides to 3 slides") {
		t.Errorf("the HTTP response does not report the count change: %v", v.Review)
	}

	// Looking at the artifact later is not a publish: no review.
	_, got := putJSON(h, "GET", "/api/artifacts/"+id, "", tok)
	if strings.Contains(got, `"review"`) {
		t.Errorf("a read carries a review: %s", got)
	}
}

func toStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, len(list))
	for i, x := range list {
		out[i], _ = x.(string)
	}
	return out
}

func TestAnAgentLearnsTheSlidesKindFromKinds(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, k := range h.agent(h.token("alice")).ok("kinds", nil)["kinds"].([]any) {
		m := k.(map[string]any)
		if m["name"] != "slides" {
			continue
		}
		if m["currentFormat"] != "1" || !strings.Contains(m["guide"].(string), "6 lines") {
			t.Errorf("the slides entry does not name format 1 or teach the capacity: %v", m)
		}
		return
	}
	t.Fatal("kinds does not list slides")
}
