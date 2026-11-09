package service

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// CopySuffix is what a copy's title gets after the source's.
const CopySuffix = " (Copy)"

// Copy makes the viewer's own artifact from one they can see: a new id, the
// viewer as author, private, version 1, the same kind, format and content and
// the artifact's uploaded files, with the file URLs in the content pointing at
// the copy's own files. The shared db, comments, editors and history stay with
// the source. version 0 copies the newest; another number copies that version's
// content, which only people who may read history (CanEdit) can ask for.
//
// The uploaded files (assets) of the artifact are copied as they are now; they
// do not belong to a version. The version's own files (script, style, data)
// are copied with it, each with the asset URLs in it pointing at the copy.
func (s *Service) Copy(ctx context.Context, v Viewer, id string, version int) (store.Artifact, error) {
	if v.Anonymous() {
		return store.Artifact{}, refuse(Forbidden, "sign in to copy")
	}
	src, err := s.Artifact(ctx, v, id)
	if err != nil {
		return store.Artifact{}, err
	}
	if !v.CanCopy(src) {
		return store.Artifact{}, errNotFound
	}
	title, format, content, fileVersion := src.Title, src.Format, src.Content, src.Version
	if version != 0 && version != src.Version {
		old, err := s.Version(ctx, v, id, version)
		if err != nil {
			return store.Artifact{}, err
		}
		title, format, content, fileVersion = old.Title, old.Format, old.Content, version
	}
	newID := store.NewArtifactID()
	in, err := s.validate(PublishInput{
		Title: title + CopySuffix, Kind: src.Kind, Format: format,
		Content: strings.ReplaceAll(content, assets.PathPrefix(src.ID), assets.PathPrefix(newID)),
		stored:  true,
	}, store.Artifact{})
	if err != nil {
		return store.Artifact{}, fmt.Errorf("copy artifact %s: %w", src.ID, err)
	}
	files, err := s.store.Assets(ctx, src.ID)
	if err != nil {
		return store.Artifact{}, fmt.Errorf("list assets of artifact %s: %w", src.ID, err)
	}
	k, err := kind.Lookup(in.Kind)
	if err != nil {
		return store.Artifact{}, refuse(Invalid, "%s", err)
	}
	manifest, set, err := s.copyVersionFiles(ctx, k, src.ID, fileVersion, newID)
	if err != nil {
		s.removeHalfMade(ctx, newID)
		return store.Artifact{}, err
	}
	if err := validateContent(k, in.Content, in.Format, set); err != nil {
		s.removeHalfMade(ctx, newID)
		return store.Artifact{}, fmt.Errorf("copy artifact %s: %w", src.ID, refuse(Invalid, "%s", err))
	}
	copied, err := s.store.CreateCopy(ctx, store.CopyInput{
		ID: newID, Title: in.Title, Kind: in.Kind, Format: in.Format, Content: in.Content,
		Author: v.Username, Source: store.Source{ID: src.ID, Title: src.Title}, Files: manifest,
	})
	if err != nil {
		s.removeHalfMade(ctx, newID)
		return store.Artifact{}, fmt.Errorf("create copy of artifact %s: %w", src.ID, err)
	}
	if err := s.copyAssets(ctx, v, src.ID, copied.ID, files); err != nil {
		// A copy without its files would show broken pictures, so none is made.
		if derr := s.store.DeleteArtifact(ctx, copied.ID); derr != nil {
			report(ctx, Failure, "could not remove a half-made copy", "id", copied.ID, "err", derr)
		}
		s.removeHalfMade(ctx, copied.ID)
		return store.Artifact{}, err
	}
	report(ctx, Information, "artifact copied", "id", copied.ID, "from", src.ID, "version", version, "user", v.Username, "files", len(files))
	return s.reload(ctx, v, copied.ID)
}

// removeHalfMade deletes the bytes stored for a copy that was not made.
func (s *Service) removeHalfMade(ctx context.Context, id string) {
	if err := s.files.RemoveArtifact(id); err != nil {
		report(ctx, Failure, "could not remove the files of a half-made copy", "id", id, "err", err)
	}
}

// copyVersionFiles stores the files of one version of the source under the
// copy's own id and returns the copy's manifest. A file that can hold the URL
// of an uploaded asset gets the copy's URL in place of the source's, and so a
// hash of its own; one that cannot (JSON) is copied as it is.
func (s *Service) copyVersionFiles(ctx context.Context, k kind.Kind, from string, version int, to string) ([]store.FileRef, files.Set, error) {
	manifest, err := s.store.VersionFiles(ctx, from, version)
	if err != nil {
		return nil, nil, fmt.Errorf("list files of artifact %s: %w", from, err)
	}
	_, judged := k.(kind.FileValidator)
	var set files.Set
	if judged && len(manifest) > 0 {
		set = make(files.Set, len(manifest))
	}
	out := make([]store.FileRef, 0, len(manifest))
	for _, f := range manifest {
		data, err := s.files.ReadBlob(from, f.SHA256)
		if err != nil {
			return nil, nil, fmt.Errorf("file %s of artifact %s is recorded but its bytes are unreadable: %w", f.Path, from, err)
		}
		if files.Rewritable(f.Path) {
			data = []byte(strings.ReplaceAll(string(data), assets.PathPrefix(from), assets.PathPrefix(to)))
		}
		f.SHA256, f.Size = sha256Hex(data), int64(len(data))
		if err := s.files.WriteBlob(to, f.SHA256, data); err != nil {
			return nil, nil, fmt.Errorf("copy file %s of artifact %s: %w", f.Path, from, err)
		}
		if set != nil {
			set[f.Path] = string(data)
		}
		out = append(out, f)
	}
	return out, set, nil
}

// copyAssets gives the copy its own file for each of the source's: a new key and
// a new record, never a shared file, so deleting or replacing one in either
// artifact cannot touch the other.
func (s *Service) copyAssets(ctx context.Context, v Viewer, from, to string, files []store.Asset) error {
	for _, rec := range files {
		data, err := s.readAsset(ctx, from, rec)
		if err != nil {
			return err
		}
		key, err := assets.NewKey()
		if err != nil {
			return err
		}
		if err := s.files.Write(to, key, data); err != nil {
			return fmt.Errorf("copy asset %s of artifact %s: %w", rec.Name, from, err)
		}
		rec.ArtifactID, rec.Blob, rec.CreatedBy, rec.CreatedAt = to, key, v.Username, nowUTC()
		if _, err := s.store.PutAsset(ctx, rec, store.AssetLimits{Files: assets.MaxFiles, Bytes: assets.MaxTotalBytes}); err != nil {
			return fmt.Errorf("record copied asset %s of artifact %s: %w", rec.Name, to, err)
		}
	}
	return nil
}

func (s *Service) readAsset(ctx context.Context, artifactID string, rec store.Asset) ([]byte, error) {
	f, err := s.files.Open(artifactID, rec.Blob)
	if err != nil {
		return nil, fmt.Errorf("asset %s of artifact %s is recorded but its file is unreadable: %w", rec.Name, artifactID, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			report(ctx, Failure, "close asset file", "id", artifactID, "name", rec.Name, "err", err)
		}
	}()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read asset %s of artifact %s: %w", rec.Name, artifactID, err)
	}
	return data, nil
}
