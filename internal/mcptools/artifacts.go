package mcptools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

const listDoc = `Find artifacts you can open: your own and those other people shared with you. Returns metadata and url,
never content (use read for that). Use it to find the id of a page you or the person published earlier,
or to check what exists before publishing a duplicate.
Each artifact carries role: author, editor or reader, and pinned: whether you pinned it (see pin and unpin).
scope: "mine" (you authored), "shared" (someone else authored and you can see it) or "all" (default).
query: keeps artifacts whose title or author contains the text, ignoring case.
order: "updated" (default: most recently updated first) or "opened": only the artifacts the person whose token you use
opened in the CodeXArtifacts viewer in a browser, most recently opened first, each with openedAt; it answers "what did I open last".
Reading with these tools is not an open, and nobody else is told of these opens.
limit: at most this many, in that order (default 50, at most 200); total says how many matched.`

type listIn struct {
	Scope string `json:"scope,omitempty" jsonschema:"mine, shared or all (default all)"`
	Query string `json:"query,omitempty" jsonschema:"keep artifacts whose title or author contains this text, ignoring case"`
	Order string `json:"order,omitempty" jsonschema:"updated (default, most recently updated first) or opened (only what the person opened in the viewer, most recently opened first)"`
	Limit int    `json:"limit,omitempty" jsonschema:"most artifacts to return, default 50, at most 200"`
}

type listOut struct {
	Artifacts []artifactOut `json:"artifacts"`
	// Total is how many artifacts matched; Artifacts holds the first Limit of them.
	Total int `json:"total"`
}

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

func (t *tools) list(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, listOut, error) {
	limit := in.Limit
	switch {
	case limit < 0 || limit > maxListLimit:
		return nil, listOut{}, fmt.Errorf("limit must be between 1 and %d", maxListLimit)
	case limit == 0:
		limit = defaultListLimit
	}
	list, err := t.e.svc.List(ctx, t.viewer(ctx), service.ListFilter{Scope: service.Scope(in.Scope), Query: in.Query, Order: service.Order(in.Order)})
	if err != nil {
		return nil, listOut{}, t.fail("list", err)
	}
	out := listOut{Artifacts: []artifactOut{}, Total: len(list)}
	for _, a := range list[:min(limit, len(list))] {
		out.Artifacts = append(out.Artifacts, t.artifact(ctx, a))
	}
	return nil, out, nil
}

const readDoc = `Read one artifact: its metadata, url, content and version. Use it before you change a page someone made (so you
start from what is there), or to look at a page you were pointed to. id is the last part of the page url
(.../a/<id>) or comes from list or publish. Pages you cannot see answer exactly like pages that do not exist.
version: optional; read a saved version (older versions are for authors and editors, see versions). A doc's content and blocks
are that saved version even when its number is the latest; omit version to read current live work. The result's
version is the one to send as baseVersion when you publish your change.
files: an html page can come with files (a script, a stylesheet, data) at relative paths; the result lists them
(path, contentType, size). file: the path of one of them; the result's file holds its text, so you can read it,
change it and publish just that file. With file the page's own content is left out; read again without it to get the page.
viewers: who has the page open right now (id, name, agent: true for an agent, guest: true for a guest, self: true for you);
empty when nobody does. A guest is a signed-out browser reading a public link, named by a random nickname such as
"Swift Otter" that the browser keeps: it is a browser, not a verified person, and nothing about who is behind it is known.
Check it before you publish over a page someone is looking at, or to know whom you are answering. Reading shows you
to them as a viewer for a few seconds.
For a live Design editor, current reads return sourceBases for design.json or the requested frame.
Keep each proof with the exact source read and pass it to publish; no last-read baseline is inferred.
File reads expose present:false for an absent valid frame path, with its creation proof.
Readers and explicit saved-version reads receive no write proof.`

