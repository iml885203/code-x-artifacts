package store

import (
	"context"
	"fmt"
	"strings"
)

// SearchUsers finds active organisation directory people whose account or name
// contains q, ignoring case (an empty q matches everyone). Those whose account
// or name starts with q come first, then account order; at most
// limit are returned.
func (s *Store) SearchUsers(ctx context.Context, q string, limit int) ([]DirectoryPerson, error) {
	like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.ToLower(q)) + "%"
	prefix := like[1:]
	rows, err := s.db.QueryContext(ctx, `
		SELECT username, display_name FROM directory_people
		WHERE active=1 AND EXISTS (SELECT 1 FROM directory_status WHERE scope=snapshot_scope) AND (lower(username) LIKE ? ESCAPE '\' OR lower(display_name) LIKE ? ESCAPE '\')
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
