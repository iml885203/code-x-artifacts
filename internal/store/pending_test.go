package store

import (
	"slices"
	"testing"
)

func TestAVersionNamesEveryoneWhoEditedItAndAPublishedOneNamesItsAuthorAlone(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	a := artifactBy(t, st, "alice")

	rev := Revision{Title: "t", Content: "<p>2", Author: "bob", Authors: []string{"bob", "alice", "bob", "carol"}}
	if err := st.UpdateArtifact(ctx, a.ID, 1, rev, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateArtifact(ctx, a.ID, 2, Revision{Title: "t", Content: "<p>3", Author: "alice"}, ""); err != nil {
		t.Fatal(err)
	}

	list, err := st.Versions(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for number, want := range map[int][]string{1: {"alice"}, 2: {"bob", "alice", "carol"}, 3: {"alice"}} {
		var got []string
		for _, v := range list {
			if v.Number == number {
				got = v.Authors
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("version %d authors = %v, want %v", number, got, want)
		}
		one, err := st.Version(ctx, a.ID, number)
		if err != nil || !slices.Equal(one.Authors, want) {
			t.Errorf("version %d read alone: authors = %v, %v, want %v", number, one.Authors, err, want)
		}
	}
}

func TestOpeningADatabaseFromBeforeVersionAuthorsGivesEveryVersionItsAuthor(t *testing.T) {
	ctx := t.Context()
	path := legacyDatabase(ctx, t)
	st := openStore(ctx, t, path)
	defer func() { _ = st.Close() }()
	v, err := st.Version(ctx, "old", 1)
	if err != nil || !slices.Equal(v.Authors, []string{"alice"}) {
		t.Errorf("version 1 of an old artifact: authors = %v, %v", v.Authors, err)
	}
}

func TestAPendingRoomRemembersWhoEditedInOrderAndForgetsThemWhenAVersionHoldsTheEdits(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	a := artifactBy(t, st, "alice")
	b := artifactBy(t, st, "alice")

	for _, who := range []string{"bob", "alice", "bob", ""} {
		if err := st.MarkPending(ctx, a.ID, who); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.MarkPending(ctx, b.ID, ""); err != nil {
		t.Fatal(err)
	}
	if rooms, err := st.PendingRooms(ctx); err != nil || len(rooms) != 2 {
		t.Fatalf("pending rooms = %v, %v, want both", rooms, err)
	}
	if who, err := st.PendingAuthors(ctx, a.ID); err != nil || !slices.Equal(who, []string{"bob", "alice"}) {
		t.Errorf("authors = %v, %v, want bob then alice, once each, and no one for an empty name", who, err)
	}
	if err := st.ClearPending(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if who, _ := st.PendingAuthors(ctx, a.ID); len(who) != 0 {
		t.Errorf("authors after the version = %v", who)
	}
	if rooms, _ := st.PendingRooms(ctx); !slices.Equal(rooms, []string{b.ID}) {
		t.Errorf("pending rooms after clearing one = %v", rooms)
	}
}

func TestAnArtifactThatDoesNotExistCannotBePending(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	if err := st.MarkPending(ctx, "nope", "bob"); err != nil {
		t.Fatal(err)
	}
	if rooms, _ := st.PendingRooms(ctx); len(rooms) != 0 {
		t.Errorf("pending rooms = %v", rooms)
	}
}

func TestDeletingAnArtifactForgetsItsPendingEdits(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	a := artifactBy(t, st, "alice")
	if err := st.MarkPending(ctx, a.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteArtifact(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if rooms, _ := st.PendingRooms(ctx); len(rooms) != 0 {
		t.Errorf("pending rooms after delete = %v", rooms)
	}
}