type readIn struct {
	ID      string `json:"id" jsonschema:"the artifact id: the last part of its url (.../a/<id>)"`
	Version int    `json:"version,omitempty" jsonschema:"a saved version number to read (older versions are for authors and editors, see versions); explicit doc versions return saved content and blocks even when latest; omit for current live work"`
	File    string `json:"file,omitempty" jsonschema:"the path of one of the artifact's files (for example app.js or styles/site.css, from the files list of an earlier read); the result's file holds its text"`
	View    string `json:"view,omitempty" jsonschema:"doc only: markdown (the default: the content as Markdown, plus an outline of the blocks with their ids) or blocks (each block with its id, type, hash and content, for doc_edit)"`
}

type readOut struct {
	SourceBases map[string]string `json:"sourceBases,omitempty"`
	artifactOut
	// Content is the artifact's content as the publish tool takes it: for a doc
	// stored as blocks, Markdown; left out when one file was asked for.
	Content string `json:"content,omitempty"`
	// File is the file that was asked for, with its text.
	File *fileOut `json:"file,omitempty"`
	// LatestVersion is the newest version number; set only when an older
	// version was asked for.
	LatestVersion int `json:"latestVersion,omitempty"`
	// Outline and Tabs are a doc's blocks (see view): the ids of its top-level
	// blocks next to the Markdown, or every block in full.
	Outline []docedit.Outline `json:"outline,omitempty"`
	Tabs    any               `json:"tabs,omitempty"` // []docedit.TabView; any because the schema of a block is recursive
	// Hashes are a board's elements' hashes by id, for board_edit's ifHash.
	Hashes map[string]string `json:"hashes,omitempty"`
	// Viewers is who has the artifact open now (see the read tool's description); it never holds the caller.
	Viewers []service.ViewerEntry `json:"viewers"`
}

// fileOut is one file of an artifact, with its text.
type fileOut struct {
	SourceBase  string `json:"sourceBase,omitempty"`
	Present     bool   `json:"present"`
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	Text        string `json:"text"`
}

func (t *tools) read(ctx context.Context, _ *mcp.CallToolRequest, in readIn) (*mcp.CallToolResult, readOut, error) {
	a, err := t.e.svc.Artifact(ctx, t.viewer(ctx), in.ID)
	if err != nil {
		return nil, readOut{}, t.fail("read", err)
	}
	viewers, err := t.e.svc.Viewers(t.asAgent(ctx), t.viewer(ctx), in.ID)
	if err != nil {
		return nil, readOut{}, t.fail("read", err)
	}
	out := readOut{Viewers: viewers}
	if in.Version != 0 && in.Version != a.Version {
		ver, err := t.e.svc.Version(ctx, t.viewer(ctx), in.ID, in.Version)
		if err != nil {
			return nil, readOut{}, t.fail("read", err)
		}
		out.LatestVersion = a.Version
		a.Title, a.Kind, a.Format, a.Content, a.Version, a.Files = ver.Title, ver.Kind, ver.Format, ver.Content, ver.Number, ver.Files
	}
	if in.Version == 0 && in.File == "" {
		// What people are looking at, which a live room holds before any version does.
		read, err := t.e.svc.ReadSource(t.asAgent(ctx), t.viewer(ctx), a.ID)
		if err != nil {
			return nil, readOut{}, t.fail("read", err)
		}
		a = read.Artifact
		out.SourceBases = read.SourceBases
	}
	out.artifactOut = t.artifact(ctx, a)
	if in.File == "" {
		content, err := service.Written(a.Kind, a.Format, a.Content)
		if err != nil {
			return nil, readOut{}, t.fail("read", err)
		}
		out.Content = content
		if err := t.withBlocks(&out, a, in.View); err != nil {
			return nil, readOut{}, err // said to the agent as it is: what to ask for instead
		}
		out.Hashes = boardHashes(a)
		return nil, out, nil
	}
	read, err := t.e.svc.ReadSourceFile(t.asAgent(ctx), t.viewer(ctx), in.ID, in.File, in.Version)
	if err != nil {
		return nil, readOut{}, t.fail("read", err)
	}
	if read.SourceBase != "" {
		out.SourceBases = map[string]string{in.File: read.SourceBase}
	}
	out.File = &fileOut{Path: read.Ref.Path, ContentType: read.Ref.ContentType, Size: read.Ref.Size, Text: read.Text, Present: read.Present, SourceBase: read.SourceBase}
	return nil, out, nil
}

