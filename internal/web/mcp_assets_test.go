package web

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestAnAgentUploadsAChartThroughMCPAndThePageShowsIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.agent(h.token("alice")), h.agent(h.token("bob"))
	made := alice.ok("publish", map[string]any{"title": "Report", "content": page})
	id := made["id"].(string)

	up := alice.ok("upload_asset", map[string]any{"id": id, "name": "Revenue Chart.png", "content_base64": b64(pngBytes)})
	url := up["url"].(string)
	if up["name"] != "Revenue-Chart.png" || up["contentType"] != "image/png" || up["replaced"] != false ||
		!strings.HasSuffix(url, "/files/"+id+"/Revenue-Chart.png") || !strings.Contains(up["next"].(string), "Reference exactly this url") {
		t.Fatalf("upload_asset = %v", up)
	}

	// The agent writes the url into the page, as the tool's description says.
	withImage := `<!doctype html><html><head></head><body><img src="` + url + `" alt="Revenue"></body></html>`
	alice.ok("publish", map[string]any{"id": id, "title": "Report", "content": withImage, "baseVersion": 1})
	_, raw := h.do(h.browser("alice", "pw-a"), "GET", "/raw/"+id, "", nil)
	m := regexp.MustCompile(`<img src="` + regexp.QuoteMeta(h.srv.URL) + `(/files/` + id + `/[^/"]+/Revenue-Chart\.png)"`).FindStringSubmatch(raw)
	if m == nil {
		t.Fatalf("the page does not reference the uploaded chart:\n%s", raw)
	}
	if res, body := h.do(nil, "GET", m[1], "", map[string]string{"Origin": "null"}); res.StatusCode != http.StatusOK || body != pngBytes || res.Header.Get("Content-Type") != "image/png" {
		t.Errorf("the chart as the sandboxed page fetches it: %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}

	agentsReachFilesByRole(t, alice, bob, id, url)
}

func agentsReachFilesByRole(t *testing.T, alice, bob *agent, id, url string) {
	t.Helper()
	list := alice.ok("list_assets", map[string]any{"id": id})["assets"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["url"] != url || list[0].(map[string]any)["size"] != float64(len(pngBytes)) {
		t.Errorf("list_assets = %v", list)
	}
	if again := alice.ok("upload_asset", map[string]any{"id": id, "name": "Revenue-Chart.png", "content_base64": b64(pngBytes + "x")}); again["replaced"] != true {
		t.Errorf("uploading the same name again: %v", again)
	}

	if got := bob.refused("upload_asset", map[string]any{"id": id, "name": "x.png", "content_base64": b64(pngBytes)}); !strings.Contains(got, "no artifact") {
		t.Errorf("a stranger uploading to a private artifact: %s", got)
	}
	alice.ok("set_visibility", map[string]any{"id": id, "visibility": "organisation"})
	if got := bob.refused("upload_asset", map[string]any{"id": id, "name": "x.png", "content_base64": b64(pngBytes)}); !strings.Contains(got, "only the author") {
		t.Errorf("a reader uploading: %s", got)
	}
	if got := bob.ok("list_assets", map[string]any{"id": id})["assets"].([]any); len(got) != 1 {
		t.Errorf("a reader's list_assets: %v", got)
	}
	if got := bob.refused("delete_asset", map[string]any{"id": id, "name": "Revenue-Chart.png"}); !strings.Contains(got, "only the author") {
		t.Errorf("a reader deleting: %s", got)
	}

	if out := alice.ok("delete_asset", map[string]any{"id": id, "name": "Revenue-Chart.png"}); out["deleted"] != true {
		t.Errorf("delete_asset = %v", out)
	}
	if got := alice.refused("delete_asset", map[string]any{"id": id, "name": "Revenue-Chart.png"}); !strings.Contains(got, "list_assets") {
		t.Errorf("deleting a missing file: %s", got)
	}
}

// smallUploadLimit is what the limit tests configure instead of the real 5 MB,
// so they prove the same boundary without pushing megabytes through a client.
const smallUploadLimit = 64 << 10

func withSmallUploadLimit(c *Config) { c.MCPUploadBytes = smallUploadLimit }

func TestAnUploadThroughMCPIsRefusedWithWhatToSendInstead(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withSmallUploadLimit)
	alice := h.agent(h.token("alice"))
	id := alice.ok("publish", map[string]any{"title": "Report", "content": page})["id"].(string)
	upload := func(name, content string) string {
		return alice.refused("upload_asset", map[string]any{"id": id, "name": name, "content_base64": content})
	}

	for label, c := range map[string]struct{ got, want string }{
		"HTML as a picture":     {upload("chart.png", b64("<script>alert(1)</script>")), "not an accepted type"},
		"a name with no type":   {upload("chart", b64(pngBytes)), "send something like chart.png"},
		"the wrong extension":   {upload("chart.pdf", b64(pngBytes)), "image/png"},
		"not base64":            {upload("chart.png", "this is not base64!"), "not valid base64"},
		"nothing":               {upload("chart.png", ""), "content_base64 is required"},
		"a file over the limit": {upload("big.png", strings.Repeat("A", smallUploadLimit*4/3+8)), "64 KB"},
	} {
		if !strings.Contains(c.got, c.want) {
			t.Errorf("%s: %q, want it to mention %q", label, c.got, c.want)
		}
	}
	if got := alice.ok("list_assets", map[string]any{"id": id})["assets"].([]any); len(got) != 0 {
		t.Errorf("refused uploads left files: %v", got)
	}

	// A data: URL and wrapped lines are what agents really send.
	wrapped := b64(pngBytes)
	wrapped = "data:image/png;base64," + wrapped[:20] + "\n" + wrapped[20:]
	alice.ok("upload_asset", map[string]any{"id": id, "name": "wrapped.png", "content_base64": wrapped})
}

func TestTheUploadToolStatesItsLimitsAndHowToReferenceTheFile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tools, err := h.agent(h.token("alice")).session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var desc string
	for _, tool := range tools.Tools {
		if tool.Name == "upload_asset" {
			desc = tool.Description
		}
	}
	for _, want := range []string{"5 MB", "15 MB", "50 files", "100 MB", "PNG", "CSV", "<img src=", "![alt](URL)", "exactly", "url"} {
		if !strings.Contains(desc, want) {
			t.Errorf("the upload_asset description never says %q:\n%s", want, desc)
		}
	}
}

