package web

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/reearth/ygo/crdt"
)

func TestUnusableLiveSourceBasesRefuseWholeHTTPPublishWithoutDurableOrMetadataChanges(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{"missing", "malformed", "principal", "file", "artifact", "expired", "evicted", "retired", "restore", "restart", "epoch"} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			var disk *assets.Disk
			var book *designsourceedit.ReadBases
			h := newLiveHarness(t, func(cfg *Config) {
				disk = cfg.Assets
				if reason == "evicted" {
					var err error
					book, err = designsourceedit.NewReadBases(service.SourceBaseTTL, service.SourceBaseLimit, cfg.Clock.Now)
					if err != nil {
						t.Fatal(err)
					}
					cfg.SourceReadBases = book
				}
			})
			tok := h.token("alice")
			a := h.publishDesign(tok, designFrames(), store.VisibilityOrganisation)
			original := liveDesignFrame(t, h, a.ID, "frames/hero.html")
			_, out := h.fileText(tok, a.ID, "frames/hero.html", 0)
			if out != original {
				t.Fatal("read mismatch")
			}
			res, _ := h.fileText(tok, a.ID, "frames/hero.html", 0)
			proof := res.Header.Get("X-Pictor-Source-Base")
			if proof == "" || res.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("missing private current proof")
			}
			if book != nil {
				proof = evictHTTPSourceProof(t, h, book, a, tok, original)
			}
			proof = unusableSourceProof(t, h, disk, a, tok, proof, reason)

			before, err := h.st.Artifact(t.Context(), a.ID)
			if err != nil {
				t.Fatal(err)
			}
			room := liveDesignFrame(t, h, a.ID, "frames/hero.html")
			rows, err := h.st.RealtimeRows(t.Context(), a.ID)
			if err != nil {
				t.Fatal(err)
			}
			activityBefore := len(activityPage(t, h.harness, tok, "").Items)
			changed := strings.Replace(original, "Hero", "Agent", 1)
			mobile, err := h.app.svc.ReadSourceFile(t.Context(), h.app.svc.Viewer("alice"), a.ID, "frames/mobile.html", 0)
			if err != nil {
				t.Fatal(err)
			}
			mobileText := strings.Replace(mobile.Text, "Mobile", "Independent change", 1)
			input := map[string]any{"title": "Must not rename", "content": designManifest, "baseVersion": before.Version, "sourceBases": map[string]string{"frames/hero.html": proof, "frames/mobile.html": mobile.SourceBase}, "files": map[string]any{"frames/hero.html": changed, "frames/mobile.html": mobileText}}
			response, body := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, input)
			if response.StatusCode != http.StatusConflict || !strings.Contains(body, "retain your draft") {
				t.Fatalf("refusal = %d %s", response.StatusCode, body)
			}
			assertRefusedSourceUnchanged(t, h, tok, a.ID, before, rows, activityBefore, room, mobile.Text)
		})
	}
}

func TestReadersAndExplicitHistoryReceiveNoWriteBasesAndRevokedEditorsCannotUseOldOnes(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	a := h.publishDesign(tok, designFrames(), store.VisibilityOrganisation)
	_ = liveDesignFrame(t, h, a.ID, "frames/hero.html")
	bob := h.agent(h.token("bob"))
	for _, args := range []map[string]any{{"id": a.ID}, {"id": a.ID, "file": "frames/hero.html"}, {"id": a.ID, "version": 1}, {"id": a.ID, "version": 1, "file": "frames/hero.html"}} {
		read := bob.ok("read", args)
		if _, ok := read["sourceBases"]; ok {
			t.Fatal("reader received write bases")
		}
		if f, ok := read["file"].(map[string]any); ok && f["sourceBase"] != nil {
			t.Fatal("reader file received proof")
		}
	}
	alice := h.agent(tok)
	for _, args := range []map[string]any{{"id": a.ID, "version": 1}, {"id": a.ID, "version": 1, "file": "frames/hero.html"}} {
		read := alice.ok("read", args)
		if _, ok := read["sourceBases"]; ok {
			t.Fatal("history received write bases")
		}
		if f, ok := read["file"].(map[string]any); ok && f["sourceBase"] != nil {
			t.Fatal("history file received proof")
		}
	}
	alice.ok("share", map[string]any{"id": a.ID, "add": []string{"bob"}})
	read := bob.ok("read", map[string]any{"id": a.ID, "file": "frames/hero.html"})
	alice.ok("share", map[string]any{"id": a.ID, "remove": []string{"bob"}})
	text := strings.Replace(read["file"].(map[string]any)["text"].(string), "Hero", "Forbidden", 1)
	refusal := bob.refused("publish", map[string]any{"id": a.ID, "title": "Landing", "content": designManifest, "baseVersion": 1, "sourceBases": read["sourceBases"], "files": map[string]any{"frames/hero.html": text}})
	if !strings.Contains(refusal, "author") && !strings.Contains(refusal, "editor") {
		t.Fatalf("revoked write: %s", refusal)
	}
	if strings.Contains(liveDesignFrame(t, h, a.ID, "frames/hero.html"), "Forbidden") {
		t.Fatal("revoked edit reached room")
	}
}

