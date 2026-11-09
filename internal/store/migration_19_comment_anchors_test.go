package store

import (
	"path/filepath"
	"testing"
)

func TestOpeningADatabaseWithCommentsFromBeforeAnchorsMakesThemUnanchoredAndForTheAgent(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "pictor.db")
	st := openStore(ctx, t, path)
	a := artifactBy(t, st, "alice")
	// Put the comments table back to what migration 18 left, with a comment written then.
	for _, q := range []string{
		`ALTER TABLE comments DROP COLUMN anchor`,
		`ALTER TABLE comments DROP COLUMN for_agent`,
		`INSERT INTO comments (id, artifact_id, author, body, parent_id, created_at) VALUES ('old1', '` + a.ID + `', 'bob', 'from before', '', 1)`,
	} {
		if _, err := st.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st = openStore(ctx, t, path)
	defer func() { _ = st.Close() }()
	got, err := st.Comment(ctx, a.ID, "old1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.ForAgent || len(got.Anchor) != 0 || got.Body != "from before" {
		t.Errorf("a comment from before anchors = %+v, want unanchored and forAgent true", got)
	}
}
