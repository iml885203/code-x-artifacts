package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestLargeLiveSourcePublishUsesExplicitCommitBasesAndRetainsUploadsAfterRefusal(t *testing.T) {
	t.Parallel()
	for _, transport := range []string{"HTTP", "MCP"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()
			h := newLiveHarness(t)
			tok := h.token("alice")
			frames := designFrames()
			frames["frames/hero.html"] = `<body><h1 data-pid="title">Title.</h1><p data-pid="copy">Body.</p></body>`
			a := h.publishDesign(tok, frames, "")
			_ = liveDesignFrame(t, h, a.ID, "frames/hero.html")
			agent := h.agent(tok)
			read := agent.ok("read", map[string]any{"id": a.ID, "file": "frames/hero.html"})
			source := read["file"].(map[string]any)["text"].(string)
			changed := strings.Replace(source, "Title.", "Title. Agent.", 1)
			texts := map[string]string{"content": designManifest, "frames/hero.html": changed}
			begin := map[string]any{"id": a.ID, "title": "Session title", "baseVersion": 1, "files": []any{declared("content", designManifest), declared("frames/hero.html", changed)}}
			var session string
			if transport == "MCP" {
				begun := agent.ok("begin_publish", begin)
				session = begun["session"].(string)
				h.curl(begun, texts)
			} else {
				begun := h.mustBegin(tok, begin)
				session = begun.Session
				h.uploadAll(begun, texts)
			}
			appendLiveDesignText(t, h, a.ID, "alice", "copy", " Human.")
			before := liveDesignFrame(t, h, a.ID, "frames/hero.html")
			if transport == "MCP" {
				if refused := agent.refused("commit_publish", map[string]any{"session": session}); !strings.Contains(refused, "retain your draft") {
					t.Fatal(refused)
				}
			} else if res, out := h.commit(tok, session); res.StatusCode != http.StatusConflict || !strings.Contains(out, "retain your draft") {
				t.Fatalf("missing base: %d %s", res.StatusCode, out)
			}
			stored, _ := h.st.Artifact(t.Context(), a.ID)
			if stored.Title != "Landing" || stored.Version != 1 || liveDesignFrame(t, h, a.ID, "frames/hero.html") != before {
				t.Fatal("refused session changed source or metadata")
			}
			commitSourceSessionOnce(t, h, agent, tok, transport, session, read["sourceBases"])

			got := liveDesignFrame(t, h, a.ID, "frames/hero.html")
			if !strings.Contains(got, "Title. Agent.") || !strings.Contains(got, "Body. Human.") {
				t.Fatalf("independent work lost: %s", got)
			}
			stored, _ = h.st.Artifact(t.Context(), a.ID)
			if stored.Title != "Session title" || stored.Version != 2 {
				t.Fatal("live metadata not applied")
			}
		})
	}
}

func TestAbsentLiveFrameReadsPermitCreationButNeverReplaceAnotherCreation(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	a := h.publishDesign(tok, designFrames(), "")
	_ = liveDesignFrame(t, h, a.ID, "frames/hero.html")
	agent := h.agent(tok)
	manifest := agent.ok("read", map[string]any{"id": a.ID})
	absent := agent.ok("read", map[string]any{"id": a.ID, "file": "frames/new.html"})
	f := absent["file"].(map[string]any)
	if f["present"] != false || f["sourceBase"] == nil {
		t.Fatal("absence not explicit")
	}
	if _, _, err := h.app.svc.ReadFile(t.Context(), h.app.svc.Viewer("alice"), a.ID, "frames/new.html", 0); err == nil {
		t.Fatal("legacy missing-file read must refuse")
	}
	bases := map[string]any{"design.json": manifest["sourceBases"].(map[string]any)["design.json"], "frames/new.html": f["sourceBase"]}
	content := strings.Replace(designManifest, "frames/footer.html", "frames/new.html", 1)
	input := map[string]any{"id": a.ID, "title": "Landing", "baseVersion": 1, "content": content, "sourceBases": bases, "files": map[string]any{"frames/new.html": "<p>First creation</p>"}}
	agent.ok("publish", input)
	before := liveDesignFrame(t, h, a.ID, "frames/new.html")
	input["files"] = map[string]any{"frames/new.html": "<p>Second creation</p>"}
	if refused := agent.refused("publish", input); !strings.Contains(refused, "retain your draft") {
		t.Fatal(refused)
	}
	if got := liveDesignFrame(t, h, a.ID, "frames/new.html"); got != before {
		t.Fatal("stale absence replaced another creation")
	}
}

func commitSourceSessionOnce(t *testing.T, h *liveHarness, agent *agent, tok, transport, session string, bases any) {
	t.Helper()
	commit := map[string]any{"session": session, "sourceBases": bases}
	if transport == "MCP" {
		result := agent.ok("commit_publish", commit)
		if result["live"] != true {
			t.Fatal("not live")
		}
		agent.refused("commit_publish", commit)
	} else {
		raw, err := json.Marshal(commit)
		if err != nil {
			t.Fatal(err)
		}
		res, out := api(h.harness, "POST", "/api/publish-sessions/"+session+"/commit", string(raw), tok)
		if res.StatusCode != http.StatusOK || !decode[artifactView](t, out).Live {
			t.Fatalf("commit: %d %s", res.StatusCode, out)
		}
		if res, _ := h.commit(tok, session); res.StatusCode != http.StatusNotFound {
			t.Fatal("session committed twice")
		}
	}
}