func TestLiveSourceNoopsAndMetadataNeedNoGuessedBaseButSuppliedInvalidBasesStillRefuse(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	frames := designFrames()
	frames["frames/hero.html"] = `<body><h1 data-pid="title">Title.</h1></body>`
	a := h.publishDesign(tok, frames, "")
	_ = liveDesignFrame(t, h, a.ID, "frames/hero.html")
	appendLiveDesignText(t, h, a.ID, "alice", "title", " Human.")
	res, current := h.fileText(tok, a.ID, "frames/hero.html", 0)
	if res.StatusCode != http.StatusOK {
		t.Fatal("read failed")
	}
	before, _ := h.st.RealtimeRows(t.Context(), a.ID)
	input := map[string]any{"title": "Landing", "content": designManifest, "baseVersion": 1, "files": map[string]any{"frames/hero.html": current}}
	if res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, input); res.StatusCode != http.StatusOK {
		t.Fatalf("identical source refused: %d %s", res.StatusCode, out)
	}
	after, _ := h.st.RealtimeRows(t.Context(), a.ID)
	if after != before {
		t.Fatal("identical-current publish wrote room rows")
	}
	input["sourceBases"] = map[string]string{"frames/hero.html": "malformed"}
	input["title"] = "Must not rename"
	if res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, input); res.StatusCode != http.StatusConflict {
		t.Fatalf("invalid no-op proof ignored: %d %s", res.StatusCode, out)
	}
	delete(input, "sourceBases")
	delete(input, "files")
	input["title"] = "Renamed"
	if res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, input); res.StatusCode != http.StatusOK {
		t.Fatalf("metadata-only refused: %d %s", res.StatusCode, out)
	}
	if got := liveDesignFrame(t, h, a.ID, "frames/hero.html"); got != current {
		t.Fatal("metadata publish overwrote live human source")
	}
	stored, _ := h.st.Artifact(t.Context(), a.ID)
	if stored.Title != "Renamed" || stored.Version != 2 {
		t.Fatal("ordinary rename behavior changed")
	}
}

func unusableSourceProof(t *testing.T, h *liveHarness, disk *assets.Disk, a artifactView, tok, proof, reason string) string {
	t.Helper()
	switch reason {
	case "missing":
		proof = ""
	case "malformed":
		proof = "invalid"
	case "principal":
		_, err := h.app.svc.Share(t.Context(), h.app.svc.Viewer("alice"), a.ID, service.ShareInput{AddEditors: []string{"bob"}})
		if err != nil {
			t.Fatal(err)
		}
		other, _ := h.fileText(h.token("bob"), a.ID, "frames/hero.html", 0)
		proof = other.Header.Get("X-Pictor-Source-Base")
	case "file":
		other, _ := h.fileText(tok, a.ID, "frames/mobile.html", 0)
		proof = other.Header.Get("X-Pictor-Source-Base")
	case "artifact":
		other := h.publishDesign(tok, designFrames(), "")
		_ = liveDesignFrame(t, h, other.ID, "frames/hero.html")
		response, _ := h.fileText(tok, other.ID, "frames/hero.html", 0)
		proof = response.Header.Get("X-Pictor-Source-Base")
	default:
		return retiredSourceProof(t, h, disk, a, proof, reason)
	}
	return proof
}

