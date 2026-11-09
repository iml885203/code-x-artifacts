// Package mcptools serves pictor to agents over MCP (streamable HTTP). The
// tools are a second front end over internal/service, next to the HTTP API:
// they decide nothing about who may see or change what, they only translate.
//
// The tool descriptions are the agent's manual. Each says when to use the
// tool, what every argument means, what its errors mean and what to do next,
// because the agent has nothing else to read.
package mcptools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/operationlog"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// Caller is who a request is for and where they reached the server. The web
// layer authenticates the request and attaches it with WithCaller; the tools
// never see a credential.
type Caller struct {
	Viewer  service.Viewer
	BaseURL string
	// Client is the agent client the caller uses, as shown to people (ClientName),
	// or empty when it is not known.
	Client     string
	Provenance store.TokenProvenance
}

type callerKey struct{}

// WithCaller attaches the authenticated caller to a request context.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// Config is what New needs.
type Config struct {
	Service *service.Service
	Logger  *slog.Logger
	// Now is the clock for confirm-token expiry; time.Now when nil.
	Now func() time.Time
	// MaxUploadBytes is the largest file upload_asset takes, decoded;
	// DefaultMaxUploadBytes when zero. Production uses the default; tests
	// that probe the limit set a small one.
	MaxUploadBytes int
	// Export makes the downloads the export tool hands over.
	Export Exporter
}

// Endpoint is the MCP server of one pictor deployment.
type Endpoint struct {
	svc      *service.Service
	log      *slog.Logger
	confirms *confirmations
	// maxUpload is the upload_asset limit, decoded bytes.
	maxUpload int
	export    Exporter
	// srv builds the server that serves every request, on the first one;
	// builds counts how often server ran (a test holds it at one).
	srv    func() *mcp.Server
	builds atomic.Int64
}

// New builds the endpoint.
func New(cfg Config) *Endpoint {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	limit := cfg.MaxUploadBytes
	if limit <= 0 {
		limit = DefaultMaxUploadBytes
	}
	e := &Endpoint{svc: cfg.Service, log: cfg.Logger, confirms: newConfirmations(now), maxUpload: limit, export: cfg.Export}
	// Registering the tools reflects every schema, which costs far more than
	// the rest of building the web server, and most servers (every one a test
	// starts) are never asked for a tool. The first request pays for it, once.
	e.srv = sync.OnceValue(e.server)
	return e
}

const instructions = `CodeXArtifacts hosts pages ("artifacts") that people share: reports, cheat sheets, dashboards, polls, forms.
An artifact's author can let named colleagues, or the whole organisation, edit it (share); every artifact says your role.
Typical flow: call kinds once to learn how to write a page, publish it, give the person the url, and say who can see it.
A new page is private to its author until set_visibility or publish with visibility shares it.
Always read an artifact before you update it and send the version you read back as baseVersion, so a newer save is never overwritten.
Pages have a small shared database that every viewer can read and write (db_* tools).
A page that needs a picture, font, PDF or CSV uploads it with upload_asset and puts the returned url in its content.
A page over about 50 KB, or one that already exists as a file, is not typed into publish: begin_publish, curl the files, commit_publish.
People comment under a page: list shows comments.unread on your own pages; read them with comments and answer with comment.`

// maxRequestBytes is what one MCP request may carry: the largest upload_asset
// as base64 plus room for the rest of the call, or a publish with files, which
// may be as large as the HTTP API's (files.MaxRequestBytes). The SDK's default
// is lower than that, which would refuse a call the tool description promises.
func (e *Endpoint) maxRequestBytes() int64 {
	return max(int64(base64.StdEncoding.EncodedLen(e.maxUpload)), files.MaxRequestBytes) + 64<<10
}

// Handler serves MCP on one URL. Every request must already carry a Caller
// (see WithCaller). It is stateless: each request stands alone, so there is no
// session to hijack and a restart loses nothing. All requests share the one
// server built on the first request: registering the tools (schema reflection) is the expensive
// part of a call, and nothing about it depends on who is calling.
func (e *Endpoint) Handler() http.Handler {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if _, ok := r.Context().Value(callerKey{}).(Caller); !ok {
			e.log.Error("mcp request without a caller")
			return nil
		}
		return e.srv()
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: e.log, MaxRequestBodyBytes: e.maxRequestBytes()})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r.WithContext(operationlog.Context(r.Context(), e.log)))
	})
}

