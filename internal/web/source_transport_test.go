package web

import (
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/reearth/ygo/crdt"
)

func TestHTTPDesignReadPairsExactLiveManifestWithEditorProofsAndReaderData(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	alice, bob := h.token("alice"), h.token("bob")
	a := h.publishDesign(alice, designFrames(), store.VisibilityOrganisation)
	_ = liveDesignFrame(t, h, a.ID, "frames/hero.html")
	held, err := h.app.live.snap.Content(t.Context(), a.ID, "design")
	if err != nil {
		t.Fatal(err)
	}
	liveManifest := strings.Replace(held.Content, "Hero", "Live Hero", 1)
	if err := h.app.live.hub.Edit(t.Context(), a.ID, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return designroom.Open(doc).Write(designroom.Design{Manifest: liveManifest, Frames: held.Files})
	}); err != nil {
		t.Fatal(err)
	}
	saved, err := h.st.Artifact(t.Context(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := h.app.live.snap.Content(t.Context(), a.ID, "design")
	if err != nil || current.Content == saved.Content {
		t.Fatal("live manifest must differ from saved source")
	}
	var editor designSourceResponse
	for _, who := range []struct {
		token  string
		editor bool
	}{{alice, true}, {bob, false}} {
		got := readHTTPDesignManifest(t, h, who.token, a.ID, current.Content)

		if (got.SourceBases["design.json"] != "") != who.editor {
			t.Fatal("manifest proof does not match edit permission")
		}
		if who.editor {
			editor = got
		}
	}
	changed := strings.Replace(*editor.Content, "Live Hero", "Published Hero", 1)
	input := map[string]any{"title": "Landing", "content": changed, "baseVersion": editor.Version, "sourceBases": editor.SourceBases}
	if res, raw := h.sendFiles("PUT", "/api/artifacts/"+a.ID, alice, input); res.StatusCode != http.StatusOK {
		t.Fatalf("paired HTTP source/proof unusable: %d %s", res.StatusCode, raw)
	}
	current, err = h.app.live.snap.Content(t.Context(), a.ID, "design")
	if err != nil || !strings.Contains(current.Content, "Published Hero") {
		t.Fatal("HTTP manifest publish did not reach room")
	}
	res, _ := h.fileText(bob, a.ID, "frames/hero.html", 1)
	if res.StatusCode != http.StatusOK || res.Header.Get("X-Pictor-Source-Base") != "" {
		t.Fatal("reader latest saved file must remain readable without proof")
	}
}

func TestHTTPNonDesignArtifactReadRetainsMetadataOnlyWhileSavedAndLiveContentDiffer(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	alice := h.token("alice")
	res, raw := api(h.harness, "POST", "/api/artifacts", `{"title":"Doc","kind":"doc","content":"# Saved\n\nSaved body"}`, alice)
	if res.StatusCode != http.StatusCreated {
		t.Fatal("doc creation failed")
	}
	doc := decode[artifactView](t, raw)
	saved, err := h.st.Artifact(t.Context(), doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed := docfmt.Doc{Tabs: []docfmt.Tab{{ID: "t_doc", Title: "Document", Blocks: []docfmt.Block{{ID: "b_doc", Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: "Unsaved live body"}}}}}}}
	if err := h.app.live.hub.Edit(t.Context(), doc.ID, realtime.Identity{Username: "alice"}, func(d *crdt.Doc) (func(*crdt.Transaction), error) { return docroom.Open(d).Write(changed), nil }); err != nil {
		t.Fatal(err)
	}
	live, err := h.app.svc.Current(t.Context(), saved)
	if err != nil || live.Content == saved.Content {
		t.Fatal("fixture must have different saved and live content")
	}
	res, raw = api(h.harness, "GET", "/api/artifacts/"+doc.ID, "", alice)
	got := decode[map[string]json.RawMessage](t, raw)
	if res.StatusCode != http.StatusOK || got["content"] != nil || got["sourceBases"] != nil {
		t.Fatal("non-Design HTTP metadata response exposed source content or bases")
	}
}

