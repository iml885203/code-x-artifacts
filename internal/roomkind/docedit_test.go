package roomkind_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

const editable = `{"tabs":[{"id":"t1","title":"Doc","blocks":[
 {"id":"h","type":"heading","level":1,"content":[{"text":"Plan"}]},
 {"id":"p1","type":"paragraph","content":[{"text":"first"}]},
 {"id":"q","type":"quote","blocks":[
   {"id":"qa","type":"paragraph","content":[{"text":"quoted a"}]},
   {"id":"qb","type":"paragraph","content":[{"text":"quoted b"}]}]},
 {"id":"p2","type":"paragraph","content":[{"text":"second"}]}
]}]}`

// editableDoc publishes the doc above as alice.
func (e *env) editableDoc() string {
	e.t.Helper()
	p, err := e.svc.Create(e.t.Context(), e.svc.Viewer("alice"), service.PublishInput{Title: "Doc", Kind: "doc", Blocks: json.RawMessage(editable)})
	if err != nil {
		e.t.Fatal(err)
	}
	return p.ID
}

func (e *env) docEdit(id string, in service.DocEditInput) (service.DocEdited, error) {
	e.t.Helper()
	return e.svc.DocEdit(e.t.Context(), e.svc.Viewer("alice"), id, in)
}

func replace(id, markdown string) docedit.Op {
	return docedit.Op{Op: docedit.Replace, ID: id, Markdown: markdown}
}

// blockHash is the hash a read gives a top-level block of the room.
func (e *env) blockHash(id, block string) string {
	e.t.Helper()
	for _, b := range e.roomBlocks(id) {
		if b.ID == block {
			return docedit.Hash(b)
		}
	}
	e.t.Fatalf("no block %s", block)
	return ""
}

func TestDocEditGivesTheSameResultWithAndWithoutARoomExceptWhenTheVersionIsCut(t *testing.T) {
	t.Parallel()
	ops := []docedit.Op{
		replace("p1", "changed 你好 😀"),
		{Op: docedit.Insert, After: "h", Blocks: json.RawMessage(`[{"id":"new","type":"image","src":"https://example.com/a.png"}]`)},
		{Op: docedit.Delete, ID: "p2"},
	}
	without := newEnv(t)
	idA := without.editableDoc()
	a, err := without.docEdit(idA, service.DocEditInput{Ops: ops})
	if err != nil {
		t.Fatal(err)
	}

	with := newEnv(t)
	idB := with.editableDoc()
	_ = with.roomText(idB) // the room is open
	b, err := with.docEdit(idB, service.DocEditInput{Ops: ops})
	if err != nil {
		t.Fatal(err)
	}

	if x, y := mustJSON(t, a.Result), mustJSON(t, b.Result); x != y {
		t.Errorf("the result differs:\n without a room: %s\n    with a room: %s", x, y)
	}
	if !a.Saved || a.Version != 2 {
		t.Errorf("without a room: saved %v version %d, want a new version now", a.Saved, a.Version)
	}
	if b.Saved || b.Version != 1 {
		t.Errorf("with a room: saved %v version %d, want the edit in the room and a version later", b.Saved, b.Version)
	}
	if len(a.Review) != 1 {
		t.Errorf("review = %v, want the image without alt", a.Review)
	}
	// The same document either way, once the room has been quiet.
	with.clock.Advance(roomkind.Quiet)
	with.sweep()
	if n := len(with.versions(idB)); n != 2 {
		t.Fatalf("%d versions with a room after the pause, want 2", n)
	}
	va, _ := without.st.Version(t.Context(), idA, 2)
	vb, _ := with.st.Version(t.Context(), idB, 2)
	if va.Content != vb.Content {
		t.Errorf("the saved docs differ:\n%s\n%s", va.Content, vb.Content)
	}
	if !slices.Equal(vb.Authors, []string{"alice"}) {
		t.Errorf("the agent's edit is by %v", vb.Authors)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEveryRefusalOfDocEditStoresNothingAndChangesNoRoom(t *testing.T) {
	t.Parallel()
	for name, open := range map[string]bool{"without a room": false, "with a room": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			id := e.editableDoc()
			if open {
				_ = e.roomText(id)
			}
			before := e.versions(id)
			for what, in := range map[string]service.DocEditInput{
				"an unknown id":       {Ops: []docedit.Op{replace("p1", "x"), {Op: docedit.Delete, ID: "nope"}}},
				"a stale ifHash":      {Ops: []docedit.Op{{Op: docedit.Delete, ID: "p1", IfHash: "000000000000"}}},
				"an invalid result":   {Ops: []docedit.Op{replace("p1", "[x](javascript:alert(1))")}},
				"a duplicate id":      {Ops: []docedit.Op{{Op: docedit.Insert, After: "h", Blocks: json.RawMessage(`[{"id":"p2","type":"rule"}]`)}}},
				"a stale baseVersion": {BaseVersion: 9, Ops: []docedit.Op{replace("p1", "x")}},
			} {
				if _, err := e.docEdit(id, in); err == nil {
					t.Errorf("%s was accepted", what)
				}
			}
			if after := e.versions(id); len(after) != len(before) {
				t.Errorf("a refusal stored a version: %d, was %d", len(after), len(before))
			}
			if open {
				if got := e.roomText(id); !slices.Equal(got, []string{"first", "second"}) {
					t.Errorf("a refusal changed the room: %v", got)
				}
			}
		})
	}
}