func TestDeletingThroughMCPPlansForTheFilesItWouldLose(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.agent(h.token("alice"))
	id := alice.ok("publish", map[string]any{"title": "Report", "content": page})["id"].(string)
	alice.ok("upload_asset", map[string]any{"id": id, "name": "a.png", "content_base64": b64(pngBytes)})
	plan := alice.ok("delete", map[string]any{"id": id})
	if got := plan["plan"].(map[string]any)["assets"]; got != float64(1) || !strings.Contains(plan["next"].(string), "uploaded files (1)") {
		t.Errorf("delete plan = %v", plan)
	}
}

// The limit the description promises must be one the transport really carries.
func TestAFileAtTheMCPLimitIsAcceptedAndOneByteOverIsNot(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withSmallUploadLimit)
	alice := h.agent(h.token("alice"))
	id := alice.ok("publish", map[string]any{"title": "Report", "content": page})["id"].(string)
	atLimit := pngBytes + strings.Repeat("\x00", smallUploadLimit-len(pngBytes))
	if got := alice.ok("upload_asset", map[string]any{"id": id, "name": "big.png", "content_base64": b64(atLimit)}); got["size"] != float64(smallUploadLimit) {
		t.Errorf("a file of exactly the limit: %v", got)
	}
	if got := alice.refused("upload_asset", map[string]any{"id": id, "name": "bigger.png", "content_base64": b64(atLimit + "\x00")}); !strings.Contains(got, "64 KB") {
		t.Errorf("a file one byte over: %s", got)
	}
}
