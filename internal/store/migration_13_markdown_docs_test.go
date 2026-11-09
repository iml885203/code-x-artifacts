package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// markdownVersions are the versions of one doc written before blocks existed:
// raw HTML, tables, code, nested lists, pictures (one with an address that
// block docs refuse), a link that block docs refuse, a quote, a comment that
// looks like a block doc's own.
var markdownVersions = []string{
	"# Spec\n\nSome **bold** and *italic* text with `code`.\n\n<script>alert(1)</script>\n\n| a | b |\n|:--|--:|\n| 1 | 2 |\n",
	"# Spec\n\nSome **bold** and *italic* text with `code`.\n\n- one\n  - nested\n- two\n\n1. first\n2. second\n\n```go\nfmt.Println(\"<hi>\")\n```\n\n![chart](https://example.com/c.png) ![local](pic.png)\n",
	"# Spec 2 規格\n\n<div>raw <b>html</b></div>\n\n> quoted [bad](javascript:alert(1)) and [good](https://example.com)\n\n<!-- pictor:pending Not a block here -->\n\n- [x] done\n- [ ] todo\n\nwww.example.com and https://bare.example.com\n\n---\n",
}

func TestOpeningADatabaseWithMarkdownDocsMakesEveryVersionABlockDocWithTheSameText(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "pictor.db")
	st := openStore(ctx, t, path)
	a, err := st.CreateArtifact(ctx, "Spec", "doc", "1", markdownVersions[0], "alice", VisibilityOrganisation)
	if err != nil {
		t.Fatal(err)
	}
	for i, md := range markdownVersions[1:] {
		if err := st.UpdateArtifact(ctx, a.ID, i+1, Revision{Title: "Spec", Format: "1", Content: md, Author: "alice"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	other, err := st.CreateArtifact(ctx, "Page", "html", "", "<p>untouched", "alice", VisibilityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st = openStore(ctx, t, path)
	defer func() { _ = st.Close() }()
	for i, md := range markdownVersions {
		checkMigratedVersion(ctx, t, st, a.ID, i+1, md)
	}
	checkMigratedArtifactRow(ctx, t, st, a.ID, other.ID)
}

func checkMigratedVersion(ctx context.Context, t *testing.T, st *Store, id string, number int, md string) {
	t.Helper()
	v, err := st.Version(ctx, id, number)
	if err != nil || v.Format != "2" {
		t.Fatalf("version %d: format %q, %v; want 2", number, v.Format, err)
	}
	doc, err := docfmt.Parse(v.Content)
	if err != nil {
		t.Fatalf("version %d is not a valid block doc: %v", number, err)
	}
	if got, want := docfmt.TextOfDoc(doc), docfmt.TextOfMarkdown(md); got != want {
		t.Errorf("version %d changed its text\n before: %q\n after:  %q", number, want, got)
	}
	if strings.Contains(v.Content, `"type":"code"`) && strings.Contains(v.Content, "# Spec") {
		t.Errorf("version %d fell back to keeping Markdown as code", number)
	}
	var original string
	if err := st.db.QueryRowContext(ctx, `SELECT original_markdown FROM artifact_versions WHERE artifact_id = ? AND number = ?`, id, number).Scan(&original); err != nil || original != md {
		t.Errorf("version %d: original Markdown %q (%v), want it kept", number, original, err)
	}
}

func checkMigratedArtifactRow(ctx context.Context, t *testing.T, st *Store, id, otherID string) {
	t.Helper()
	cur, err := st.Artifact(ctx, id)
	if err != nil || cur.Format != "2" || cur.Content == "" || cur.Version != 3 {
		t.Fatalf("the artifact row: %+v %v", cur, err)
	}
	if newest, _ := st.Version(ctx, id, 3); newest.Content != cur.Content {
		t.Error("the artifact row and its newest version disagree")
	}
	if h, err := st.Artifact(ctx, otherID); err != nil || h.Content != "<p>untouched" || h.Format != "" {
		t.Errorf("another kind changed: %+v %v", h, err)
	}
	if !strings.Contains(cur.Content, "https://example.com") || strings.Contains(cur.Content, "javascript:") || strings.Contains(cur.Content, "pic.png") {
		t.Errorf("a good link stays, a refused address goes: %s", cur.Content)
	}
	first, _ := st.Version(ctx, id, 1)
	second, _ := st.Version(ctx, id, 2)
	d1, _ := docfmt.Parse(first.Content)
	d2, _ := docfmt.Parse(second.Content)
	if d1.Tabs[0].ID != d2.Tabs[0].ID || d1.Tabs[0].Blocks[0].ID != d2.Tabs[0].Blocks[0].ID {
		t.Error("the tab and the unchanged heading changed id between versions")
	}
}

func TestMigratingMarkdownDocsTwiceChangesNothingAndAMigratedVersionRestores(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "pictor.db")
	st := openStore(ctx, t, path)
	a, err := st.CreateArtifact(ctx, "Spec", "doc", "1", markdownVersions[0], "alice", VisibilityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateArtifact(ctx, a.ID, 1, Revision{Title: "Spec", Format: "1", Content: markdownVersions[1], Author: "alice"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st = openStore(ctx, t, path)
	before, _ := st.Versions(ctx, a.ID)
	var contents []string
	for _, v := range before {
		full, _ := st.Version(ctx, a.ID, v.Number)
		contents = append(contents, full.Content)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st = openStore(ctx, t, path)
	defer func() { _ = st.Close() }()
	for i, v := range before {
		full, _ := st.Version(ctx, a.ID, v.Number)
		if full.Content != contents[i] || full.Format != "2" {
			t.Errorf("version %d changed when the migration ran again", v.Number)
		}
	}
	old, _ := st.Version(ctx, a.ID, 1)
	if err := st.UpdateArtifact(ctx, a.ID, 2, Revision{Title: old.Title, Format: old.Format, Content: old.Content, Author: "alice", RestoredFrom: 1}, ""); err != nil {
		t.Fatalf("restore of a migrated version: %v", err)
	}
	if got, err := st.Artifact(ctx, a.ID); err != nil || got.Version != 3 || got.Content != old.Content || got.Format != "2" {
		t.Errorf("after the restore: %+v %v", got, err)
	}
}
