package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestSpecificIsRefusedWhereverAnAudienceIsChosen(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	alice := h.browser("alice", "pw-a")
	tok := h.token("alice")
	bearer := map[string]string{"Authorization": "Bearer " + tok}
	a := h.publish(tok, "Notes", "<p>x")

	for name, c := range map[string]struct {
		method, path, body string
		headers            map[string]string
		client             *http.Client
	}{
		"publish":   {"POST", "/api/artifacts", `{"title":"t","html":"x","visibility":"specific"}`, bearer, nil},
		"republish": {"PUT", "/api/artifacts/" + a.ID, `{"title":"t","html":"x","visibility":"specific"}`, bearer, nil},
		"patch":     {"PATCH", "/api/artifacts/" + a.ID, `{"visibility":"specific"}`, bearer, nil},
		"viewer":    {"PATCH", "/api/artifacts/" + a.ID, `{"visibility":"specific"}`, bridgeHeaders(h), alice},
	} {
		res, out := h.do(c.client, c.method, c.path, c.body, c.headers)
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "must be") {
			t.Errorf("%s: %d %s, want 400", name, res.StatusCode, out)
		}
	}
	if got, _ := h.st.Artifact(t.Context(), a.ID); got.Visibility != store.VisibilityPrivate {
		t.Errorf("a refused request changed the audience to %q", got.Visibility)
	}
}

func TestGroupsAndUsersNameNoAudienceAndAnArtifactCarriesNoGrants(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.browser("alice", "pw-a")
	tok := h.token("alice")
	bearer := map[string]string{"Authorization": "Bearer " + tok}

	res, out := h.do(nil, "POST", "/api/artifacts", `{"title":"t","html":"x","groups":["SE"],"users":["bob"]}`, bearer)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish: %d %s", res.StatusCode, out)
	}
	created := decode[artifactView](t, out)
	if created.Visibility != store.VisibilityPrivate {
		t.Errorf("naming groups widened the audience to %q", created.Visibility)
	}
	if res, _ := h.do(h.browser("bob", "pw-b"), "GET", "/a/"+created.ID, "", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("a named person opens the artifact: %d", res.StatusCode)
	}

	_, got := h.do(nil, "GET", "/api/artifacts/"+created.ID, "", bearer)
	_, list := h.do(nil, "GET", "/api/artifacts", "", bearer)
	for name, body := range map[string]string{"artifact": got, "list": list, "publish": out} {
		if strings.Contains(body, "grants") || strings.Contains(body, "groups") {
			t.Errorf("the %s response still carries sharing details: %s", name, body)
		}
	}
}

func TestWhoamiReportsTheGroupsReadAtSignIn(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.dir.setGroups("bob", "Ops")
	bob := h.browser("bob", "pw-b")
	a := h.publish(h.token("bob"), "Notes", "<p>x")
	_, body := h.do(bob, "GET", "/api/artifacts/"+a.ID+"/me", "", bridgeHeaders(h))
	if me := decode[store.User](t, body); len(me.Groups) != 1 || me.Groups[0] != "Ops" {
		t.Errorf("whoami groups: %s", body)
	}
}
