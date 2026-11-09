package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

var errNoAsset = &Error{Code: NotFound, Msg: "not found", What: "asset"}

// Upload is the result of storing an asset.
type Upload struct {
	store.Asset
	// Replaced is true when a file of that name was already there.
	Replaced bool
}

// UploadAsset stores a file for an artifact the viewer may edit under the name it
// is given after sanitising (the returned Asset says which). The type is
// decided from the bytes, never from what the uploader claims. A file with the
// name of an existing one replaces it.
func (s *Service) UploadAsset(ctx context.Context, v Viewer, id, name string, data []byte) (Upload, error) {
	a, err := s.Edit(ctx, v, id)
	if err != nil {
		return Upload{}, err
	}
	return s.putAsset(ctx, v, a, name, data)
}

func (s *Service) putAsset(ctx context.Context, v Viewer, a store.Artifact, name string, data []byte) (Upload, error) {
	if len(data) > assets.MaxBytes {
		return Upload{}, refuse(Invalid, "this file is %d bytes; a file can be at most %d MB", len(data), assets.MaxBytes>>20)
	}
	clean, err := assets.Name(name)
	if err != nil {
		return Upload{}, refuse(Invalid, "%s", err)
	}
	typ, err := assets.Detect(clean, data)
	if err != nil {
		return Upload{}, refuse(Invalid, "%s: %s", clean, err)
	}
	key, err := assets.NewKey()
	if err != nil {
		return Upload{}, err
	}
	if err := s.files.Write(a.ID, key, data); err != nil {
		return Upload{}, fmt.Errorf("store asset %s of artifact %s: %w", clean, a.ID, err)
	}
	sum := sha256Hex(data)
	rec := store.Asset{
		ArtifactID: a.ID, Name: clean, Blob: key, ContentType: typ.ContentType, Size: int64(len(data)),
		SHA256: sum, CreatedBy: v.Username, CreatedAt: nowUTC(),
	}
	replaced, err := s.store.PutAsset(ctx, rec, store.AssetLimits{Files: assets.MaxFiles, Bytes: assets.MaxTotalBytes})
	if err != nil {
		s.dropFile(ctx, a.ID, key)
		if errors.Is(err, store.ErrAssetLimit) {
			return Upload{}, refuse(Invalid, "this artifact already holds as much as it may: at most %d files and %d MB in total. Delete files that are no longer used, then upload again", assets.MaxFiles, assets.MaxTotalBytes>>20)
		}
		return Upload{}, fmt.Errorf("record asset %s of artifact %s: %w", clean, a.ID, err)
	}
	if replaced != nil {
		s.dropFile(ctx, a.ID, replaced.Blob)
	}
	report(ctx, Information, "asset stored", "id", a.ID, "name", clean, "type", typ.ContentType, "bytes", len(data), "user", v.Username, "replaced", replaced != nil)
	return Upload{Asset: rec, Replaced: replaced != nil}, nil
}

// dropFile deletes a file nothing refers to any more. A failure leaves an
// orphan on the volume that costs space and nothing else, so it is logged and
// the request that caused it is not failed.
func (s *Service) dropFile(ctx context.Context, artifactID, key string) {
	if err := s.files.Remove(artifactID, key); err != nil {
		report(ctx, Failure, "could not delete an unreferenced asset file", "id", artifactID, "key", key, "err", err)
	}
}

// Assets lists the files of an artifact the viewer may see.
func (s *Service) Assets(ctx context.Context, v Viewer, id string) ([]store.Asset, error) {
	if _, err := s.Artifact(ctx, v, id); err != nil {
		return nil, err
	}
	list, err := s.store.Assets(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list assets of artifact %s: %w", id, err)
	}
	return list, nil
}

// OpenAsset opens a file of an artifact the viewer may see. Whoever may not
// gets the answer for an artifact that does not exist, and a name that is not
// there gets the same one.
func (s *Service) OpenAsset(ctx context.Context, v Viewer, id, name string) (store.Asset, assets.Readable, error) {
	if _, err := s.Artifact(ctx, v, id); err != nil {
		return store.Asset{}, nil, err
	}
	rec, err := s.store.Asset(ctx, id, name)
	if errors.Is(err, store.ErrNotFound) {
		return store.Asset{}, nil, errNoAsset
	}
	if err != nil {
		return store.Asset{}, nil, fmt.Errorf("read asset %s of artifact %s: %w", name, id, err)
	}
	f, err := s.files.Open(id, rec.Blob)
	if err != nil {
		return store.Asset{}, nil, fmt.Errorf("asset %s of artifact %s is recorded but its file is unreadable: %w", name, id, err)
	}
	return rec, f, nil
}

// DeleteAsset removes one file of an artifact the viewer may edit.
func (s *Service) DeleteAsset(ctx context.Context, v Viewer, id, name string) error {
	a, err := s.Edit(ctx, v, id)
	if err != nil {
		return err
	}
	rec, err := s.store.DeleteAsset(ctx, a.ID, name)
	if errors.Is(err, store.ErrNotFound) {
		return errNoAsset
	}
	if err != nil {
		return fmt.Errorf("delete asset %s of artifact %s: %w", name, a.ID, err)
	}
	s.dropFile(ctx, a.ID, rec.Blob)
	report(ctx, Information, "asset deleted", "id", a.ID, "name", name, "user", v.Username)
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func nowUTC() time.Time { return time.Now().UTC().Truncate(time.Second) }
