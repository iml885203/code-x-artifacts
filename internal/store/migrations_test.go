package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMigrationsAreNumberedInOrder(t *testing.T) {
	t.Parallel()
	names := map[string]bool{}
	for i, m := range migrations {
		if m.number != i+1 {
			t.Errorf("migrations[%d] is numbered %d, want %d: the list must be contiguous, from 1, in order", i, m.number, i+1)
		}
		if m.name == "" || m.run == nil {
			t.Errorf("migration %d needs a name and a function", m.number)
		}
		if names[m.name] {
			t.Errorf("migration %d repeats the name %q", m.number, m.name)
		}
		names[m.name] = true
	}
}

// dump returns the whole database as text: every table's definition and rows.
func dump(ctx context.Context, t *testing.T, db *sql.DB) string {
	t.Helper()
	var out strings.Builder
	rows, err := db.QueryContext(ctx, `SELECT name, sql FROM sqlite_master WHERE sql IS NOT NULL ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&out, "%s: %s\n", name, def)
		if !strings.Contains(def, "CREATE TABLE") {
			continue
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		data, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT * FROM %q`, table))
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := data.Columns()
		for data.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := data.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&out, "%s %v\n", table, vals)
		}
		if err := data.Err(); err != nil {
			t.Fatal(err)
		}
		if err := data.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return out.String()
}

// columns lists each table's columns and their definitions, ignoring the
// order ALTER TABLE put them in.
func columns(ctx context.Context, t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT m.name, p.name, p.type, p."notnull", COALESCE(p.dflt_value, ''), p.pk
		FROM sqlite_master m, pragma_table_info(m.name) p
		WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%'
		ORDER BY m.name, p.name`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := map[string][]string{}
	for rows.Next() {
		var table, name, typ, dflt string
		var notNull, pk int
		if err := rows.Scan(&table, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		got[table] = append(got[table], fmt.Sprintf("%s %s notnull=%d default=%s pk=%d", name, typ, notNull, dflt, pk))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestEveryMigrationIsANoOpOnAnUpToDateDatabase(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	// Only the end state matters here: the migrated template is exactly what a
	// restart finds. TestAnOldSchemaUpgradesToTheLatest does the real upgrade.
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	defer func() { _ = st.Close() }()
	before := dump(ctx, t, st.db)
	// Dumping the whole database after every migration costs more than running
	// them. SQLite counts every schema change and every row written on this
	// connection (the store has exactly one), which names the migration that
	// touched anything; one dump at the end then compares what is stored.
	touched := func() (schemaVersion, changes int) {
		if err := st.db.QueryRowContext(ctx, `SELECT (SELECT schema_version FROM pragma_schema_version), total_changes()`).Scan(&schemaVersion, &changes); err != nil {
			t.Fatal(err)
		}
		return schemaVersion, changes
	}
	for _, m := range migrations {
		schemaVersion, changes := touched()
		if err := m.run(ctx, st.db); err != nil {
			t.Fatalf("migration %d (%s) on an up-to-date database: %v", m.number, m.name, err)
		}
		if nowSchema, nowChanges := touched(); nowSchema != schemaVersion || nowChanges != changes {
			t.Fatalf("migration %d (%s) wrote to an up-to-date database (schema version %d -> %d, rows written %d -> %d)", m.number, m.name, schemaVersion, nowSchema, changes, nowChanges)
		}
	}
	if after := dump(ctx, t, st.db); after != before {
		t.Fatalf("the migrations changed an up-to-date database:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestAnOldSchemaUpgradesToTheLatest(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	upgraded := openStore(ctx, t, legacyDatabase(ctx, t))
	defer func() { _ = upgraded.Close() }()
	fresh := openStore(ctx, t, filepath.Join(t.TempDir(), "fresh.db"))
	defer func() { _ = fresh.Close() }()

	want, got := columns(ctx, t, fresh.db), columns(ctx, t, upgraded.db)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("a database upgraded from the first schema differs from a fresh one:\nupgraded: %v\nfresh:    %v", got, want)
	}
	if _, ok := got["artifacts"]; !ok || len(got) < 10 {
		t.Errorf("the comparison saw too little to mean anything: %v", got)
	}
}
