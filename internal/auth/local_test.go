package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalAccountsAuthenticateAndExposeOnlyTheConfiguredRoster(t *testing.T) {
	t.Parallel()
	// A fixed synthetic cost-10 hash exercises real verification without regenerating a fixture.
	const hash = "$2a$10$Xgi5p9b1m0HTZrLnN7FU3u8ggy93P/i0K4SV3EGXfGcjzEfDX.1/6"
	file := filepath.Join(t.TempDir(), "users.json")
	data, err := json.Marshal([]LocalAccount{{Username: "alice", DisplayName: "Alice Example", PasswordHash: hash, Groups: []string{"team"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	local, err := LoadLocal(file)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := local.Authenticate(t.Context(), "ALICE", "a-long-test-password")
	if err != nil || signed.User.Username != "alice" || signed.User.DisplayName != "Alice Example" || len(signed.User.Groups) != 1 || !signed.Directory.Complete || len(signed.Directory.People) != 1 {
		t.Fatalf("valid account did not produce its profile and roster: %v", err)
	}
	for _, pair := range [][2]string{{"alice", "wrong"}, {"unknown", "a-long-test-password"}, {"alice", ""}} {
		if _, err := local.Authenticate(t.Context(), pair[0], pair[1]); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("invalid credentials were not rejected: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := local.Authenticate(ctx, "alice", "a-long-test-password"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled login: %v", err)
	}
}

func TestLocalAccountConfigurationFailsClosed(t *testing.T) {
	t.Parallel()
	for _, data := range []string{`[]`, `[{"username":"alice","passwordHash":"plaintext"}]`, `[{"username":"alice","unknown":true}]`, `{broken`} {
		file := filepath.Join(t.TempDir(), "users.json")
		if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadLocal(file); err == nil {
			t.Fatal("invalid local account configuration accepted")
		}
	}
}
