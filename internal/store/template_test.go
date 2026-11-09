package store

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// A store built from nothing runs the schema and every migration, each a
// synced write; most tests want exactly that result and not the building of
// it. It is built once per test binary and a test that opens a path with no
// file gets a copy (the tests of the migrations themselves put an old file
// there first and so still migrate it). internal/store/storetest does the same
// for the packages above this one, which this package cannot import.
var (
	templateOnce  sync.Once
	templateBytes []byte
	templateErr   error
)

func migratedTemplate(ctx context.Context) ([]byte, error) {
	templateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "pictor-store-template-")
		if err != nil {
			templateErr = err
			return
		}
		defer func() { _ = os.RemoveAll(dir) }()
		path := filepath.Join(dir, "pictor.db")
		st, err := Open(ctx, path)
		if err != nil {
			templateErr = err
			return
		}
		// Closing checkpoints the write-ahead log into the main file.
		if templateErr = st.Close(); templateErr != nil {
			return
		}
		templateBytes, templateErr = os.ReadFile(path) //nolint:gosec // G304: a path built above
	})
	return templateBytes, templateErr
}

// seedFresh puts the migrated template at path when nothing is there yet.
func seedFresh(ctx context.Context, t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		return
	}
	raw, err := migratedTemplate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
