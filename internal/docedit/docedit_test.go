package docedit_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

const sample = `{"tabs":[{"id":"t1","title":"Doc","blocks":[
 {"id":"h","type":"heading","level":1,"content":[{"text":"Plan"}]},
 {"id":"p1","type":"paragraph","content":[{"text":"first"}]},
 {"id":"q","type":"quote","blocks":[{"id":"qp","type":"paragraph","content":[{"text":"quoted"}]}]},
 {"id":"p2","type":"paragraph","content":[{"text":"second 😀 世界"}]}
]},{"id":"t2","title":"Other","blocks":[]}]}`

func doc(t *testing.T) docfmt.Doc {
	t.Helper()
	d, err := docfmt.Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func hashOf(t *testing.T, d docfmt.Doc, id string) string {
	t.Helper()
	for _, tab := range d.Tabs {
		for _, b := range tab.Blocks {
			if b.ID == id {
				return docedit.Hash(b)
			}
		}
	}
	t.Fatalf("no block %s", id)
	return ""
}

func ids(d docfmt.Doc) []string {
	var out []string
	for _, b := range d.Tabs[0].Blocks {
		out = append(out, b.ID)
	}
	return out
}

func text(b docfmt.Block) string {
	var sb strings.Builder
	for _, r := range b.Content {
		sb.WriteString(r.Text)
	}
	return sb.String()
}

func apply(t *testing.T, d docfmt.Doc, ops ...docedit.Op) (docfmt.Doc, docedit.Result, error) {
	t.Helper()
	n := 0
	return docedit.Apply(d, ops, func(prefix string) string {
		n++
		return prefix + "new" + strconv.Itoa(n)
	})
}

func TestReplaceKeepsTheIdOfTheBlockItReplacesAndReportsTheNewHash(t *testing.T) {
	t.Parallel()
	got, res, err := apply(t, doc(t), docedit.Op{Op: "replace", ID: "p1", Markdown: "changed *text*"})
	if err != nil {
		t.Fatal(err)
	}
	if b := got.Tabs[0].Blocks[1]; b.ID != "p1" || text(b) != "changed text" {
		t.Errorf("block = %+v", b)
	}
	if len(res.Blocks) != 1 || res.Blocks[0].ID != "p1" || res.Blocks[0].Hash != docedit.Hash(got.Tabs[0].Blocks[1]) || len(res.Deleted) != 0 {
		t.Errorf("result = %+v", res)
	}
}

func TestReplaceBySeveralBlocksAndByBlocksAsJSON(t *testing.T) {
	t.Parallel()
	d := doc(t)
	got, res, err := apply(t, d, docedit.Op{Op: "replace", ID: "p1", Markdown: "one\n\ntwo"})
	if err != nil || !slices.Equal(ids(got)[:2], []string{"h", "p1"}) || len(ids(got)) != 5 || len(res.Blocks) != 2 {
		t.Errorf("ids = %v, result %+v, err %v", ids(got), res, err)
	}
	got, res, err = apply(t, d, docedit.Op{Op: "replace", ID: "p2", Blocks: json.RawMessage(`[{"id":"mine","type":"paragraph","content":[{"text":"x"}]}]`)})
	if err != nil || got.Tabs[0].Blocks[3].ID != "mine" || !slices.Equal(res.Deleted, []string{"p2"}) {
		t.Errorf("ids = %v, result %+v, err %v", ids(got), res, err)
	}
}

func TestInsertAfterBeforeAndAtTheStartAndEndOfATab(t *testing.T) {
	t.Parallel()
	got, _, err := apply(t, doc(t),
		docedit.Op{Op: "insert", After: "p1", Markdown: "after p1"},
		docedit.Op{Op: "insert", Before: "h", Markdown: "before h"},
		docedit.Op{Op: "insert", Tab: "t1", At: "end", Markdown: "at the end"},
		docedit.Op{Op: "insert", Tab: "t2", At: "start", Markdown: "in the empty tab"},
	)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, b := range got.Tabs[0].Blocks {
		texts = append(texts, text(b))
	}
	if want := []string{"before h", "Plan", "first", "after p1", "", "second 😀 世界", "at the end"}; !slices.Equal(texts, want) {
		t.Errorf("texts = %q, want %q", texts, want)
	}
	if len(got.Tabs[1].Blocks) != 1 {
		t.Errorf("the empty tab holds %d blocks", len(got.Tabs[1].Blocks))
	}
}

func TestDeleteAndABlockInsideAQuoteCanBeNamed(t *testing.T) {
	t.Parallel()
	got, res, err := apply(t, doc(t), docedit.Op{Op: "delete", ID: "p1"}, docedit.Op{Op: "replace", ID: "qp", Markdown: "inside"}, docedit.Op{Op: "insert", After: "qp", Markdown: "also inside"})
	if err != nil || !slices.Equal(res.Deleted, []string{"p1"}) {
		t.Fatalf("result %+v, err %v", res, err)
	}
	if got.Tabs[0].Blocks[1].Type != docfmt.Quote || len(got.Tabs[0].Blocks[1].Blocks) != 2 || text(got.Tabs[0].Blocks[1].Blocks[0]) != "inside" {
		t.Errorf("quote = %+v", got.Tabs[0].Blocks[1])
	}
}

func TestABlockAnEarlierOperationMadeCanBeChangedByALaterOne(t *testing.T) {
	t.Parallel()
	got, _, err := apply(t, doc(t),
		docedit.Op{Op: "insert", After: "h", Blocks: json.RawMessage(`[{"id":"fresh","type":"paragraph","content":[{"text":"v1"}]}]`)},
		docedit.Op{Op: "replace", ID: "fresh", Markdown: "v2"})
	if err != nil || text(got.Tabs[0].Blocks[1]) != "v2" {
		t.Errorf("%+v, %v", got.Tabs[0].Blocks[1], err)
	}
}

func TestEveryRefusalChangesNothingAndSaysWhy(t *testing.T) {
	t.Parallel()
	d := doc(t)
	before, _ := json.Marshal(d)
	for name, c := range map[string]struct {
		ops  []docedit.Op
		want string
	}{
		"no ops":                      {nil, "ops is empty"},
		"an unknown op":               {[]docedit.Op{{Op: "move", ID: "p1"}}, `unknown op "move"`},
		"no op":                       {[]docedit.Op{{ID: "p1"}}, "op is missing"},
		"an unknown id":               {[]docedit.Op{{Op: "delete", ID: "nope"}}, `no block "nope"`},
		"the second op unknown":       {[]docedit.Op{{Op: "delete", ID: "p1"}, {Op: "replace", ID: "nope", Markdown: "x"}}, "ops[1]"},
		"an insert with no place":     {[]docedit.Op{{Op: "insert", Markdown: "x"}}, "exactly one of before"},
		"two places":                  {[]docedit.Op{{Op: "insert", Before: "h", After: "p1", Markdown: "x"}}, "exactly one of before"},
		"an unknown tab":              {[]docedit.Op{{Op: "insert", Tab: "zz", At: "end", Markdown: "x"}}, `no tab "zz"`},
		"a bad at":                    {[]docedit.Op{{Op: "insert", Tab: "t1", At: "middle", Markdown: "x"}}, "start"},
		"nothing to write":            {[]docedit.Op{{Op: "replace", ID: "p1"}}, "needs the new content"},
		"both markdown and blocks":    {[]docedit.Op{{Op: "replace", ID: "p1", Markdown: "x", Blocks: json.RawMessage(`[]`)}}, "not both"},
		"a script link":               {[]docedit.Op{{Op: "replace", ID: "p1", Markdown: "[x](javascript:alert(1))"}}, "refused"},
		"a script link in blocks":     {[]docedit.Op{{Op: "replace", ID: "p1", Blocks: json.RawMessage(`[{"type":"paragraph","content":[{"text":"x","marks":["link"],"href":"javascript:1"}]}]`)}}, "refused"},
		"a duplicate id":              {[]docedit.Op{{Op: "insert", After: "h", Blocks: json.RawMessage(`[{"id":"p1","type":"paragraph"}]`)}}, "not a valid doc"},
		"too many":                    {slices.Repeat([]docedit.Op{{Op: "delete", ID: "p1"}}, docedit.MaxOps+1), "at most"},
		"a hash of a block not there": {[]docedit.Op{{Op: "delete", ID: "p1", IfHash: "abc"}, {Op: "insert", After: "gone", IfHash: "abc", Markdown: "x"}}, "not in the doc"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, res, err := docedit.Apply(d, c.ops, docfmtID)
			var refusal *docedit.Refusal
			if !errors.As(err, &refusal) || !strings.Contains(refusal.Msg, c.want) {
				t.Fatalf("err = %v, want a refusal with %q", err, c.want)
			}
			if len(res.Blocks) != 0 || len(res.Deleted) != 0 {
				t.Errorf("a refusal returned a result: %+v", res)
			}
		})
	}
	if after, _ := json.Marshal(d); string(after) != string(before) {
		t.Error("a refused call changed the doc it was given")
	}
}

