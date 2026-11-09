package service

import (
	"errors"
	"slices"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestAVersionFromARoomNamesEveryoneWhoEditedAndKeepsTheTitleAndFiles(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)
	p, err := svc.Create(t.Context(), svc.Viewer("alice"), PublishInput{Title: "Plan", Kind: "doc", Content: "# Plan\n\nx\n"})
	if err != nil {
		t.Fatal(err)
	}
	content := `{"tabs":[{"id":"t1","title":"Document","blocks":[{"id":"b1","type":"paragraph","content":[{"text":"edited"}]}]}]}`
	if err := svc.SaveFromRoom(t.Context(), p.ID, 1, kind.Bundle{Content: content}, []string{"bob", "carol"}); err != nil {
		t.Fatal(err)
	}
	v, err := svc.store.Version(t.Context(), p.ID, 2)
	if err != nil || v.Title != "Plan" || v.Content != content || v.Author != "bob" || !slices.Equal(v.Authors, []string{"bob", "carol"}) {
		t.Errorf("version 2 = %+v, %v", v, err)
	}
}

func TestAVersionFromARoomIsRefusedWhenItIsNotBuiltOnTheNewestOrIsNotAValidDoc(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)
	p, err := svc.Create(t.Context(), svc.Viewer("alice"), PublishInput{Title: "Plan", Kind: "doc", Content: "# Plan\n\nx\n"})
	if err != nil {
		t.Fatal(err)
	}
	valid := `{"tabs":[{"id":"t1","title":"Document","blocks":[]}]}`
	if err := svc.SaveFromRoom(t.Context(), p.ID, 7, kind.Bundle{Content: valid}, []string{"bob"}); !errors.Is(err, store.ErrStale) {
		t.Errorf("a stale base = %v, want store.ErrStale", err)
	}
	unsafe := `{"tabs":[{"id":"t1","title":"Document","blocks":[{"id":"b1","type":"paragraph","content":[{"text":"x","marks":["link"],"href":"javascript:alert(1)"}]}]}]}`
	if err := svc.SaveFromRoom(t.Context(), p.ID, 1, kind.Bundle{Content: unsafe}, []string{"bob"}); err == nil {
		t.Error("a link that runs script was saved as a version")
	}
	if err := svc.SaveFromRoom(t.Context(), "nope", 1, kind.Bundle{Content: valid}, nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a missing artifact = %v", err)
	}
	if list, _ := svc.store.Versions(t.Context(), p.ID); len(list) != 1 {
		t.Errorf("%d versions, nothing should have been saved", len(list))
	}
}
