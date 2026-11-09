package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
)

// boardFlow is a board an agent writes: two shapes, a label in the first and
// an arrow bound to both. The label has no position; its shape gives it one.
const boardFlow = `{"version": 1, "elements": [
  {"id": "a", "type": "rectangle", "x": 0, "y": 0, "width": 200, "height": 100, "index": "a0",
   "boundElements": [{"id": "la", "type": "text"}, {"id": "ar", "type": "arrow"}]},
  {"id": "la", "type": "text", "width": 60, "height": 25, "text": "Login 登入", "originalText": "Login 登入",
   "containerId": "a", "textAlign": "center", "verticalAlign": "middle", "index": "a1"},
  {"id": "b", "type": "rectangle", "x": 300, "y": 0, "width": 120, "height": 60, "index": "a2", "boundElements": [{"id": "ar", "type": "arrow"}]},
  {"id": "ar", "type": "arrow", "x": 205, "y": 50, "points": [[0, 0], [90, -20]], "index": "a3", "endArrowhead": "arrow",
   "startBinding": {"elementId": "a", "focus": 0, "gap": 5}, "endBinding": {"elementId": "b", "focus": 0, "gap": 5}}
]}`

func boardElements(t *testing.T, content string) map[string]boardfmt.Element {
	t.Helper()
	b, err := boardfmt.Parse(content)
	if err != nil {
		t.Fatalf("%v: %s", err, content)
	}
	out := map[string]boardfmt.Element{}
	for _, el := range b.Elements {
		out[el.ID()] = el
	}
	return out
}

func TestAnAgentLearnsTheBoardKindFromKindsPublishesABoardAndReadsTheSameElementsBack(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ag := h.agent(h.token("alice"))

	guide := boardGuideFromKinds(t, ag.ok("kinds", nil))
	for _, want := range []string{"containerId", "startBinding", "boundElements", "https:"} {
		if !strings.Contains(guide, want) {
			t.Errorf("the board guide does not teach %s", want)
		}
	}

	out := ag.ok("publish", map[string]any{"title": "Login flow", "kind": "board", "content": boardFlow})
	if out["kind"] != "board" || out["format"] != "1" {
		t.Errorf("publish result: %v", out)
	}
	read := ag.ok("read", map[string]any{"id": out["id"]})
	got := boardElements(t, read["content"].(string))
	sameFieldsBesidesPosition(t, boardElements(t, boardFlow), got)
	if la := got["la"]; la["x"] != 70.0 || la["y"] != 37.5 {
		t.Errorf("the label is at %v,%v, not centred on its shape", la["x"], la["y"])
	}

	res, page := h.do(h.browser("alice", "pw-a"), "GET", "/raw/"+out["id"].(string), "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(page, "<svg") || !strings.Contains(page, ">Login 登入</tspan>") {
		t.Errorf("/raw does not draw the board: %d %s", res.StatusCode, page)
	}
	res, svg := putJSON(h, "GET", "/api/artifacts/"+out["id"].(string)+"/export/svg", "", h.token("alice"))
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/svg+xml" || !strings.Contains(res.Header.Get("Content-Disposition"), "Login flow.svg") || !strings.Contains(svg, "Login 登入") {
		t.Errorf("export svg: %d %q %q", res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("Content-Disposition"))
	}
	exported := ag.ok("export", map[string]any{"id": out["id"], "format": "svg"})
	if !strings.Contains(fmt.Sprint(exported), "svg") {
		t.Errorf("an agent's export: %v", exported)
	}
}

// boardGuideFromKinds is the board entry's guide in a kinds result; the entry
// must name format 1.
func boardGuideFromKinds(t *testing.T, kinds map[string]any) string {
	t.Helper()
	for _, k := range kinds["kinds"].([]any) {
		if m := k.(map[string]any); m["name"] == "board" {
			if m["currentFormat"] != "1" {
				t.Errorf("board's current format is %v", m["currentFormat"])
			}
			guide, _ := m["guide"].(string)
			return guide
		}
	}
	t.Fatal("kinds does not list board")
	return ""
}

// sameFieldsBesidesPosition fails for a field sent that reads back different;
// x and y may move, a label's are worked out from its shape.
func sameFieldsBesidesPosition(t *testing.T, sent, got map[string]boardfmt.Element) {
	t.Helper()
	if len(got) != len(sent) {
		t.Fatalf("read gives %d elements, %d were sent", len(got), len(sent))
	}
	for id, el := range sent {
		for field, value := range el {
			if field == "x" || field == "y" {
				continue
			}
			a, _ := json.Marshal(value)
			b, _ := json.Marshal(got[id][field])
			if string(a) != string(b) {
				t.Errorf("%s.%s reads back as %s, was %s", id, field, b, a)
			}
		}
	}
}

