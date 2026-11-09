// Package auth signs people in against Active Directory.
package auth

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// Sign-in outcomes a person can act on, as opposed to directory failures.
var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrNotAllowed         = errors.New("account is not in an allowed group")
)

// Authenticator checks a username and password and returns a profile with a separate directory lookup outcome.
type Authenticator interface {
	Authenticate(ctx context.Context, username, password string) (SignIn, error)
}

// LDAP binds as the person signing in, so the service holds no directory
// credential of its own: the bind proves the password, and the same
// connection reads the person's own entry, groups and bounded colleague lookup.
type LDAP struct {
	URL           string
	Domain        string // UPN suffix and DNS domain, e.g. example.com
	BaseDN        string
	AllowedGroups []string // empty allows every account that can bind
}

// Authenticate binds as the person and reads their profile and transitive groups.
func (l *LDAP) Authenticate(ctx context.Context, username, password string) (SignIn, error) {
	username = NormalizeUsername(username)
	// An empty password is an unauthenticated bind, which AD accepts.
	if username == "" || password == "" {
		return SignIn{}, ErrInvalidCredentials
	}

	conn, err := ldap.DialURL(l.URL, ldap.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}))
	if err != nil {
		return SignIn{}, fmt.Errorf("ldap dial: %w", err)
	}
	defer func() { _ = conn.Close() }() // read-only session; nothing to flush
	conn.SetTimeout(10 * time.Second)

	if err := l.bind(conn, username, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return SignIn{}, ErrInvalidCredentials
		}
		return SignIn{}, fmt.Errorf("ldap bind: %w", err)
	}

	entry, err := searchOne(conn, l.BaseDN,
		fmt.Sprintf("(&(objectClass=user)(sAMAccountName=%s))", ldap.EscapeFilter(username)),
		"displayName", "mail", "department")
	if err != nil {
		return SignIn{}, err
	}
	groups, err := searchGroups(conn, l.BaseDN, entry.DN)
	if err != nil {
		return SignIn{}, err
	}
	if len(l.AllowedGroups) > 0 && !slices.ContainsFunc(groups, func(g string) bool {
		return slices.ContainsFunc(l.AllowedGroups, func(a string) bool { return strings.EqualFold(a, g) })
	}) {
		return SignIn{}, ErrNotAllowed
	}

	user := store.User{
		Username:    username,
		DisplayName: cmp.Or(entry.GetAttributeValue("displayName"), username),
		Email:       entry.GetAttributeValue("mail"),
		Groups:      groups,
		Department:  strings.TrimSpace(entry.GetAttributeValue("department")),
	}
	directory, directoryErr := l.readDirectory(ctx, conn, username)
	return SignIn{User: user, Directory: directory, DirectoryError: directoryErr}, nil
}

// bind never puts the password on the wire in clear: a simple bind only inside
// TLS, otherwise NTLM's challenge-response. The example.com DCs serve no LDAPS
// certificate, so in practice this is NTLM on 389.
func (l *LDAP) bind(conn *ldap.Conn, username, password string) error {
	upn := username + "@" + l.Domain
	if simpleBindIsSafe(l.URL) {
		return conn.Bind(upn, password)
	}
	return conn.NTLMBind("", upn, password)
}

func simpleBindIsSafe(url string) bool {
	return strings.HasPrefix(strings.ToLower(url), "ldaps://")
}

func searchOne(conn *ldap.Conn, base, filter string, attrs ...string) (*ldap.Entry, error) {
	res, err := conn.Search(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 10, false, filter, attrs, nil))
	if err != nil {
		return nil, fmt.Errorf("ldap search: %w", err)
	}
	if len(res.Entries) != 1 {
		return nil, fmt.Errorf("ldap search %s: %d entries", filter, len(res.Entries))
	}
	return res.Entries[0], nil
}

// searchGroups resolves nested membership with AD's transitive matching rule,
// the same filter OpenBao and Grafana use against this directory.
func searchGroups(conn *ldap.Conn, base, userDN string) ([]string, error) {
	filter := fmt.Sprintf("(&(objectClass=group)(member:1.2.840.113556.1.4.1941:=%s))", ldap.EscapeFilter(userDN))
	res, err := conn.SearchWithPaging(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 10, false, filter, []string{"cn"}, nil), 500)
	if err != nil {
		return nil, fmt.Errorf("ldap group search: %w", err)
	}
	groups := make([]string, 0, len(res.Entries))
	for _, e := range res.Entries {
		groups = append(groups, e.GetAttributeValue("cn"))
	}
	slices.Sort(groups)
	return groups, nil
}

// NormalizeUsername is the account name of whatever a person typed: no domain,
// lower case. Sign-in and the artifact's editors use the same rule, so the name
// an author types for a colleague is the name that colleague signs in as.
func NormalizeUsername(u string) string {
	u = strings.TrimSpace(u)
	if i := strings.LastIndex(u, `\`); i >= 0 {
		u = u[i+1:]
	}
	if i := strings.Index(u, "@"); i >= 0 {
		u = u[:i]
	}
	return strings.ToLower(u)
}