const copyDoc = `Make your own copy of an artifact you can see, to change it without touching the original. The copy is a new artifact: new id,
you are its author, private (only you can open it; use set_visibility to share it), version 1, titled "<title> (Copy)", with the
same kind, format and content and the same uploaded files (their urls in the content point at the copy's own files). It does not
take the shared database, the comments, the editors or the history, and it records where it came from (copiedFrom).
Use it when the person wants their own version of someone else's page, or a fresh start from one of your own; then update the
copy with publish. To change someone's page itself, ask its author to make you an editor instead.
version: optional. Copies that version's content instead of the newest; only the author and editors can read older versions.`

type copyIn struct {
	ID      string `json:"id" jsonschema:"the id of the artifact to copy"`
	Version int    `json:"version,omitempty" jsonschema:"copy this version instead of the newest; only for artifacts you may edit"`
}

func (t *tools) copyArtifact(ctx context.Context, _ *mcp.CallToolRequest, in copyIn) (*mcp.CallToolResult, publishOut, error) {
	if in.Version < 0 {
		return nil, publishOut{}, errors.New("version must be a version number from 1, or left out to copy the newest")
	}
	a, err := t.e.svc.Copy(ctx, t.viewer(ctx), in.ID, in.Version)
	if err != nil {
		return nil, publishOut{}, t.fail("copy", err)
	}
	out := t.artifact(ctx, a)
	return nil, publishOut{artifactOut: out, Created: true, Next: fmt.Sprintf("Copied: %s is yours and private. Change it with publish (read it first), and give the person the url.", out.URL)}, nil
}

const publishDoc = `Create an artifact, or update one you may edit (you authored it or are an editor), and get its permanent url.
Without id: creates a new artifact. It is private (only you can open it) unless you pass visibility.
With id: replaces the title and content of that artifact; the url stays the same, so people who have the link keep
it. Update instead of publishing again, otherwise you leave a duplicate behind. The kind of an existing artifact
cannot change, and omitting visibility keeps the audience it has.
Every publish is saved as a version (the result's version) and older versions are kept, see versions and restore.
baseVersion: when updating, ALWAYS send the version you started from (the version that read or your last publish
returned). If someone saved a newer version meanwhile, the update is refused, nothing is saved, and the message names
the newer version: read it, merge your change into it, and publish again with that version as baseVersion. Never
retry unchanged. Leaving it out skips the check and overwrites whatever is newest; do that only for a page nobody
else edits.
The author and the editors the author named (role in read and list) can update; with organisation editing on, everyone
signed in can. If you are told you may not edit it, the artifact is someone else's: publish a new one and say so.
Only the author can pass a different visibility; an editor leaves it out.
title: required, every time (when updating, send the current title to keep it).
content: the whole page in the format of its kind. Call kinds first if you have not written this kind of page yet.
blocks: doc only, instead of content: the document as structured blocks (a list of blocks, or {"tabs": [...]}), each block
with a stable id the server adds when you leave it out. The doc guide from kinds describes the blocks.
kind: optional, "html" by default; an update keeps the kind it has.
format: optional content format version of the kind (the kinds tool lists them; html has none). A new artifact gets the
kind's current version; an update keeps the version the artifact was published with, which read returns. Pass a version
only to upgrade the artifact on purpose, with content written for that version.
sourceBases: live Design source changes also require the exact per-file proofs returned by current read.
Use design.json for changed content and frame paths for changed files. Missing or expired proofs refuse without publishing;
retain the draft, reread and reconcile. Later reads never replace the proof of an earlier draft.
visibility: "private" (only you), "organisation" (everyone who can sign in) or "public" (anyone with the link, only
if whoami says publicLinks). Choose from what the person asked for, and tell them which you chose; when they said
nothing about sharing, leave it out.
review: the result may carry notes from the kind about what you cannot see for yourself (slides that will overflow the
canvas, slides without notes, a changed slide count, a design frame with no text or a picture without alt). Act on them, or say so, before you tell the person it is done.
files: only for html, react and design. The page can come with files (a script, a stylesheet, data) that it refers to by RELATIVE
path, as <script src="app.js">, <link rel="stylesheet" href="styles/site.css"> or fetch("data/rows.json"); never start a path
with "/". A react component imports its files the same way, import Chart from "./Chart", and may also send .tsx, .ts and .jsx
sources, which are compiled into the page and not served. A design's files are its frames, frames/<name>.html, and only .html; the
result's rewritten holds each frame as it was stored, with its data-pid ids (see the design guide from kinds). Send an object from path to the file's complete text: {"app.js": "...", "styles/site.css": "..."}. A path
you send is added, or replaces the file there; null removes it; a path you do not mention is carried over unchanged, so
to change one file send only that one (read it first with the file argument), and an update without files keeps all of
them. Files are text with the extension .html, .js, .mjs, .css or .json (.json must parse); the paths are letters,
digits, dot, dash and underscore, at most 5 levels deep, and index.html is the page itself, so send that as content. Each
file is saved with the version, so restore brings back its files too. A design that people have open takes your files as a change
merged into what they are editing (the result says live: true and no new version is saved yet; see the design guide from kinds). Limits: 16 MB a file, 255 files a publish, 511 files
and 256 MB in all in a version. The other kinds (doc, slides) take no files. Pictures, fonts and PDFs are not files in this sense: upload
them with upload_asset and use the url it returns. The page runs in a sandbox with no localStorage or cookies, so keep
what must be saved in the shared database (window.pictor.db), not in the browser.
Errors tell you what to send instead: fix the argument and call again. Do not retry unchanged.
Large or existing files: content over about 50 KB, or a page that already exists as a file on disk, does not go through this
tool (nobody can type 300 KB into a call reliably): use begin_publish, upload the files with curl, then commit_publish.`