// server builds the MCP server with every tool. The tools read the caller from
// each request's context.
func (e *Endpoint) server() *mcp.Server {
	e.builds.Add(1)
	s := mcp.NewServer(&mcp.Implementation{Name: "pictor", Version: "1"}, &mcp.ServerOptions{Instructions: instructions})
	t := &tools{e: e}
	for _, register := range []func(*mcp.Server){
		t.registerIdentity, t.registerArtifacts, t.registerPublishSessions, t.registerVersions, t.registerDocEdit, t.registerBoardEdit, t.registerSlides, t.registerChanges,
		t.registerActivity, t.registerDB, t.registerAssets, t.registerComments, t.registerPins, t.registerExport, t.registerUsage,
	} {
		register(s)
	}
	s.AddReceivingMiddleware(e.observeClient)
	return s
}

func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: ptr(false)}
}

// readOnlyExceptMark is for comments, which only looks unless mark_read is set.
func readOnlyExceptMark(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: ptr(false)}
}

func ptr[T any](v T) *T { return &v }

type tools struct {
	e *Endpoint
}

// caller is who this request is for. The tools are built once and serve every
// request, so the caller comes from the request's context, never from a field.
// Handler refuses a request without one, so a tool always finds it.
func (t *tools) caller(ctx context.Context) Caller {
	c, _ := ctx.Value(callerKey{}).(Caller)
	return c
}

func (t *tools) viewer(ctx context.Context) service.Viewer { return t.caller(ctx).Viewer }

// asAgent marks the request as the agent's, so a doc it writes through a live
// room shows it in the roster.
func (t *tools) asAgent(ctx context.Context) context.Context {
	return service.WithAgent(ctx, t.caller(ctx).Client)
}

// fail turns a service error into the message the agent reads. Refusals carry
// what to do next; anything else is an internal failure whose detail stays in
// the log.
func (t *tools) fail(tool string, err error) error {
	refusal, ok := errors.AsType[*service.Error](err)
	if !ok {
		t.e.log.Error("mcp tool", "tool", tool, "err", err)
		return errors.New("internal error: nothing was changed that you can rely on; try once more, and if it fails again tell the person")
	}
	switch refusal.Code {
	case service.NotFound:
		return notFound(refusal)
	case service.Forbidden:
		return fmt.Errorf("%s. If the artifact is someone else's, publish a new artifact instead of updating it, or ask its author to make you an editor", refusal.Msg)
	case service.Invalid, service.Busy:
		return errors.New(refusal.Msg)
	case service.Conflict:
		if len(refusal.Stale) > 0 {
			state, _ := json.Marshal(refusal.Stale)
			return fmt.Errorf("%s\nCurrent state of those blocks: %s", refusal.Msg, state)
		}
		if len(refusal.StaleElements) > 0 {
			state, _ := json.Marshal(refusal.StaleElements)
			return fmt.Errorf("%s\nCurrent state of those elements: %s", refusal.Msg, state)
		}
		if c := refusal.Doc; c != nil {
			now := "It does not exist now"
			if c.Current != nil {
				if o, err := docValue(*c.Current); err == nil {
					if b, err := json.Marshal(o); err == nil {
						now = "The document now is " + string(b)
					}
				}
			}
			return fmt.Errorf("%s. %s. Merge your change into that and send it again with ifVersion set to its version; do not retry unchanged: that would be refused again", refusal.Msg, now)
		}
		return fmt.Errorf("%s. Call read with {\"id\": %q} to get that newest version (title and content) now. Do not retry unchanged: that would be refused again", refusal.Msg, refusal.ArtifactID)
	}
	return errors.New(refusal.Msg)
}

// notFound is what an agent reads for a thing that is not there, by what it was
// looking for.
func notFound(refusal *service.Error) error {
	switch refusal.What {
	case "document":
		return errors.New("no such document in that collection; use db_list to see the ids in the collection")
	case "file":
		return errors.New(strings.TrimPrefix(refusal.Msg, "not found: ") + ". Read the artifact without file to see the list of its files")
	case "session", "upload":
		return errors.New(refusal.Msg)
	case "asset":
		return errors.New("that artifact has no file with that name; use list_assets to see its files and their exact names")
	}
	return errors.New("no artifact with that id is visible to you: it does not exist, or it belongs to someone who has not shared it. Use list to find ids")
}

// artifactOut mirrors the HTTP API's artifact: the same fields for the same
// viewer (TestAToolAndItsHTTPCounterpartGiveTheSameResult).
type artifactOut struct {
	store.Artifact
	URL string `json:"url"`
	// Role is what you are on this artifact: "author", "editor" (you may
	// publish, restore and manage files) or "reader".
	Role string `json:"role"`
}

func (t *tools) artifact(ctx context.Context, a store.Artifact) artifactOut {
	c := t.caller(ctx)
	return artifactOut{Artifact: a, URL: c.BaseURL + "/a/" + a.ID, Role: c.Viewer.Role(a)}
}

type idIn struct {
	ID string `json:"id" jsonschema:"the artifact id: the last part of its url (.../a/<id>)"`
}
