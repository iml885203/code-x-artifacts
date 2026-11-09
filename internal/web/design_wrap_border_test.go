package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/reearth/ygo/crdt"
)

func TestWrapAndSideBorderPublishPreserveConcurrentStylesAndRefuseInvalidValuesOverHTTPAndMCP(t *testing.T) {
	t.Parallel()
	for _, transport := range []string{"HTTP", "MCP"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()
			h := newLiveHarness(t)
			tok := h.token("alice")
			frames := designFrames()
			frames["frames/hero.html"] = `<!doctype html><html><head><meta charset="utf-8"></head><body><div data-pid="card" style="border:1px solid black;border-top-width:2px;border-right-color:green;flex-wrap:nowrap;color:red"><span>Keep child</span></div><!-- keep  bytes --></body></html>`
			a := h.publishDesign(tok, frames, store.VisibilityOrganisation)
			original := liveDesignFrame(t, h, a.ID, "frames/hero.html")
			untouched := liveDesignFrame(t, h, a.ID, "frames/mobile.html")
			publishWrapAndBorderEdits(t, h, tok, a, transport)
			want := strings.Replace(strings.Replace(strings.Replace(original, "flex-wrap:nowrap", "flex-wrap:wrap", 1), "border-top-width:2px", "border-top-width:3px", 1), "color:red", "color:blue", 1)
			if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != want {
				t.Fatalf("source identity or concurrent edit lost: %s", got)
			}
			if got := liveDesignFrame(t, h, a.ID, "frames/mobile.html"); got != untouched {
				t.Fatal("unrelated frame changed")
			}
			h.clock.Advance(roomkind.Quiet)
			h.app.live.snap.Sweep(t.Context())
			saved, err := h.app.svc.FileTexts(t.Context(), a.ID, 2)
			if err != nil || saved["frames/hero.html"] != want || saved["frames/mobile.html"] != untouched {
				t.Fatalf("saved version lost styles or source: %v", err)
			}
			reader := h.token("bob")
			input := map[string]any{"id": a.ID, "title": "Forbidden", "content": designManifest, "baseVersion": a.Version, "files": map[string]any{"frames/hero.html": want}}
			if transport == "MCP" {
				h.agent(reader).refused("publish", input)
			} else if res, raw := h.sendFiles("PUT", "/api/artifacts/"+a.ID, reader, input); res.StatusCode != http.StatusForbidden {
				t.Fatalf("reader edit: %d %s", res.StatusCode, raw)
			}
			if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != want {
				t.Fatal("reader refusal changed source")
			}
		})
	}
}

func publishWrapAndBorderEdits(t *testing.T, h *liveHarness, tok string, a artifactView, transport string) {
	t.Helper()
	var client *agent
	if transport == "MCP" {
		client = h.agent(tok)
	}
	source, proof, base := readStyleEditSource(t, h, client, tok, a.ID, transport, a.Version)
	if err := h.app.live.hub.Edit(t.Context(), a.ID, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return designroom.Open(doc).SetStyle("frames/hero.html", "card", "color", "blue")
	}); err != nil {
		t.Fatal(err)
	}
	before := liveDesignFrame(t, h, a.ID, "frames/hero.html")
	// Refusals share the unchanged read; the final valid write proves its basis remains usable.
	for _, c := range []struct {
		prop, from, to string
		refuse         bool
	}{
		{"flex-wrap", "nowrap", "wrap", false},
		{"flex-wrap", "wrap", "wrap nowrap", true},
		{"border-top-width", "2px", "3px 4px", true},
		{"border-right-color", "green", "red blue", true},
		{"border-top-width", "2px", "3px", false},
	} {
		next := strings.Replace(source, c.prop+":"+c.from, c.prop+":"+c.to, 1)
		if next == source {
			t.Fatal("fixture did not exercise property change")
		}
		input := map[string]any{"id": a.ID, "title": "Landing", "content": designManifest, "baseVersion": base, "sourceBases": proof, "files": map[string]any{"frames/hero.html": next}}
		if transport == "MCP" && c.refuse {
			if refusal := client.refused("publish", input); !strings.Contains(refusal, c.prop) {
				t.Fatalf("expected %s validation refusal, not a lost read basis: %s", c.prop, refusal)
			}
		} else {
			assertStylePublish(t, h, client, tok, a.ID, transport, input, c.refuse)
		}
		want := before
		if !c.refuse {
			want = strings.Replace(before, c.prop+":"+c.from, c.prop+":"+c.to, 1)
		}
		got := liveDesignFrame(t, h, a.ID, "frames/hero.html")
		if got != want {
			t.Fatalf("changed unrelated or refused source:\nwant %s\ngot %s", want, got)
		}
		before = got
		if !c.refuse && c.prop == "flex-wrap" {
			source, proof, base = readStyleEditSource(t, h, client, tok, a.ID, transport, a.Version)
		}
	}
}

