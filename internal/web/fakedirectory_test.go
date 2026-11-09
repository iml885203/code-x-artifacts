package web

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/auth"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// fakeDirectory stands in for AD: accounts with a password and the groups the
// directory reports at sign-in. A test moves someone between groups with
// setGroups; the change reaches pictor only when that person signs in again.
type fakeDirectory struct {
	mu         sync.Mutex
	accounts   map[string]*fakeAccount
	incomplete bool
}

type fakeAccount struct {
	password   string
	groups     []string
	department string
}

func newFakeDirectory() *fakeDirectory {
	return &fakeDirectory{accounts: map[string]*fakeAccount{
		"alice": {"pw-a", []string{"QA", "SE"}, "Engineering"},
		"bob":   {"pw-b", []string{"SE"}, ""},
		"carol": {"pw-c", []string{"Ops"}, "Operations"},
	}}
}

func (d *fakeDirectory) setGroups(username string, groups ...string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.accounts[username].groups = groups
}

func (d *fakeDirectory) Authenticate(_ context.Context, username, password string) (auth.SignIn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	acc, ok := d.accounts[username]
	if !ok || acc.password != password {
		return auth.SignIn{}, auth.ErrInvalidCredentials
	}
	user := store.User{
		Username: username, DisplayName: strings.ToUpper(username), Email: username + "@example.com",
		Groups: slices.Clone(acc.groups), Department: acc.department,
	}
	people := make([]store.DirectoryPerson, 0, len(d.accounts))
	for account := range d.accounts {
		people = append(people, store.DirectoryPerson{Username: account, DisplayName: strings.ToUpper(account)})
	}
	return auth.SignIn{User: user, Directory: store.DirectorySnapshot{Scope: "fake-organisation", StartedAt: time.Now().UnixNano(), Complete: !d.incomplete, People: people}}, nil
}