type publishIn struct {
	SourceBases map[string]string `json:"sourceBases,omitempty" jsonschema:"design live source baselines returned by read: design.json for content and frame paths for files; retain each exact read proof"`
	ID          string            `json:"id,omitempty" jsonschema:"id of the artifact to update; omit to create a new one"`
	Title       string            `json:"title" jsonschema:"page title, required every time"`
	Content     string            `json:"content,omitempty" jsonschema:"the whole content in the format of the kind (for html, the complete HTML document); for a doc with blocks, leave it out"`
	Blocks      any               `json:"blocks,omitempty" jsonschema:"doc only: the content as structured blocks, a JSON list of blocks or {\"tabs\": [...]}; see the doc guide from kinds. Send the value itself, not a string"`
	Kind        string            `json:"kind,omitempty" jsonschema:"artifact kind, see the kinds tool; default html; an update keeps its kind"`
	Format      string            `json:"format,omitempty" jsonschema:"content format version, see the kinds tool; omit for the kind's current one on a new artifact, or to keep the artifact's own version on an update"`
	BaseVersion int               `json:"baseVersion,omitempty" jsonschema:"when updating: the version you started from, as returned by read or your last publish; an update on top of a newer version is refused so nothing is overwritten"`
	Visibility  string            `json:"visibility,omitempty" jsonschema:"private, organisation or public; omit to keep the current audience (private for a new artifact)"`
	// Files is the files that come with the page, as a difference.
	Files map[string]*string `json:"files,omitempty" jsonschema:"html, react and design only: files the page refers to by relative path, as {path: complete text}; a path with null removes that file; paths not named are kept; omit to keep all the files as they are"`
}

type publishOut struct {
	artifactOut
	// Created is true when a new artifact was made, false when id was updated.
	Created bool `json:"created"`
	// Next is what to tell the person or do next.
	Next string `json:"next"`
	// Review is what the artifact's kind noticed about the content that was
	// just saved: things the agent cannot see and should fix or mention.
	Review []string `json:"review,omitempty"`
	// Rewritten is the text of each file as it was stored when that is not what
	// was sent: a design's frames come back with a data-pid on every element.
	Rewritten map[string]string `json:"rewritten,omitempty"`
	// Live is true when people are editing the artifact and the publish was
	// merged into what they are editing instead of being saved as a version.
	Live bool `json:"live,omitempty"`
}