func TestCoveredBorderAndFillSourceEditsBecomeWinningOverridesOverHTTPAndMCP(t *testing.T) {
	t.Parallel()
	for _, transport := range []string{"HTTP", "MCP", "HTTP-legacy", "MCP-legacy"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()
			h := newLiveHarness(t)
			tok := h.token("alice")
			frames := designFrames()
			frames["frames/hero.html"] = `<html><body><div data-pid="card" style="border-top-color:blue !important;border:2px solid red !important;background-color:blue !important;background:red !important;transform:rotate(2deg)">Keep</div></body></html>`
			a := h.publishDesign(tok, frames, "")
			original := liveDesignFrame(t, h, a.ID, "frames/hero.html")
			if strings.HasSuffix(transport, "-legacy") {
				if err := h.app.live.hub.Edit(t.Context(), a.ID, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
					m := doc.GetMap(designroom.Map)
					return func(txn *crdt.Transaction) {
						m.Set(txn, "frames/hero.html|n|card|rest", "border-top-color:border-top-color;border:2px solid red !important;background-color:background-color;background:red !important;transform:rotate(2deg)")
						m.Set(txn, "frames/hero.html|n|card|s|border-top-color", "blue !important")
						m.Set(txn, "frames/hero.html|n|card|s|background-color", "blue !important")
					}, nil
				}); err != nil {
					t.Fatal(err)
				}
				if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != original {
					t.Fatal("legacy fixture changed authored source")
				}
			}

			client := h.agent(tok)
			read := client.ok("read", map[string]any{"id": a.ID, "file": "frames/hero.html"})
			next := strings.Replace(original, "border-top-color:blue !important", "border-top-color:green !important", 1)
			next = strings.Replace(next, "background-color:blue !important", "background-color:transparent !important", 1)
			input := map[string]any{"id": a.ID, "title": "Landing", "content": designManifest, "baseVersion": read["version"], "sourceBases": read["sourceBases"], "files": map[string]any{"frames/hero.html": next}}
			if strings.HasPrefix(transport, "MCP") {
				client.ok("publish", input)
			} else if res, raw := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, input); res.StatusCode != http.StatusOK {
				t.Fatalf("publish: %d %s", res.StatusCode, raw)
			}
			want := strings.Replace(original, "transform:rotate(2deg)", "transform:rotate(2deg);background-color:transparent !important;border-top-color:green !important", 1)
			if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != want {
				t.Fatalf("source edit must win after shorthand without changing authored source:\nwant %s\ngot %s", want, got)
			}
			h.clock.Advance(roomkind.Quiet)
			h.app.live.snap.Sweep(t.Context())
			saved, err := h.app.svc.FileTexts(t.Context(), a.ID, 2)
			if err != nil || saved["frames/hero.html"] != want {
				t.Fatalf("snapshot lost winning declaration order: %v", err)
			}
		})
	}
}

func readStyleEditSource(t *testing.T, h *liveHarness, client *agent, tok, id, transport string, version int) (string, any, int) {
	t.Helper()
	if transport == "MCP" {
		read := client.ok("read", map[string]any{"id": id, "file": "frames/hero.html"})
		return read["file"].(map[string]any)["text"].(string), read["sourceBases"], int(read["version"].(float64))
	}
	res, text := h.fileText(tok, id, "frames/hero.html", 0)
	if res.StatusCode != http.StatusOK {
		t.Fatal(text)
	}
	return text, map[string]string{"frames/hero.html": res.Header.Get("X-Pictor-Source-Base")}, version
}

