package store

import (
	"context"
	"database/sql"
	"fmt"
)

// A migration upgrades a database created by an earlier version. It must be
// safe to run on every start, so it checks before it changes anything.
// Columns added after the first release live in migrations, not in schema, so
// a fresh database and an upgraded one take the same path.
//
// Each migration has its own file, migration_NN_name.go. To add one: write the
// file, append it to migrations with the next number. Never reorder, renumber
// or edit a shipped one; the sequence is checked by TestMigrationsAreNumberedInOrder.
type migration struct {
	number int
	name   string
	run    func(context.Context, *sql.DB) error
}

// migrations is the whole sequence, in the order it runs.
var migrations = []migration{
	{1, "artifact visibility", migration01Visibility},
	{2, "artifact kind", migration02Kind},
	{3, "artifact format", migration03Format},
	{4, "versions", migration04Versions},
	{5, "specific grants dropped", migration05SpecificGrants},
	{6, "shared document version", migration06DocVersion},
	{7, "assets", migration07Assets},
	{8, "comments", migration08Comments},
	{9, "editors", migration09Editors},
	{10, "copied from", migration10CopiedFrom},
	{11, "version files", migration11VersionFiles},
	{12, "access changes", migration12AccessChanges},
	{13, "markdown docs to block docs", migrateMarkdownDocs},
	{14, "realtime update log", migrateRealtime},
	{15, "version authors", migration15VersionAuthors},
	{16, "pending edits of rooms", migration16PendingEdits},
	{17, "publish sessions", migration17PublishSessions},
	{18, "pins", migration18Pins},
	{19, "comment anchors", migration19CommentAnchors},
	{20, "version agents", migration20VersionAgents},
	{21, "directory lookup metadata", migration21Directory},
	{22, "agent activity", migration22AgentActivity},
	{23, "opens", migration23Opens},
	{24, "comment deletion", migration24CommentDeletion},
	{25, "usage", migration25Usage},
}

func migrate(ctx context.Context, db *sql.DB) error {
	for _, m := range migrations {
		if err := m.run(ctx, db); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.number, m.name, err)
		}
	}
	return nil
}

func addArtifactColumn(ctx context.Context, db *sql.DB, column, definition string) error {
	var present int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('artifacts') WHERE name = ?`, column).Scan(&present); err != nil {
		return err
	}
	if present > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE artifacts ADD COLUMN %s %s`, column, definition))
	return err
}