func docfmtID(prefix string) string { return docfmt.NewID(prefix) }

func TestAStaleHashRefusesTheWholeCallAndGivesTheCurrentStateOfEveryStaleBlock(t *testing.T) {
	t.Parallel()
	d := doc(t)
	_, _, err := apply(t, d,
		docedit.Op{Op: "replace", ID: "p1", IfHash: hashOf(t, d, "p1"), Markdown: "fine"},
		docedit.Op{Op: "replace", ID: "p2", IfHash: "000000000000", Markdown: "stale"},
		docedit.Op{Op: "insert", After: "h", IfHash: "111111111111", Markdown: "stale too"},
	)
	var stale *docedit.Stale
	if !errors.As(err, &stale) {
		t.Fatalf("err = %v, want a stale refusal", err)
	}
	if len(stale.Blocks) != 2 || stale.Blocks[0].ID != "p2" || stale.Blocks[0].Hash != hashOf(t, d, "p2") || stale.Blocks[1].ID != "h" || stale.Blocks[0].Tab != "t1" {
		t.Errorf("the states = %+v", stale.Blocks)
	}
}

func TestAGuardIsCheckedAgainstTheDocAsItWasBeforeTheCall(t *testing.T) {
	t.Parallel()
	d := doc(t)
	h := hashOf(t, d, "p1")
	got, _, err := apply(t, d,
		docedit.Op{Op: "replace", ID: "p1", IfHash: h, Markdown: "changed"},
		docedit.Op{Op: "insert", After: "p1", IfHash: h, Markdown: "after"},
	)
	if err != nil || text(got.Tabs[0].Blocks[1]) != "changed" || text(got.Tabs[0].Blocks[2]) != "after" {
		t.Errorf("err %v, blocks %v", err, ids(got))
	}
}

