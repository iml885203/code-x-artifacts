package web

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// TestServeForBrowserSmoke serves the real handler with fake credentials so a
// browser can drive the bridge end to end: PICTOR_SMOKE_ADDR=:18080 go test -run Smoke ./internal/web
// It runs the server's background work as cmd/pictor does, so rooms are tidied
// and versions are cut from quiet rooms; PICTOR_SMOKE_ROOM_QUIET (a Go duration)
// shortens the two-minute pause for the browser tests. PICTOR_SMOKE_DB_DIR
// puts the database elsewhere: every request waits on its one connection, so a
// commit stalled on a shared disk stalls every page at once.
func TestServeForBrowserSmoke(t *testing.T) { //nolint:paralleltest // reads PICTOR_SMOKE_ADDR from the environment and serves until killed
	addr := os.Getenv("PICTOR_SMOKE_ADDR")
	if addr == "" {
		t.Skip("PICTOR_SMOKE_ADDR not set")
	}
	dbDir := t.TempDir()
	if parent := os.Getenv("PICTOR_SMOKE_DB_DIR"); parent != "" {
		// Left behind, as t.TempDir is: the server only ever ends by being killed.
		var err error
		if dbDir, err = os.MkdirTemp(parent, "pictor-smoke-"); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(t.Context(), filepath.Join(dbDir, "pictor.db"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := assets.OpenDisk(filepath.Join(t.TempDir(), "assets"))
	if err != nil {
		t.Fatal(err)
	}
	var quiet time.Duration
	if v := os.Getenv("PICTOR_SMOKE_ROOM_QUIET"); v != "" {
		if quiet, err = time.ParseDuration(v); err != nil {
			t.Fatalf("PICTOR_SMOKE_ROOM_QUIET: %v", err)
		}
	}
	var viewerStays time.Duration
	if v := os.Getenv("PICTOR_SMOKE_VIEWER_STAYS"); v != "" {
		if viewerStays, err = time.ParseDuration(v); err != nil {
			t.Fatalf("PICTOR_SMOKE_VIEWER_STAYS: %v", err)
		}
	}
	directory := newFakeDirectory()
	directory.accounts["dana"] = &fakeAccount{password: "unused-test-password", groups: []string{"SE"}}
	s := New(Config{Store: st, Assets: files, Authenticator: directory, SessionKey: []byte("smoke"), Logger: slog.Default(), PublicLinks: true, RoomQuiet: quiet, ViewerStays: viewerStays, OAuthOrigin: "http://" + addr})
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go s.Run(ctx)
	srv := &http.Server{Addr: addr, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	t.Fatal(srv.ListenAndServe())
}
