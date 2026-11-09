package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/kind/kindtest"
)

func putJSON(h *harness, method, path, body, tok string) (response, string) {
	return h.do(nil, method, path, body, map[string]string{"Authorization": "Bearer " + tok})
}

func TestAnArtifactPublishedUnderV1StillRendersWithV1AfterTheBinaryShipsV2(t *testing.T) { //nolint:paralleltest // kindtest.Register changes the global kind registry
	h := newHarness(t)
	tok := h.token("alice")

	removeV1Only := kindtest.Register(t, "v1")
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"Old","kind":"versioned","content":"hello","visibility":"organisation"}`, tok)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish: %d %s", res.StatusCode, out)
	}
	old := decode[artifactView](t, out)
	if old.Format != "v1" {
		t.Fatalf("a new artifact of a kind with one version got format %q", old.Format)
	}

	removeV1Only() // the next binary ships v2 as well
	kindtest.Register(t, "v1", "v2")

	bob := h.browser("bob", "pw-b")
	if _, body := h.do(bob, "GET", "/raw/"+old.ID, "", nil); !strings.Contains(body, `data-runtime="v1"`) {
		t.Errorf("the old artifact was drawn with another runtime: %s", body)
	}

	res, out = putJSON(h, "POST", "/api/artifacts", `{"title":"New","kind":"versioned","content":"hello","visibility":"organisation"}`, tok)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish new: %d %s", res.StatusCode, out)
	}
	fresh := decode[artifactView](t, out)
	if fresh.Format != "v2" {
		t.Errorf("a new artifact got format %q, want the current v2", fresh.Format)
	}
	if _, body := h.do(bob, "GET", "/raw/"+fresh.ID, "", nil); !strings.Contains(body, `data-runtime="v2"`) {
		t.Errorf("the new artifact was not drawn with v2: %s", body)
	}
}

func TestRepublishingKeepsTheFormatUnlessTheAuthorUpgradesIt(t *testing.T) { //nolint:paralleltest // kindtest.Register changes the global kind registry
	h := newHarness(t)
	tok := h.token("alice")
	removeV1Only := kindtest.Register(t, "v1")
	_, out := putJSON(h, "POST", "/api/artifacts", `{"title":"T","kind":"versioned","content":"`+kindtest.RejectedInV2+`"}`, tok)
	a := decode[artifactView](t, out)
	removeV1Only()
	kindtest.Register(t, "v1", "v2")

	// Content only v1 accepts is still republishable: the artifact stays on v1.
	res, out := putJSON(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"T","content":"`+kindtest.RejectedInV2+`"}`, tok)
	if got := decode[artifactView](t, out); res.StatusCode != http.StatusOK || got.Format != "v1" {
		t.Fatalf("republish: %d %+v", res.StatusCode, got)
	}
	// Upgrading validates the content against the version it upgrades to.
	res, out = putJSON(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"T","format":"v2","content":"`+kindtest.RejectedInV2+`"}`, tok)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "v2 no longer accepts") {
		t.Fatalf("upgrade with content v2 refuses: %d %s", res.StatusCode, out)
	}
	if got, _ := h.st.Artifact(t.Context(), a.ID); got.Format != "v1" {
		t.Errorf("a refused upgrade changed the format to %q", got.Format)
	}
	res, out = putJSON(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"T","format":"v2","content":"rewritten"}`, tok)
	if got := decode[artifactView](t, out); res.StatusCode != http.StatusOK || got.Format != "v2" {
		t.Fatalf("upgrade: %d %s", res.StatusCode, out)
	}
	// And the upgrade sticks on the next plain republish.
	_, out = putJSON(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"T","content":"again"}`, tok)
	if got := decode[artifactView](t, out); got.Format != "v2" {
		t.Errorf("republish after upgrade fell back to %q", got.Format)
	}
}

func TestAFormatTheKindDoesNotHaveIsRefusedWithTheVersionsItHas(t *testing.T) { //nolint:paralleltest // kindtest.Register changes the global kind registry
	h := newHarness(t)
	tok := h.token("alice")
	kindtest.Register(t, "v1", "v2")

	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"T","kind":"versioned","format":"v9","content":"x"}`, tok)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "v9") || !strings.Contains(out, "v1, v2") {
		t.Errorf("unknown version: %d %s", res.StatusCode, out)
	}
	res, out = putJSON(h, "POST", "/api/artifacts", `{"title":"T","format":"v1","content":"<p>x"}`, tok)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "omit format") {
		t.Errorf("html has no versions: %d %s", res.StatusCode, out)
	}
}

