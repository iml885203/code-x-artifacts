package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/dbquery"
)

// DocOp kinds.
const (
	OpSet    = "set"
	OpUpdate = "update"
	OpDelete = "delete"
)

// DocOp is one write to a page's shared documents.
type DocOp struct {
	// Kind is OpSet (create or replace), OpUpdate (merge fields into an
	// existing object) or OpDelete.
	Kind       string
	Collection string
	ID         string
	// Data is the whole document for a set and the fields to merge for an update.
	Data json.RawMessage
	// IfVersion, when set, is the version the write was built on: 0 means the
	// document must not exist yet.
	IfVersion *int
}

// DocFailure says why a set of writes was refused, and which write it was.
// Nothing has been changed when one is returned.
type DocFailure struct {
	// Index is the position of the failing write in the list that was sent.
	Index int
	Op    DocOp
	Why   DocWhy
	// Current is the document as it is now; nil when there is none.
	Current *Doc
	// Err explains a DocUnmergeable.
	Err error
}

// DocWhy classifies a DocFailure.
type DocWhy int

const (
	// DocStale means the write named a version that is not the document's current one.
	DocStale DocWhy = iota + 1
	// DocMissing means an update or delete of a document that does not exist.
	DocMissing
	// DocUnmergeable means an update of a document that is not an object, or with fields that are not.
	DocUnmergeable
)

func (f *DocFailure) Error() string {
	return fmt.Sprintf("write %d (%s %s/%s) refused", f.Index+1, f.Op.Kind, f.Op.Collection, f.Op.ID)
}

// WriteDocs applies the writes in order in one transaction, recording `by` as
// the writer. Either every write is applied or none is: the first one that
// fails (stale, missing, not mergeable) rolls the rest back and is returned as
// a *DocFailure. Each write sees the ones before it. The result has one entry
// per write: the document as written, or for a delete a Doc with only the ID.
func (s *Store) WriteDocs(ctx context.Context, artifactID, by string, ops []DocOp) ([]Doc, error) {
	out := make([]Doc, 0, len(ops))
	at := time.Now().UTC()
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		for i, op := range ops {
			d, err := applyDocOp(ctx, tx, artifactID, by, at, i, op)
			if err != nil {
				return err
			}
			out = append(out, d)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// applyDocOp does one write of a WriteDocs inside its transaction.
func applyDocOp(ctx context.Context, tx *sql.Tx, artifactID, by string, at time.Time, index int, op DocOp) (Doc, error) {
	cur, err := docIn(ctx, tx, artifactID, op.Collection, op.ID)
	if err != nil {
		return Doc{}, err
	}
	fail := func(why DocWhy, e error) error {
		return &DocFailure{Index: index, Op: op, Why: why, Current: cur, Err: e}
	}
	have := 0
	if cur != nil {
		have = cur.Version
	}
	if op.IfVersion != nil && have != *op.IfVersion {
		return Doc{}, fail(DocStale, nil)
	}
	next := Doc{ID: op.ID, UpdatedBy: by, UpdatedAt: time.Unix(at.Unix(), 0).UTC(), Version: have + 1}
	switch op.Kind {
	case OpSet:
		next.Data = op.Data
	case OpUpdate:
		if cur == nil {
			return Doc{}, fail(DocMissing, nil)
		}
		if next.Data, err = dbquery.Merge(cur.Data, op.Data); err != nil {
			return Doc{}, fail(DocUnmergeable, err)
		}
	case OpDelete:
		if cur == nil {
			return Doc{}, fail(DocMissing, nil)
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM docs WHERE artifact_id = ? AND collection = ? AND doc_id = ?`, artifactID, op.Collection, op.ID)
		return Doc{ID: op.ID}, err
	default:
		return Doc{}, fmt.Errorf("unknown document write %q", op.Kind)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO docs (artifact_id, collection, doc_id, data, updated_by, updated_at, version) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (artifact_id, collection, doc_id) DO UPDATE SET
			data = excluded.data, updated_by = excluded.updated_by, updated_at = excluded.updated_at, version = excluded.version`,
		artifactID, op.Collection, op.ID, string(next.Data), by, next.UpdatedAt.Unix(), next.Version)
	return next, err
}

// PutDoc creates or replaces one shared document, unconditionally.
func (s *Store) PutDoc(ctx context.Context, artifactID, collection, docID string, data json.RawMessage, by string) error {
	_, err := s.WriteDocs(ctx, artifactID, by, []DocOp{{Kind: OpSet, Collection: collection, ID: docID, Data: data}})
	return err
}

// docIn reads a document inside the transaction; a nil Doc means there is none.
func docIn(ctx context.Context, tx *sql.Tx, artifactID, collection, docID string) (*Doc, error) {
	d := Doc{ID: docID}
	var data string
	var updated int64
	err := tx.QueryRowContext(ctx, `SELECT data, updated_by, updated_at, version FROM docs WHERE artifact_id = ? AND collection = ? AND doc_id = ?`,
		artifactID, collection, docID).Scan(&data, &d.UpdatedBy, &updated, &d.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // no row is an answer, not an error
	}
	if err != nil {
		return nil, err
	}
	d.Data, d.UpdatedAt = json.RawMessage(data), time.Unix(updated, 0).UTC()
	return &d, nil
}
