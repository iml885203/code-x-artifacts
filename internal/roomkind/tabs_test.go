package roomkind_test

import (
	"slices"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

func tabsOf(t *testing.T, e *env, id string) []string {
	t.Helper()
	var out []string
	for _, tab := range readDocOf(t, e, id).Tabs {
		out = append(out, tab.ID+":"+tab.Title)
	}
	return out
}

func titlesOf(t *testing.T, content string) []string {
	t.Helper()
	d, err := docfmt.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, tab := range d.Tabs {
		out = append(out, tab.Title)
	}
	return out
}

// typeAtEndOfP2 is a person typing at the end of p2 in the first tab.
func typeAtEndOfP2(person *client, text string) {
	tab := firstTab(person.doc)
	for _, child := range tab.Children() {
		el := child.(*crdt.YXmlElement)
		if v, _ := el.GetAttribute("id"); v == "p2" {
			run := el.Children()[0].(*crdt.YXmlText)
			person.doc.Transact(func(txn *crdt.Transaction) { run.Insert(txn, run.Len(), text, nil) })
		}
	}
}

func TestAnAgentsTabOpsInALiveDocShowUpForPeopleAndLeaveWhatAPersonTypesInAnotherTab(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.editableDoc()
	person := e.connect(id, bob)
	person.pump()
	typeAtEndOfP2(person, " + bob")
	person.pump()

	added, err := e.docEdit(id, service.DocEditInput{Ops: []docedit.Op{{Op: docedit.TabAdd, Title: "Notes"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(added.Tabs) != 2 || added.Tabs[1].Title != "Notes" {
		t.Fatalf("the result lists the new tab: %+v", added.Tabs)
	}
	notes := added.Tabs[1].ID
	person.pump()
	d := readDocOf(t, e, id)
	if got := tabsOf(t, e, id); !slices.Equal(got, []string{"t1:Doc", notes + ":Notes"}) {
		t.Fatalf("tabs = %v", got)
	}
	if got := d.Tabs[0].Blocks[3].Content[0].Text; got != "second + bob" {
		t.Errorf("what bob typed in another tab = %q", got)
	}
	if first := d.Tabs[1].Blocks[0]; first.Type != "heading" || first.Level != 1 || len(first.Content) != 0 {
		t.Errorf("a new tab starts with an empty H1, got %+v", first)
	}
}

func TestAnAgentRenamesMovesAndDeletesTabsInALiveDocAndTheVersionThatFollowsHasThem(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.editableDoc()
	person := e.connect(id, bob)
	person.pump()
	added, err := e.docEdit(id, service.DocEditInput{Ops: []docedit.Op{{Op: docedit.TabAdd, Title: "Notes"}}})
	if err != nil {
		t.Fatal(err)
	}
	notes := added.Tabs[1].ID

	for _, step := range []struct {
		op   docedit.Op
		want []string
	}{
		{docedit.Op{Op: docedit.TabRename, Tab: "t1", Title: "Plan"}, []string{"t1:Plan", notes + ":Notes"}},
		{docedit.Op{Op: docedit.TabMove, Tab: notes, At: "start"}, []string{notes + ":Notes", "t1:Plan"}},
	} {
		if _, err := e.docEdit(id, service.DocEditInput{Ops: []docedit.Op{step.op}}); err != nil {
			t.Fatal(err)
		}
		person.pump()
		if got := tabsOf(t, e, id); !slices.Equal(got, step.want) {
			t.Fatalf("after %s: tabs = %v, want %v", step.op.Op, got, step.want)
		}
	}

	// The version that follows carries the same tabs in the same order.
	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	saved, err := e.st.Artifact(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(t, saved.Content); !slices.Equal(got, []string{"Notes", "Plan"}) {
		t.Errorf("the saved doc has tabs %v, want Notes then Plan", got)
	}

	if _, err := e.docEdit(id, service.DocEditInput{Ops: []docedit.Op{{Op: docedit.TabDelete, Tab: notes}}}); err != nil {
		t.Fatal(err)
	}
	person.pump()
	if got := tabsOf(t, e, id); !slices.Equal(got, []string{"t1:Plan"}) {
		t.Errorf("after the delete: %v", got)
	}
}
