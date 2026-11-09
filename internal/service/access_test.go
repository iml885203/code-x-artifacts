package service

import (
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestAccessRules(t *testing.T) {
	anonymous := Viewer{PublicLinks: true}
	cases := []struct {
		name   string
		Viewer Viewer
		art    store.Artifact
		view   bool
		change bool
		write  bool
	}{
		{"author of a private artifact", Viewer{Username: "alice"}, store.Artifact{Visibility: store.VisibilityPrivate}, true, true, true},
		{"someone else, private", Viewer{Username: "bob"}, store.Artifact{Visibility: store.VisibilityPrivate}, false, false, false},
		{"author of an organisation artifact", Viewer{Username: "alice"}, store.Artifact{Visibility: store.VisibilityOrganisation}, true, true, true},
		{"someone else, organisation", Viewer{Username: "bob"}, store.Artifact{Visibility: store.VisibilityOrganisation}, true, false, true},
		{"someone else, a visibility this version does not know", Viewer{Username: "bob"}, store.Artifact{Visibility: "everyone-on-earth"}, false, false, false},
		{"author, a visibility this version does not know", Viewer{Username: "alice"}, store.Artifact{Visibility: "everyone-on-earth"}, true, true, true},

		{"someone else, public", Viewer{Username: "bob", PublicLinks: true}, store.Artifact{Visibility: store.VisibilityPublic}, true, false, true},
		{"someone else, public with public links off", Viewer{Username: "bob"}, store.Artifact{Visibility: store.VisibilityPublic}, true, false, true},
		{"anonymous, public", anonymous, store.Artifact{Visibility: store.VisibilityPublic}, true, false, false},
		{"anonymous, public with public links off", Viewer{}, store.Artifact{Visibility: store.VisibilityPublic}, false, false, false},
		{"anonymous, organisation", anonymous, store.Artifact{Visibility: store.VisibilityOrganisation}, false, false, false},
		{"anonymous, private", anonymous, store.Artifact{Visibility: store.VisibilityPrivate}, false, false, false},
		{"anonymous, a visibility this version does not know", anonymous, store.Artifact{Visibility: "everyone-on-earth"}, false, false, false},
		{"someone else, specific, which no longer exists", Viewer{Username: "bob"}, store.Artifact{Visibility: "specific"}, false, false, false},
		{"anonymous, specific, which no longer exists", anonymous, store.Artifact{Visibility: "specific"}, false, false, false},
	}
	for _, c := range cases {
		c.art.Author = "alice"
		if got := c.Viewer.CanView(c.art); got != c.view {
			t.Errorf("%s: CanView = %v, want %v", c.name, got, c.view)
		}
		if got := c.Viewer.CanChange(c.art); got != c.change {
			t.Errorf("%s: CanChange = %v, want %v", c.name, got, c.change)
		}
		if got := c.Viewer.CanWrite(c.art); got != c.write {
			t.Errorf("%s: CanWrite = %v, want %v", c.name, got, c.write)
		}
		// The room's unsaved work is for the people who may join it: everyone signed in who sees the artifact.
		if got := c.Viewer.CanSeeLive(c.art); got != c.write {
			t.Errorf("%s: CanSeeLive = %v, want %v", c.name, got, c.write)
		}
		// Who is viewing now is for the same people; a guest is exactly the anonymous viewer who may see it.
		if got := c.Viewer.CanSeeViewers(c.art); got != c.write {
			t.Errorf("%s: CanSeeViewers = %v, want %v", c.name, got, c.write)
		}
		if got, want := c.Viewer.CanBeAGuest(c.art), c.Viewer.Anonymous() && c.view; got != want {
			t.Errorf("%s: CanBeAGuest = %v, want %v", c.name, got, want)
		}
	}

	// Usage is for the accounts the deployment named, whatever artifacts anyone owns.
	readers := []string{"carol"}
	usage := []struct {
		name   string
		Viewer Viewer
		read   bool
	}{
		{"a named reader", Viewer{Username: "carol", UsageReaders: readers}, true},
		{"someone not named, who authors artifacts", Viewer{Username: "alice", UsageReaders: readers}, false},
		{"an author when nobody is named", Viewer{Username: "alice"}, false},
		{"a reader's name when nobody is named", Viewer{Username: "carol"}, false},
		{"anonymous", Viewer{PublicLinks: true, UsageReaders: readers}, false},
		{"anonymous, with an empty name among the readers", Viewer{UsageReaders: []string{""}}, false},
		{"a page's grant", Viewer{UsageReaders: readers, grant: Grant{ArtifactID: "art", Visibility: store.VisibilityPublic}}, false},
	}
	for _, c := range usage {
		if got := c.Viewer.CanReadUsage(); got != c.read {
			t.Errorf("%s: CanReadUsage = %v, want %v", c.name, got, c.read)
		}
	}
}

func TestAPagesGrantSeesItsArtifactsVersionsButNeverItsRoom(t *testing.T) {
	a := store.Artifact{ID: "art", Author: "alice", Visibility: store.VisibilityPrivate}
	granted := (&Service{}).Granted(Grant{ArtifactID: "art", Visibility: store.VisibilityPrivate, Version: 1})
	if !granted.CanView(a) || granted.CanSeeLive(a) || granted.CanSeeViewers(a) {
		t.Errorf("a page's grant: CanView = %v, CanSeeLive = %v, CanSeeViewers = %v", granted.CanView(a), granted.CanSeeLive(a), granted.CanSeeViewers(a))
	}
	public := store.Artifact{ID: "art", Author: "alice", Visibility: store.VisibilityPublic}
	if g := (&Service{publicLinks: true}).Granted(Grant{ArtifactID: "art", Visibility: store.VisibilityPublic, Version: 1}); g.CanBeAGuest(public) {
		t.Error("a page's grant is listed as a guest")
	}
}

// The anonymous viewer has an empty username; an artifact whose author is
// empty must not make them its author.
func TestTheAnonymousViewerIsNeverAnAuthor(t *testing.T) {
	a := store.Artifact{Author: "", Visibility: store.VisibilityPrivate}
	v := Viewer{PublicLinks: true}
	if v.CanView(a) || v.CanChange(a) || v.CanWrite(a) {
		t.Error("an anonymous viewer matched an artifact with no author")
	}
}

func TestOnlySignedInViewersSeeWhoWroteADocument(t *testing.T) {
	d := store.Doc{ID: "1", UpdatedBy: "alice"}
	if got := (Viewer{}).Redact(d).UpdatedBy; got != "" {
		t.Errorf("anonymous sees %q", got)
	}
	if got := (Viewer{Username: "bob"}).Redact(d).UpdatedBy; got != "alice" {
		t.Errorf("signed-in sees %q", got)
	}
}

func TestAGrantOpensOnlyItsOwnArtifactWhileItsAudienceIsUnchanged(t *testing.T) {
	svc := &Service{}
	granted := svc.Granted(Grant{ArtifactID: "a1", Visibility: store.VisibilityPrivate})
	cases := []struct {
		name string
		art  store.Artifact
		view bool
	}{
		{"the artifact it names", store.Artifact{ID: "a1", Author: "alice", Visibility: store.VisibilityPrivate}, true},
		{"the same artifact after it was shared", store.Artifact{ID: "a1", Author: "alice", Visibility: store.VisibilityOrganisation}, false},
		{"the same artifact after it was made public", store.Artifact{ID: "a1", Author: "alice", Visibility: store.VisibilityPublic}, false},
		{"another artifact", store.Artifact{ID: "a2", Author: "alice", Visibility: store.VisibilityPrivate}, false},
	}
	for _, c := range cases {
		if got := granted.CanView(c.art); got != c.view {
			t.Errorf("%s: CanView = %v, want %v", c.name, got, c.view)
		}
		if granted.CanChange(c.art) || granted.CanWrite(c.art) {
			t.Errorf("%s: a grant may change or write", c.name)
		}
	}
	if (Viewer{}).CanView(store.Artifact{ID: "", Visibility: store.VisibilityPrivate}) {
		t.Error("an empty grant matched an artifact with an empty id")
	}
}

func TestOnlySignedInPeopleWhoCanSeeAnArtifactMayComment(t *testing.T) {
	cases := []struct {
		name   string
		viewer Viewer
		art    store.Artifact
		want   bool
	}{
		{"the author of a private artifact", Viewer{Username: "alice"}, store.Artifact{Visibility: store.VisibilityPrivate}, true},
		{"someone else, private", Viewer{Username: "bob"}, store.Artifact{Visibility: store.VisibilityPrivate}, false},
		{"someone else, organisation", Viewer{Username: "bob"}, store.Artifact{Visibility: store.VisibilityOrganisation}, true},
		{"someone else, public", Viewer{Username: "bob", PublicLinks: true}, store.Artifact{Visibility: store.VisibilityPublic}, true},
		{"anonymous, public", Viewer{PublicLinks: true}, store.Artifact{Visibility: store.VisibilityPublic}, false},
		{"someone else, a visibility this version does not know", Viewer{Username: "bob"}, store.Artifact{Visibility: "everyone-on-earth"}, false},
	}
	for _, c := range cases {
		c.art.Author = "alice"
		if got := c.viewer.CanComment(c.art); got != c.want {
			t.Errorf("%s: CanComment = %v, want %v", c.name, got, c.want)
		}
	}
	granted := (&Service{}).Granted(Grant{ArtifactID: "a1", Visibility: store.VisibilityPrivate})
	if granted.CanComment(store.Artifact{ID: "a1", Author: "alice", Visibility: store.VisibilityPrivate}) {
		t.Error("a file grant may comment")
	}
}

func TestEditingRules(t *testing.T) {
	anonymous := Viewer{PublicLinks: true}
	base := func(v store.Visibility, orgEdit bool, editors ...string) store.Artifact {
		return store.Artifact{Author: "alice", Visibility: v, OrganisationEdit: orgEdit, Editors: editors}
	}
	type outcome struct{ view, publish, write, change bool }
	// publish stands for publish, restore, history and files: all are CanEdit.
	// change stands for the audience, the editors and delete: all are CanChange.
	cases := []struct {
		name   string
		viewer Viewer
		art    store.Artifact
		want   outcome
	}{
		{"author", Viewer{Username: "alice"}, base(store.VisibilityPrivate, false), outcome{true, true, true, true}},
		{"named editor, private", Viewer{Username: "bob"}, base(store.VisibilityPrivate, false, "bob"), outcome{true, true, true, false}},
		{"named editor, organisation", Viewer{Username: "bob"}, base(store.VisibilityOrganisation, false, "bob"), outcome{true, true, true, false}},
		{"organisation editor, switch on", Viewer{Username: "carol"}, base(store.VisibilityOrganisation, true), outcome{true, true, true, false}},
		{"organisation editor, public", Viewer{Username: "carol", PublicLinks: true}, base(store.VisibilityPublic, true), outcome{true, true, true, false}},
		{"organisation editor, public links off", Viewer{Username: "carol"}, base(store.VisibilityPublic, true), outcome{true, true, true, false}},
		{"reader, switch off", Viewer{Username: "carol"}, base(store.VisibilityOrganisation, false, "bob"), outcome{true, false, true, false}},
		{"reader, switch on but the audience is private", Viewer{Username: "carol"}, base(store.VisibilityPrivate, true), outcome{false, false, false, false}},
		{"stranger, private", Viewer{Username: "dave"}, base(store.VisibilityPrivate, false, "bob"), outcome{false, false, false, false}},
		{"stranger, an audience this version does not know, switch on", Viewer{Username: "dave"}, base("everyone-on-earth", true), outcome{false, false, false, false}},
		{"anonymous, public, switch on", anonymous, base(store.VisibilityPublic, true), outcome{true, false, false, false}},
		{"anonymous, private", anonymous, base(store.VisibilityPrivate, false, "bob"), outcome{false, false, false, false}},
	}
	for _, c := range cases {
		got := outcome{c.viewer.CanView(c.art), c.viewer.CanEdit(c.art), c.viewer.CanWrite(c.art), c.viewer.CanChange(c.art)}
		if got != c.want {
			t.Errorf("%s: view/publish/write/change = %+v, want %+v", c.name, got, c.want)
		}
	}
	if (Viewer{}).CanEdit(store.Artifact{Visibility: store.VisibilityPublic, OrganisationEdit: true}) {
		t.Error("the anonymous viewer edits")
	}
}

func TestAViewerIsToldTheirRoleAndAnonymousViewersAreNotToldTheEditors(t *testing.T) {
	a := store.Artifact{Author: "alice", Visibility: store.VisibilityOrganisation, Editors: []string{"bob"}}
	for user, want := range map[string]string{"alice": RoleAuthor, "bob": RoleEditor, "carol": RoleReader} {
		if got := (Viewer{Username: user}).Role(a); got != want {
			t.Errorf("%s is %q, want %q", user, got, want)
		}
	}
	if got := (Viewer{}).RedactArtifact(a).Editors; len(got) != 0 || got == nil {
		t.Errorf("an anonymous viewer sees editors %v", got)
	}
	if got := (Viewer{Username: "carol"}).RedactArtifact(a).Editors; len(got) != 1 {
		t.Errorf("a signed-in viewer sees editors %v", got)
	}
}
