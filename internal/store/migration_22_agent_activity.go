package store

import (
	"context"
	"database/sql"
)

func migration22AgentActivity(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
 CREATE INDEX IF NOT EXISTS tokens_owner_name ON tokens(username,name,id);
 CREATE TABLE IF NOT EXISTS agent_activity (
 id TEXT PRIMARY KEY, owner TEXT NOT NULL, token_id TEXT NOT NULL, token_label TEXT NOT NULL,
 transport TEXT NOT NULL, client TEXT NOT NULL, client_version TEXT NOT NULL,
 occurred_at INTEGER NOT NULL, operation TEXT NOT NULL, artifact_id TEXT NOT NULL DEFAULT '', version INTEGER NOT NULL DEFAULT 0,
 connection_bucket INTEGER
 );
 CREATE INDEX IF NOT EXISTS activity_owner_time ON agent_activity(owner,occurred_at DESC,id DESC);
 CREATE INDEX IF NOT EXISTS activity_token_time ON agent_activity(owner,token_id,occurred_at DESC,id DESC);
 CREATE UNIQUE INDEX IF NOT EXISTS activity_connection ON agent_activity(owner,token_id,transport,client,client_version,connection_bucket) WHERE connection_bucket IS NOT NULL;
 CREATE TABLE IF NOT EXISTS agent_token_usage (
 owner TEXT NOT NULL, token_id TEXT NOT NULL, token_label TEXT NOT NULL,
 last_used INTEGER NOT NULL, last_connected INTEGER,
 PRIMARY KEY(owner,token_id)
 );
 CREATE INDEX IF NOT EXISTS usage_owner_time ON agent_token_usage(owner,last_used DESC,token_id);
 CREATE INDEX IF NOT EXISTS activity_expiry ON agent_activity(occurred_at);
 CREATE INDEX IF NOT EXISTS usage_expiry ON agent_token_usage(last_used);
 `)
	if err != nil {
		return err
	}
	return tx.Commit()
}
