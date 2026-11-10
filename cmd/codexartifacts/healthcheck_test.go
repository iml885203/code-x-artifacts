package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthcheckCommandProbesWithoutInitializingAccountsOrDatabase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" || r.Method != "GET" {
			t.Errorf("unexpected probe %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	directory := t.TempDir()
	t.Setenv("PICTOR_ADDR", strings.TrimPrefix(server.URL, "http://"))
	t.Setenv("PICTOR_DB", filepath.Join(directory, "must-not-exist.db"))
	t.Setenv("PICTOR_AUTH", "oidc")
	if err := runCommand([]string{"healthcheck"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatal("probe initialized state", err)
	}
	if err := runCommand([]string{"healthcheck", "unexpected"}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("extra argument accepted")
	}
}

func TestHealthcheckRejectsRedirectsUnhealthyResponsesAndBadAddresses(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusFound, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "http://127.0.0.1:1/")
				w.WriteHeader(status)
			}))
			defer server.Close()
			if err := healthcheck(strings.TrimPrefix(server.URL, "http://")); err == nil {
				t.Fatal("unhealthy probe accepted")
			}
		})
	}
	if err := healthcheck("not-an-address"); err == nil {
		t.Fatal("invalid address accepted")
	}
}