func (t *tools) publish(ctx context.Context, _ *mcp.CallToolRequest, in publishIn) (*mcp.CallToolResult, publishOut, error) {
	blocks, err := jsonArg(in.Blocks)
	if err != nil {
		return nil, publishOut{}, errors.New("blocks must be JSON: a list of blocks or {\"tabs\": [...]}")
	}
	input := service.PublishInput{Title: in.Title, Kind: in.Kind, Format: in.Format, Content: in.Content, Blocks: blocks, BaseVersion: in.BaseVersion, Visibility: store.Visibility(in.Visibility), Files: in.Files, SourceBases: in.SourceBases}
	var (
		p       service.Published
		created = in.ID == ""
	)
	if created {
		p, err = t.e.svc.Create(ctx, t.viewer(ctx), input)
	} else {
		p, err = t.e.svc.Update(t.asAgent(ctx), t.viewer(ctx), in.ID, input)
	}
	if err != nil {
		return nil, publishOut{}, t.fail("publish", err)
	}
	return nil, t.published(ctx, p, created), nil
}

// published is what a publish that succeeded tells the agent, whether the
// content came in the call or from a file (commit_publish).
func (t *tools) published(ctx context.Context, p service.Published, created bool) publishOut {
	next := nextAfterPublish(p.Artifact, created)
	if p.Live {
		next = fmt.Sprintf("Merged into the live design: people are editing it, so your change is on their screens now and was merged with what they changed; it is saved as a version when they have paused. "+
			"The saved version is still %d. Give the person the url. Audience: %s. Read the design before you change it again, and send its sourceBases with baseVersion %d.", p.Version, audience(p.Visibility), p.Version)
	}
	if len(p.Review) > 0 {
		next += " Review before you tell the person it is done: " + strings.Join(p.Review, " ")
	}
	if len(p.Rewritten) > 0 {
		next += " Some files were stored in a normal form that differs from what you sent (see rewritten); start from that text when you change them."
	}
	return publishOut{artifactOut: t.artifact(ctx, p.Artifact), Created: created, Next: next, Review: p.Review, Rewritten: p.Rewritten, Live: p.Live}
}

func nextAfterPublish(a store.Artifact, created bool) string {
	verb := "Updated"
	if created {
		verb = "Published"
	}
	msg := fmt.Sprintf("%s as version %d. Give the person the url. Audience: %s. If you change it again, send baseVersion %d.", verb, a.Version, audience(a.Visibility), a.Version)
	if a.Visibility == store.VisibilityPrivate {
		msg += " To share it, call set_visibility with \"organisation\" once the person asks."
	}
	return msg
}

func audience(v store.Visibility) string {
	switch v {
	case store.VisibilityPrivate:
		return "private, only the author can open it"
	case store.VisibilityOrganisation:
		return "organisation, everyone who can sign in"
	case store.VisibilityPublic:
		return "public, anyone with the link"
	}
	return string(v)
}

const setVisibilityDoc = `Change who can see an artifact you authored, without touching its content.
visibility: "private" (only you), "organisation" (everyone who can sign in) or "public" (anyone with the link,
only if whoami says publicLinks is true). Narrowing to private takes effect immediately for everyone, including
people holding the link. Use it when the person says "share this", "make it public" or "keep it to myself".
Only the author can change it.`

type setVisibilityIn struct {
	ID         string `json:"id" jsonschema:"the artifact id"`
	Visibility string `json:"visibility" jsonschema:"private, organisation or public"`
}

func (t *tools) setVisibility(ctx context.Context, _ *mcp.CallToolRequest, in setVisibilityIn) (*mcp.CallToolResult, publishOut, error) {
	a, err := t.e.svc.SetVisibility(ctx, t.viewer(ctx), in.ID, store.Visibility(in.Visibility))
	if err != nil {
		return nil, publishOut{}, t.fail("set_visibility", err)
	}
	return nil, publishOut{artifactOut: t.artifact(ctx, a), Next: fmt.Sprintf("Audience is now %s.", audience(a.Visibility))}, nil
}

