package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// migrateMarkdownDocs converts every version of every doc artifact from content
// format 1 (Markdown) to format 2 (blocks), one transaction per artifact. The
// Markdown the version had is kept in original_markdown, which nothing serves,
// so a conversion that turns out wrong can be run again from it after a fix;
// the column is dropped before release. A version already in format 2 is left
// alone, so running this on every start changes nothing once it has run.
func migrateMarkdownDocs(ctx context.Context, db *sql.DB) error {
	var present int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('artifact_versions') WHERE name = 'original_markdown'`).Scan(&present); err != nil {
		return err
	}
	if present == 0 {
		if _, err := db.ExecContext(ctx, `ALTER TABLE artifact_versions ADD COLUMN original_markdown TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	ids, err := docsInFormat1(ctx, db)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := migrateMarkdownDoc(ctx, db, id); err != nil {
			return fmt.Errorf("doc %s: %w", id, err)
		}
	}
	return nil
}

func docsInFormat1(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT artifact_id FROM artifact_versions WHERE kind = 'doc' AND format = '1' ORDER BY artifact_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type versionRow struct {
	number          int
	format, content string
}

func migrateMarkdownDoc(ctx context.Context, db *sql.DB, id string) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	rows, err := tx.QueryContext(ctx, `SELECT number, format, content FROM artifact_versions WHERE artifact_id = ? ORDER BY number`, id)
	if err != nil {
		return err
	}
	var versions []versionRow
	for rows.Next() {
		var v versionRow
		if err = rows.Scan(&v.number, &v.format, &v.content); err != nil {
			return errors.Join(err, rows.Close())
		}
		versions = append(versions, v)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}

	var previous *docfmt.Doc // the version before, so ids hold through the history
	newest := ""
	for _, v := range versions {
		content := v.content
		if v.format == "1" {
			doc, cerr := docfmt.FromLegacyMarkdown(v.content, previous, docfmt.NewID)
			if cerr != nil {
				return fmt.Errorf("version %d: %w", v.number, cerr)
			}
			if content, err = docfmt.Canonical(doc); err != nil {
				return fmt.Errorf("version %d: %w", v.number, err)
			}
			if _, err = tx.ExecContext(ctx, `UPDATE artifact_versions SET content = ?, format = '2', original_markdown = ? WHERE artifact_id = ? AND number = ?`, content, v.content, id, v.number); err != nil {
				return err
			}
		}
		if d, perr := docfmt.Parse(content); perr == nil {
			previous = &d
		}
		newest = content
	}
	// The artifact row holds the newest version's content.
	_, err = tx.ExecContext(ctx, `UPDATE artifacts SET html = ?, format = '2' WHERE id = ? AND format = '1'`, newest, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