func TestAStaleHashWithARoomOpenCarriesTheStateAPersonMadeAndChangesNothing(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.editableDoc()
	c := e.connect(id, bob)
	read := e.blockHash(id, "p1") // what the agent read

	c.typeParagraph("a person types a new paragraph")
	err := e.hub.Edit(t.Context(), id, bob, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return replaceText(doc, "p1", "first, typed on by bob"), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.docEdit(id, service.DocEditInput{Ops: []docedit.Op{{Op: docedit.Replace, ID: "p1", IfHash: read, Markdown: "agent's version"}}})
	var refusal *service.Error
	if !errors.As(err, &refusal) || refusal.Code != service.Conflict || len(refusal.Stale) != 1 {
		t.Fatalf("err = %v, want a conflict with the stale block", err)
	}
	if got := refusal.Stale[0]; got.ID != "p1" || got.Hash != e.blockHash(id, "p1") || got.Content[0].Text != "first, typed on by bob" {
		t.Errorf("the stale state is %+v", got)
	}
	if got := e.roomText(id); got[0] != "first, typed on by bob" {
		t.Errorf("the room holds %v", got)
	}
}

// replaceText is a person's edit of the text of a top-level paragraph.
func replaceText(doc *crdt.Doc, id, text string) func(*crdt.Transaction) {
	d, err := docroom.Open(doc).Read()
	if err != nil {
		panic(err)
	}
	for i := range d.Tabs[0].Blocks {
		if d.Tabs[0].Blocks[i].ID == id {
			d.Tabs[0].Blocks[i].Content = []docfmt.Inline{{Text: text}}
		}
	}
	write, err := docroom.Open(doc).Apply(d)
	if err != nil {
		panic(err)
	}
	return write
}

func TestAnAgentsReplaceOfOneBlockLeavesWhatAPersonTypesInAnotherPartOfTheSameBlockAndInOtherBlocks(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.editableDoc()
	person := e.connect(id, bob)

	// Bob types at the end of the second paragraph of the quote and in p2 while the agent works.
	typeInto := func(block string, inner int, text string) {
		doc := person.doc
		tab := firstTab(doc)
		for _, child := range tab.Children() {
			el := child.(*crdt.YXmlElement)
			if v, _ := el.GetAttribute("id"); v != block {
				continue
			}
			if inner >= 0 {
				el = el.Children()[inner].(*crdt.YXmlElement)
			}
			run := el.Children()[0].(*crdt.YXmlText)
			doc.Transact(func(txn *crdt.Transaction) { run.Insert(txn, run.Len(), text, nil) })
		}
	}
	person.pump()
	typeInto("q", 1, " + bob")
	typeInto("p2", -1, " + bob")
	person.pump()

	// The agent replaces the first paragraph of the quote and p1, not what bob is in.
	if _, err := e.docEdit(id, service.DocEditInput{Ops: []docedit.Op{replace("qa", "agent a"), replace("p1", "agent p1")}}); err != nil {
		t.Fatal(err)
	}
	person.pump()
	d := readDocOf(t, e, id)
	var texts []string
	for _, b := range d.Tabs[0].Blocks {
		if b.Type == docfmt.Quote {
			for _, in := range b.Blocks {
				texts = append(texts, in.Content[0].Text)
			}
			continue
		}
		if b.Type == docfmt.Paragraph {
			texts = append(texts, b.Content[0].Text)
		}
	}
	if want := []string{"agent p1", "agent a", "quoted b + bob", "second + bob"}; !slices.Equal(texts, want) {
		t.Errorf("texts = %q, want %q", texts, want)
	}
}

func readDocOf(t *testing.T, e *env, id string) docfmt.Doc {
	t.Helper()
	var d docfmt.Doc
	err := e.hub.Read(t.Context(), id, func(doc *crdt.Doc) (err error) {
		d, err = docroom.Open(doc).Read()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAnAgentsEmojiAndCJKEditSurvivesARestartWithAndWithoutARoom(t *testing.T) {
	t.Parallel()
	for name, open := range map[string]bool{"without a room": false, "with a room": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			id := e.editableDoc()
			if open {
				_ = e.roomText(id)
			}
			if _, err := e.docEdit(id, service.DocEditInput{Ops: []docedit.Op{replace("p1", "你好 😀 𠮷野家")}}); err != nil {
				t.Fatal(err)
			}
			e.restart()
			e.sweep() // the restarted snapshotter sees a room waiting for a version
			e.clock.Advance(roomkind.Quiet)
			e.sweep()
			latest, err := e.st.Artifact(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			d, _ := docfmt.Parse(latest.Content)
			if got := d.Tabs[0].Blocks[1].Content[0].Text; got != "你好 😀 𠮷野家" {
				t.Errorf("after the restart the doc holds %q", got)
			}
			if open {
				if got := e.roomText(id); got[0] != "你好 😀 𠮷野家" {
					t.Errorf("the room holds %v", got)
				}
			}
		})
	}
}

func TestDocEditIsForDocsAndForPeopleWhoMayEditThem(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.editableDoc()
	page, err := e.svc.Create(t.Context(), e.svc.Viewer("alice"), service.PublishInput{Title: "Page", Content: "<p>x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.docEdit(page.ID, service.DocEditInput{Ops: []docedit.Op{{Op: docedit.Delete, ID: "x"}}}); service.CodeOf(err) != service.Invalid {
		t.Errorf("a page: %v", err)
	}
	if _, err := e.svc.DocEdit(t.Context(), e.svc.Viewer("mallory"), id, service.DocEditInput{Ops: []docedit.Op{{Op: docedit.Delete, ID: "p1"}}}); service.CodeOf(err) != service.NotFound {
		t.Errorf("a stranger: %v", err)
	}
	if _, err := e.svc.DocEdit(t.Context(), e.svc.Anonymous(), id, service.DocEditInput{Ops: []docedit.Op{{Op: docedit.Delete, ID: "p1"}}}); err == nil {
		t.Error("an anonymous caller edited a doc")
	}
}

func TestReadingADocWithARoomShowsWhatPeopleHaveTyped(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.editableDoc()
	e.edit(id, bob, "typed by bob")
	read, err := e.svc.ReadDoc(t.Context(), e.svc.Viewer("alice"), id)
	if err != nil {
		t.Fatal(err)
	}
	if read.Artifact.Version != 1 || len(read.Doc.Tabs[0].Blocks) != 5 {
		t.Errorf("version %d with %d blocks: the live content with the saved version number", read.Artifact.Version, len(read.Doc.Tabs[0].Blocks))
	}
}
