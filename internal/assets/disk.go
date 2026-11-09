package assets

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Disk keeps asset bytes in a directory on the data volume, one subdirectory
// per artifact and one file per stored asset. File names are random keys the
// database hands out, never anything an uploader chose, so no name can reach
// outside the directory.
type Disk struct{ dir string }

// OpenDisk uses dir, creating it when it is missing.
func OpenDisk(dir string) (*Disk, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create assets directory: %w", err)
	}
	return &Disk{dir: dir}, nil
}

// NewKey returns a fresh random file key.
func NewKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("make a file key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// path joins an artifact's directory and a key, refusing anything that is not
// a plain name: the callers pass ids and keys the server made, and this is the
// second lock on that.
func (d *Disk) path(artifactID, key string) (string, error) {
	for _, part := range []string{artifactID, key} {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, `/\`+"\x00") {
			return "", fmt.Errorf("%q is not a valid asset location", part)
		}
	}
	return filepath.Join(d.dir, artifactID, key), nil
}

// Write stores data under key. The file appears whole or not at all.
func (d *Disk) Write(artifactID, key string, data []byte) error {
	p, err := d.path(artifactID, key)
	if err != nil {
		return err
	}
	return writeAtomic(p, data)
}

// writeAtomic writes data to p so that the file appears whole or not at all.
func writeAtomic(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("create asset directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return fmt.Errorf("create asset file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return errors.Join(fmt.Errorf("write asset file: %w", err), tmp.Close(), os.Remove(tmp.Name()))
	}
	if err := tmp.Close(); err != nil {
		return errors.Join(fmt.Errorf("close asset file: %w", err), os.Remove(tmp.Name()))
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return errors.Join(fmt.Errorf("place asset file: %w", err), os.Remove(tmp.Name()))
	}
	return nil
}

// Open opens a stored file for reading.
func (d *Disk) Open(artifactID, key string) (Readable, error) {
	p, err := d.path(artifactID, key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p) //nolint:gosec // G304: p comes from path(), which accepts only plain names (TestADiskKeepsEachArtifactsFilesApartAndRefusesAnyPathOutsideIt)
	if err != nil {
		return nil, fmt.Errorf("open asset file: %w", err)
	}
	return f, nil
}

// Remove deletes one stored file; one that is already gone is not an error.
func (d *Disk) Remove(artifactID, key string) error {
	p, err := d.path(artifactID, key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove asset file: %w", err)
	}
	return nil
}

// RemoveArtifact deletes every file of an artifact.
func (d *Disk) RemoveArtifact(artifactID string) error {
	p, err := d.path(artifactID, "x")
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Dir(p)); err != nil {
		return fmt.Errorf("remove asset directory of artifact %s: %w", artifactID, err)
	}
	return nil
}

// Blobs are the bytes of the files that come with a version of an artifact
// (internal/files). Unlike assets they are named by the SHA-256 of their
// content, so the same content in many versions is stored once, writing one
// again changes nothing, and a version's manifest names exactly the bytes it
// was published with. They live in a blobs directory inside the artifact's own,
// so deleting the artifact deletes them.

const blobsDir = "blobs"

// blobPath refuses anything that is not a SHA-256 in lower-case hex, so no
// caller can name a path outside the blobs directory.
func (d *Disk) blobPath(artifactID, sha string) (string, error) {
	if _, err := d.path(artifactID, "x"); err != nil {
		return "", err
	}
	if len(sha) != sha256.Size*2 {
		return "", fmt.Errorf("%q is not a blob hash", sha)
	}
	for _, c := range sha {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", fmt.Errorf("%q is not a blob hash", sha)
		}
	}
	return filepath.Join(d.dir, artifactID, blobsDir, sha), nil
}

// WriteBlob stores data, which must hash to sha. Storing what is already there
// is a no-op, so a publish can be retried.
func (d *Disk) WriteBlob(artifactID, sha string, data []byte) error {
	p, err := d.blobPath(artifactID, sha)
	if err != nil {
		return err
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != sha {
		return fmt.Errorf("blob content does not match its hash %s", sha)
	}
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	return writeAtomic(p, data)
}

// OpenBlob opens a blob for reading. A blob that is missing is an error, never
// an empty file.
func (d *Disk) OpenBlob(artifactID, sha string) (Readable, error) {
	p, err := d.blobPath(artifactID, sha)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p) //nolint:gosec // G304: p comes from blobPath(), which accepts only a SHA-256 (TestABlobIsNamedByTheHashOfItsContent)
	if err != nil {
		return nil, fmt.Errorf("open blob: %w", err)
	}
	return f, nil
}

// ReadBlob reads a whole blob.
func (d *Disk) ReadBlob(artifactID, sha string) ([]byte, error) {
	f, err := d.OpenBlob(artifactID, sha)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("read blob: %w", err), f.Close())
	}
	return data, f.Close()
}

// SweepBlobs deletes the blobs that no manifest names (refs maps each artifact
// to the hashes still in use) and that were written before olderThan. The age
// keeps it from deleting a blob a publish has written and not yet recorded.
// It returns how many it deleted.
func (d *Disk) SweepBlobs(refs map[string]map[string]struct{}, olderThan time.Time) (int, error) {
	artifacts, err := os.ReadDir(d.dir)
	if err != nil {
		return 0, fmt.Errorf("list assets directory: %w", err)
	}
	removed := 0
	var errs []error
	for _, a := range artifacts {
		if !a.IsDir() {
			continue
		}
		blobs, err := os.ReadDir(filepath.Join(d.dir, a.Name(), blobsDir))
		if err != nil {
			continue // an artifact without blobs
		}
		for _, b := range blobs {
			info, err := b.Info()
			if err != nil || b.IsDir() || info.ModTime().After(olderThan) {
				continue
			}
			if _, used := refs[a.Name()][b.Name()]; used {
				continue
			}
			if err := os.Remove(filepath.Join(d.dir, a.Name(), blobsDir, b.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
				continue
			}
			removed++
		}
	}
	return removed, errors.Join(errs...)
}

// DirsWithPrefix lists the directories of the assets directory whose name starts
// with prefix and that were last changed before olderThan: the places the
// service keeps bytes that belong to no artifact yet.
func (d *Disk) DirsWithPrefix(prefix string, olderThan time.Time) ([]string, error) {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return nil, fmt.Errorf("list assets directory: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		if info, err := e.Info(); err == nil && !info.ModTime().After(olderThan) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
