package mcptools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

const beginPublishDoc = `Publish a page, or update one, from files that already exist on disk, without typing their content into a tool call.
Use this instead of publish when the content is over about 50 KB, or is already a file (a generated report, a build output, a
page you wrote with a shell). It works for every kind, for new artifacts and for updates, and it has three steps:
1. Hash each file in a shell: shasum -a 256 report.html (the 64 hex digits) and wc -c < report.html (the size in bytes).
2. Call begin_publish with the same arguments as publish except content and files, and files: a list of {path, sha256, size},
   one entry for every file you send. The page itself has path "content" (always send it); a file that comes with the page has its
   relative path, such as app.js or styles/site.css (the rules of publish's files argument apply: html, react and design only,
   .html .js .mjs .css .json, relative paths). remove: paths of files to take out. For an update send id and baseVersion, as
   with publish. The answer lists missing: for each blob the server does not have yet, the paths that hold it and an upload url.
   A file whose bytes the artifact already holds is not in missing (stored lists it): republishing a big page that only
   changed in one file costs one upload.
3. For each entry of missing, send the bytes with curl, from the file that has that path:
   curl -sS --fail-with-body -X PUT --data-binary @report.html "<url>"
   Then call commit_publish with the session. It publishes as publish does and returns the same result.
The url is a secret: it needs no token, works once, for the 10 minutes the session lasts, and only for the bytes you described.
Never print it, echo it or put it in a message to the person; pass it to curl and nothing else. A body that is not exactly the
file you hashed is refused (nothing is stored); fix the file or hash again and call begin_publish again. Do not alter a file between
hashing and uploading. At most 10 sessions are open per person; they expire on their own.
Limits: at most {fileBytes} for any one file (the page included), 255 files in a publish, 256 MB together. The page's own
size limit and every other rule of its kind are checked as for publish, at commit_publish, with the same messages.
Nothing is published until commit_publish succeeds. If it is refused, read the message, change what it says, and begin again.`

type fileIn struct {
	Path   string `json:"path" jsonschema:"\"content\" for the page itself, otherwise the file's relative path, such as app.js or styles/site.css"`
	SHA256 string `json:"sha256" jsonschema:"the file's SHA-256 as 64 lower-case hex digits (shasum -a 256 <file>)"`
	Size   int64  `json:"size" jsonschema:"the file's size in bytes (wc -c < <file>)"`
}

type beginPublishIn struct {
	ID          string   `json:"id,omitempty" jsonschema:"id of the artifact to update; omit to create a new one"`
	Title       string   `json:"title" jsonschema:"page title, required every time"`
	Kind        string   `json:"kind,omitempty" jsonschema:"artifact kind, see the kinds tool; default html; an update keeps its kind"`
	Format      string   `json:"format,omitempty" jsonschema:"content format version, as for publish"`
	BaseVersion int      `json:"baseVersion,omitempty" jsonschema:"when updating: the version you started from, as for publish"`
	Visibility  string   `json:"visibility,omitempty" jsonschema:"private, organisation or public; omit to keep the current audience (private for a new artifact)"`
	Files       []fileIn `json:"files" jsonschema:"every file you will send, each with path, sha256 and size; the page itself has path content"`
	Remove      []string `json:"remove,omitempty" jsonschema:"paths of the artifact's files to take out"`
}

type beginPublishOut struct {
	service.BegunView
	// Next is what to do now.
	Next string `json:"next"`
}

func (t *tools) beginPublish(ctx context.Context, _ *mcp.CallToolRequest, in beginPublishIn) (*mcp.CallToolResult, beginPublishOut, error) {
	declared := make([]store.SessionFile, len(in.Files))
	for i, f := range in.Files {
		declared[i] = store.SessionFile{Path: f.Path, SHA256: f.SHA256, Size: f.Size}
	}
	begun, err := t.e.svc.BeginPublish(ctx, t.viewer(ctx), service.BeginInput{
		ArtifactID: in.ID, Title: in.Title, Kind: in.Kind, Format: in.Format, BaseVersion: in.BaseVersion,
		Visibility: store.Visibility(in.Visibility), Files: declared, Remove: in.Remove,
	})
	if err != nil {
		return nil, beginPublishOut{}, t.fail("begin_publish", err)
	}
	view := begun.View(t.caller(ctx).BaseURL)
	next := fmt.Sprintf("Session %s works for %d minutes. ", view.Session, view.ExpiresInSeconds/60)
	if len(view.Missing) == 0 {
		next += "Nothing needs uploading: call commit_publish with the session."
	} else {
		next += "Upload each entry of missing with curl -sS --fail-with-body -X PUT --data-binary @<file with that path> \"<url>\" (never print the url), then call commit_publish with the session."
	}
	return nil, beginPublishOut{BegunView: view, Next: next}, nil
}

const commitPublishDoc = `Finish a begin_publish: publish what you uploaded. It creates the artifact (or the new version) exactly as publish would, with
the same checks, and returns the same result: the artifact, its url, the version, review notes and rewritten files.
sourceBases: for changed live Design source, the per-file proofs from the reads used to prepare the draft, as for publish.
Send proofs only, never duplicate baseline bodies. Missing or stale proofs refuse the whole commit and retain uploads until expiry.
session: the id begin_publish returned. A session commits once. Without every upload having arrived the call is refused and
nothing is published; upload what is missing and call again. If the artifact was changed by someone else meanwhile the refusal is the
same conflict as for publish: read the newest version, merge, and begin again. Then give the person the url.`

type commitPublishIn struct {
	SourceBases map[string]string `json:"sourceBases,omitempty" jsonschema:"design per-file source bases from the reads used to prepare this session; same contract as publish, without repeating source bodies"`
	Session     string            `json:"session" jsonschema:"the session id from begin_publish"`
}

func (t *tools) commitPublish(ctx context.Context, _ *mcp.CallToolRequest, in commitPublishIn) (*mcp.CallToolResult, publishOut, error) {
	if in.Session == "" {
		return nil, publishOut{}, errors.New("session is required: the session id begin_publish returned")
	}
	p, created, err := t.e.svc.CommitPublish(ctx, t.viewer(ctx), in.Session, in.SourceBases)
	if err != nil {
		return nil, publishOut{}, t.fail("commit_publish", err)
	}
	return nil, t.published(ctx, p, created), nil
}

// registerPublishSessions adds the tools of this feature to the server.
func (t *tools) registerPublishSessions(s *mcp.Server) {
	begin := strings.ReplaceAll(beginPublishDoc, "{fileBytes}", sizeText(t.e.svc.FileLimits().FileBytes))
	mcp.AddTool(s, &mcp.Tool{Name: "begin_publish", Description: begin, Annotations: &mcp.ToolAnnotations{Title: "Start publishing from files on disk", DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}}, t.beginPublish)
	mcp.AddTool(s, &mcp.Tool{Name: "commit_publish", Description: commitPublishDoc, Annotations: &mcp.ToolAnnotations{Title: "Publish what was uploaded", DestructiveHint: ptr(true), OpenWorldHint: ptr(false)}}, t.commitPublish)
}
