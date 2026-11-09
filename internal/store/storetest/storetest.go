// Package storetest opens stores for tests. Building a store (schema and every
// migration) is the most expensive step of most tests, and the result is the
// same every time, so it is built once per test binary and each test gets its
// own copy of the file.
package storetest

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

var (
	once     sync.Once
	template []byte
	built    error
)

// build opens a fresh store in a scratch directory and keeps the file's bytes.
func build(t testing.TB) ([]byte, error) {
	once.Do(func() {
		dir, err := os.MkdirTemp("", "pictor-storetest-")
		if err != nil {
			built = err
			return
		}
		defer func() { _ = os.RemoveAll(dir) }()
		path := filepath.Join(dir, "pictor.db")
		st, err := store.Open(t.Context(), path)
		if err != nil {
			built = err
			return
		}
		// Closing checkpoints the write-ahead log into the main file.
		if built = st.Close(); built != nil {
			return
		}
		template, built = os.ReadFile(path) //nolint:gosec // G304: a path built above
	})
	return template, built
}

// Open returns a new, empty, fully migrated store on its own file, closed when
// the test ends.
func Open(t testing.TB) *store.Store {
	t.Helper()
	raw, err := build(t)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pictor.db")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
