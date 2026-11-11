// Command codexartifacts serves the self-hosted artifact workspace.
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/artifactguard"
	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/iml885203/code-x-artifacts/internal/web"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	var err error
	if len(os.Args) > 1 {
		err = runCommand(os.Args[1:], os.Stdin, os.Stdout)
	} else {
		err = run(log)
	}
	if err != nil {
		log.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbPath := cmp.Or(os.Getenv("PICTOR_DB"), ".local/codexartifacts.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Error("close store", "err", err)
		}
	}()
	key, err := st.SessionKey(ctx)
	if err != nil {
		return err
	}

	files, err := assets.OpenDisk(assetsDir(dbPath, os.Getenv("PICTOR_ASSETS_DIR")))
	if err != nil {
		return err
	}

	authMode := cmp.Or(os.Getenv("PICTOR_AUTH"), "local")
	authenticator, oidcLogin, err := configuredLogin(ctx, authMode)
	if err != nil {
		return err
	}

	publicLinks, err := parseBool("PICTOR_PUBLIC_LINKS")
	if err != nil {
		return err
	}
	oauthOrigin := os.Getenv("PICTOR_OAUTH_ORIGIN")
	if oidcLogin != nil && oauthOrigin != "" && oidcLogin.Origin() != oauthOrigin {
		return fmt.Errorf("OIDC_ORIGIN and PICTOR_OAUTH_ORIGIN must match when both are enabled")
	}
	if err := web.ValidateOAuthOrigin(oauthOrigin); err != nil {
		return fmt.Errorf("PICTOR_OAUTH_ORIGIN: %w", err)
	}
	thumbnailBudget, err := thumbnailCacheBudget(os.Getenv("PICTOR_THUMBNAIL_CACHE_MB"))
	if err != nil {
		return err
	}
	app := web.New(web.Config{
		ThumbnailDir: filepath.Join(filepath.Dir(dbPath), "thumbnails"), ThumbnailCacheBytes: thumbnailBudget,
		Admission:     &artifactguard.Registry{},
		Store:         st,
		Authenticator: authenticator,
		OIDC:          oidcLogin,
		SessionKey:    key,
		Assets:        files,
		Logger:        log,
		PublicLinks:   publicLinks,
		OAuthOrigin:   oauthOrigin,
		UsageReaders:  splitList(os.Getenv("PICTOR_USAGE_READERS")),
	})
	go app.Maintain(ctx, 24*time.Hour)
	// The live sockets are hijacked from the HTTP server, so its shutdown does not
	// wait for them: the app's Run closes them when ctx ends.
	appDone := make(chan struct{})
	go func() {
		app.Run(ctx)
		close(appDone)
	}()
	// Deferred after the store's close, so it runs before it: the hub writes its last compactions on the way out.
	defer func() { <-appDone }()
	srv := &http.Server{
		Addr:              listenAddress(),
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			log.Error("shutdown", "err", err)
		}
	}()
	log.Info("listening", "addr", srv.Addr, "authentication", authMode, "publicLinks", publicLinks)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// assetsDir is where asset files live: the configured directory, or "assets"
// next to the database, so one volume holds all of a deployment's data.
func assetsDir(dbPath, configured string) string {
	return cmp.Or(configured, filepath.Join(filepath.Dir(dbPath), "assets"))
}

// parseBool reads an on/off environment variable; unset means off, and a value
// that is not a boolean stops the start instead of silently meaning off.
func parseBool(name string) (bool, error) {
	v := os.Getenv(name)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false, got %q", name, v)
	}
	return b, nil
}

func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func thumbnailCacheBudget(value string) (int64, error) {
	if value == "" {
		return 200 << 20, nil
	}
	mb, err := strconv.ParseInt(value, 10, 32)
	if err != nil || mb < 1 || mb > 4096 {
		return 0, fmt.Errorf("PICTOR_THUMBNAIL_CACHE_MB must be 1..4096")
	}
	return mb << 20, nil
}
