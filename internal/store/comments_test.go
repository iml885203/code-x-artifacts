package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openForComments(t *testing.T) *Store {
	t.Helper()
	st := openStore(t.Context(), t, filepath.Join(t.TempDir(), "pictor.db"))
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	return st
}

func artifactBy(t *testing.T, st *Store, author string) Artifact {
	t.Helper()
	a, err := st.CreateArtifact(t.Context(), "Page by "+author, "html", "", "<p>x", author, VisibilityOrganisation)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func commentBy(t *testing.T, st *Store, artifact, author string) Comment {
	t.Helper()
	c, err := st.AddComment(t.Context(), Comment{ArtifactID: artifact, Author: author, Body: "hi", CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestUnreadCommentsAreCountedForTheAuthorOnly(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	mine, theirs := artifactBy(t, st, "alice"), artifactBy(t, st, "bob")
	commentBy(t, st, mine.ID, "bob")
	commentBy(t, st, mine.ID, "bob")
	commentBy(t, st, mine.ID, "alice")
	commentBy(t, st, theirs.ID, "alice")

	sums, err := st.CommentSummaries(ctx, "alice", "")
	if err != nil {
		t.Fatal(err)
	}
	if sums[mine.ID] != (CommentSummary{Count: 3, Unread: 2}) || sums[theirs.ID] != (CommentSummary{Count: 1}) {
		t.Errorf("summaries = %+v", sums)
	}
	if one, err := st.CommentSummaries(ctx, "alice", theirs.ID); err != nil || len(one) != 1 {
		t.Errorf("narrowed to one artifact: %v, %v", one, err)
	}
	// Someone who is not the author is never told about new comments.
	if sums, _ = st.CommentSummaries(ctx, "bob", mine.ID); sums[mine.ID].Unread != 0 {
		t.Errorf("a non-author's unread = %+v", sums[mine.ID])
	}
}

func TestSeeingCommentsOnlyMovesForward(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	mine := artifactBy(t, st, "alice")
	first, second := commentBy(t, st, mine.ID, "bob"), commentBy(t, st, mine.ID, "bob")

	for _, c := range []Comment{second, first} {
		if err := st.MarkSeen(ctx, mine.ID, "alice", c.Seq); err != nil {
			t.Fatal(err)
		}
	}
	if seen, err := st.SeenSeq(ctx, mine.ID, "alice"); err != nil || seen != second.Seq {
		t.Errorf("seen = %d, %v after marking an older comment, want %d", seen, err, second.Seq)
	}
	if sums, _ := st.CommentSummaries(ctx, "alice", mine.ID); sums[mine.ID].Unread != 0 {
		t.Errorf("unread after seeing them = %+v", sums[mine.ID])
	}
}

func TestACommentKeepsItsAnchorAndWhetherItIsForTheAgent(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	a := artifactBy(t, st, "alice")
	anchored, err := st.AddComment(ctx, Comment{ArtifactID: a.ID, Author: "bob", Body: "here", Anchor: map[string]any{"block": "b1", "quote": "x"}, ForAgent: true, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	plain := commentBy(t, st, a.ID, "bob")

	got, err := st.Comment(ctx, a.ID, anchored.ID)
	if err != nil || got.Anchor["block"] != "b1" || got.Anchor["quote"] != "x" || !got.ForAgent {
		t.Errorf("anchored comment = %+v, %v", got, err)
	}
	list, err := st.Comments(ctx, a.ID)
	if err != nil || len(list) != 2 || list[1].ID != plain.ID || len(list[1].Anchor) != 0 || list[1].ForAgent {
		t.Errorf("list = %+v, %v", list, err)
	}
}
