package store

import (
	"context"
	"database/sql"
)

// The agents that made a version, for the history to say so: a version saved
// by an agent of someone's account is that person's, and names the client too.
// A version cut from a live room has the agents that wrote into the room since
// the one before, which wait in rt_pending_agents until a version holds them.
func migration20VersionAgents(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS version_agents (
			artifact_id TEXT NOT NULL,
			version     INTEGER NOT NULL,
			position    INTEGER NOT NULL,
			username    TEXT NOT NULL,
			client      TEXT NOT NULL,
			PRIMARY KEY (artifact_id, version, username, client),
			FOREIGN KEY (artifact_id, version) REFERENCES artifact_versions(artifact_id, number) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS rt_pending_agents (
			seq      INTEGER PRIMARY KEY AUTOINCREMENT,
			room     TEXT NOT NULL REFERENCES rt_pending(room) ON DELETE CASCADE,
			username TEXT NOT NULL,
			client   TEXT NOT NULL,
			UNIQUE (room, username, client)
		)`)
	return err
}
