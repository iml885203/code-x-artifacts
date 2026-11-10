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
	"github.com/iml885203/code-x-artifacts/internal/auth"
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

	var authenticator auth.Authenticator
	authMode := cmp.Or(os.Getenv("PICTOR_AUTH"), "local")
	switch authMode {
	case "local":
		authenticator, err = auth.LoadLocal(cmp.Or(os.Getenv("PICTOR_USERS_FILE"), ".local/users.json"))
		if err != nil {
			return fmt.Errorf("%w; run codexartifacts init first, or set PICTOR_USERS_FILE", err)
		}
	case "ldap":
		if os.Getenv("LDAP_URL") == "" || os.Getenv("LDAP_DOMAIN") == "" || os.Getenv("LDAP_BASE_DN") == "" {
			return fmt.Errorf("LDAP_URL, LDAP_DOMAIN and LDAP_BASE_DN are required for LDAP authentication")
		}
		authenticator = &auth.LDAP{URL: os.Getenv("LDAP_URL"), Domain: os.Getenv("LDAP_DOMAIN"), BaseDN: os.Getenv("LDAP_BASE_DN"), AllowedGroups: splitList(os.Getenv("LDAP_ALLOWED_GROUPS"))}
	default:
		return fmt.Errorf("PICTOR_AUTH must be local or ldap")
	}
	publicLinks, err := parseBool("PICTOR_PUBLIC_LINKS")
	if err != nil {
		return err
	}
	oauthOrigin := os.Getenv("PICTOR_OAUTH_ORIGIN")
	if err := web.ValidateOAuthOrigin(oauthOrigin); err != nil {
		return fmt.Errorf("PICTOR_OAUTH_ORIGIN: %w", err)
	}
	app := web.New(web.Config{
		Admission:     &artifactguard.Registry{},
		Store:         st,
		Authenticator: authenticator,
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
		Addr:              cmp.Or(os.Getenv("PICTOR_ADDR"), "127.0.0.1:8080"),
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
