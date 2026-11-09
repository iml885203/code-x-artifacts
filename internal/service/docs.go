package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/dbquery"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// MaxBatchOps is the most writes one batch holds, and MaxBatchBytes the most
// document data they carry together.
const (
	MaxBatchOps   = 100
	MaxBatchBytes = 4 << 20
)

// DocConflict is what a refused conditional write carries, so the caller can
// merge and retry without asking again.
type DocConflict struct {
	// Index is which write of a batch was stale; 0 for a single write.
	Index      int
	Collection string
	ID         string
	// Current is the document as it is now; nil when it does not exist.
	Current *store.Doc
}

// QueryDocs lists the documents of one collection that match the filter, in
// the order List gives them (oldest write first). limit 0 means all.
func (s *Service) QueryDocs(ctx context.Context, v Viewer, id, collection string, filter json.RawMessage, limit int) ([]store.Doc, error) {
	f, err := dbquery.Compile(filter)
	if err != nil {
		if _, e := s.Artifact(ctx, v, id); e != nil {
			return nil, e
		}
		return nil, refuse(Invalid, "%s", err.Error())
	}
	if limit < 0 {
		return nil, refuse(Invalid, "limit must be 0 (all) or more")
	}
	docs, err := s.Docs(ctx, v, id, collection)
	if err != nil {
		return nil, err
	}
	out := make([]store.Doc, 0, len(docs))
	for _, d := range docs {
		if limit > 0 && len(out) == limit {
			break
		}
		if f.Match(d.ID, d.Data) {
			out = append(out, d)
		}
	}
	return out, nil
}

// PutDoc creates or replaces a shared document and records the viewer as its
// writer. With ifVersion it is refused unless the document is at that version
// (0: unless it does not exist).
func (s *Service) PutDoc(ctx context.Context, v Viewer, id, collection, doc string, data json.RawMessage, ifVersion *int) (store.Doc, error) {
	return s.writeOne(ctx, v, id, store.DocOp{Kind: store.OpSet, Collection: collection, ID: doc, Data: data, IfVersion: ifVersion})
}

// UpdateDoc merges fields into an existing document (null removes a field).
func (s *Service) UpdateDoc(ctx context.Context, v Viewer, id, collection, doc string, fields json.RawMessage, ifVersion *int) (store.Doc, error) {
	return s.writeOne(ctx, v, id, store.DocOp{Kind: store.OpUpdate, Collection: collection, ID: doc, Data: fields, IfVersion: ifVersion})
}

// DeleteDoc removes one shared document.
func (s *Service) DeleteDoc(ctx context.Context, v Viewer, id, collection, doc string, ifVersion *int) error {
	_, err := s.writeOne(ctx, v, id, store.DocOp{Kind: store.OpDelete, Collection: collection, ID: doc, IfVersion: ifVersion})
	return err
}

// BatchDocs applies the writes together: every one or none. The result has an
// entry per write, in order (a delete's has only the id).
func (s *Service) BatchDocs(ctx context.Context, v Viewer, id string, ops []store.DocOp) ([]store.Doc, error) {
	return s.write(ctx, v, id, ops, true)
}

func (s *Service) writeOne(ctx context.Context, v Viewer, id string, op store.DocOp) (store.Doc, error) {
	docs, err := s.write(ctx, v, id, []store.DocOp{op}, false)
	if err != nil {
		return store.Doc{}, err
	}
	return docs[0], nil
}

func (s *Service) write(ctx context.Context, v Viewer, id string, ops []store.DocOp, batch bool) ([]store.Doc, error) {
	if err := s.writable(ctx, v, id); err != nil {
		return nil, err
	}
	if err := checkOps(ops, batch); err != nil {
		return nil, err
	}
	docs, err := s.store.WriteDocs(ctx, id, v.Username, ops)
	if f, ok := errors.AsType[*store.DocFailure](err); ok {
		return nil, s.refusal(v, f, batch)
	}
	if err != nil {
		return nil, fmt.Errorf("write documents of artifact %s: %w", id, err)
	}
	return docs, nil
}

func where(i int, batch bool) string {
	if !batch {
		return ""
	}
	return fmt.Sprintf("operation %d: ", i+1)
}

func checkOps(ops []store.DocOp, batch bool) error {
	if len(ops) == 0 || len(ops) > MaxBatchOps {
		return refuse(Invalid, "a batch holds 1 to %d operations", MaxBatchOps)
	}
	total := 0
	for i, op := range ops {
		w := where(i, batch)
		switch op.Kind {
		case store.OpSet, store.OpUpdate, store.OpDelete:
		default:
			return refuse(Invalid, "%sop must be \"set\", \"update\" or \"delete\"", w)
		}
		if op.Collection == "" || op.ID == "" {
			return refuse(Invalid, "%sname the collection and the document id", w)
		}
		if op.IfVersion != nil && *op.IfVersion < 0 {
			return refuse(Invalid, "%sifVersion is a version number (0 means the document must not exist yet)", w)
		}
		if op.Kind == store.OpDelete {
			continue
		}
		if len(op.Data) > MaxDocBytes || !json.Valid(op.Data) {
			return refuse(Invalid, "%sa document must be JSON under %d MB", w, MaxDocBytes>>20)
		}
		if op.Kind == store.OpUpdate {
			if _, err := dbquery.Merge([]byte(`{}`), op.Data); err != nil {
				return refuse(Invalid, "%s%s", w, err.Error())
			}
		}
		if total += len(op.Data); total > MaxBatchBytes {
			return refuse(Invalid, "a batch carries at most %d MB of data", MaxBatchBytes>>20)
		}
	}
	return nil
}

// refusal turns what the store reports into what the caller is told.
func (s *Service) refusal(v Viewer, f *store.DocFailure, batch bool) error {
	w := where(f.Index, batch)
	name := f.Op.Collection + "/" + f.Op.ID
	switch f.Why {
	case store.DocStale:
		var cur *store.Doc
		have := "does not exist"
		if f.Current != nil {
			d := v.Redact(*f.Current)
			cur, have = &d, fmt.Sprintf("is at version %d", d.Version)
		}
		want := fmt.Sprintf("version %d", *f.Op.IfVersion)
		if *f.Op.IfVersion == 0 {
			want = "no existing document"
		}
		msg := fmt.Sprintf("%s%s was written against %s but now %s; nothing was changed", w, name, want, have)
		if batch {
			msg += " by any operation of the batch"
		}
		return &Error{Code: Conflict, Msg: msg, Doc: &DocConflict{Index: f.Index, Collection: f.Op.Collection, ID: f.Op.ID, Current: cur}}
	case store.DocMissing:
		if !batch {
			return errNoDocument
		}
		return refuse(Invalid, "%sthere is no document %s to %s; nothing was changed", w, name, f.Op.Kind)
	case store.DocUnmergeable:
		return refuse(Invalid, "%s%s: %s; nothing was changed", w, name, f.Err)
	}
	return fmt.Errorf("unknown reason %d for refusing %s", f.Why, name)
}