func assertStylePublish(t *testing.T, h *liveHarness, client *agent, tok, id, transport string, input map[string]any, refuse bool) {
	t.Helper()
	if transport == "MCP" {
		if refuse {
			client.refused("publish", input)
		} else {
			client.ok("publish", input)
		}
		return
	}
	want := http.StatusOK
	if refuse {
		want = http.StatusBadRequest
	}
	res, raw := h.sendFiles("PUT", "/api/artifacts/"+id, tok, input)
	if res.StatusCode != want {
		t.Fatalf("publish status %d, want %d: %s", res.StatusCode, want, raw)
	}
}

func TestAStaleSourceEditRecreatesAnOverrideAfterPeerUndoEvenWhenItMatchesTheAuthoredSlot(t *testing.T) {
	t.Parallel()
	for _, transport := range []string{"HTTP", "MCP"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()
			h := newLiveHarness(t)
			tok := h.token("alice")
			frames := designFrames()
			frames["frames/hero.html"] = `<html><body><div data-pid="card" style="border-top-color:blue !important;border:2px solid red !important;transform:rotate(2deg)">Keep</div></body></html>`
			a := h.publishDesign(tok, frames, "")
			original := liveDesignFrame(t, h, a.ID, "frames/hero.html")
			if err := h.app.live.hub.Edit(t.Context(), a.ID, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
				m := doc.GetMap(designroom.Map)
				return func(txn *crdt.Transaction) {
					m.Set(txn, "frames/hero.html|n|card|rest", "border-top-color:border-top-color;border:2px solid red !important;transform:rotate(2deg)")
					m.Set(txn, "frames/hero.html|n|card|s|border-top-color", "blue !important")
					m.Set(txn, "frames/hero.html|n|card|override|border-top-color", "green !important")
				}, nil
			}); err != nil {
				t.Fatal(err)
			}
			client := h.agent(tok)
			source, proof, base := readStyleEditSource(t, h, client, tok, a.ID, transport, a.Version)
			if !strings.Contains(source, ";border-top-color:green !important") {
				t.Fatal("agent must read the explicit green override")
			}
			if err := h.app.live.hub.Edit(t.Context(), a.ID, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
				m := doc.GetMap(designroom.Map)
				return func(txn *crdt.Transaction) { m.Delete(txn, "frames/hero.html|n|card|override|border-top-color") }, nil
			}); err != nil {
				t.Fatal(err)
			}
			if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != original {
				t.Fatalf("peer Undo must reveal authored red shorthand: %s", got)
			}
			next := strings.Replace(source, ";border-top-color:green !important", ";border-top-color:blue !important", 1)
			input := map[string]any{"id": a.ID, "title": "Landing", "content": designManifest, "baseVersion": base, "sourceBases": proof, "files": map[string]any{"frames/hero.html": next}}
			assertStylePublish(t, h, client, tok, a.ID, transport, input, false)
			if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != next {
				t.Fatalf("stale edit must recreate explicit blue after shorthand:\nwant %s\ngot %s", next, got)
			}
			assertStyleSnapshotReload(t, h, a.ID, next)
		})
	}
}

func assertStyleSnapshotReload(t *testing.T, h *liveHarness, id, want string) {
	t.Helper()
	h.clock.Advance(roomkind.Quiet)
	h.app.live.snap.Sweep(t.Context())
	saved, err := h.app.svc.FileTexts(t.Context(), id, 2)
	if err != nil || saved["frames/hero.html"] != want {
		t.Fatalf("snapshot lost recreated override: %v", err)
	}
	state, err := designroom.State(designroom.Design{Manifest: designManifest, Frames: saved})
	if err != nil {
		t.Fatal(err)
	}
	fresh := crdt.New()
	defer fresh.Destroy()
	if err := crdt.ApplyUpdateV1(fresh, state, nil); err != nil {
		t.Fatal(err)
	}
	reloaded, err := designroom.Open(fresh).Read()
	if err != nil || reloaded.Frames["frames/hero.html"] != want {
		t.Fatalf("source reload lost recreated override: %v", err)
	}
}
