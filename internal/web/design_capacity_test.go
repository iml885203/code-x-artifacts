package web

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
)

func TestARefusedDesignCapacityPublishNamesTheBoundAndKeepsEveryWritersWorkAndActivity(t *testing.T) {
	t.Parallel()
	for _, transport := range []string{"HTTP", "MCP"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()
			h := newLiveHarness(t, func(c *Config) { c.Realtime = realtime.Limits{DocBytes: 16 << 10} })
			tok := h.token("alice")
			frames := designFrames()
			frames["frames/hero.html"] = `<body><h1 data-pid="title">Title.</h1><p data-pid="copy">Body.</p></body>`
			a := h.publishDesign(tok, frames, "")
			_ = liveDesignFrame(t, h, a.ID, "frames/hero.html")
			appendLiveDesignText(t, h, a.ID, "alice", "title", " First writer.")
			appendLiveDesignText(t, h, a.ID, "bob", "copy", " Second writer.")
			agent := h.agent(tok)
			read := agent.ok("read", map[string]any{"id": a.ID, "file": "frames/hero.html"})
			text := read["file"].(map[string]any)["text"].(string)
			before := agent.ok("read", map[string]any{"id": a.ID})
			durableBefore := designCapacityState(t, h, tok, a.ID)
			input := map[string]any{"id": a.ID, "title": "Must not rename", "content": designManifest, "baseVersion": 1, "sourceBases": read["sourceBases"], "files": map[string]any{"frames/hero.html": strings.Replace(text, "Title.", strings.Repeat("x", 20<<10), 1)}}
			var refusal string
			if transport == "MCP" {
				refusal = agent.refused("publish", input)
			} else {
				res, body := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, input)
				if res.StatusCode != http.StatusBadRequest {
					t.Fatalf("status=%d, want 400", res.StatusCode)
				}
				refusal = body
			}
			for _, want := range []string{"document size (bytes)", "16384", "nothing was published", "retain your draft"} {
				if !strings.Contains(refusal, want) {
					t.Errorf("refusal lacks %q: %s", want, refusal)
				}
			}
			if strings.Contains(refusal, "try once more") {
				t.Error("capacity refusal suggests unchanged retry")
			}
			after := agent.ok("read", map[string]any{"id": a.ID})
			for _, key := range []string{"content", "title", "version", "visibility"} {
				if !reflect.DeepEqual(before[key], after[key]) {
					t.Errorf("%s changed", key)
				}
			}
			if !reflect.DeepEqual(durableBefore, designCapacityState(t, h, tok, a.ID)) {
				t.Fatal("refusal changed durable room/versions/files/activity")
			}
			if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != text {
				t.Fatal("refusal lost a writer's work")
			}
		})
	}
}

func designCapacityState(t *testing.T, h *liveHarness, tok, id string) map[string]any {
	t.Helper()
	log, err := h.st.RealtimeLog().Load(t.Context(), id, "")
	if err != nil {
		t.Fatal(err)
	}
	versions, err := h.st.Versions(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	files, err := h.st.VersionFiles(t.Context(), id, 1)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{"log": log, "versions": versions, "files": files, "activity": activityPage(t, h.harness, tok, "").Items}
}
