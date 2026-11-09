package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func newSlide(title string) map[string]any {
	return map[string]any{"title": title, "notes": "said about " + title, "elements": []any{map[string]any{"type": "text", "text": "about " + title}}}
}

// slideTitles is the slide list of a slide tool's result as "1 Q3,2 Two".
func slideTitles(t *testing.T, out map[string]any) string {
	t.Helper()
	list, ok := out["slides"].([]any)
	if !ok {
		t.Fatalf("no slide list in %v", out)
	}
	var b []string
	for _, s := range list {
		m := s.(map[string]any)
		b = append(b, fmt.Sprintf("%v %v", m["number"], m["title"]))
	}
	return strings.Join(b, ",")
}

// deckTitles reads the deck as saved and lists its slide titles.
func deckTitles(t *testing.T, ag *agent, id string) (string, float64) {
	t.Helper()
	read := ag.ok("read", map[string]any{"id": id})
	var d struct {
		Slides []struct{ Title string } `json:"slides"`
	}
	if err := json.Unmarshal([]byte(read["content"].(string)), &d); err != nil {
		t.Fatalf("the saved deck is not JSON: %v", err)
	}
	var titles []string
	for _, s := range d.Slides {
		titles = append(titles, s.Title)
	}
	return strings.Join(titles, ","), read["version"].(float64)
}

func TestEachSlideToolGivesTheExpectedDeckAndSlideListThroughTheMCPProtocol(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ag := h.agent(h.token("alice"))
	id := ag.ok("publish", map[string]any{"title": "Deck", "kind": "slides", "content": threeSlides})["id"].(string)

	got := ag.ok("get_slide", map[string]any{"id": id, "number": 2})
	if s := got["slide"].(map[string]any); s["title"] != "Two" || got["number"] != float64(2) || got["version"] != float64(1) {
		t.Errorf("get_slide 2 = %v", got)
	}
	if list := slideTitles(t, got); list != "1 Q3,2 Two,3 Three" {
		t.Errorf("get_slide lists %s", list)
	}

	steps := []struct {
		tool  string
		args  map[string]any
		list  string
		saved string
	}{
		{"replace_slide", map[string]any{"number": 2, "slide": newSlide("Second")}, "1 Q3,2 Second,3 Three", "Q3,Second,Three"},
		{"insert_slide", map[string]any{"after": 0, "slide": newSlide("Agenda")}, "1 Agenda,2 Q3,3 Second,4 Three", "Agenda,Q3,Second,Three"},
		{"insert_slide", map[string]any{"after": 4, "slide": newSlide("End")}, "1 Agenda,2 Q3,3 Second,4 Three,5 End", "Agenda,Q3,Second,Three,End"},
		{"move_slide", map[string]any{"number": 1, "to": 3}, "1 Q3,2 Second,3 Agenda,4 Three,5 End", "Q3,Second,Agenda,Three,End"},
		{"delete_slide", map[string]any{"number": 4}, "1 Q3,2 Second,3 Agenda,4 End", "Q3,Second,Agenda,End"},
	}
	for i, s := range steps {
		s.args["id"], s.args["baseVersion"] = id, i+1
		out := ag.ok(s.tool, s.args)
		if list := slideTitles(t, out); list != s.list {
			t.Errorf("%s: lists %s, want %s", s.tool, list, s.list)
		}
		if out["version"] != float64(i+2) || !strings.Contains(out["next"].(string), "version") {
			t.Errorf("%s: version %v, next %v", s.tool, out["version"], out["next"])
		}
		if saved, version := deckTitles(t, ag, id); saved != s.saved || version != float64(i+2) {
			t.Errorf("%s: the saved deck is %s at version %v, want %s", s.tool, saved, version, s.saved)
		}
	}
	if out := ag.ok("get_slide", map[string]any{"id": id, "number": 4}); out["slide"].(map[string]any)["notes"] != "said about End" {
		t.Errorf("the inserted slide did not keep what was sent: %v", out["slide"])
	}
	review := toStrings(ag.ok("delete_slide", map[string]any{"id": id, "number": 1, "baseVersion": 6})["review"])
	if !strings.Contains(strings.Join(review, "\n"), "from 4 slides to 3 slides") {
		t.Errorf("a slide edit's review = %v, want the count change a publish reports", review)
	}
}

