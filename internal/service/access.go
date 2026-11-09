package service

import (
	"slices"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// Viewer is the person an access decision is made for: someone signed in, or
// the anonymous viewer (empty Username) who opened a public link.
type Viewer struct {
	Username string
	// PublicLinks is the deployment's switch for public sharing. It is part of
	// the viewer so the rules below stay the only place that reads it.
	PublicLinks bool
	// UsageReaders are the accounts the deployment named to read the usage
	// counts (CanReadUsage); nobody when it named none.
	UsageReaders []string
	// grant is the capability a page's own requests carry: see Grant.
	grant Grant
}

// Grant is what an artifact's page holds to read the artifact's own files. A
// page runs in an opaque origin and cannot send the viewer's cookie, so the
// server hands a page that a viewer was allowed to open a signed token for it
// (internal/web does the signing). The token names the audience the artifact
// had at that moment; it stops working the moment the audience changes.
//
// A grant for an artifact's page and its files also names the version: its
// address only ever reads that version, so a page that stays open while a new
// version is published keeps loading its own version's files. Version is 0 for
// the grant of an uploaded asset, which belongs to the artifact, not a version.
type Grant struct {
	ArtifactID string
	Visibility store.Visibility
	Version    int
}

// Granted is the viewer a valid grant stands for: nobody in particular, who may
// see exactly the artifact the grant names, for as long as its audience is what
// the grant was made for.
func (s *Service) Granted(g Grant) Viewer {
	return Viewer{PublicLinks: s.publicLinks, grant: g}
}

// Anonymous reports whether the viewer is nobody in particular.
func (v Viewer) Anonymous() bool { return v.Username == "" }

// CanDeleteComment never grants an artifact owner authority over another person's comment.
func (v Viewer) CanDeleteComment(a store.Artifact, c store.Comment) bool {
	return v.CanComment(a) && c.Author == v.Username
}

// CanView is the single answer to "may this person see this artifact?". Every
// route that reveals an artifact or anything that belongs to it (its page, its
// content, its shared db, whoami, the gallery) asks here. An unknown
// visibility denies, so a value written by a newer version is never widened by
// an older one.
//
// Public links that the deployment has turned off fall back to organisation:
// people who can sign in still see the artifact, anonymous viewers do not, and
// the stored value survives so turning the switch back on restores the link.
func (v Viewer) CanView(a store.Artifact) bool {
	if v.Anonymous() && v.grant.ArtifactID != "" && v.grant.ArtifactID == a.ID && v.grant.Visibility == a.Visibility {
		return true
	}
	if v.Anonymous() {
		return a.Visibility == store.VisibilityPublic && v.PublicLinks
	}
	if a.Author == v.Username || v.isNamedEditor(a) {
		return true
	}
	switch a.Visibility {
	case store.VisibilityOrganisation, store.VisibilityPublic:
		return true
	case store.VisibilityPrivate:
		return false
	default:
		return false
	}
}

// CanSeeLive is the single answer to "may this person see the work in the
// artifact's room that no version holds yet?": everyone signed in who may see
// it, the same people who may join the room. The anonymous viewer of a public
// link, and a page's grant, see versions only.
func (v Viewer) CanSeeLive(a store.Artifact) bool {
	return !v.Anonymous() && v.CanView(a)
}

// CanSeeViewers is the single answer to "may this person read who has the
// artifact open now?": everyone signed in who may see it. The anonymous viewer
// of a public link reads no names, and a page's grant stands for a page, not a
// person.
func (v Viewer) CanSeeViewers(a store.Artifact) bool {
	return !v.Anonymous() && v.CanView(a)
}

// CanBeAGuest is the single answer to "may this request be listed as a guest of
// the artifact?": the anonymous viewer of a public link while the deployment
// allows public links, and nobody else. A page's grant is not a guest: the
// listing would outlive the link being closed by as long as the grant lasts.
// Being a guest grants nothing; it is only a name in the list of who is here.
func (v Viewer) CanBeAGuest(a store.Artifact) bool {
	return v.Anonymous() && v.grant.ArtifactID == "" && v.PublicLinks && a.Visibility == store.VisibilityPublic
}

// Roles a signed-in viewer can have on an artifact they can see.
const (
	RoleAuthor = "author"
	RoleEditor = "editor"
	RoleReader = "reader"
)

// CanChange is the single answer to "may this person change who the artifact is
// shared with, who edits it, or delete it?": only its author. Seeing or even
// editing an artifact never implies it.
func (v Viewer) CanChange(a store.Artifact) bool {
	return !v.Anonymous() && a.Author == v.Username
}

// CanEdit is the single answer to "may this person publish new versions of the
// artifact, restore one, read its history, and upload or delete its files?".
// Its author may, and so may the accounts the author named, and, while the
// artifact is shared with the organisation or publicly and the author switched
// it on, everyone who is signed in. An audience this version does not know
// grants nothing beyond the named accounts. Editing never reaches CanChange.
func (v Viewer) CanEdit(a store.Artifact) bool {
	if v.Anonymous() {
		return false
	}
	if a.Author == v.Username || v.isNamedEditor(a) {
		return true
	}
	switch a.Visibility {
	case store.VisibilityOrganisation, store.VisibilityPublic:
		return a.OrganisationEdit
	case store.VisibilityPrivate:
		return false
	default:
		return false
	}
}

// Role is how the artifact sees this viewer, for front ends to show: the
// author, an editor (named, or by the organisation switch), or a reader.
func (v Viewer) Role(a store.Artifact) string {
	switch {
	case v.CanChange(a):
		return RoleAuthor
	case v.CanEdit(a):
		return RoleEditor
	default:
		return RoleReader
	}
}

func (v Viewer) isNamedEditor(a store.Artifact) bool {
	return !v.Anonymous() && slices.Contains(a.Editors, v.Username)
}

// CanCopy is the single answer to "may this person make their own copy of this
// artifact?". Anyone who can see it may, except the anonymous viewer: a copy is
// authored by someone, and nobody is on record. Copying an older version is
// reading history, which CanEdit alone allows (see Service.Copy).
func (v Viewer) CanCopy(a store.Artifact) bool {
	return !v.Anonymous() && v.CanView(a)
}

// CanWrite is the single answer to "may this person write to the artifact's
// shared db?". Anyone who can see it may, except the anonymous viewer: a write
// records who made it, and nobody is on record.
func (v Viewer) CanWrite(a store.Artifact) bool {
	return !v.Anonymous() && v.CanView(a)
}

// CanComment is the single answer to "may this person read and write the
// artifact's comments?". Anyone who can see it may, except the anonymous
// viewer: a comment is signed, and nobody is on record.
func (v Viewer) CanComment(a store.Artifact) bool {
	return !v.Anonymous() && v.CanView(a)
}

// Redact is the single answer to "whose name may this person read in the
// artifact's shared data?". Writer identities are internal account names, so
// the anonymous viewer gets the documents with updatedBy emptied.
func (v Viewer) Redact(d store.Doc) store.Doc {
	if v.Anonymous() {
		d.UpdatedBy = ""
	}
	return d
}

// RedactArtifact is the single answer to "which accounts may this person read
// on the artifact?": the anonymous viewer is not told who the editors are.
func (v Viewer) RedactArtifact(a store.Artifact) store.Artifact {
	if v.Anonymous() {
		a.Editors = []string{}
	}
	return a
}

// CanReadAgentActivity restricts credential activity to a signed-in account.
func (v Viewer) CanReadAgentActivity() bool { return !v.Anonymous() }

// CanReadUsage is the single answer to "may this person read how often each
// department opens and publishes artifacts?": only the signed-in accounts the
// deployment named. Owning or editing artifacts grants nothing here, and with
// no accounts named nobody may.
func (v Viewer) CanReadUsage() bool {
	return !v.Anonymous() && slices.Contains(v.UsageReaders, v.Username)
}