const shareDoc = `Choose who may edit an artifact you authored, besides you. Editors publish new versions, restore versions, upload and
delete files and write the shared database; they can neither change the audience nor the editors nor delete it.
Changes take effect on the next request of everyone concerned. Use it when the person says "let Bob edit this" or
"everyone can edit". It never changes who can see the artifact, except that a named editor can always open it.
add: accounts to make editors, the sign-in name such as "bob" (a name is not checked against the directory, so spell it
exactly; the result's editors lists who now edits).
remove: accounts to stop being editors.
organisation_edit: true lets everyone who can sign in edit (the artifact must be shared with "organisation" or "public"
first, see set_visibility); false switches it off. Making the artifact private switches it off too.
Send at least one of add, remove, organisation_edit. Only the author can use it.`

type shareIn struct {
	ID               string   `json:"id" jsonschema:"the artifact id"`
	Add              []string `json:"add,omitempty" jsonschema:"sign-in names to make editors"`
	Remove           []string `json:"remove,omitempty" jsonschema:"sign-in names to stop being editors"`
	OrganisationEdit *bool    `json:"organisation_edit,omitempty" jsonschema:"true: everyone who can sign in may edit; false: only the author and named editors"`
}

func (t *tools) share(ctx context.Context, _ *mcp.CallToolRequest, in shareIn) (*mcp.CallToolResult, publishOut, error) {
	a, err := t.e.svc.Share(ctx, t.viewer(ctx), in.ID, service.ShareInput{AddEditors: in.Add, RemoveEditors: in.Remove, OrganisationEdit: in.OrganisationEdit})
	if err != nil {
		return nil, publishOut{}, t.fail("share", err)
	}
	who := "only you"
	if len(a.Editors) > 0 {
		who = "you and " + strings.Join(a.Editors, ", ")
	}
	if a.OrganisationEdit {
		who += ", and everyone who can sign in"
	}
	return nil, publishOut{artifactOut: t.artifact(ctx, a), Next: fmt.Sprintf("Who can edit is now: %s.", who)}, nil
}

const deleteDoc = `Delete an artifact you authored, with all of its shared data and uploaded files. This cannot be undone and the link stops working
for everyone, so it asks twice and only when the person asked for it.
Step 1: call with just id. Nothing is deleted. You get a plan (what would be lost) and a confirm token.
Show the person the plan and wait for their yes.
Step 2: call again with the same id and the confirm token. The token works once, for ten minutes, only for this
caller and only for this id; a different id, an old token or a used token deletes nothing and tells you to ask for a
new plan.
Never delete just to update a page: publish with its id instead.`

type deleteIn struct {
	ID      string `json:"id" jsonschema:"the artifact id"`
	Confirm string `json:"confirm,omitempty" jsonschema:"the confirm token from the plan; omit on the first call"`
}

// deleteArgs is what a confirm token is bound to: everything that decides what
// the delete does, and not the token itself.
type deleteArgs struct {
	ID string `json:"id"`
}

type deleteOut struct {
	Deleted bool `json:"deleted"`
	// Plan is set on step 1: what the delete would remove.
	Plan *deletePlan `json:"plan,omitempty"`
	// Confirm is the token that applies the plan, set on step 1.
	Confirm string `json:"confirm,omitempty"`
	// ExpiresInSeconds is how long Confirm works.
	ExpiresInSeconds int    `json:"expiresInSeconds,omitempty"`
	Next             string `json:"next"`
}

