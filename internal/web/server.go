// Package web serves the gallery, the sandboxed viewer, the publishing API
// and the bridge that answers a page's window.pictor calls.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/artifactguard"
	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/auth"
	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/i18n"
	"github.com/iml885203/code-x-artifacts/internal/mcptools"
	"github.com/iml885203/code-x-artifacts/internal/operationlog"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Server holds the handlers' dependencies.
type Server struct {
	oauth    *oauthServer
	store    *store.Store
	svc      *service.Service
	mcp      *mcptools.Endpoint
	auth     auth.Authenticator
	sessions sessions
	// shell is the page the React app mounts on; every page of the site is the
	// same shell with a different boot block. bundle is the build it loads, or
	// the reason there is none.
	shell     *template.Template
	bundle    bundle
	bundleErr error
	assets    fs.FS
	langs     *i18n.Catalogues
	log       *slog.Logger
	// publicLinks is whether an artifact may be public; see service.Viewer.CanView.
	publicLinks bool
	// live is the realtime sockets: the hub of rooms, the tickets that open them.
	live *live
	// clients are the agent clients that started an MCP session (agents.go).
}

// Config is what New needs to build a Server.
type Config struct {
	// OAuthOrigin is the canonical public origin. Empty disables OAuth.
	OAuthOrigin string
	// SourceReadBases optionally supplies the pre-use source snapshot registry.
	SourceReadBases *designsourceedit.ReadBases
	// Admission is one process registry, shared by service, joins and peer messages.
	Admission     artifactguard.Admission
	Store         *store.Store
	Authenticator auth.Authenticator
	SessionKey    []byte
	Logger        *slog.Logger
	// Assets holds the bytes of the files artifacts reference; required.
	Assets *assets.Disk
	// PublicLinks lets authors share an artifact with anyone who has the link.
	// Off by default; see README.
	PublicLinks bool
	// UsageReaders are the accounts that may read the usage summary; nobody
	// when empty. See docs/usage.md.
	UsageReaders []string
	// Bundle is the built front end (Vite's output folder). Nil uses the one
	// embedded in the binary; tests give their own.
	Bundle fs.FS
	// FileLimits replaces the limits on the files that come with a page, for
	// tests that must reach one; the documented ones (files.DefaultLimits) when nil.
	FileLimits *files.Limits
	// MCPUploadBytes is the largest file the MCP upload_asset tool takes;
	// the tool's default when zero. Tests that probe the limit set a small one.
	MCPUploadBytes int
	// Clock is the time source for ticket expiry, connection lifetime and
	// heartbeat, and idle rooms; the system clock when nil.
	Clock Clock
	// RoomQuiet is how long a room goes without an edit before what it holds
	// becomes a version; roomkind.Quiet (two minutes) when zero. The browser
	// tests shorten it so a version can be waited for; the sweep that cuts it
	// then runs often enough to keep up.
	RoomQuiet time.Duration
	// ViewerStays is how long a person stays in the list of who is viewing after their last heartbeat;
	// a minute when zero. Only the browser tests shorten it.
	ViewerStays time.Duration
	// Realtime overrides the realtime hub's limits; a zero field keeps its default.
	Realtime realtime.Limits
}

// New builds a Server and parses its page templates and translations.
func New(cfg Config) *Server {
	langs, err := i18n.Load()
	if err != nil {
		panic(fmt.Sprintf("load translations: %v", err)) // embedded files; TestEveryLanguageHasExactlyTheKeysOfEnglish guards them
	}
	s := &Server{
		store:    cfg.Store,
		svc:      newService(cfg),
		auth:     cfg.Authenticator,
		sessions: sessions{key: cfg.SessionKey, ttl: 7 * 24 * time.Hour},
		langs:    langs,
		log:      cfg.Logger,

		publicLinks: cfg.PublicLinks,
	}
	s.oauth = newOAuthServer(cfg.OAuthOrigin, s.store)
	if cfg.FileLimits != nil {
		s.svc.SetFileLimits(*cfg.FileLimits)
	}
	if cfg.Clock != nil {
		s.svc.SetClock(cfg.Clock.Now)
	}
	s.live = newLive(cfg, s.svc)
	s.svc.OnAccessChange(s.liveAccessChanged)
	s.mcp = mcptools.New(mcptools.Config{Service: s.svc, Logger: cfg.Logger, MaxUploadBytes: cfg.MCPUploadBytes, Export: s.export})
	s.assets = cfg.Bundle
	if s.assets == nil {
		s.assets = embeddedBundle()
	}
	s.bundle, s.bundleErr = loadBundle(s.assets)
	if s.bundleErr != nil {
		cfg.Logger.Error("the front end is not built into this binary; pages the React app serves will fail", "err", s.bundleErr)
	}
	s.shell = template.Must(template.ParseFS(templateFS, "templates/app.html"))
	return s
}

