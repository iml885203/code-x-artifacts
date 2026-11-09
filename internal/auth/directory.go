package auth

import (
	"cmp"
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// SignIn keeps optional directory lookup metadata apart from the authenticated
// profile. A failed roster read never changes a successful password/policy check.
type SignIn struct {
	User           store.User
	Directory      store.DirectorySnapshot
	DirectoryError error
}

type directoryReader interface {
	Search(*ldap.SearchRequest) (*ldap.SearchResult, error)
	SetTimeout(time.Duration)
}

// readDirectory uses only the caller's already-bound connection. It does not
// retain it or credentials. Every page must finish within one bounded read;
// referrals, missing controls, invalid rows or an unresolved policy scope make
// the entire read incomplete, never evidence that somebody departed.
func (l *LDAP) readDirectory(ctx context.Context, conn directoryReader, caller string) (store.DirectorySnapshot, error) {
	groups := slices.Clone(l.AllowedGroups)
	for i := range groups {
		groups[i] = strings.ToLower(groups[i])
	}
	slices.Sort(groups)
	scope := sha256.Sum256([]byte(strings.ToLower(l.URL+"\x00"+l.BaseDN) + "\x00" + strings.Join(groups, "\x00")))
	out := store.DirectorySnapshot{Scope: fmt.Sprintf("%x", scope), StartedAt: time.Now().UnixNano()}
	read := directoryRead{ctx: ctx, conn: conn, deadline: time.Now().Add(3 * time.Second)}
	filter, err := l.directoryFilter(read.search)
	if err != nil {
		return out, err
	}
	people, err := l.directoryPages(read.search, filter, caller)
	if err != nil {
		return out, err
	}
	out.People = people
	out.Complete = true
	return out, nil
}

type directoryRead struct {
	ctx      context.Context
	conn     directoryReader
	deadline time.Time
}

func (r directoryRead) search(req *ldap.SearchRequest) (*ldap.SearchResult, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	left := time.Until(r.deadline)
	if left <= 0 {
		return nil, fmt.Errorf("directory read exceeded its deadline")
	}
	r.conn.SetTimeout(left)
	result, err := r.conn.Search(req)
	if err != nil {
		return nil, fmt.Errorf("directory read: %w", err)
	}
	if r.ctx.Err() != nil || time.Now().After(r.deadline) {
		return nil, fmt.Errorf("directory read did not finish within its context/deadline")
	}
	if result == nil || len(result.Referrals) > 0 {
		return nil, fmt.Errorf("directory read returned referrals or no result")
	}
	return result, nil
}

func (l *LDAP) directoryFilter(search func(*ldap.SearchRequest) (*ldap.SearchResult, error)) (string, error) {
	membership := ""
	if len(l.AllowedGroups) > 100 {
		return "", fmt.Errorf("directory policy has too many group scopes")
	}
	for _, g := range l.AllowedGroups {
		r, err := search(ldap.NewSearchRequest(l.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 2, false, "(&(objectClass=group)(cn="+ldap.EscapeFilter(g)+"))", []string{"cn"}, nil))
		if err != nil {
			return "", err
		}
		if len(r.Entries) != 1 || !strings.EqualFold(r.Entries[0].GetAttributeValue("cn"), g) || !directoryDNWithin(l.BaseDN, r.Entries[0].DN) {
			return "", fmt.Errorf("directory policy group is missing or ambiguous")
		}
		membership += "(memberOf:1.2.840.113556.1.4.1941:=" + ldap.EscapeFilter(r.Entries[0].DN) + ")"
	}
	if membership != "" {
		membership = "(|" + membership + ")"
	}
	return "(&(objectCategory=person)(objectClass=user)(sAMAccountType=805306368)(!(userAccountControl:1.2.840.113556.1.4.803:=2))" + membership + ")", nil
}

func (l *LDAP) directoryPages(search func(*ldap.SearchRequest) (*ldap.SearchResult, error), filter, caller string) ([]store.DirectoryPerson, error) {
	paging := ldap.NewControlPaging(500)
	seen := map[string]bool{}
	cookies := map[string]bool{}
	var people []store.DirectoryPerson
	for range 20 {
		r, err := search(ldap.NewSearchRequest(l.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 2, false, filter, []string{"sAMAccountName", "displayName"}, []ldap.Control{paging}))
		if err != nil {
			return nil, err
		}
		control, ok := ldap.FindControl(r.Controls, ldap.ControlTypePaging).(*ldap.ControlPaging)
		if !ok {
			return nil, fmt.Errorf("directory did not confirm paged results")
		}
		people, err = appendDirectoryRows(people, seen, r.Entries, l.BaseDN)
		if err != nil {
			return nil, err
		}
		if len(control.Cookie) == 0 {
			if !seen[caller] {
				return nil, fmt.Errorf("directory scope did not include the signed-in account")
			}
			return people, nil
		}
		cookie := string(control.Cookie)
		if cookies[cookie] {
			return nil, fmt.Errorf("directory repeated a paging cookie")
		}
		cookies[cookie] = true
		paging.SetCookie(control.Cookie)
	}
	return nil, fmt.Errorf("directory exceeded its page limit")
}

func appendDirectoryRows(people []store.DirectoryPerson, seen map[string]bool, entries []*ldap.Entry, base string) ([]store.DirectoryPerson, error) {
	for _, entry := range entries {
		if entry == nil || !directoryDNWithin(base, entry.DN) {
			return nil, fmt.Errorf("directory row is outside the configured scope")
		}
		accounts := entry.GetAttributeValues("sAMAccountName")
		if len(accounts) != 1 {
			return nil, fmt.Errorf("directory row has no unique account")
		}
		account := strings.ToLower(accounts[0])
		p := store.DirectoryPerson{Username: account, DisplayName: cmp.Or(entry.GetAttributeValue("displayName"), account)}
		if !store.ValidDirectoryPerson(p) || seen[account] {
			return nil, fmt.Errorf("directory row is invalid or duplicated")
		}
		seen[account] = true
		people = append(people, p)
		if len(people) > 10000 {
			return nil, fmt.Errorf("directory exceeds the bounded roster size")
		}
	}
	return people, nil
}

func directoryDNWithin(base, dn string) bool {
	parent, err := ldap.ParseDN(base)
	if err != nil || len(parent.RDNs) == 0 {
		return false
	}
	child, err := ldap.ParseDN(dn)
	return err == nil && (parent.EqualFold(child) || parent.AncestorOfFold(child))
}