func TestPublishingABoardOverHTTPAndMCPGivesTheSameResultAndTheSameRefusalNamingTheElementAndField(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)

	res, body := putJSON(h, "POST", "/api/artifacts", `{"title":"Flow","kind":"board","content":`+jsonString(boardFlow)+`}`, tok)
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		t.Fatalf("HTTP publish: %d %s", res.StatusCode, body)
	}
	var viaHTTP struct{ ID string }
	if err := json.Unmarshal([]byte(body), &viaHTTP); err != nil {
		t.Fatal(err)
	}
	viaMCP := ag.ok("publish", map[string]any{"title": "Flow", "kind": "board", "content": boardFlow})
	a := ag.ok("read", map[string]any{"id": viaHTTP.ID})["content"]
	b := ag.ok("read", map[string]any{"id": viaMCP["id"]})["content"]
	if a != b {
		t.Errorf("one board is stored two ways:\nhttp %s\nmcp  %s", a, b)
	}

	bad := strings.Replace(boardFlow, `"index": "a2",`, `"index": "a2", "link": "javascript:alert(1)",`, 1)
	res, body = putJSON(h, "POST", "/api/artifacts", `{"title":"Bad","kind":"board","content":`+jsonString(bad)+`}`, tok)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("HTTP publish of a bad board: %d %s", res.StatusCode, body)
	}
	var httpErr struct{ Error string }
	if err := json.Unmarshal([]byte(body), &httpErr); err != nil {
		t.Fatal(err)
	}
	mcpErr := ag.refused("publish", map[string]any{"title": "Bad", "kind": "board", "content": bad})
	if !strings.Contains(httpErr.Error, "b.link") || !strings.Contains(httpErr.Error, "https: or mailto:") {
		t.Errorf("the refusal does not say where and what: %s", httpErr.Error)
	}
	if !strings.Contains(mcpErr, httpErr.Error) {
		t.Errorf("the two entrances refuse differently:\nhttp: %s\nmcp:  %s", httpErr.Error, mcpErr)
	}
}

func TestABoardPageAndItsCardAllowOnlyTheScriptThePlatformInjectsByHash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	hostile := strings.Replace(boardFlow, `"text": "Login 登入"`, `"text": "<script>window.pwned=1</script>"`, 1)
	out := h.agent(tok).ok("publish", map[string]any{"title": "Flow", "kind": "board", "content": hostile})
	id := out["id"].(string)
	alice := h.browser("alice", "pw-a")
	for path, injected := range map[string]string{"/raw/" + id: bridgeJS, "/raw/" + id + "?preview": previewJS} {
		res, body := h.do(alice, "GET", path, "", nil)
		csp := res.Header.Get("Content-Security-Policy")
		if !strings.HasPrefix(csp, artifactSandbox+"; ") {
			t.Errorf("%s: csp %q: the sandbox must be every kind's", path, csp)
		}
		if m := regexp.MustCompile(`script-src ([^;]*)`).FindStringSubmatch(csp); m == nil || m[1] != scriptHash(injected) {
			t.Errorf("%s: csp %q: want a script-src of the injected script's hash alone", path, csp)
		}
		if inline := regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindAllStringSubmatch(body, -1); len(inline) != 1 || inline[0][1] != injected {
			t.Errorf("%s: %d inline scripts, want the one the server injects", path, len(inline))
		}
		if !strings.Contains(body, "&lt;script&gt;window.pwned=1&lt;/script&gt;") {
			t.Errorf("%s: the board's text is not drawn as text", path)
		}
	}
}

// boardSkeleton is how an agent draws a diagram: labelled boxes and arrows by from and to.
const boardSkeleton = `{"version": 1, "elements": [
  {"id": "a", "type": "rectangle", "x": 0, "y": 0, "label": "Start"},
  {"id": "b", "type": "rectangle", "x": 240, "y": 0, "label": "Check"},
  {"id": "c", "type": "ellipse", "x": 480, "y": 0, "label": "Done"},
  {"id": "ab", "type": "arrow", "from": "a", "to": "b"},
  {"id": "bc", "type": "arrow", "from": "b", "to": "c"}
]}`

