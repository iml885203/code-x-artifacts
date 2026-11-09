package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// legacySchema is the artifacts table as it was before visibility existed.
const legacySchema = `
CREATE TABLE artifacts (
	id         TEXT PRIMARY KEY,
	title      TEXT NOT NULL,
	html       TEXT NOT NULL,
	author     TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
INSERT INTO artifacts VALUES ('old', 'Old report', '<p>old', 'alice', 1, 1);
`

func legacyDatabase(ctx context.Context, t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pictor.db")
	legacy, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, legacySchema); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpeningADatabaseFromBeforeVersionsMakesEachArtifactVersionOne(t *testing.T) {
	ctx := t.Context()
	path := legacyDatabase(ctx, t)

	st := openStore(ctx, t, path)
	old, err := st.Artifact(ctx, "old")
	if err != nil || old.Version != 1 {
		t.Errorf("an artifact from before versions: %+v %v, want version 1", old, err)
	}
	if v, err := st.Version(ctx, "old", 1); err != nil || v.Content != "<p>old" || v.Title != "Old report" || v.Kind != "html" || v.Author != "alice" {
		t.Errorf("version 1 of an old artifact: %+v %v", v, err)
	}
	if err := st.UpdateArtifact(ctx, "old", 1, Revision{Title: "Old report", Content: "<p>new", Author: "alice"}, ""); err != nil {
		t.Fatalf("update of a migrated artifact: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	// Starting again must neither add a second version 1 nor lose version 2.
	st = openStore(ctx, t, path)
	defer func() { _ = st.Close() }()
	if vs, err := st.Versions(ctx, "old"); err != nil || len(vs) != 2 || vs[0].Number != 2 || vs[1].Number != 1 {
		t.Errorf("versions after a restart: %+v %v", vs, err)
	}
}

func TestOpeningAnExistingDatabaseKeepsOldArtifactsVisibleToTheOrganisation(t *testing.T) {
	ctx := t.Context()
	path := legacyDatabase(ctx, t)

	// Open twice: the second start must not trip over the migration it already ran.
	for range 2 {
		st := openStore(ctx, t, path)
		old, err := st.Artifact(ctx, "old")
		if err != nil {
			t.Fatal(err)
		}
		if old.Visibility != VisibilityOrganisation {
			t.Errorf("an artifact from before visibility is %q, want organisation", old.Visibility)
		}
		if old.Format != "" {
			t.Errorf("an artifact from before formats is %q, want unversioned", old.Format)
		}
		if old.Kind != "html" {
			t.Errorf("an artifact from before kinds is %q, want html", old.Kind)
		}
		list, err := st.Artifacts(ctx)
		if err != nil || len(list) != 1 || list[0].Visibility != VisibilityOrganisation || list[0].Kind != "html" {
			t.Errorf("list: %+v %v", list, err)
		}
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVisibilityRoundTripsAndRejectsUnknownValues(t *testing.T) {
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()

	a, err := st.CreateArtifact(ctx, "t", "html", "", "<p>x", "alice", VisibilityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSharing(ctx, a.ID, VisibilityOrganisation); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Artifact(ctx, a.ID); got.Visibility != VisibilityOrganisation {
		t.Errorf("got %q", got.Visibility)
	}
	if _, err := st.CreateArtifact(ctx, "t", "html", "", "x", "alice", "everyone"); err == nil {
		t.Error("created an artifact with an unknown visibility")
	}
	if err := st.SetSharing(ctx, a.ID, "everyone"); err == nil {
		t.Error("set an unknown visibility")
	}
	if err := st.UpdateArtifact(ctx, a.ID, 0, Revision{Title: "t", Format: "", Content: "x", Author: "alice"}, "everyone"); err == nil {
		t.Error("updated to an unknown visibility")
	}
}

func TestAnArtifactKeepsTheKindItWasCreatedWithThroughUpdates(t *testing.T) {
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()

	a, err := st.CreateArtifact(ctx, "t", "slides", "v1", "# one", "alice", VisibilityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateArtifact(ctx, a.ID, 0, Revision{Title: "t2", Format: "v1", Content: "# two", Author: "alice"}, ""); err != nil {
		t.Fatal(err)
	}
	got, err := st.Artifact(ctx, a.ID)
	if err != nil || got.Kind != "slides" || got.Format != "v1" || got.Content != "# two" {
		t.Errorf("got %+v, %v", got, err)
	}
	list, err := st.Artifacts(ctx)
	if err != nil || len(list) != 1 || list[0].Kind != "slides" || list[0].Format != "v1" {
		t.Errorf("list: %+v %v", list, err)
	}
}

func TestAnArtifactKeepsItsFormatUntilTheAuthorChangesIt(t *testing.T) {
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()

	a, err := st.CreateArtifact(ctx, "t", "react", "v1", "x", "alice", VisibilityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSharing(ctx, a.ID, VisibilityOrganisation); err != nil {
		t.Fatal(err)
	}
	if got, err := st.Artifact(ctx, a.ID); err != nil || got.Format != "v1" {
		t.Errorf("sharing changed the format: %+v, %v", got, err)
	}
	if err := st.UpdateArtifact(ctx, a.ID, 0, Revision{Title: "t", Format: "v2", Content: "y", Author: "alice"}, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := st.Artifact(ctx, a.ID); err != nil || got.Format != "v2" {
		t.Errorf("an explicit upgrade was not stored: %+v, %v", got, err)
	}
}

func openStore(ctx context.Context, t *testing.T, path string) *Store {
	t.Helper()
	seedFresh(ctx, t, path)
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestSpecificIsRefusedAsAnUnknownVisibility(t *testing.T) {
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()

	if _, err := st.CreateArtifact(ctx, "t", "html", "", "x", "alice", "specific"); err == nil {
		t.Error("created a specific artifact")
	}
	a, err := st.CreateArtifact(ctx, "t", "html", "", "x", "alice", VisibilityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSharing(ctx, a.ID, "specific"); err == nil {
		t.Error("set an artifact to specific")
	}
	if err := st.UpdateArtifact(ctx, a.ID, 0, Revision{Title: "t", Format: "", Content: "x", Author: "alice"}, "specific"); err == nil {
		t.Error("updated an artifact to specific")
	}
}

func TestOpeningADatabaseWithSpecificArtifactsTurnsThemPrivateAndDropsTheGrants(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "pictor.db")
	old, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	const withGrants = `
CREATE TABLE artifacts (
	id TEXT PRIMARY KEY, title TEXT NOT NULL, kind TEXT NOT NULL DEFAULT 'html', html TEXT NOT NULL,
	author TEXT NOT NULL, visibility TEXT NOT NULL DEFAULT 'organisation',
	created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE TABLE artifact_grants (
	artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
	kind TEXT NOT NULL, name TEXT NOT NULL, PRIMARY KEY (artifact_id, kind, name)
);
INSERT INTO artifacts (id, title, html, author, visibility, created_at, updated_at) VALUES
	('narrow', 'Squad', '<p>a', 'alice', 'specific', 1, 1),
	('open', 'Org', '<p>b', 'alice', 'organisation', 2, 2),
	('link', 'Pub', '<p>c', 'alice', 'public', 3, 3);
INSERT INTO artifact_grants VALUES ('narrow', 'group', 'SE'), ('narrow', 'user', 'bob');
`
	if _, err := old.ExecContext(ctx, withGrants); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	// Open twice: the second start must find nothing left to do.
	for range 2 {
		st := openStore(ctx, t, path)
		for id, want := range map[string]Visibility{"narrow": VisibilityPrivate, "open": VisibilityOrganisation, "link": VisibilityPublic} {
			got, err := st.Artifact(ctx, id)
			if err != nil || got.Visibility != want {
				t.Errorf("%s: %q %v, want %q", id, got.Visibility, err, want)
			}
		}
		var tables int
		if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'artifact_grants'`).Scan(&tables); err != nil || tables != 0 {
			t.Errorf("the grants table is still there: %d %v", tables, err)
		}
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTwoSavesFromTheSameVersionCannotBothWin(t *testing.T) {
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()
	a, err := st.CreateArtifact(ctx, "t", "html", "", "x", "alice", VisibilityPrivate)
	if err != nil {
		t.Fatal(err)
	}

	const writers = 8
	results := make(chan error, writers)
	for i := range writers {
		go func() {
			results <- st.UpdateArtifact(ctx, a.ID, 1, Revision{Title: "t", Content: fmt.Sprint("from ", i), Author: "alice"}, "")
		}()
	}
	var won, stale int
	for range writers {
		switch err := <-results; {
		case err == nil:
			won++
		case errors.Is(err, ErrStale):
			stale++
		default:
			t.Fatal(err)
		}
	}
	if won != 1 || stale != writers-1 {
		t.Errorf("%d saves won and %d were refused, want exactly one winner", won, stale)
	}
	if vs, err := st.Versions(ctx, a.ID); err != nil || len(vs) != 2 {
		t.Errorf("versions = %+v %v, want the original and the winner", vs, err)
	}
}

func TestAMissingArtifactIsNotFoundWhateverTheBase(t *testing.T) {
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()
	if err := st.UpdateArtifact(ctx, "nope", 3, Revision{Title: "t", Content: "x"}, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func newAssetFixture(t *testing.T) (context.Context, *Store, Artifact, func(name, blob string, size int64) (*Asset, error)) {
	t.Helper()
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	t.Cleanup(func() { _ = st.Close() })
	a, err := st.CreateArtifact(ctx, "t", "html", "", "<p>", "alice", VisibilityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	limits := AssetLimits{Files: 2, Bytes: 100}
	return ctx, st, a, func(name, blob string, size int64) (*Asset, error) {
		return st.PutAsset(ctx, Asset{ArtifactID: a.ID, Name: name, Blob: blob, ContentType: "image/png", Size: size, SHA256: "h", CreatedBy: "alice", CreatedAt: time.Now()}, limits)
	}
}

func TestAnArtifactsAssetsAreLimitedInFilesAndBytesAndReplacingCountsWhatIsLeft(t *testing.T) {
	ctx, st, a, put := newAssetFixture(t)
	if replaced, err := put("a.png", "k1", 40); err != nil || replaced != nil {
		t.Fatalf("first put: %v %v", replaced, err)
	}
	if _, err := put("b.png", "k2", 40); err != nil {
		t.Fatal(err)
	}
	if _, err := put("c.png", "k3", 1); !errors.Is(err, ErrAssetLimit) {
		t.Errorf("a third file with room in bytes: %v, want ErrAssetLimit", err)
	}
	// 40 + 60 fits, 40 + 61 does not.
	if replaced, err := put("a.png", "k4", 60); err != nil || replaced == nil || replaced.Blob != "k1" {
		t.Errorf("replace: %+v %v, want the old record with blob k1", replaced, err)
	}
	if _, err := put("a.png", "k5", 61); !errors.Is(err, ErrAssetLimit) {
		t.Errorf("replace over the byte limit: %v", err)
	}
	if got, err := st.Asset(ctx, a.ID, "a.png"); err != nil || got.Blob != "k4" || got.Size != 60 {
		t.Errorf("after a refused replace the record changed: %+v %v", got, err)
	}
}

func TestAnArtifactsAssetsAreListedByName(t *testing.T) {
	ctx, st, a, put := newAssetFixture(t)
	for _, name := range []string{"b.png", "a.png"} {
		if _, err := put(name, "k-"+name, 1); err != nil {
			t.Fatal(err)
		}
	}
	list, err := st.Assets(ctx, a.ID)
	if err != nil || len(list) != 2 || list[0].Name != "a.png" || list[1].Name != "b.png" {
		t.Errorf("list = %+v %v", list, err)
	}
}

func TestAssetsCanBeDeletedAndGoWithTheirArtifact(t *testing.T) {
	ctx, st, a, put := newAssetFixture(t)
	if _, err := put("a.png", "k1", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := put("b.png", "k2", 10); err != nil {
		t.Fatal(err)
	}
	if gone, err := st.DeleteAsset(ctx, a.ID, "b.png"); err != nil || gone.Blob != "k2" {
		t.Errorf("delete: %+v %v", gone, err)
	}
	if _, err := st.DeleteAsset(ctx, a.ID, "b.png"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete twice: %v", err)
	}
	if err := st.DeleteArtifact(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if list, err := st.Assets(ctx, a.ID); err != nil || len(list) != 0 {
		t.Errorf("a deleted artifact keeps asset records: %+v %v", list, err)
	}
}

func TestTwoConditionalWritesFromTheSameVersionCannotBothWin(t *testing.T) {
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()
	a, err := st.CreateArtifact(ctx, "t", "html", "", "x", "alice", VisibilityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutDoc(ctx, a.ID, "c", "d", []byte(`{"n":0}`), "alice"); err != nil {
		t.Fatal(err)
	}

	const writers = 8
	one := 1
	results := make(chan error, writers)
	for i := range writers {
		go func() {
			_, err := st.WriteDocs(ctx, a.ID, "alice", []DocOp{{Kind: OpUpdate, Collection: "c", ID: "d", Data: []byte(fmt.Sprintf(`{"n":%d}`, i+1)), IfVersion: &one}})
			results <- err
		}()
	}
	var won, stale int
	for range writers {
		switch err := <-results; {
		case err == nil:
			won++
		case errors.As(err, new(*DocFailure)):
			stale++
		default:
			t.Fatal(err)
		}
	}
	if won != 1 || stale != writers-1 {
		t.Errorf("%d writes won and %d were refused, want exactly one winner", won, stale)
	}
	if d, err := st.Doc(ctx, a.ID, "c", "d"); err != nil || d.Version != 2 {
		t.Errorf("document = %+v %v, want version 2", d, err)
	}
}

func TestOpeningADatabaseFromBeforeDocumentVersionsMakesEachDocumentVersionOne(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "pictor.db")
	legacy, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, legacySchema+`
		CREATE TABLE docs (artifact_id TEXT NOT NULL, collection TEXT NOT NULL, doc_id TEXT NOT NULL, data TEXT NOT NULL, updated_by TEXT NOT NULL, updated_at INTEGER NOT NULL, PRIMARY KEY (artifact_id, collection, doc_id));
		INSERT INTO docs VALUES ('old', 'c', 'd', '{"n":1}', 'alice', 1);`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	st := openStore(ctx, t, path)
	defer func() { _ = st.Close() }()
	if d, err := st.Doc(ctx, "old", "c", "d"); err != nil || d.Version != 1 {
		t.Fatalf("document = %+v %v, want version 1", d, err)
	}
	if err := st.PutDoc(ctx, "old", "c", "d", []byte(`{"n":2}`), "alice"); err != nil {
		t.Fatal(err)
	}
	if d, _ := st.Doc(ctx, "old", "c", "d"); d.Version != 2 {
		t.Errorf("version after a write = %d", d.Version)
	}
}
