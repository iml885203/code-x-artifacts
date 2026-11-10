package store

import (
	"context"
	"fmt"
	"strings"
)

// SearchUsers finds active directory people and already-known OIDC accounts whose account or name
// contains q, ignoring case (an empty q matches everyone). Those whose account
// or name starts with q come first, then account order; at most
// limit are returned.
func (s *Store) SearchUsers(ctx context.Context, q string, limit int) ([]DirectoryPerson, error) {
	like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.ToLower(q)) + "%"
	prefix := like[1:]
	rows, err := s.db.QueryContext(ctx, `
		WITH known_people AS (
			SELECT username, display_name FROM directory_people
			WHERE active=1 AND EXISTS (SELECT 1 FROM directory_status WHERE scope=snapshot_scope)
			UNION
			SELECT users.username, users.display_name FROM users
			JOIN oidc_identities ON oidc_identities.username=users.username
		)
		SELECT username, display_name FROM known_people
		WHERE (lower(username) LIKE ? ESCAPE '\' OR lower(display_name) LIKE ? ESCAPE '\')
		ORDER BY (lower(username) LIKE ? ESCAPE '\' OR lower(display_name) LIKE ? ESCAPE '\') DESC, username
		LIMIT ?`, like, like, prefix, prefix, limit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DirectoryPerson
	for rows.Next() {
		var u DirectoryPerson
		if err := rows.Scan(&u.Username, &u.DisplayName); err != nil {
			return nil, fmt.Errorf("read user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