func retiredSourceProof(t *testing.T, h *liveHarness, disk *assets.Disk, a artifactView, proof, reason string) string {
	t.Helper()
	switch reason {
	case "expired":
		h.clock.Advance(30 * time.Minute)
	case "retired":
		if _, err := h.app.svc.Share(t.Context(), h.app.svc.Viewer("alice"), a.ID, service.ShareInput{AddEditors: []string{"bob"}}); err != nil {
			t.Fatal(err)
		}
	case "restore":
		if _, err := h.app.svc.Restore(t.Context(), h.app.svc.Viewer("alice"), a.ID, 1, 1); err != nil {
			t.Fatal(err)
		}
	case "restart":
		fresh := service.New(h.st, disk, false)
		fresh.SetArtifactGuard(h.app.svc.ArtifactGuard())
		fresh.SetClock(h.clock.Now)
		fresh.SetLiveRooms(h.app.live.snap)
		h.app.svc = fresh
	case "epoch":
		var state []byte
		if err := h.app.live.hub.Read(t.Context(), a.ID, func(doc *crdt.Doc) error { state = crdt.EncodeStateAsUpdateV1(doc, nil); return nil }); err != nil {
			t.Fatal(err)
		}
		if err := h.app.live.hub.Replace(t.Context(), a.ID, func(doc *crdt.Doc) error { return crdt.ApplyUpdateV1(doc, state, nil) }); err != nil {
			t.Fatal(err)
		}
	}
	return proof
}

func assertRefusedSourceUnchanged(t *testing.T, h *liveHarness, tok, id string, before store.Artifact, rows, activityBefore int, room, mobile string) {
	t.Helper()
	after, _ := h.st.Artifact(t.Context(), id)
	afterRows, _ := h.st.RealtimeRows(t.Context(), id)
	if len(activityPage(t, h.harness, tok, "").Items) != activityBefore || after.Title != before.Title || after.Version != before.Version || afterRows != rows || liveDesignFrame(t, h, id, "frames/hero.html") != room || liveDesignFrame(t, h, id, "frames/mobile.html") != mobile {
		t.Fatal("refusal changed durable rows, metadata or source")
	}
}

// evictHTTPSourceProof keeps transport minting and real registry eviction in one fixture.
func evictHTTPSourceProof(t *testing.T, h *liveHarness, book *designsourceedit.ReadBases, a artifactView, tok, original string) string {
	t.Helper()
	// A late collaborator registration cannot replace the live registry.
	replacement, err := designsourceedit.NewReadBases(service.SourceBaseTTL, service.SourceBaseLimit, h.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	h.app.svc.SetSourceReadBases(replacement)
	res, _ := h.fileText(tok, a.ID, "frames/hero.html", 0)
	proof := res.Header.Get("X-Pictor-Source-Base")
	_, epoch, err := h.app.live.snap.SourceSnapshot(t.Context(), a.ID, a.Kind)
	if err != nil {
		t.Fatal(err)
	}
	scope := designsourceedit.Scope{ArtifactID: a.ID, File: "frames/hero.html", Format: a.Format, LiveEpoch: epoch, Principal: "alice"}
	if read, err := book.Load(proof, scope); err != nil || read.Source != original {
		t.Fatalf("HTTP proof must use the original injected registry: %v", err)
	}
	// Fill the same production-sized registry without re-reading SQLite
	// and decoding the room for each unrelated snapshot.
	basis := designsourceedit.Basis{ArtifactID: "other", File: "frames/fill.html", Format: "1", SavedVersion: 1, LiveEpoch: "fill", Principal: "alice"}
	for range service.SourceBaseLimit {
		if _, err := book.Remember(basis, ""); err != nil {
			t.Fatal(err)
		}
	}
	return proof
}
