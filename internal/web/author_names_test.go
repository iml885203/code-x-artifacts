package web

import (
	"net/http"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestTheLibraryUsesCurrentAuthorNamesWithoutChangingIdentityOrAccess(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := t.Context()
	owner, err := h.st.ResolveOIDCUser(ctx, "https://identity.example", "author", store.User{DisplayName: "Example Author"})
	if err != nil {
		t.Fatal(err)
	}
	ownerToken := h.token(owner.Username)
	shared := h.publishShared(ownerToken, "Shared report", page)
	private := h.publish(ownerToken, "Private report", page)
	viewer := h.browser("bob", "pw-b")
	assertLibraryAuthorName(t, h, viewer, shared.ID, owner.Username, "Example Author")
	renamed, err := h.st.ResolveOIDCUser(ctx, "https://identity.example", "author", store.User{DisplayName: "Renamed Author"})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Username != owner.Username {
		t.Fatal("profile rename changed identity")
	}
	assertLibraryAuthorName(t, h, viewer, shared.ID, owner.Username, "Renamed Author")
	res, _ := h.do(viewer, "GET", "/api/artifacts/"+private.ID, "", bridgeHeaders(h))
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("private artifact = %d", res.StatusCode)
	}
	res, _ = h.do(viewer, "PUT", "/api/artifacts/"+shared.ID+"/title", `{"title":"Taken over"}`, bridgeHeaders(h))
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("reader rename = %d", res.StatusCode)
	}
	agent := h.agent(h.token("bob"))
	for _, scope := range []string{"all", "shared", "mine"} {
		got := agent.ok("list", map[string]any{"query": "RENAMED AUTHOR", "scope": scope})
		want := float64(1)
		if scope == "mine" {
			want = 0
		}
		if got["total"] != want {
			t.Fatalf("display-name search %s = %v", scope, got)
		}
	}
}

func assertLibraryAuthorName(t *testing.T, h *harness, viewer *http.Client, id, owner, want string) {
	t.Helper()
	list := decode[[]artifactView](t, galleryList(h, viewer))
	if len(list) != 1 || list[0].ID != id || list[0].Author != owner || list[0].AuthorDisplayName != want || list[0].CanChange || list[0].CanEdit {
		t.Fatalf("library = %+v, want artifact by %q named %q without edit rights", list, owner, want)
	}
	res, body := h.do(viewer, "GET", "/api/artifacts/"+id, "", bridgeHeaders(h))
	got := decode[artifactView](t, body)
	if res.StatusCode != http.StatusOK || got.Author != owner || got.AuthorDisplayName != want {
		t.Fatalf("artifact = %d %+v", res.StatusCode, got)
	}
}

func TestLibraryAuthorNamesFallBackForMissingOrBlankProfilesAndKeepLocalNames(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, tc := range []struct {
		account, name, want string
		profile             bool
	}{
		{"missing", "", "missing", false},
		{"blank", " \t\n", "blank", true},
		{"local", "Local Author", "Local Author", true},
	} {
		if tc.profile {
			if err := h.st.SaveUser(t.Context(), store.User{Username: tc.account, DisplayName: tc.name}); err != nil {
				t.Fatal(err)
			}
		}
		a := h.publishShared(h.token(tc.account), tc.account+" report", page)
		if a.Author != tc.account || a.AuthorDisplayName != tc.want {
			t.Fatalf("published author = %+v", a)
		}
	}
	viewer := h.browser("bob", "pw-b")
	list := decode[[]artifactView](t, galleryList(h, viewer))
	names := map[string]string{}
	for _, a := range list {
		names[a.Author] = a.AuthorDisplayName
	}
	if len(names) != 3 || names["missing"] != "missing" || names["blank"] != "blank" || names["local"] != "Local Author" {
		t.Fatalf("names = %v", names)
	}
}

func TestNewArtifactsImmediatelyCarryTheAuthorsDisplayName(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	owner, err := h.st.ResolveOIDCUser(t.Context(), "https://identity.example", "creator", store.User{DisplayName: "Example Creator"})
	if err != nil {
		t.Fatal(err)
	}
	token := h.token(owner.Username)
	published := h.publish(token, "New report", page)
	if published.Author != owner.Username || published.AuthorDisplayName != "Example Creator" {
		t.Fatalf("published author = %+v", published)
	}
	res, body := api(h, "POST", "/api/new", `{"kind":"doc","title":"New document"}`, token)
	created := decode[artifactView](t, body)
	if res.StatusCode != http.StatusCreated || created.Author != owner.Username || created.AuthorDisplayName != "Example Creator" {
		t.Fatalf("new document author = %d %+v", res.StatusCode, created)
	}
	res, body = api(h, "POST", "/api/artifacts/"+published.ID+"/copy", "{}", token)
	copied := decode[artifactView](t, body)
	if res.StatusCode != http.StatusCreated || copied.Author != owner.Username || copied.AuthorDisplayName != "Example Creator" {
		t.Fatalf("copied author = %d %+v", res.StatusCode, copied)
	}
}