func TestASkeletonPublishedOverHTTPAndMCPBecomesTheSameBoundBoardAndIsRefusedTheSameWay(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)

	res, body := putJSON(h, "POST", "/api/artifacts", `{"title":"Flow","kind":"board","content":`+jsonString(boardSkeleton)+`}`, tok)
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		t.Fatalf("HTTP publish: %d %s", res.StatusCode, body)
	}
	var viaHTTP struct{ ID string }
	if err := json.Unmarshal([]byte(body), &viaHTTP); err != nil {
		t.Fatal(err)
	}
	viaMCP := ag.ok("publish", map[string]any{"title": "Flow", "kind": "board", "content": boardSkeleton})
	stored := ag.ok("read", map[string]any{"id": viaHTTP.ID})["content"].(string)
	if other := ag.ok("read", map[string]any{"id": viaMCP["id"]})["content"]; other != stored {
		t.Errorf("one skeleton is stored two ways:\nhttp %s\nmcp  %s", stored, other)
	}
	els := boardElements(t, stored)
	for _, c := range []struct{ arrow, from, to string }{{"ab", "a", "b"}, {"bc", "b", "c"}} {
		start, _ := els[c.arrow]["startBinding"].(map[string]any)
		end, _ := els[c.arrow]["endBinding"].(map[string]any)
		if start["elementId"] != c.from || end["elementId"] != c.to {
			t.Errorf("%s is bound %v → %v", c.arrow, start, end)
		}
	}
	if els["b-label"]["text"] != "Check" || els["b-label"]["containerId"] != "b" {
		t.Errorf("b's label: %v", els["b-label"])
	}

	bad := strings.Replace(boardSkeleton, `"to": "c"`, `"to": "z"`, 1)
	res, body = putJSON(h, "POST", "/api/artifacts", `{"title":"Bad","kind":"board","content":`+jsonString(bad)+`}`, tok)
	var httpErr struct{ Error string }
	_ = json.Unmarshal([]byte(body), &httpErr)
	mcpErr := ag.refused("publish", map[string]any{"title": "Bad", "kind": "board", "content": bad})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(httpErr.Error, `arrow bc: to names no element "z"`) || !strings.Contains(mcpErr, httpErr.Error) {
		t.Errorf("the refusals differ or do not name the arrow:\nhttp %d %s\nmcp  %s", res.StatusCode, httpErr.Error, mcpErr)
	}
}

func TestABoardEditOverHTTPAndMCPGivesTheSameResultAndTheSameRefusalForAStaleHash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)
	make2 := func(title string) (string, map[string]any) {
		out := ag.ok("publish", map[string]any{"title": title, "kind": "board", "content": boardSkeleton})
		id := out["id"].(string)
		read := ag.ok("read", map[string]any{"id": id})
		hashes, _ := read["hashes"].(map[string]any)
		if len(hashes) != 8 {
			t.Fatalf("read gives %d hashes: %v", len(hashes), hashes)
		}
		return id, hashes
	}
	viaMCP, mcpHashes := make2("MCP")
	viaHTTP, httpHashes := make2("HTTP")
	res, body := putJSON(h, "GET", "/api/artifacts/"+viaHTTP+"/board", "", tok)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, fmt.Sprint(httpHashes["b"])) {
		t.Fatalf("GET board: %d %s", res.StatusCode, body)
	}
	ops := func(hashes map[string]any) []map[string]any {
		return []map[string]any{
			{"op": "change", "id": "b", "ifHash": hashes["b"], "fields": map[string]any{"label": "Review", "backgroundColor": "#ffc9c9"}},
			{"op": "add", "element": map[string]any{"id": "d", "type": "rectangle", "x": 720, "y": 0, "label": "Ship"}},
			{"op": "add", "element": map[string]any{"id": "cd", "type": "arrow", "from": "c", "to": "d"}},
			{"op": "delete", "id": "ab", "ifHash": hashes["ab"]},
		}
	}
	mcpOut := ag.ok("board_edit", map[string]any{"id": viaMCP, "ops": ops(mcpHashes)})
	res, body = putJSON(h, "POST", "/api/artifacts/"+viaHTTP+"/board-edit", string(mustJSON(t, map[string]any{"ops": ops(httpHashes)})), tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST board-edit: %d %s", res.StatusCode, body)
	}
	httpOut := decode[map[string]any](t, body)
	delete(mcpOut, "next")
	if string(mustJSON(t, mcpOut)) != string(mustJSON(t, httpOut)) {
		t.Errorf("the two entrances answer differently:\nmcp  %s\nhttp %s", mustJSON(t, mcpOut), body)
	}
	a := ag.ok("read", map[string]any{"id": viaMCP})["content"]
	if b := ag.ok("read", map[string]any{"id": viaHTTP})["content"]; a != b {
		t.Errorf("the two boards differ:\nmcp  %s\nhttp %s", a, b)
	}
	els := boardElements(t, a.(string))
	if els["b-label"]["text"] != "Review" || els["d-label"]["text"] != "Ship" || els["ab"] != nil {
		t.Errorf("the edit did not do what it said: %v", els)
	}

	// The hashes read before the edit are stale now: both entrances refuse, the same way, and say to read again.
	mcpErr := ag.refused("board_edit", map[string]any{"id": viaMCP, "ops": []map[string]any{{"op": "change", "id": "b", "ifHash": mcpHashes["b"], "fields": map[string]any{"x": 1}}}})
	res, body = putJSON(h, "POST", "/api/artifacts/"+viaHTTP+"/board-edit", `{"ops":[{"op":"change","id":"b","ifHash":`+string(mustJSON(t, httpHashes["b"]))+`,"fields":{"x":1}}]}`, tok)
	var httpErr struct {
		Error string
		Stale map[string]struct{ Hash string }
	}
	_ = json.Unmarshal([]byte(body), &httpErr)
	if res.StatusCode != http.StatusConflict || !strings.Contains(httpErr.Error, "b changed since you read it") || httpErr.Stale["b"].Hash == "" {
		t.Errorf("HTTP: %d %s", res.StatusCode, body)
	}
	if !strings.Contains(mcpErr, httpErr.Error) || !strings.Contains(mcpErr, "Current state of those elements") {
		t.Errorf("MCP refuses differently: %s", mcpErr)
	}
}

