package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sum(data string) string {
	s := sha256.Sum256([]byte(data))
	return hex.EncodeToString(s[:])
}

func TestTheSameContentInManyVersionsIsStoredOnce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d, err := OpenDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := d.WriteBlob("art1", sum("console.log(1)"), []byte("console.log(1)")); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dir, "art1", "blobs"))
	if err != nil || len(entries) != 1 {
		t.Errorf("blobs after three writes of one content = %d entries, %v", len(entries), err)
	}
	got, err := d.ReadBlob("art1", sum("console.log(1)"))
	if err != nil || string(got) != "console.log(1)" {
		t.Errorf("read back = %q, %v", got, err)
	}
	// Another artifact keeps its own copy: deleting one artifact is deleting its directory.
	if err := d.WriteBlob("art2", sum("console.log(1)"), []byte("console.log(1)")); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveArtifact("art1"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReadBlob("art2", sum("console.log(1)")); err != nil {
		t.Errorf("another artifact's blob went with the first: %v", err)
	}
}

func TestABlobIsNamedByTheHashOfItsContent(t *testing.T) {
	t.Parallel()
	d, err := OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := d.WriteBlob("art", sum("other"), []byte("content")); err == nil || !strings.Contains(err.Error(), "hash") {
		t.Errorf("a blob stored under a hash that is not its own: %v", err)
	}
	for _, bad := range []string{"", "../x", strings.Repeat("g", 64), sum("x")[:63], sum("x") + "0"} {
		if _, err := d.ReadBlob("art", bad); err == nil {
			t.Errorf("read of blob %q was not refused", bad)
		}
	}
}

func TestAMissingBlobIsAnErrorNotAnEmptyFile(t *testing.T) {
	t.Parallel()
	d, err := OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := d.ReadBlob("art", sum("never stored"))
	if err == nil || data != nil {
		t.Errorf("a missing blob read as %q, %v", data, err)
	}
	f, err := d.OpenBlob("art", sum("never stored"))
	if err == nil {
		_ = f.Close()
		t.Error("a missing blob opened")
	}
}

func TestTheSweepRemovesOnlyOldUnreferencedBlobs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d, err := OpenDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"kept", "old orphan", "fresh orphan"} {
		if err := d.WriteBlob("art", sum(c), []byte(c)); err != nil {
			t.Fatal(err)
		}
	}
	// An asset file next to the blobs directory is not a blob and must survive.
	if err := d.Write("art", "0123456789abcdef0123456789abcdef", []byte("asset")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	for _, c := range []string{"kept", "old orphan"} {
		if err := os.Chtimes(filepath.Join(dir, "art", "blobs", sum(c)), old, old); err != nil {
			t.Fatal(err)
		}
	}
	refs := map[string]map[string]struct{}{"art": {sum("kept"): {}}}
	removed, err := d.SweepBlobs(refs, time.Now().Add(-24*time.Hour))
	if err != nil || removed != 1 {
		t.Fatalf("sweep removed %d, %v; want exactly the old orphan", removed, err)
	}
	for c, want := range map[string]bool{"kept": true, "old orphan": false, "fresh orphan": true} {
		_, err := d.ReadBlob("art", sum(c))
		if (err == nil) != want {
			t.Errorf("blob %q present = %v, want %v", c, err == nil, want)
		}
	}
	f, err := d.Open("art", "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("the sweep removed an asset: %v", err)
	}
	defer func() { _ = f.Close() }()
	if b, _ := io.ReadAll(f); string(b) != "asset" {
		t.Errorf("asset = %q", b)
	}
}

func TestTheSweepRemovesTheBlobsOfAnArtifactThatNoLongerExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d, err := OpenDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.WriteBlob("gone", sum("x"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "gone", "blobs", sum("x")), old, old); err != nil {
		t.Fatal(err)
	}
	if removed, err := d.SweepBlobs(nil, time.Now().Add(-24*time.Hour)); err != nil || removed != 1 {
		t.Errorf("sweep removed %d, %v", removed, err)
	}
}

func TestDirsWithPrefixListsOnlyOldDirectoriesWithThatPrefix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d, err := OpenDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"~old", "~new", "art1"} {
		if err := d.WriteBlob(name, sum("x"), []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	ago := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "~old"), ago, ago); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, "art1"), ago, ago); err != nil {
		t.Fatal(err)
	}
	got, err := d.DirsWithPrefix("~", time.Now().Add(-time.Hour))
	if err != nil || len(got) != 1 || got[0] != "~old" {
		t.Errorf("DirsWithPrefix = %v, %v; want only ~old", got, err)
	}
}