func TestAnInvalidSlideEditIsRefusedAndLeavesTheDeckUnchanged(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)
	id := ag.ok("publish", map[string]any{"title": "Deck", "kind": "slides", "content": threeSlides})["id"].(string)
	page := ag.ok("publish", map[string]any{"title": "Page", "content": "<p>hi</p>"})["id"].(string)

	badTable := map[string]any{"title": "Bad", "elements": []any{map[string]any{"type": "table", "columns": []any{"a"}}}}
	refusals := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"replace_slide", map[string]any{"id": id, "number": 2, "slide": badTable}, `Slide 2 element 1 (table)`},
		{"insert_slide", map[string]any{"id": id, "after": 3, "slide": badTable}, `Slide 4 element 1 (table)`},
		{"insert_slide", map[string]any{"id": id, "after": 1, "slide": map[string]any{"title": "x", "elements": []any{map[string]any{"type": "video"}}}}, `Slide 2 element 1: unknown type "video"`},
		{"replace_slide", map[string]any{"id": id, "number": 1, "slide": "not a slide"}, "slide object"},
		{"replace_slide", map[string]any{"id": id, "number": 1}, `"slide"`},
		{"replace_slide", map[string]any{"id": id, "number": 1, "slide": newSlide("x"), "baseVersion": 7}, "no version 7"},
		{"replace_slide", map[string]any{"id": page, "number": 1, "slide": newSlide("x")}, "not a deck"},
		{"get_slide", map[string]any{"id": page, "number": 1}, "not a deck"},
	}
	for _, r := range refusals {
		if got := ag.refused(r.tool, r.args); !strings.Contains(got, r.want) {
			t.Errorf("%s %v: refusal %q, want it to say %q", r.tool, r.args, got, r.want)
		}
	}
	if res, body := api(h, "PUT", "/api/artifacts/"+id+"/slides/1", `{}`, tok); res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "slide is missing") {
		t.Errorf("a replace without a slide over HTTP: %d %s", res.StatusCode, body)
	}
	ag.ok("replace_slide", map[string]any{"id": id, "number": 1, "slide": newSlide("Changed"), "baseVersion": 1})
	if got := ag.refused("delete_slide", map[string]any{"id": id, "number": 1, "baseVersion": 1}); !strings.Contains(got, "now at version 2") {
		t.Errorf("a stale baseVersion: %q", got)
	}
	res, body := api(h, "PUT", "/api/artifacts/"+id+"/slides/1", `{"slide":{"title":"x","elements":[{"type":"text","text":"x"}]},"baseVersion":1}`, tok)
	if res.StatusCode != http.StatusConflict || !strings.Contains(body, `"current":{`) {
		t.Errorf("a stale baseVersion over HTTP: %d %s", res.StatusCode, body)
	}
	read := ag.ok("read", map[string]any{"id": id})
	if read["version"] != float64(2) {
		t.Errorf("a refused edit saved a version: %v", read["version"])
	}
	if titles, _ := deckTitles(t, ag, id); titles != "Changed,Two,Three" {
		t.Errorf("the deck after refusals is %s", titles)
	}
}

func TestASlideNumberOutOfRangeIsRefusedWithTheValidRangeOverHTTPAndMCP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)
	id := h.publishSlides(tok, threeSlides).ID

	for _, c := range []struct {
		tool       string
		args       map[string]any
		method     string
		path, body string
		want       string
	}{
		{"get_slide", map[string]any{"number": 4}, "GET", "/slides/4", "", "slides 1 to 3"},
		{"replace_slide", map[string]any{"number": 0, "slide": newSlide("x")}, "PUT", "/slides/0", `{"slide":{"title":"x"}}`, "slides 1 to 3"},
		{"delete_slide", map[string]any{"number": 9}, "DELETE", "/slides/9", "", "slides 1 to 3"},
		{"move_slide", map[string]any{"number": 5, "to": 1}, "POST", "/slides/5/move", `{"to":1}`, "slides 1 to 3"},
		{"move_slide", map[string]any{"number": 1, "to": 4}, "POST", "/slides/1/move", `{"to":4}`, "from 1 to 3"},
		{"insert_slide", map[string]any{"after": 4, "slide": newSlide("x")}, "POST", "/slides", `{"after":4,"slide":{"title":"x"}}`, "from 0 to 3"},
	} {
		c.args["id"] = id
		refusal := ag.refused(c.tool, c.args)
		res, body := api(h, c.method, "/api/artifacts/"+id+c.path, c.body, tok)
		var httpErr struct{ Error string }
		_ = json.Unmarshal([]byte(body), &httpErr)
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(httpErr.Error, c.want) || refusal != httpErr.Error {
			t.Errorf("%s: http %d %q, mcp %q, want both to name %q", c.tool, res.StatusCode, httpErr.Error, refusal, c.want)
		}
	}
	if _, v := deckTitles(t, ag, id); v != 1 {
		t.Errorf("a refused edit saved version %v", v)
	}
}