func (t *tools) delete(ctx context.Context, _ *mcp.CallToolRequest, in deleteIn) (*mcp.CallToolResult, deleteOut, error) {
	v := t.viewer(ctx)
	args := deleteArgs{ID: in.ID}
	a, err := t.e.svc.Own(ctx, v, in.ID)
	if err != nil {
		return nil, deleteOut{}, t.fail("delete", err)
	}
	if in.Confirm == "" {
		return t.deletePlan(ctx, v, a, args)
	}
	ok, err := t.e.confirms.redeem(v.Username, "delete", args, in.Confirm)
	if err != nil {
		return nil, deleteOut{}, t.fail("delete", err)
	}
	if !ok {
		return nil, deleteOut{}, errors.New("that confirm token does not apply here: it is unknown, expired, already used, or was issued for a different id. " +
			"Nothing was deleted. Call delete again with just the id to get a new plan, and show it to the person")
	}
	if err := t.e.svc.Delete(ctx, v, in.ID); err != nil {
		return nil, deleteOut{}, t.fail("delete", err)
	}
	return nil, deleteOut{Deleted: true, Next: fmt.Sprintf("Deleted %q. Its link no longer works.", a.Title)}, nil
}

func (t *tools) deletePlan(ctx context.Context, v service.Viewer, a store.Artifact, args deleteArgs) (*mcp.CallToolResult, deleteOut, error) {
	docs, err := t.e.svc.DocCount(ctx, v, a.ID)
	if err != nil {
		return nil, deleteOut{}, t.fail("delete", err)
	}
	files, err := t.e.svc.AssetCount(ctx, v, a.ID)
	if err != nil {
		return nil, deleteOut{}, t.fail("delete", err)
	}
	token, err := t.e.confirms.issue(v.Username, "delete", args)
	if err != nil {
		return nil, deleteOut{}, t.fail("delete", err)
	}
	art := t.artifact(ctx, a)
	return nil, deleteOut{
		Plan:             &deletePlan{ID: a.ID, Title: a.Title, Kind: a.Kind, Visibility: string(a.Visibility), URL: art.URL, SharedDocuments: docs, Assets: files, Files: len(a.Files)},
		Confirm:          token,
		ExpiresInSeconds: int(confirmTTL / time.Second),
		Next: fmt.Sprintf("Nothing is deleted yet. Show the person: %q (%s) and its shared documents (%d), uploaded files (%d), page files (%d) and every older version would be removed for everyone who has the link, and this cannot be undone. "+
			"If they agree, call delete again with this id and confirm.", a.Title, audience(a.Visibility), docs, files, len(a.Files)),
	}, nil
}

type deletePlan struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Kind            string `json:"kind"`
	Visibility      string `json:"visibility"`
	URL             string `json:"url"`
	SharedDocuments int    `json:"sharedDocuments"`
	Assets          int    `json:"assets"`
	// Files is how many files the newest version has besides its page.
	Files int `json:"files"`
}

// registerArtifacts adds the tools of this feature to the server.
func (t *tools) registerArtifacts(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "list", Description: listDoc, Annotations: readOnly("List artifacts")}, t.list)
	mcp.AddTool(s, &mcp.Tool{Name: "read", Description: readDoc, Annotations: readOnly("Read an artifact")}, t.read)
	mcp.AddTool(s, &mcp.Tool{Name: "copy", Description: copyDoc, Annotations: &mcp.ToolAnnotations{Title: "Make your own copy of an artifact", DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}}, t.copyArtifact)
	mcp.AddTool(s, &mcp.Tool{Name: "publish", Description: publishDoc, Annotations: &mcp.ToolAnnotations{Title: "Create or update an artifact", DestructiveHint: ptr(true), OpenWorldHint: ptr(false)}}, t.publish)
	mcp.AddTool(s, &mcp.Tool{Name: "set_visibility", Description: setVisibilityDoc, Annotations: &mcp.ToolAnnotations{Title: "Change who can see an artifact", DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: ptr(false)}}, t.setVisibility)
	mcp.AddTool(s, &mcp.Tool{Name: "share", Description: shareDoc, Annotations: &mcp.ToolAnnotations{Title: "Choose who can edit an artifact", DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: ptr(false)}}, t.share)
	mcp.AddTool(s, &mcp.Tool{Name: "delete", Description: deleteDoc, Annotations: &mcp.ToolAnnotations{Title: "Delete an artifact (asks twice)", DestructiveHint: ptr(true), OpenWorldHint: ptr(false)}}, t.delete)
}