// Handler routes every page and API endpoint. Each feature file registers its
// own routes in a routesX method; this is the one place that lists them.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routesStatic(mux)
	s.routesPages(mux)
	s.routesRaw(mux)
	mux.Handle("/api/mcp", s.mcpEndpoint())
	s.routesAgentActivity(mux)
	s.routesTokens(mux)
	s.routesOAuth(mux)
	s.routesArtifacts(mux)
	s.routesPublishSessions(mux)
	s.routesVersions(mux)
	s.routesDocEdit(mux)
	s.routesBoardEdit(mux)
	s.routesSlides(mux)
	s.routesPeople(mux)
	s.routesCreate(mux)
	s.routesLive(mux)
	s.routesChanges(mux)
	s.routesExport(mux)
	s.routesFiles(mux)
	s.routesDesign(mux)
	s.routesDB(mux)
	s.routesAssets(mux)
	s.routesComments(mux)
	s.routesPins(mux)
	s.routesOpens(mux)
	s.routesUsage(mux)
	s.routesViewers(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(operationlog.Context(r.Context(), s.log))
		if asksForAPage(r) {
			if _, pattern := mux.Handler(r); pattern == "" {
				s.notFoundPage(w, r)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

// fail answers a service error: a refusal with its status and message, anything
// else as an internal error whose detail stays in the log.
func (s *Server) fail(w http.ResponseWriter, err error) {
	if refusal, ok := errors.AsType[*service.Error](err); ok {
		status := http.StatusBadRequest
		switch refusal.Code {
		case service.NotFound:
			status = http.StatusNotFound
		case service.Forbidden:
			status = http.StatusForbidden
		case service.Busy:
			status = http.StatusTooManyRequests
		case service.Conflict:
			if c := refusal.Doc; c != nil {
				writeJSON(w, http.StatusConflict, map[string]any{"error": refusal.Msg, "conflict": map[string]any{"index": c.Index, "collection": c.Collection, "id": c.ID, "current": c.Current}})
				return
			}
			// The agent merges and retries; the number is there for code
			// that does not want to parse the message.
			// current is the newest version in full, so the merge needs no
			// second request.
			body := map[string]any{"error": refusal.Msg, "currentVersion": refusal.Current}
			if len(refusal.Stale) > 0 {
				body["stale"] = refusal.Stale
			}
			if len(refusal.StaleElements) > 0 {
				body["stale"] = refusal.StaleElements
			}
			if l := refusal.Latest; l != nil {
				body["current"] = map[string]any{"version": l.Version, "title": l.Title, "kind": l.Kind, "format": l.Format, "content": l.Content, "author": l.Author}
			}
			writeJSON(w, http.StatusConflict, body)
			return
		case service.Invalid:
		}
		jsonError(w, status, refusal.Msg)
		return
	}
	s.serverError(w, err)
}

func (s *Server) notFoundOr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	s.serverError(w, err)
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	s.log.Error("request", "err", err)
	jsonError(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // the client went away; nothing left to tell it
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func newService(cfg Config) *service.Service {
	svc := service.New(cfg.Store, cfg.Assets, cfg.PublicLinks)
	svc.SetUsageReaders(cfg.UsageReaders)
	svc.SetSourceReadBases(cfg.SourceReadBases)
	if cfg.Admission != nil {
		svc.SetArtifactGuard(cfg.Admission)
	}
	if cfg.ViewerStays > 0 {
		svc.SetViewerStays(cfg.ViewerStays)
	}
	return svc
}
