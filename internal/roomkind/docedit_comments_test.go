package roomkind_test

import (
	"encoding/json"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

func TestDeletingACommentedBlockWarnsTheSameWithAndWithoutARoom(t *testing.T) {
	t.Parallel()
	results := map[bool]service.DocEdited{}
	threads := map[bool]string{}
	for _, live := range []bool{false, true} {
		e := newEnv(t)
		id := e.editableDoc()
		if live {
			_ = e.roomText(id)
		}
		c, err := e.svc.AddComment(t.Context(), e.svc.Viewer("alice"), id, service.NewComment{Body: "Reword this", Anchor: json.RawMessage(`{"tab":"t1","block":"qa","quote":"quoted a"}`)})
		if err != nil {
			t.Fatal(err)
		}
		threads[live] = c.ID
		res, err := e.docEdit(id, service.DocEditInput{Ops: []docedit.Op{{Op: docedit.Delete, ID: "q"}}})
		if err != nil {
			t.Fatal(err)
		}
		if res.Saved == live {
			t.Errorf("live %v: saved %v", live, res.Saved)
		}
		results[live] = res
	}
	for live, res := range results {
		want := []service.DetachedThread{{ID: threads[live], Tab: "t1", Block: "qa", Quote: "quoted a"}}
		if mustJSON(t, res.CommentsOnDeletedBlocks) != mustJSON(t, want) || len(res.Review) != 1 {
			t.Errorf("live %v: warned %s, review %v; want %s", live, mustJSON(t, res.CommentsOnDeletedBlocks), res.Review, mustJSON(t, want))
		}
	}
}
