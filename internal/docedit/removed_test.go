package docedit_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docedit"
)

func TestTheBlocksAnEditRemovedAreEveryIdThatIsNoLongerInTheDocNestedOnesIncluded(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ops  []docedit.Op
		want []string
	}{
		{"a deleted block", []docedit.Op{{Op: "delete", ID: "p2"}}, []string{"p2"}},
		{"a deleted quote takes the blocks inside it", []docedit.Op{{Op: "delete", ID: "q"}}, []string{"q", "qp"}},
		{"a replace by a block with an id of its own", []docedit.Op{{Op: "replace", ID: "p1", Blocks: json.RawMessage(`[{"id":"mine","type":"paragraph","content":[{"text":"x"}]}]`)}}, []string{"p1"}},
		{"a deleted tab takes its blocks", []docedit.Op{{Op: "tab_delete", Tab: "t1"}}, []string{"h", "p1", "q", "qp", "p2"}},
		{"a replace that keeps the id only changes the text", []docedit.Op{{Op: "replace", ID: "p1", Markdown: "other words"}}, nil},
		{"a block deleted and put back with its id is still there", []docedit.Op{{Op: "delete", ID: "p2"}, {Op: "insert", After: "p1", Blocks: json.RawMessage(`[{"id":"p2","type":"paragraph","content":[{"text":"back"}]}]`)}}, nil},
		{"an insert removes nothing", []docedit.Op{{Op: "insert", After: "h", Markdown: "new"}}, nil},
	}
	for _, c := range cases {
		before := doc(t)
		after, _, err := apply(t, before, c.ops...)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := docedit.Removed(before, after); !slices.Equal(got, c.want) {
			t.Errorf("%s: removed %v, want %v", c.name, got, c.want)
		}
	}
}
