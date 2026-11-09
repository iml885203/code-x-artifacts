package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func ref(path, sha string) FileRef {
	return FileRef{Path: path, SHA256: sha, ContentType: "text/javascript; charset=utf-8", Size: 3}
}

func TestAVersionKeepsItsFilesAndTheNextVersionHasItsOwn(t *testing.T) {
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()

	a, err := st.CreateArtifactFrom(ctx, NewArtifact{Title: "t", Kind: "html", Content: "<p>x", Author: "alice", Visibility: VisibilityPrivate, Files: []FileRef{ref("app.js", "aa"), ref("lib/b.js", "bb")}})
	if err != nil {
		t.Fatal(err)
	}
	rev := Revision{Title: "t", Content: "<p>y", Author: "alice", Files: []FileRef{ref("app.js", "cc")}, FilesFrom: 1}
	if err := st.UpdateArtifact(ctx, a.ID, 1, rev, ""); err != nil {
		t.Fatal(err)
	}
	v1, err := st.VersionFiles(ctx, a.ID, 1)
	if err != nil || len(v1) != 2 || v1[0].Path != "app.js" || v1[0].SHA256 != "aa" || v1[1].Path != "lib/b.js" {
		t.Errorf("version 1 files = %+v, %v; a new version must not change them", v1, err)
	}
	v2, err := st.VersionFiles(ctx, a.ID, 2)
	if err != nil || len(v2) != 1 || v2[0].SHA256 != "cc" {
		t.Errorf("version 2 files = %+v, %v", v2, err)
	}
	list, err := st.Versions(ctx, a.ID)
	if err != nil || list[0].FileCount != 1 || list[1].FileCount != 2 {
		t.Errorf("file counts in the history = %+v, %v", list, err)
	}
}

func TestAFailedSaveLeavesNoVersionAndNoManifestRows(t *testing.T) {
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()

	a, err := st.CreateArtifactFrom(ctx, NewArtifact{Title: "t", Kind: "html", Content: "<p>x", Author: "alice", Visibility: VisibilityPrivate, Files: []FileRef{ref("app.js", "aa")}})
	if err != nil {
		t.Fatal(err)
	}
	// Two rows for one path break the manifest's key half way through the save.
	bad := Revision{Title: "t", Content: "<p>y", Author: "alice", Files: []FileRef{ref("a.js", "11"), ref("a.js", "22")}}
	if err := st.UpdateArtifact(ctx, a.ID, 1, bad, ""); err == nil {
		t.Fatal("saved a manifest that names a path twice")
	}
	if got, _ := st.Artifact(ctx, a.ID); got.Version != 1 || got.Content != "<p>x" {
		t.Errorf("the artifact after a failed save = %+v", got)
	}
	if files, _ := st.VersionFiles(ctx, a.ID, 2); len(files) != 0 {
		t.Errorf("a failed save left manifest rows: %+v", files)
	}
	if refs, _ := st.BlobRefs(ctx); len(refs[a.ID]) != 1 {
		t.Errorf("referenced blobs = %v, want only version 1's", refs)
	}
}

func TestAStaleFilesBaseIsRefusedEvenWithoutABaseVersion(t *testing.T) {
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()

	a, err := st.CreateArtifactFrom(ctx, NewArtifact{Title: "t", Kind: "html", Content: "<p>x", Author: "alice", Visibility: VisibilityPrivate, Files: []FileRef{ref("app.js", "aa")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateArtifact(ctx, a.ID, 0, Revision{Title: "t", Content: "2", Author: "bob"}, ""); err != nil {
		t.Fatal(err)
	}
	// Built on version 1's files while version 2 is the newest: carrying them
	// over would silently undo whatever version 2 did to the files.
	rev := Revision{Title: "t", Content: "3", Author: "alice", Files: []FileRef{ref("app.js", "aa")}, FilesFrom: 1}
	if err := st.UpdateArtifact(ctx, a.ID, 0, rev, ""); !errors.Is(err, ErrStale) {
		t.Errorf("err = %v, want ErrStale", err)
	}
}

func TestDeletingAnArtifactDeletesItsManifest(t *testing.T) {
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()

	a, err := st.CreateArtifactFrom(ctx, NewArtifact{Title: "t", Kind: "html", Content: "<p>x", Author: "alice", Visibility: VisibilityPrivate, Files: []FileRef{ref("app.js", "aa")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteArtifact(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if refs, err := st.BlobRefs(ctx); err != nil || len(refs) != 0 {
		t.Errorf("blobs still referenced after the artifact was deleted: %v, %v", refs, err)
	}
}