func TestABoardShowsOnlyImagesOfItsOwnFilesWhetherPublishedOrAddedByBoardEdit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)
	image := func(file string) string {
		return `{"version":1,"elements":[{"id":"pic","type":"image","x":0,"y":0,"width":100,"height":100,"fileId":` + jsonString(file) + `,"status":"saved"}]}`
	}
	if got := ag.refused("publish", map[string]any{"title": "New", "kind": "board", "content": image("chart.png")}); !strings.Contains(got, "a new board has none") {
		t.Errorf("a new board with an image: %s", got)
	}
	id := ag.ok("publish", map[string]any{"title": "Shots", "kind": "board", "content": `{"version":1,"elements":[]}`})["id"].(string)
	other := ag.ok("publish", map[string]any{"title": "Other", "kind": "board", "content": `{"version":1,"elements":[]}`})["id"].(string)
	h.mustUpload(tok, other, "theirs.png", pngBytes)
	h.mustUpload(tok, id, "chart.png", pngBytes)

	// A file of another artifact is not this board's: refused by publish over MCP and HTTP alike, and by board_edit.
	mcpErr := ag.refused("publish", map[string]any{"id": id, "title": "Shots", "content": image("theirs.png"), "baseVersion": 1})
	res, body := putJSON(h, "PUT", "/api/artifacts/"+id, `{"title":"Shots","content":`+jsonString(image("theirs.png"))+`,"baseVersion":1}`, tok)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, `image pic shows \"theirs.png\", which is not a file of this board`) || !strings.Contains(mcpErr, `image pic shows "theirs.png"`) {
		t.Errorf("an image of another artifact's file:\nhttp %d %s\nmcp %s", res.StatusCode, body, mcpErr)
	}
	if got := ag.refused("board_edit", map[string]any{"id": id, "ops": []map[string]any{{"op": "add", "element": map[string]any{"id": "pic", "type": "image", "x": 0, "y": 0, "width": 50, "height": 50, "fileId": "nope.png"}}}}); !strings.Contains(got, `"nope.png", which is not a file of this board`) {
		t.Errorf("board_edit of a missing file: %s", got)
	}

	// Its own file is shown.
	ag.ok("publish", map[string]any{"id": id, "title": "Shots", "content": image("chart.png"), "baseVersion": 1})
	if els := boardElements(t, ag.ok("read", map[string]any{"id": id})["content"].(string)); els["pic"]["fileId"] != "chart.png" {
		t.Errorf("the image: %v", els["pic"])
	}
}