func TestAnImageWithoutAltIsFlaggedForReview(t *testing.T) {
	t.Parallel()
	_, res, err := apply(t, doc(t), docedit.Op{Op: "insert", After: "h", Markdown: "![](https://example.com/a.png)\n\n![described](https://example.com/b.png)"})
	if err != nil || len(res.Review) != 1 || !strings.Contains(res.Review[0], "no alt") {
		t.Errorf("review = %v, err %v", res.Review, err)
	}
}

func TestTheViewHasEveryBlockWithItsHashAndTheOutlineListsTheTopLevelOnes(t *testing.T) {
	t.Parallel()
	d := doc(t)
	view := docedit.View(d)
	if len(view) != 2 || len(view[0].Blocks) != 4 || view[0].Blocks[1].Hash != hashOf(t, d, "p1") || view[0].Blocks[1].ID != "p1" {
		t.Errorf("view = %+v", view)
	}
	raw, _ := json.Marshal(view[0].Blocks[1])
	if !strings.Contains(string(raw), `"hash"`) || !strings.Contains(string(raw), `"type":"paragraph"`) || !strings.Contains(string(raw), `"content"`) {
		t.Errorf("a block reads as %s", raw)
	}
	outline := docedit.Outlines(d)
	if len(outline) != 4 || outline[1].ID != "p1" || outline[1].Preview != "first" || outline[3].Preview != "second 😀 世界" {
		t.Errorf("outline = %+v", outline)
	}
}

func TestAHashChangesWithAnythingInTheBlock(t *testing.T) {
	t.Parallel()
	d := doc(t)
	base := hashOf(t, d, "q")
	d.Tabs[0].Blocks[2].Blocks[0].Content[0].Text = "quoted!"
	if hashOf(t, d, "q") == base {
		t.Error("a change inside a quote did not change the quote's hash")
	}
}

func TestAnImageAPersonAddedWithoutAltIsFlaggedOnTheAgentsNextEditEvenWhenItIsNotTheBlockEdited(t *testing.T) {
	t.Parallel()
	d := doc(t)
	d.Tabs[0].Blocks = append(d.Tabs[0].Blocks, docfmt.Block{ID: "pic", Type: docfmt.Image, Src: "/files/abc/shot.png"}, docfmt.Block{ID: "described", Type: docfmt.Image, Src: "/files/abc/b.png", Alt: "A chart"})
	_, res, err := apply(t, d, docedit.Op{Op: "replace", ID: "p1", Markdown: "changed"})
	if err != nil || len(res.Review) != 1 || !strings.Contains(res.Review[0], "block pic is an image with no alt") {
		t.Errorf("review = %v, err %v", res.Review, err)
	}
}
