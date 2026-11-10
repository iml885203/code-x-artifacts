package web

import (
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/auth"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// An isolated application/provider boundary for the browser suite. It exercises
// native cross-origin navigation and cookies without changing the local-login
// server used by the other browser tests. None of these routes exist in a build.
func newOIDCBrowserSmoke(t *testing.T) *httptest.Server {
	t.Helper()
	provider := newOIDCProvider(t)
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "oidc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	disk, err := assets.OpenDisk(filepath.Join(t.TempDir(), "assets"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	oidc, err := auth.NewOIDC(t.Context(), provider.config(origin))
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	app := New(Config{Store: st, Assets: disk, OIDC: oidc, OAuthOrigin: origin, SessionKey: []byte("oidc-browser-smoke"), Logger: slog.Default()})
	server.Config.Handler = app.Handler()
	server.Start()
	t.Cleanup(server.Close)
	return server
}
