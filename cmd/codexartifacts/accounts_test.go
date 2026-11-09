package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/auth"
)

func TestInitializerCreatesWorkingPrivateCredentialsWithoutOverwriting(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "accounts")
	if err := initAccounts(dir, io.Discard); err != nil {
		t.Fatal(err)
	}
	passwordPath := filepath.Join(dir, "admin-password")
	password, err := os.ReadFile(passwordPath) //nolint:gosec // G304: test-owned temporary credential file.
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"users.json", "admin-password"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("credential file is not private: %s", name)
		}
	}
	local, err := auth.LoadLocal(filepath.Join(dir, "users.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.Authenticate(t.Context(), "admin", strings.TrimSpace(string(password))); err != nil {
		t.Fatal(err)
	}
	if err := initAccounts(dir, io.Discard); err == nil {
		t.Fatal("initializer overwrote existing credentials")
	}
	after, err := os.ReadFile(passwordPath) //nolint:gosec // G304: test-owned temporary credential file.
	if err != nil || string(after) != string(password) {
		t.Fatal("existing password was changed")
	}
}
