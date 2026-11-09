package docedit_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

func tabTitles(d docfmt.Doc) []string {
	out := make([]string, len(d.Tabs))
	for i, t := range d.Tabs {
		out[i] = t.ID + ":" + t.Title
	}
	return out
}

func TestATabIsAddedWithAnEmptyHeadingToTypeIn(t *testing.T) {
	t.Parallel()
	d := doc(t)
	got, res, err := apply(t, d, docedit.Op{Op: docedit.TabAdd, Title: "Notes"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"t1:Doc", "t2:Other", "t_new1:Notes"}; !slices.Equal(tabTitles(got), want) {
		t.Errorf("tabs = %v, want %v", tabTitles(got), want)
	}
	added := got.Tabs[2]
	if len(added.Blocks) != 1 || added.Blocks[0].Type != docfmt.Heading || added.Blocks[0].Level != 1 || len(added.Blocks[0].Content) != 0 {
		t.Errorf("a new tab starts with one empty H1, got %+v", added.Blocks)
	}
	if len(res.Blocks) != 1 || res.Blocks[0].ID != added.Blocks[0].ID {
		t.Errorf("the result names the heading to type in: %+v", res.Blocks)
	}
	if len(res.Tabs) != 3 || res.Tabs[2].ID != "t_new1" || res.Tabs[2].Title != "Notes" {
		t.Errorf("the result lists the tabs with the new id: %+v", res.Tabs)
	}
}

func TestATabIsPlacedAtAnEndOrNextToAnotherTab(t *testing.T) {
	t.Parallel()
	d := doc(t)
	for name, c := range map[string]struct {
		op   docedit.Op
		want []string
	}{
		"at the start": {docedit.Op{Op: docedit.TabAdd, Title: "N", At: "start"}, []string{"t_new1:N", "t1:Doc", "t2:Other"}},
		"at the end":   {docedit.Op{Op: docedit.TabAdd, Title: "N", At: "end"}, []string{"t1:Doc", "t2:Other", "t_new1:N"}},
		"after a tab":  {docedit.Op{Op: docedit.TabAdd, Title: "N", After: "t1"}, []string{"t1:Doc", "t_new1:N", "t2:Other"}},
		"before a tab": {docedit.Op{Op: docedit.TabAdd, Title: "N", Before: "t2"}, []string{"t1:Doc", "t_new1:N", "t2:Other"}},
	} {
		got, _, err := apply(t, d, c.op)
		if err != nil || !slices.Equal(tabTitles(got), c.want) {
			t.Errorf("%s: %v, %v; want %v", name, tabTitles(got), err, c.want)
		}
	}
}

func TestANewTabCanBringItsContent(t *testing.T) {
	t.Parallel()
	got, _, err := apply(t, doc(t), docedit.Op{Op: docedit.TabAdd, Title: "Md", Markdown: "# Hi\n\nthere"})
	if err != nil || len(got.Tabs[2].Blocks) != 2 || text(got.Tabs[2].Blocks[0]) != "Hi" {
		t.Errorf("a new tab can bring its content: %+v, %v", got.Tabs[2].Blocks, err)
	}
}

func TestATabIsRenamedAndKeepsItsIdAndBlocks(t *testing.T) {
	t.Parallel()
	d := doc(t)
	got, res, err := apply(t, d, docedit.Op{Op: docedit.TabRename, Tab: "t1", Title: "  Plan B "})
	if err != nil {
		t.Fatal(err)
	}
	if got.Tabs[0].ID != "t1" || got.Tabs[0].Title != "Plan B" || len(got.Tabs[0].Blocks) != 4 {
		t.Errorf("renamed tab = %+v", got.Tabs[0])
	}
	if len(res.Blocks) != 0 || len(res.Deleted) != 0 {
		t.Errorf("a rename writes no block: %+v", res)
	}
}

func TestATabIsDeletedWithItsBlocksButTheLastOneStays(t *testing.T) {
	t.Parallel()
	d := doc(t)
	got, res, err := apply(t, d, docedit.Op{Op: docedit.TabDelete, Tab: "t1"})
	if err != nil || !slices.Equal(tabTitles(got), []string{"t2:Other"}) {
		t.Fatalf("%v, %v", tabTitles(got), err)
	}
	if want := []string{"h", "p1", "q", "qp", "p2"}; !slices.Equal(res.Deleted, want) {
		t.Errorf("deleted = %v, want %v", res.Deleted, want)
	}
	_, _, err = apply(t, d, docedit.Op{Op: docedit.TabDelete, Tab: "t1"}, docedit.Op{Op: docedit.TabDelete, Tab: "t2"})
	var refusal *docedit.Refusal
	if !errors.As(err, &refusal) || !strings.Contains(refusal.Msg, "last tab") || !strings.Contains(refusal.Msg, "ops[1]") {
		t.Errorf("deleting the last tab: %v", err)
	}
}

func TestATabMovesBeforeAfterAnotherOrToAnEnd(t *testing.T) {
	t.Parallel()
	three := func() docfmt.Doc {
		d := doc(t)
		got, _, err := apply(t, d, docedit.Op{Op: docedit.TabAdd, Title: "Third"})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}()
	for name, c := range map[string]struct {
		op   docedit.Op
		want []string
	}{
		"to the end":       {docedit.Op{Op: docedit.TabMove, Tab: "t1", At: "end"}, []string{"t2:Other", "t_new1:Third", "t1:Doc"}},
		"to the start":     {docedit.Op{Op: docedit.TabMove, Tab: "t_new1", At: "start"}, []string{"t_new1:Third", "t1:Doc", "t2:Other"}},
		"after a tab":      {docedit.Op{Op: docedit.TabMove, Tab: "t1", After: "t2"}, []string{"t2:Other", "t1:Doc", "t_new1:Third"}},
		"before a tab":     {docedit.Op{Op: docedit.TabMove, Tab: "t_new1", Before: "t2"}, []string{"t1:Doc", "t_new1:Third", "t2:Other"}},
		"where it already": {docedit.Op{Op: docedit.TabMove, Tab: "t1", At: "start"}, []string{"t1:Doc", "t2:Other", "t_new1:Third"}},
	} {
		got, res, err := apply(t, three, c.op)
		if err != nil || !slices.Equal(tabTitles(got), c.want) {
			t.Errorf("%s: %v, %v; want %v", name, tabTitles(got), err, c.want)
		}
		if err == nil && (len(res.Deleted) != 0 || len(res.Blocks) != 0) {
			t.Errorf("%s: a move deletes and writes no block: %+v", name, res)
		}
		if err == nil && len(got.Tabs[slices.IndexFunc(got.Tabs, func(x docfmt.Tab) bool { return x.ID == "t1" })].Blocks) != 4 {
			t.Errorf("%s: the moved tab lost blocks", name)
		}
	}
}

func TestEveryRefusalOfATabOpChangesNothingAndSaysWhy(t *testing.T) {
	t.Parallel()
	d := doc(t)
	before, _ := json.Marshal(d)
	for name, c := range map[string]struct {
		op   docedit.Op
		want string
	}{
		"add without a title":       {docedit.Op{Op: docedit.TabAdd}, "title"},
		"add with a blank title":    {docedit.Op{Op: docedit.TabAdd, Title: "  "}, "title"},
		"add with a two-line title": {docedit.Op{Op: docedit.TabAdd, Title: "a\nb"}, "one line"},
		"add with two places":       {docedit.Op{Op: docedit.TabAdd, Title: "x", At: "end", After: "t1"}, "exactly one"},
		"add after an unknown tab":  {docedit.Op{Op: docedit.TabAdd, Title: "x", After: "zz"}, `no tab "zz"`},
		"add with a bad at":         {docedit.Op{Op: docedit.TabAdd, Title: "x", At: "middle"}, "start"},
		"add with both contents":    {docedit.Op{Op: docedit.TabAdd, Title: "x", Markdown: "a", Blocks: json.RawMessage(`[]`)}, "not both"},
		"rename an unknown tab":     {docedit.Op{Op: docedit.TabRename, Tab: "zz", Title: "x"}, `no tab "zz"`},
		"rename without a tab":      {docedit.Op{Op: docedit.TabRename, Title: "x"}, "tab"},
		"rename without a title":    {docedit.Op{Op: docedit.TabRename, Tab: "t1"}, "title"},
		"delete an unknown tab":     {docedit.Op{Op: docedit.TabDelete, Tab: "zz"}, `no tab "zz"`},
		"move an unknown tab":       {docedit.Op{Op: docedit.TabMove, Tab: "zz", At: "end"}, `no tab "zz"`},
		"move without a place":      {docedit.Op{Op: docedit.TabMove, Tab: "t1"}, "exactly one"},
		"move next to itself":       {docedit.Op{Op: docedit.TabMove, Tab: "t1", After: "t1"}, "itself"},
		"move next to an unknown":   {docedit.Op{Op: docedit.TabMove, Tab: "t1", Before: "zz"}, `no tab "zz"`},
		"a block id for a tab":      {docedit.Op{Op: docedit.TabRename, ID: "p1", Tab: "t1", Title: "x"}, "tab, not id"},
		"an ifHash on a tab op":     {docedit.Op{Op: docedit.TabRename, Tab: "t1", Title: "x", IfHash: "abc"}, "no ifHash"},
	} {
		_, res, err := docedit.Apply(d, []docedit.Op{c.op}, docfmtID)
		var refusal *docedit.Refusal
		if !errors.As(err, &refusal) || !strings.Contains(refusal.Msg, c.want) {
			t.Errorf("%s: err = %v, want a refusal with %q", name, err, c.want)
		}
		if len(res.Blocks) != 0 || len(res.Deleted) != 0 || len(res.Tabs) != 0 {
			t.Errorf("%s: a refusal returned a result: %+v", name, res)
		}
	}
	// A tab op and a block op in one call are all or none.
	_, _, err := apply(t, d, docedit.Op{Op: docedit.TabAdd, Title: "ok"}, docedit.Op{Op: docedit.Delete, ID: "nope"})
	if err == nil {
		t.Error("a bad second op must refuse the call")
	}
	// More tabs than a doc may have.
	var ops []docedit.Op
	for range docfmt.MaxTabs {
		ops = append(ops, docedit.Op{Op: docedit.TabAdd, Title: "x"})
	}
	if _, _, err := apply(t, d, ops...); err == nil || !strings.Contains(err.Error(), "tabs") {
		t.Errorf("too many tabs: %v", err)
	}
	if after, _ := json.Marshal(d); string(after) != string(before) {
		t.Error("a refused call changed the doc it was given")
	}
}

func TestATabAddedByAnEarlierOpCanBeFilledByALaterOne(t *testing.T) {
	t.Parallel()
	got, _, err := apply(t, doc(t),
		docedit.Op{Op: docedit.TabAdd, Title: "N"},
		docedit.Op{Op: docedit.Insert, Tab: "t_new1", At: "end", Markdown: "body"},
	)
	if err != nil || len(got.Tabs[2].Blocks) != 2 || text(got.Tabs[2].Blocks[1]) != "body" {
		t.Errorf("%+v, %v", got.Tabs, err)
	}
}

func TestTheTabsOfADocAreListedWithTheirIdsAndTitles(t *testing.T) {
	t.Parallel()
	d := doc(t)
	if got := docedit.Tabs(d); len(got) != 2 || got[1].ID != "t2" || got[1].Title != "Other" {
		t.Errorf("Tabs = %+v", got)
	}
	raw, _ := json.Marshal(docedit.Tabs(d))
	if strings.Contains(string(raw), "blocks") {
		t.Errorf("the tab list has no blocks: %s", raw)
	}
	full, _ := json.Marshal(docedit.View(d))
	if !strings.Contains(string(full), `"blocks":[]`) {
		t.Errorf("an empty tab read as blocks still says blocks: %s", full)
	}
}
