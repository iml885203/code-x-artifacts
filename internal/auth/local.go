package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// LocalAccount is an operator-managed account. Only password hashes are stored.
type LocalAccount struct {
	Username     string   `json:"username"`
	DisplayName  string   `json:"displayName"`
	PasswordHash string   `json:"passwordHash"`
	Groups       []string `json:"groups"`
}

// Local provides a bounded, explicitly configured roster for self-hosted use.
type Local struct {
	accounts []LocalAccount
	dummy    []byte
	slots    chan struct{}
}

// LoadLocal validates a private operator-managed account file before serving.
func LoadLocal(path string) (*Local, error) {
	f, err := os.Open(path) //nolint:gosec // G304: operator-selected credential file, never an HTTP input.
	if err != nil {
		return nil, fmt.Errorf("open local accounts: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.Size() > 1024*1024 {
		return nil, fmt.Errorf("local accounts file is unreadable or exceeds 1 MiB")
	}
	var accounts []LocalAccount
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&accounts); err != nil {
		return nil, fmt.Errorf("decode local accounts: %w", err)
	}
	if len(accounts) == 0 || len(accounts) > 1000 {
		return nil, fmt.Errorf("configure between 1 and 1000 local accounts")
	}
	seen := map[string]bool{}
	for i := range accounts {
		a := &accounts[i]
		a.Username = NormalizeUsername(a.Username)
		if !store.ValidDirectoryPerson(store.DirectoryPerson{Username: a.Username, DisplayName: localDisplayName(*a)}) || seen[a.Username] {
			return nil, fmt.Errorf("local account %d has an invalid or duplicate username", i+1)
		}
		cost, err := bcrypt.Cost([]byte(a.PasswordHash))
		if err != nil || cost < bcrypt.DefaultCost || cost > 14 {
			return nil, fmt.Errorf("local account %d requires a bcrypt hash with cost 10–14", i+1)
		}
		seen[a.Username] = true
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("non-account-timing-guard"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("prepare password check: %w", err)
	}
	return &Local{accounts: accounts, dummy: dummy, slots: make(chan struct{}, 4)}, nil
}

// Authenticate verifies a password and returns the configured profile and roster.
func (l *Local) Authenticate(ctx context.Context, username, password string) (SignIn, error) {
	if err := ctx.Err(); err != nil {
		return SignIn{}, err
	}
	select {
	case l.slots <- struct{}{}:
		defer func() { <-l.slots }()
	default:
		return SignIn{}, fmt.Errorf("password verification is busy; retry later")
	}
	username = NormalizeUsername(username)
	var account *LocalAccount
	for i := range l.accounts {
		if l.accounts[i].Username == username {
			account = &l.accounts[i]
			break
		}
	}
	hash := l.dummy
	if account != nil {
		hash = []byte(account.PasswordHash)
	}
	err := bcrypt.CompareHashAndPassword(hash, []byte(password))
	if err != nil || account == nil || password == "" {
		return SignIn{}, ErrInvalidCredentials
	}
	if err := ctx.Err(); err != nil {
		return SignIn{}, err
	}
	people := make([]store.DirectoryPerson, 0, len(l.accounts))
	for _, a := range l.accounts {
		people = append(people, store.DirectoryPerson{Username: a.Username, DisplayName: localDisplayName(a)})
	}
	return SignIn{
		User:      store.User{Username: account.Username, DisplayName: localDisplayName(*account), Groups: append([]string(nil), account.Groups...)},
		Directory: store.DirectorySnapshot{Scope: "local-accounts", StartedAt: time.Now().UnixNano(), Complete: true, People: people},
	}, nil
}

func localDisplayName(a LocalAccount) string {
	if a.DisplayName != "" {
		return a.DisplayName
	}
	return a.Username
}
