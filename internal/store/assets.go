package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrAssetLimit reports that storing an asset would give its artifact more
// files or more bytes than the limits passed to PutAsset allow.
var ErrAssetLimit = errors.New("asset limit reached")

// Asset is the record of a file an artifact references. The bytes are not in
// the database: Blob is the key of the file on the data volume.
type Asset struct {
	ArtifactID  string    `json:"-"`
	Name        string    `json:"name"`
	Blob        string    `json:"-"`
	ContentType string    `json:"contentType"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

// AssetLimits bound what one artifact holds.
type AssetLimits struct {
	Files int
	Bytes int64
}

// PutAsset records an asset, replacing the one with the same name. The limits
// are checked against what the artifact holds after the replacement, in the
// same transaction, so two uploads cannot both squeeze under a limit. It
// returns the record that was replaced, if any, so the caller can delete the
// file it pointed to.
func (s *Store) PutAsset(ctx context.Context, a Asset, limits AssetLimits) (replaced *Asset, err error) {
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		old, err := queryAsset(ctx, tx, a.ArtifactID, a.Name)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if err == nil {
			replaced = &old
		}
		var files int
		var bytes int64
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(size), 0) FROM assets WHERE artifact_id = ? AND name <> ?`, a.ArtifactID, a.Name).Scan(&files, &bytes); err != nil {
			return err
		}
		if files+1 > limits.Files || bytes+a.Size > limits.Bytes {
			return ErrAssetLimit
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO assets (artifact_id, name, blob, content_type, size, sha256, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (artifact_id, name) DO UPDATE SET
				blob = excluded.blob, content_type = excluded.content_type, size = excluded.size,
				sha256 = excluded.sha256, created_by = excluded.created_by, created_at = excluded.created_at`,
			a.ArtifactID, a.Name, a.Blob, a.ContentType, a.Size, a.SHA256, a.CreatedBy, a.CreatedAt.Unix())
		return err
	})
	if err != nil {
		return nil, err
	}
	return replaced, nil
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func queryAsset(ctx context.Context, q rowQuerier, artifactID, name string) (Asset, error) {
	a := Asset{ArtifactID: artifactID, Name: name}
	var created int64
	err := q.QueryRowContext(ctx, `SELECT blob, content_type, size, sha256, created_by, created_at FROM assets WHERE artifact_id = ? AND name = ?`, artifactID, name).
		Scan(&a.Blob, &a.ContentType, &a.Size, &a.SHA256, &a.CreatedBy, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Asset{}, ErrNotFound
	}
	if err != nil {
		return Asset{}, err
	}
	a.CreatedAt = time.Unix(created, 0).UTC()
	return a, nil
}

// Asset returns one asset's record.
func (s *Store) Asset(ctx context.Context, artifactID, name string) (Asset, error) {
	return queryAsset(ctx, s.db, artifactID, name)
}

// Assets lists an artifact's assets by name.
func (s *Store) Assets(ctx context.Context, artifactID string) ([]Asset, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, blob, content_type, size, sha256, created_by, created_at FROM assets WHERE artifact_id = ? ORDER BY name`, artifactID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := []Asset{}
	for rows.Next() {
		a := Asset{ArtifactID: artifactID}
		var created int64
		if err := rows.Scan(&a.Name, &a.Blob, &a.ContentType, &a.Size, &a.SHA256, &a.CreatedBy, &created); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(created, 0).UTC()
		list = append(list, a)
	}
	return list, rows.Err()
}

// DeleteAsset removes an asset's record and returns it, so the caller can
// delete the file.
func (s *Store) DeleteAsset(ctx context.Context, artifactID, name string) (Asset, error) {
	var deleted Asset
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		a, err := queryAsset(ctx, tx, artifactID, name)
		if err != nil {
			return err
		}
		deleted = a
		_, err = tx.ExecContext(ctx, `DELETE FROM assets WHERE artifact_id = ? AND name = ?`, artifactID, name)
		return err
	})
	return deleted, err
}