func TestAnArtifactOfAFormatThisBuildLacksIsNotRendered(t *testing.T) { //nolint:paralleltest // kindtest.Register changes the global kind registry
	h := newHarness(t)
	remove := kindtest.Register(t, "v1", "v2")
	_, out := putJSON(h, "POST", "/api/artifacts", `{"title":"T","kind":"versioned","content":"x","visibility":"organisation"}`, h.token("alice"))
	a := decode[artifactView](t, out)
	remove() // rolled back to a build that only knows v1
	kindtest.Register(t, "v1")

	res, _ := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil)
	if res.StatusCode != http.StatusNotImplemented {
		t.Errorf("raw of a newer format: %d, want 501", res.StatusCode)
	}
}

func TestAnHTMLArtifactHasNoFormatAndItsAPIAnswersStayTheSame(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Report", "<p>hi")
	if a.Format != "" {
		t.Errorf("html artifact has format %q", a.Format)
	}
	_, out := putJSON(h, "GET", "/api/artifacts/"+a.ID, "", h.token("alice"))
	if strings.Contains(out, `"format"`) {
		t.Errorf("an unversioned artifact grew a format field: %s", out)
	}
}

func TestAnAgentSeesFormatVersionsAndPublishesAndReadsTheFormat(t *testing.T) { //nolint:paralleltest // kindtest.Register changes the global kind registry
	h := newHarness(t)
	remove := kindtest.Register(t, "v1")
	alice := h.agent(h.token("alice"))
	made := alice.ok("publish", map[string]any{"title": "T", "kind": "versioned", "content": "x"})
	id := made["id"].(string)
	if made["format"] != "v1" {
		t.Fatalf("publish = %v", made)
	}
	remove()
	kindtest.Register(t, "v1", "v2")

	for _, k := range alice.ok("kinds", nil)["kinds"].([]any) {
		k := k.(map[string]any)
		switch k["name"] {
		case "versioned":
			if v := k["formatVersions"].([]any); len(v) != 2 || k["currentFormat"] != "v2" {
				t.Errorf("versioned kind = %v", k)
			}
		case "html":
			if v := k["formatVersions"].([]any); len(v) != 0 || k["currentFormat"] != "" {
				t.Errorf("html kind = %v", k)
			}
		}
	}
	if got := alice.ok("read", map[string]any{"id": id}); got["format"] != "v1" {
		t.Errorf("read = %v", got)
	}
	if got := alice.ok("publish", map[string]any{"id": id, "title": "T", "content": "y"}); got["format"] != "v1" {
		t.Errorf("update kept %v", got["format"])
	}
	if got := alice.ok("publish", map[string]any{"id": id, "title": "T", "content": "y", "format": "v2"}); got["format"] != "v2" {
		t.Errorf("upgrade gave %v", got["format"])
	}
}

func TestAnArtifactOfAKindThisBuildLacksIsNotServedAsHTML(t *testing.T) { //nolint:paralleltest // kindtest.Register changes the global kind registry
	h := newHarness(t)
	remove := kindtest.Register(t, "v1")
	const secretContent = "content-that-must-not-be-rendered"
	_, out := putJSON(h, "POST", "/api/artifacts", `{"title":"T","kind":"versioned","content":"`+secretContent+`","visibility":"organisation"}`, h.token("alice"))
	a := decode[artifactView](t, out)
	remove() // a build that has never heard of the kind

	for who, client := range map[string]*http.Client{"a colleague": h.browser("bob", "pw-b"), "the author": h.browser("alice", "pw-a")} {
		res, body := h.do(client, "GET", "/raw/"+a.ID, "", nil)
		if res.StatusCode != http.StatusNotImplemented {
			t.Errorf("raw for %s: %d, want 501", who, res.StatusCode)
		}
		if strings.Contains(body, secretContent) || strings.Contains(strings.ToLower(body), "<!doctype html") {
			t.Errorf("raw for %s rendered the artifact anyway: %s", who, body)
		}
	}
	res, body := h.do(h.browser("bob", "pw-b"), "GET", "/a/"+a.ID, "", nil)
	if strings.Contains(body, secretContent) {
		t.Errorf("the viewer page (%d) carries the content of a kind it cannot render", res.StatusCode)
	}
}
