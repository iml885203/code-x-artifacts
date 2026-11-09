package store

import (
	"context"
	"errors"
	"testing"
)

func TestOnlyCompleteDirectoryGenerationsReplaceMembershipAndKeepNames(t *testing.T) {
	t.Parallel()
	s := openForComments(t)
	ctx := context.Background()
	people := []DirectoryPerson{{"alice", "Alice"}, {"never-signed-in", "陳同事"}}
	save := func(at int64, scope string, complete bool, ps []DirectoryPerson) {
		t.Helper()
		if err := s.RecordDirectory(ctx, DirectorySnapshot{scope, at, complete, ps}); err != nil {
			t.Fatal(err)
		}
	}
	save(1000000000, "org", true, people)
	if _, err := s.User(ctx, "never-signed-in"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lookup metadata became sign-in profile: %v", err)
	}
	got, err := s.SearchUsers(ctx, "陳", 8)
	if err != nil || len(got) != 1 {
		t.Fatalf("never-signed-in colleague: %+v %v", got, err)
	}
	save(2000000000, "org", false, nil)
	got, _ = s.SearchUsers(ctx, "", 8)
	state, _ := s.DirectoryState(ctx)
	if len(got) != 2 || state.Complete {
		t.Fatalf("incomplete read lost roster or claimed complete: %+v %+v", got, state)
	}
	save(1500000000, "org", true, people[:1])
	got, _ = s.SearchUsers(ctx, "", 8)
	if len(got) != 2 {
		t.Fatal("older concurrent generation replaced later outcome")
	}
	save(3000000000, "org", true, people[:1])
	got, _ = s.SearchUsers(ctx, "陳", 8)
	if len(got) != 0 {
		t.Fatal("departed colleague still suggested")
	}
	var name string
	var active bool
	if err := s.db.QueryRowContext(ctx, "SELECT display_name,active FROM directory_people WHERE username=?", "never-signed-in").Scan(&name, &active); err != nil || name != "陳同事" || active {
		t.Fatalf("departed stored name: %q %v %v", name, active, err)
	}
	save(4000000000, "other-scope", false, nil)
	got, _ = s.SearchUsers(ctx, "", 8)
	if len(got) != 0 {
		t.Fatal("failed scope change exposed previous organisation roster")
	}
}

func TestAnInvalidDirectoryGenerationCannotPartlyReplaceTheRoster(t *testing.T) {
	t.Parallel()
	s := openForComments(t)
	ctx := context.Background()
	if err := s.RecordDirectory(ctx, DirectorySnapshot{"org", 1, true, []DirectoryPerson{{"alice", "Alice"}}}); err != nil {
		t.Fatal(err)
	}
	for _, ps := range [][]DirectoryPerson{nil, {{"alice", ""}}, {{"alice", "Alice"}, {"alice", "Other"}}, {{"bad user", "Name"}}, {{"alice", "line\nbreak"}}} {
		if err := s.RecordDirectory(ctx, DirectorySnapshot{"org", 2, true, ps}); err == nil {
			t.Fatalf("invalid generation accepted: %+v", ps)
		}
		got, _ := s.SearchUsers(ctx, "", 8)
		if len(got) != 1 || got[0].DisplayName != "Alice" {
			t.Fatalf("partial replacement: %+v", got)
		}
	}
}

func TestAWriteFailureRollsBackTheEntireDirectoryGeneration(t *testing.T) {
	t.Parallel()
	s := openForComments(t)
	ctx := t.Context()
	initial := DirectorySnapshot{"org", 1, true, []DirectoryPerson{{"alice", "Original Alice"}, {"bob", "Original Bob"}}}
	if err := s.RecordDirectory(ctx, initial); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER reject_directory_row BEFORE INSERT ON directory_people WHEN NEW.username='carol' BEGIN SELECT RAISE(ABORT,'write rejected'); END`); err != nil {
		t.Fatal(err)
	}
	next := DirectorySnapshot{"org", 2, true, []DirectoryPerson{{"alice", "Changed Alice"}, {"carol", "Carol"}}}
	if err := s.RecordDirectory(ctx, next); err == nil {
		t.Fatal("write failure accepted as complete")
	}
	got, err := s.SearchUsers(ctx, "", 8)
	if err != nil || len(got) != 2 || got[0].DisplayName != "Original Alice" || got[1].DisplayName != "Original Bob" {
		t.Fatalf("failed generation partly committed: %+v %v", got, err)
	}
}