func TestUnopenedDesignProofRefusalExplainsHowToPublishItsRetainedDraft(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	a := h.publishDesign(tok, designFrames(), "")
	if rows, err := h.st.RealtimeRows(t.Context(), a.ID); err != nil || rows != 0 {
		t.Fatal("fixture must genuinely have no durable room")
	}
	changed := strings.Replace(designFrames()["frames/hero.html"], "Hero", "Draft", 1)
	input := map[string]any{"title": "Landing", "content": designManifest, "baseVersion": 1, "sourceBases": map[string]string{"frames/hero.html": "unusable"}, "files": map[string]any{"frames/hero.html": changed}}
	res, raw := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, input)
	if res.StatusCode != http.StatusConflict || !strings.Contains(raw, "without sourceBases") || !strings.Contains(raw, "baseVersion") {
		t.Fatalf("unactionable refusal: %d %s", res.StatusCode, raw)
	}
	_, raw = api(h.harness, "GET", "/api/artifacts/"+a.ID, "", tok)
	read := decode[artifactView](t, raw)
	if len(read.SourceBases) != 0 {
		t.Fatal("unopened design issued proofs")
	}
	delete(input, "sourceBases")
	input["baseVersion"] = read.Version
	if res, raw := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, input); res.StatusCode != http.StatusOK {
		t.Fatalf("reconciled static draft refused: %d %s", res.StatusCode, raw)
	}
}

func TestWholeFrameRemovalRequiresAFreshExplicitProofBeforeAnyMutation(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	frames := designFrames()
	frames["frames/footer.html"] = `<p data-pid="foot">Footer</p>`
	a := h.publishDesign(tok, frames, "")
	_ = liveDesignFrame(t, h, a.ID, "frames/footer.html")
	_, raw := api(h.harness, "GET", "/api/artifacts/"+a.ID, "", tok)
	manifest := decode[artifactView](t, raw)
	res, _ := h.fileText(tok, a.ID, "frames/footer.html", 0)
	old := res.Header.Get("X-Pictor-Source-Base")
	bases := map[string]string{"design.json": manifest.SourceBases["design.json"]}
	input := map[string]any{"title": "Removal", "content": twoFrameManifest, "baseVersion": 1, "sourceBases": bases, "files": map[string]any{"frames/footer.html": nil}}
	assertRemovalRefusesUnchanged(t, h, tok, a.ID, input)
	if err := h.app.live.hub.Edit(t.Context(), a.ID, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return designroom.Open(doc).SetStyle("frames/footer.html", "foot", "color", "blue")
	}); err != nil {
		t.Fatal(err)
	}
	bases["frames/footer.html"] = old
	assertRemovalRefusesUnchanged(t, h, tok, a.ID, input)
	fresh, _ := h.fileText(tok, a.ID, "frames/footer.html", 0)
	bases["frames/footer.html"] = fresh.Header.Get("X-Pictor-Source-Base")
	if res, raw := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, input); res.StatusCode != http.StatusOK {
		t.Fatalf("fresh removal refused: %d %s", res.StatusCode, raw)
	}
	held, err := h.app.live.snap.Content(t.Context(), a.ID, "design")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := held.Files["frames/footer.html"]; exists || strings.Contains(held.Content, "footer") {
		t.Fatal("fresh removal did not update frame and manifest together")
	}
}

func assertRemovalRefusesUnchanged(t *testing.T, h *liveHarness, tok, id string, input map[string]any) {
	t.Helper()
	before, _ := h.st.Artifact(t.Context(), id)
	rows, _ := h.st.RealtimeRows(t.Context(), id)
	held, err := h.app.live.snap.Content(t.Context(), id, "design")
	if err != nil {
		t.Fatal(err)
	}
	activity := len(activityPage(t, h.harness, tok, "").Items)
	res, raw := h.sendFiles("PUT", "/api/artifacts/"+id, tok, input)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("unsafe removal accepted: %d %s", res.StatusCode, raw)
	}
	after, _ := h.st.Artifact(t.Context(), id)
	afterRows, _ := h.st.RealtimeRows(t.Context(), id)
	heldAfter, err := h.app.live.snap.Content(t.Context(), id, "design")
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before.Version || after.Title != before.Title || rows != afterRows || len(activityPage(t, h.harness, tok, "").Items) != activity || heldAfter.Content != held.Content || !maps.Equal(heldAfter.Files, held.Files) {
		t.Fatal("refused removal changed frame, log, metadata or activity")
	}
}

type designSourceResponse struct {
	artifactView
	Content *string `json:"content"`
}

func readHTTPDesignManifest(t *testing.T, h *liveHarness, tok, id, want string) designSourceResponse {
	t.Helper()
	res, raw := api(h.harness, "GET", "/api/artifacts/"+id, "", tok)
	got := decode[designSourceResponse](t, raw)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("manifest response must succeed without caching")
	}
	if got.Content == nil || *got.Content != want {
		t.Fatal("HTTP manifest is absent or differs from exact live bytes")
	}
	return got
}