func TestASlideEditByANonAuthorIsForbiddenAndByWhoCannotSeeTheDeckNotFoundLikePublish(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	bob := h.agent(bobTok)
	shared := h.publishSlides(aliceTok, threeSlides).ID
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"Private","kind":"slides","content":`+jsonString(threeSlides)+`}`, aliceTok)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish: %d %s", res.StatusCode, out)
	}
	private := decode[artifactView](t, out).ID

	for _, c := range []struct {
		id     string
		status int
	}{{shared, http.StatusForbidden}, {private, http.StatusNotFound}} {
		pubRes, pubBody := api(h, "PUT", "/api/artifacts/"+c.id, `{"title":"x","content":`+jsonString(twoSlides)+`}`, bobTok)
		pubRefusal := bob.refused("publish", map[string]any{"id": c.id, "title": "x", "content": twoSlides})
		for _, e := range []struct {
			tool         string
			args         map[string]any
			method, path string
			body         string
		}{
			{"replace_slide", map[string]any{"number": 1, "slide": newSlide("x")}, "PUT", "/slides/1", `{"slide":{"title":"x","elements":[{"type":"text","text":"x"}]}}`},
			{"insert_slide", map[string]any{"after": 1, "slide": newSlide("x")}, "POST", "/slides", `{"after":1,"slide":{"title":"x","elements":[{"type":"text","text":"x"}]}}`},
			{"move_slide", map[string]any{"number": 1, "to": 2}, "POST", "/slides/1/move", `{"to":2}`},
			{"delete_slide", map[string]any{"number": 1}, "DELETE", "/slides/1", ""},
		} {
			e.args["id"] = c.id
			res, body := api(h, e.method, "/api/artifacts/"+c.id+e.path, e.body, bobTok)
			if res.StatusCode != c.status || res.StatusCode != pubRes.StatusCode || body != pubBody {
				t.Errorf("%s over HTTP: %d %s, publish: %d %s", e.tool, res.StatusCode, body, pubRes.StatusCode, pubBody)
			}
			if refusal := bob.refused(e.tool, e.args); refusal != pubRefusal {
				t.Errorf("%s over MCP: %q, publish: %q", e.tool, refusal, pubRefusal)
			}
		}
	}
	res, body := api(h, "GET", "/api/artifacts/"+private+"/slides/1", "", bobTok)
	if refusal := bob.refused("get_slide", map[string]any{"id": private, "number": 1}); res.StatusCode != http.StatusNotFound || refusal != bob.refused("read", map[string]any{"id": private}) {
		t.Errorf("get_slide of a deck bob cannot see: http %d %s, mcp %q", res.StatusCode, body, refusal)
	}
	if got := bob.ok("get_slide", map[string]any{"id": shared, "number": 1}); got["slide"].(map[string]any)["title"] != "Q3" {
		t.Errorf("a reader cannot read a slide of a deck they can see: %v", got)
	}
	if _, v := deckTitles(t, h.agent(aliceTok), shared); v != 1 {
		t.Errorf("a refused edit saved version %v", v)
	}
}

func TestASlideToolAndItsHTTPCounterpartGiveTheSameResult(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)
	viaHTTP, viaMCP := h.publishSlides(tok, threeSlides).ID, h.publishSlides(tok, threeSlides).ID
	slide := `{"title":"New","notes":"n","elements":[{"type":"text","text":"x"}]}`
	var slideValue any
	if err := jsonUnmarshal(slide, &slideValue); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		tool         string
		args         map[string]any
		method, path string
		body         string
	}{
		{"get_slide", map[string]any{"number": 3}, "GET", "/slides/3", ""},
		{"replace_slide", map[string]any{"number": 2, "slide": slideValue, "baseVersion": 1}, "PUT", "/slides/2", `{"slide":` + slide + `,"baseVersion":1}`},
		{"insert_slide", map[string]any{"after": 1, "slide": slideValue, "baseVersion": 2}, "POST", "/slides", `{"after":1,"slide":` + slide + `,"baseVersion":2}`},
		{"move_slide", map[string]any{"number": 1, "to": 4, "baseVersion": 3}, "POST", "/slides/1/move", `{"to":4,"baseVersion":3}`},
		{"delete_slide", map[string]any{"number": 2, "baseVersion": 4}, "DELETE", "/slides/2?baseVersion=4", ""},
	} {
		res, body := api(h, c.method, "/api/artifacts/"+viaHTTP+c.path, c.body, tok)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s over HTTP: %d %s", c.tool, res.StatusCode, body)
		}
		c.args["id"] = viaMCP
		want := ag.ok(c.tool, c.args)
		delete(want, "next")
		got := decode[map[string]any](t, body)
		if string(mustJSON(t, got)) != string(mustJSON(t, want)) {
			t.Errorf("%s differs:\n http: %s\n  mcp: %s", c.tool, mustJSON(t, got), mustJSON(t, want))
		}
	}
	a, _ := deckTitles(t, ag, viaHTTP)
	b, _ := deckTitles(t, ag, viaMCP)
	if a != b || a != "New,Three,Q3" {
		t.Errorf("the decks differ: http %s, mcp %s", a, b)
	}
}

func TestTheSlidesGuideTeachesTheToolsThatEditOneSlide(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, k := range h.agent(h.token("alice")).ok("kinds", nil)["kinds"].([]any) {
		m := k.(map[string]any)
		if m["name"] != "slides" {
			continue
		}
		for _, tool := range []string{"get_slide", "replace_slide", "insert_slide", "move_slide", "delete_slide"} {
			if !strings.Contains(m["guide"].(string), "`"+tool+"`") {
				t.Errorf("the slides guide does not mention %s", tool)
			}
		}
		return
	}
	t.Fatal("kinds does not list slides")
}
