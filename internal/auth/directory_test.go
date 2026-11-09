package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
)

type directoryPages struct {
	pages []*ldap.SearchResult
	err   error
}

func (d *directoryPages) SetTimeout(time.Duration) {}
func (d *directoryPages) Search(_ *ldap.SearchRequest) (*ldap.SearchResult, error) {
	if d.err != nil {
		return nil, d.err
	}
	if len(d.pages) == 0 {
		return nil, errors.New("unexpected extra page")
	}
	r := d.pages[0]
	d.pages = d.pages[1:]
	return r, nil
}

func directoryPage(cookie string, entries ...*ldap.Entry) *ldap.SearchResult {
	return &ldap.SearchResult{Entries: entries, Controls: []ldap.Control{&ldap.ControlPaging{PagingSize: 500, Cookie: []byte(cookie)}}}
}

func colleague(account, name string) *ldap.Entry {
	return ldap.NewEntry("CN="+account+",DC=example,DC=test", map[string][]string{"sAMAccountName": {account}, "displayName": {name}})
}

func TestDirectoryPagingIsCompleteOnlyAfterAValidatedTerminalPage(t *testing.T) {
	t.Parallel()
	l := &LDAP{BaseDN: "DC=example,DC=test"}
	cases := []struct {
		name  string
		pages []*ldap.SearchResult
		bad   bool
	}{
		{"complete", []*ldap.SearchResult{directoryPage("next", colleague("alice", "Alice")), directoryPage("", colleague("never-signed-in", "陳同事"))}, false},
		{"truncated", []*ldap.SearchResult{directoryPage("next", colleague("alice", "Alice"))}, true},
		{"missing paging control", []*ldap.SearchResult{{Entries: []*ldap.Entry{colleague("alice", "Alice")}}}, true},
		{"invalid row", []*ldap.SearchResult{directoryPage("", colleague("alice", "Alice"), colleague("bad user", "Bad"))}, true},
		{"duplicate", []*ldap.SearchResult{directoryPage("next", colleague("alice", "Alice")), directoryPage("", colleague("alice", "Alice"))}, true},
		{"missing caller", []*ldap.SearchResult{directoryPage("", colleague("bob", "Bob"))}, true},
		{"referral", []*ldap.SearchResult{{Entries: []*ldap.Entry{colleague("alice", "Alice")}, Referrals: []string{"ldap://other.invalid"}}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := l.readDirectory(context.Background(), &directoryPages{pages: c.pages}, "alice")
			if (err != nil) != c.bad || got.Complete == c.bad {
				t.Fatalf("snapshot complete=%v err=%v", got.Complete, err)
			}
			if !c.bad && len(got.People) != 2 {
				t.Fatalf("missing never-signed-in colleague: %+v", got)
			}
		})
	}
}

func TestDirectoryGroupScopeMustResolveInsideTheConfiguredBase(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, dn, cn string
		bad          bool
	}{
		{"valid", "CN=SE,DC=example,DC=test", "SE", false},
		{"outside base", "CN=SE,DC=other,DC=test", "SE", true},
		{"different group", "CN=SE,DC=example,DC=test", "Other", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := &LDAP{BaseDN: "DC=example,DC=test", AllowedGroups: []string{"SE"}}
			group := ldap.NewEntry(tc.dn, map[string][]string{"cn": {tc.cn}})
			d := &directoryPages{pages: []*ldap.SearchResult{{Entries: []*ldap.Entry{group}}, directoryPage("", colleague("alice", "Alice"))}}
			got, err := l.readDirectory(context.Background(), d, "alice")
			if (err != nil) != tc.bad || got.Complete == tc.bad {
				t.Fatalf("scope complete=%v err=%v", got.Complete, err)
			}
		})
	}
}

func TestDirectoryFailureAndCancellationCannotBecomeAnEmptyCompleteRoster(t *testing.T) {
	t.Parallel()
	l := &LDAP{BaseDN: "DC=example,DC=test"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []context.Context{ctx, context.Background()} {
		got, err := l.readDirectory(c, &directoryPages{err: ldap.NewError(ldap.LDAPResultSizeLimitExceeded, errors.New("size limit"))}, "alice")
		if err == nil || got.Complete || len(got.People) > 0 {
			t.Fatalf("partial generation: %+v %v", got, err)
		}
	}
}

func TestDirectoryQueriesKeepTheExistingAllowedGroupAndDisabledAccountScope(t *testing.T) {
	t.Parallel()
	l := &LDAP{BaseDN: "DC=example,DC=test", AllowedGroups: []string{"SE"}}
	filter, err := l.directoryFilter(func(req *ldap.SearchRequest) (*ldap.SearchResult, error) {
		if req.BaseDN != l.BaseDN || req.Filter != "(&(objectClass=group)(cn=SE))" {
			t.Fatalf("group resolution scope: %+v", req)
		}
		return &ldap.SearchResult{Entries: []*ldap.Entry{ldap.NewEntry("CN=SE,DC=example,DC=test", map[string][]string{"cn": {"SE"}})}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"(memberOf:1.2.840.113556.1.4.1941:=CN=SE,DC=example,DC=test)", "(!(userAccountControl:1.2.840.113556.1.4.803:=2))", "(sAMAccountType=805306368)"} {
		if !strings.Contains(filter, required) {
			t.Errorf("eligible colleague filter lost %s: %s", required, filter)
		}
	}
}

func TestDirectoryReadsCannotRunPastThePageOrPersonBounds(t *testing.T) {
	t.Parallel()
	l := &LDAP{BaseDN: "DC=example,DC=test"}
	pages := []*ldap.SearchResult{}
	for i := range 20 {
		entries := []*ldap.Entry{}
		if i == 0 {
			entries = append(entries, colleague("alice", "Alice"))
		}
		pages = append(pages, directoryPage(fmt.Sprintf("page%d", i), entries...))
	}
	got, err := l.readDirectory(context.Background(), &directoryPages{pages: pages}, "alice")
	if err == nil || got.Complete {
		t.Fatal("nonterminal twentieth page accepted")
	}
	entries := make([]*ldap.Entry, 10001)
	for i := range entries {
		entries[i] = colleague(fmt.Sprintf("account%d", i), "Person")
	}
	got, err = l.readDirectory(context.Background(), &directoryPages{pages: []*ldap.SearchResult{directoryPage("", entries...)}}, "account0")
	if err == nil || got.Complete {
		t.Fatal("oversized roster accepted")
	}
}
