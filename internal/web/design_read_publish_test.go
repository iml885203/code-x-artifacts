package web

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
)

func appendLiveDesignText(t *testing.T, h *liveHarness, id, user, pid, text string) {
	t.Helper()
	err := h.app.live.hub.Edit(t.Context(), id, realtime.Identity{Username: user}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		rich, err := editableDesignText(doc, pid)
		if err != nil {
			return nil, err
		}
		return func(txn *crdt.Transaction) { rich.Insert(txn, rich.Len(), text, nil) }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func editableDesignText(doc *crdt.Doc, pid string) (*crdt.YText, error) {
	m := doc.GetMap(designroom.Map)
	value, _ := m.Get("frames/hero.html|n|" + pid + "|rich")
	if rich, ok := value.(*crdt.YText); ok {
		return rich, nil
	}
	for key, parent := range m.Entries() {
		if strings.HasSuffix(key, "|parent") && parent == pid {
			value, _ := m.Get(strings.TrimSuffix(key, "parent") + "text")
			if plain, ok := value.(*crdt.YText); ok {
				return plain, nil
			}
		}
	}
	return nil, fmt.Errorf("%s has no text", pid)
}

func liveDesignFrame(t *testing.T, h *liveHarness, id, path string) string {
	t.Helper()
	var source string
	err := h.app.live.hub.Read(t.Context(), id, func(doc *crdt.Doc) error {
		held, err := designroom.Open(doc).Read()
		source = held.Frames[path]
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestAnMCPFrameReadAndPublishExtendHumanTextExactlyOnce(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	frames := designFrames()
	frames["frames/hero.html"] = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0"><h1 id="title" data-pid="title">Design title.</h1><p id="copy" data-pid="copy">Design body.</p><!-- keep  these bytes --></body></html>`
	a := h.publishDesign(tok, frames, "")
	untouched := liveDesignFrame(t, h, a.ID, "frames/mobile.html")
	appendLiveDesignText(t, h, a.ID, "alice", "title", " Alice.")
	appendLiveDesignText(t, h, a.ID, "bob", "copy", " Bob.")
	agent := h.agent(tok)
	read := agent.ok("read", map[string]any{"id": a.ID, "file": "frames/hero.html"})
	source := read["file"].(map[string]any)["text"].(string)
	if !strings.Contains(source, ">Design title. Alice.</h1>") || !strings.Contains(source, ">Design body. Bob.</p>") {
		t.Fatalf("read did not contain both people: %s", source)
	}
	want := strings.Replace(source, "Design title. Alice.", "Design title. Alice. Agent.", 1)
	result := agent.ok("publish", map[string]any{"id": a.ID, "title": "Landing", "content": designManifest, "baseVersion": read["version"], "sourceBases": read["sourceBases"], "files": map[string]any{"frames/hero.html": want}})
	if result["live"] != true {
		t.Fatalf("publish did not use the room: %v", result)
	}
	if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != want {
		t.Fatalf("human and agent text must occur exactly once, other bytes unchanged:\nwant %s\ngot  %s", want, got)
	}
	if got := liveDesignFrame(t, h, a.ID, "frames/mobile.html"); got != untouched {
		t.Fatal("an unrelated frame changed")
	}
	h.clock.Advance(roomkind.Quiet)
	h.app.live.snap.Sweep(t.Context())
	stored, err := h.app.svc.FileTexts(t.Context(), a.ID, 2)
	if err != nil || stored["frames/hero.html"] != want {
		t.Fatalf("snapshot did not persist the merged source: %v %s", err, stored["frames/hero.html"])
	}
}

func TestLiveFrameReadPublishKeepsRichTextRepeatedWordsAndLaterIndependentEditsOverHTTPAndMCP(t *testing.T) {
	t.Parallel()
	for _, transport := range []string{"HTTP", "MCP"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()
			h := newLiveHarness(t)
			tok := h.token("alice")
			frames := designFrames()
			frames["frames/hero.html"] = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0"><h1 data-pid="title">Title <b data-pid="bold">bold</b>. Repeat. Repeat.</h1><p data-pid="copy">Body.</p><!-- keep  these bytes --></body></html>`
			a := h.publishDesign(tok, frames, "")
			appendLiveDesignText(t, h, a.ID, "alice", "title", " Alice.")
			var source string
			var proof any
			var agent *agent
			base := a.Version
			if transport == "MCP" {
				agent = h.agent(tok)
				read := agent.ok("read", map[string]any{"id": a.ID, "file": "frames/hero.html"})
				source = read["file"].(map[string]any)["text"].(string)
				base = int(read["version"].(float64))
				proof = read["sourceBases"]
			} else {
				res, text := h.fileText(tok, a.ID, "frames/hero.html", 0)
				if res.StatusCode != http.StatusOK {
					t.Fatalf("read: %d %s", res.StatusCode, text)
				}
				source = text
				proof = map[string]string{"frames/hero.html": res.Header.Get("X-Pictor-Source-Base")}
			}
			appendLiveDesignText(t, h, a.ID, "bob", "copy", " Bob.")
			// A later read from the same principal/client must not replace the earlier baseline.
			if transport == "MCP" {
				agent.ok("read", map[string]any{"id": a.ID, "file": "frames/hero.html"})
			} else {
				h.fileText(tok, a.ID, "frames/hero.html", 0)
			}
			next := strings.Replace(source, " Repeat. Repeat. Alice.", " Repeat. Repeat. Repeat. Alice. Agent.", 1)
			if next == source {
				t.Fatal("fixture did not contain the rich text and repeated words")
			}
			input := map[string]any{"id": a.ID, "title": "Landing", "content": designManifest, "baseVersion": base, "sourceBases": proof, "files": map[string]any{"frames/hero.html": next}}
			if transport == "MCP" {
				agent.ok("publish", input)
			} else if res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, input); res.StatusCode != http.StatusOK {
				t.Fatalf("publish: %d %s", res.StatusCode, out)
			}
			want := strings.Replace(next, ">Body.</p>", ">Body. Bob.</p>", 1)
			if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != want {
				t.Fatalf("rich markup, deliberate repetitions, both people and other source bytes must stay:\nwant %s\ngot  %s", want, got)
			}
		})
	}
}

func TestDifferentMCPClientsKeepTheirOwnFrameReadBaseAndAStaleOverlappingEditIsRefused(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	frames := designFrames()
	frames["frames/hero.html"] = `<body><h1 data-pid="title">Original title.</h1><p data-pid="copy">Body.</p></body>`
	a := h.publishDesign(tok, frames, "")
	appendLiveDesignText(t, h, a.ID, "alice", "title", " Alice.")
	first := h.agent(tok)
	read := first.ok("read", map[string]any{"id": a.ID, "file": "frames/hero.html"})
	source := read["file"].(map[string]any)["text"].(string)
	err := h.app.live.hub.Edit(t.Context(), a.ID, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		text, err := editableDesignText(doc, "title")
		if err != nil {
			return nil, err
		}
		return func(txn *crdt.Transaction) {
			text.Delete(txn, 0, text.Len())
			text.Insert(txn, 0, "Human replacement.", nil)
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	current := liveDesignFrame(t, h, a.ID, "frames/hero.html")
	// A second client of the same account must not replace the first client's base.
	client := mcp.NewClient(&mcp.Implementation{Name: "codex-mcp-client", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: h.srv.URL + "/api/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token: h.token("alice"), next: h.srv.Client().Transport}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	second := &agent{t: t, session: session}
	other := second.ok("read", map[string]any{"id": a.ID, "file": "frames/hero.html"})
	if other["file"].(map[string]any)["text"] != current {
		t.Fatal("second client did not read the current source")
	}
	next := strings.Replace(source, "Original title. Alice.", "Agent replacement.", 1)
	if next == source {
		t.Fatal("fixture did not contain the title")
	}
	refusal := first.refused("publish", map[string]any{"id": a.ID, "title": "Landing", "content": designManifest, "baseVersion": read["version"], "sourceBases": read["sourceBases"], "files": map[string]any{"frames/hero.html": next}})
	if !strings.Contains(refusal, "someone changed the words") {
		t.Fatalf("stale overlapping source was not a conflict: %s", refusal)
	}
	if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != current {
		t.Fatalf("refused publish changed the source:\nwant %s\ngot  %s", current, got)
	}
}

func TestHTTPFramePublishRefusesStaleOverlappingWordsWithoutChangingTheRoom(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	frames := designFrames()
	frames["frames/hero.html"] = `<body><h1 data-pid="title">Original title.</h1></body>`
	a := h.publishDesign(tok, frames, "")
	appendLiveDesignText(t, h, a.ID, "alice", "title", " Alice.")
	res, source := h.fileText(tok, a.ID, "frames/hero.html", 0)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("read: %d %s", res.StatusCode, source)
	}
	err := h.app.live.hub.Edit(t.Context(), a.ID, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		text, err := editableDesignText(doc, "title")
		if err != nil {
			return nil, err
		}
		return func(txn *crdt.Transaction) {
			text.Delete(txn, 0, text.Len())
			text.Insert(txn, 0, "Human replacement.", nil)
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	current := liveDesignFrame(t, h, a.ID, "frames/hero.html")
	next := strings.Replace(source, "Original title. Alice.", "Agent replacement.", 1)
	proof := res.Header.Get("X-Pictor-Source-Base")
	res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, map[string]any{"title": "Landing", "content": designManifest, "baseVersion": a.Version, "sourceBases": map[string]string{"frames/hero.html": proof}, "files": map[string]any{"frames/hero.html": next}})
	if res.StatusCode != http.StatusConflict || !strings.Contains(out, "someone changed the words") {
		t.Fatalf("stale overlapping source was not a conflict: %d %s", res.StatusCode, out)
	}
	if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != current {
		t.Fatalf("HTTP refusal changed the source:\nwant %s\ngot  %s", current, got)
	}
}
