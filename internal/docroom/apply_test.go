package docroom_test

import (
	"slices"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
)

// change applies next to a room the way Hub.Edit does: plan, then write.
func change(t *testing.T, doc *crdt.Doc, next docfmt.Doc) {
	t.Helper()
	write, err := docroom.Open(doc).Apply(next)
	if err != nil {
		t.Fatalf("plan the change: %v", err)
	}
	doc.Transact(write)
}

// merge exchanges everything the two documents hold, as a sync does.
func merge(t *testing.T, a, b *crdt.Doc) {
	t.Helper()
	for _, p := range [][2]*crdt.Doc{{a, b}, {b, a}} {
		if err := crdt.ApplyUpdateV1(p[1], crdt.EncodeStateAsUpdateV1(p[0], nil), nil); err != nil {
			t.Fatal(err)
		}
	}
}

func paragraphDoc(blocks ...docfmt.Block) docfmt.Doc {
	return docfmt.Doc{Tabs: []docfmt.Tab{{ID: "t1", Title: "T", Blocks: blocks}}}
}

func para(id, text string) docfmt.Block {
	return docfmt.Block{ID: id, Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: text}}}
}

func listOf(id string, items ...string) docfmt.Block {
	b := docfmt.Block{ID: id, Type: docfmt.BulletList}
	for _, s := range items {
		b.Items = append(b.Items, docfmt.Item{Content: []docfmt.Inline{{Text: s}}})
	}
	return b
}

// textOf finds the text of the block with this id (the item'th item of a list
// when item >= 0) in the document, to type into it as a person would.
func textOf(t *testing.T, doc *crdt.Doc, id string, item int) *crdt.YXmlText {
	t.Helper()
	tab, _ := docroom.Open(doc).TabElement("t1")
	for _, child := range tab.Children() {
		e := child.(*crdt.YXmlElement)
		if v, _ := e.GetAttribute("id"); v != id {
			continue
		}
		if item >= 0 {
			e = e.Children()[item].(*crdt.YXmlElement).Children()[0].(*crdt.YXmlElement)
		}
		return e.Children()[0].(*crdt.YXmlText)
	}
	t.Fatalf("no block %s", id)
	return nil
}

func typeAt(doc *crdt.Doc, text *crdt.YXmlText, at int, s string) {
	doc.Transact(func(txn *crdt.Transaction) { text.Insert(txn, at, s, nil) })
}

func TestApplyTurnsAnyDocIntoAnyOtherAndReadsBackTheSame(t *testing.T) {
	t.Parallel()
	cs := cases(t)
	for from, a := range cs {
		for to, b := range cs {
			t.Run(from+" to "+to, func(t *testing.T) {
				t.Parallel()
				doc := roomHolding(t, a, 1)
				change(t, doc, b)
				want := jsonOf(t, docroom.Canon(b))
				if got := jsonOf(t, read(t, doc)); got != want {
					t.Errorf("read back:\n got: %s\nwant: %s", got, want)
				}
				if got := jsonOf(t, read(t, overTheWire(t, doc))); got != want {
					t.Errorf("a peer reads:\n got: %s\nwant: %s", got, want)
				}
			})
		}
	}
}

func TestApplyChangesOnlyTheTextThatDiffersAndMovesNothingElse(t *testing.T) {
	t.Parallel()
	a := paragraphDoc(para("b1", "first"), para("b2", "second"), para("b3", "third"))
	b := paragraphDoc(para("b1", "first"), para("b2", "second, changed"), para("b3", "third"))
	server := roomHolding(t, a, 1)
	client := overTheWire(t, server)

	// A person is typing in the paragraph before and in the one after.
	typeAt(client, textOf(t, client, "b1", -1), 5, " (typed)")
	typeAt(client, textOf(t, client, "b3", -1), 0, "typed: ")
	change(t, server, b)
	merge(t, server, client)

	for name, doc := range map[string]*crdt.Doc{"server": server, "client": client} {
		got := read(t, doc).Tabs[0].Blocks
		want := []string{"first (typed)", "second, changed", "typed: third"}
		for i, w := range want {
			if text := got[i].Content[0].Text; text != w {
				t.Errorf("%s block %d = %q, want %q", name, i+1, text, w)
			}
		}
	}
}

func TestApplyEditsTextInPlaceSoWhatIsTypedOutsideTheChangedStretchSurvives(t *testing.T) {
	t.Parallel()
	server := roomHolding(t, paragraphDoc(para("b1", "Hello world")), 1)
	client := overTheWire(t, server)
	typeAt(client, textOf(t, client, "b1", -1), 11, "!")
	change(t, server, paragraphDoc(para("b1", "Howdy world")))
	merge(t, server, client)
	if got := read(t, server).Tabs[0].Blocks[0].Content[0].Text; got != "Howdy world!" {
		t.Errorf("got %q, want the change and the typed character together", got)
	}
}

func TestApplyKeepsTheItemsOfAListNobodyChangedWhileAPersonTypesInOne(t *testing.T) {
	t.Parallel()
	server := roomHolding(t, paragraphDoc(listOf("l1", "one", "two", "three")), 1)
	client := overTheWire(t, server)
	typeAt(client, textOf(t, client, "l1", 1), 3, " (typed)")
	change(t, server, paragraphDoc(listOf("l1", "ONE", "two", "three")))
	merge(t, server, client)
	items := read(t, server).Tabs[0].Blocks[0].Items
	if len(items) != 3 || items[0].Content[0].Text != "ONE" || items[1].Content[0].Text != "two (typed)" || items[2].Content[0].Text != "three" {
		t.Errorf("items = %+v", items)
	}
}

func TestApplyCountsInUTF16SoEmojiAndCJKStayWhereTheyAre(t *testing.T) {
	t.Parallel()
	doc := roomHolding(t, paragraphDoc(para("b1", "😀 世界 𠮷野家 end")), 1)
	change(t, doc, paragraphDoc(para("b1", "😀 世界 𠮷野家 fin 🎉")))
	if got := read(t, doc).Tabs[0].Blocks[0].Content[0].Text; got != "😀 世界 𠮷野家 fin 🎉" {
		t.Errorf("got %q", got)
	}
	change(t, doc, paragraphDoc(para("b1", "世界 𠮷野家 fin")))
	if got := read(t, doc).Tabs[0].Blocks[0].Content[0].Text; got != "世界 𠮷野家 fin" {
		t.Errorf("got %q", got)
	}
}

func TestApplyKeepsTheIdsOfBlocksThatStayAndReordersAndRemovesTheRest(t *testing.T) {
	t.Parallel()
	doc := roomHolding(t, paragraphDoc(para("a", "A"), para("b", "B"), para("c", "C"), para("d", "D")), 1)
	change(t, doc, paragraphDoc(para("c", "C"), para("a", "A"), para("n", "new"), para("d", "D")))
	var ids []string
	for _, b := range read(t, doc).Tabs[0].Blocks {
		ids = append(ids, b.ID)
	}
	if want := []string{"c", "a", "n", "d"}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestApplyOfWhatTheRoomAlreadyHoldsWritesNothing(t *testing.T) {
	t.Parallel()
	d := cases(t)["everything"]
	doc := roomHolding(t, d, 1)
	var updates int
	doc.OnUpdate(func(u []byte, _ any) {
		if len(u) > 2 {
			updates++
		}
	})
	change(t, doc, d)
	if updates != 0 {
		t.Errorf("%d updates for a change that changes nothing", updates)
	}
}

func TestAnAgentChangesAnImagesAltAndSizeInPlaceAndTheRoomReadsThemBack(t *testing.T) {
	t.Parallel()
	image := docfmt.Block{ID: "i", Type: docfmt.Image, Src: "/files/abc/p.png"}
	doc := roomHolding(t, paragraphDoc(para("p", "text"), image), 1)
	image.Alt, image.Size = "A chart", "xlarge"
	change(t, doc, paragraphDoc(para("p", "text"), image))
	got := read(t, doc).Tabs[0].Blocks[1]
	if got.Alt != "A chart" || got.Size != "xlarge" || got.ID != "i" || got.Src != "/files/abc/p.png" {
		t.Fatalf("the image after the change: %+v", got)
	}
	image.Size = ""
	change(t, doc, paragraphDoc(para("p", "text"), image))
	if got := read(t, doc).Tabs[0].Blocks[1]; got.Size != "" || got.Alt != "A chart" {
		t.Fatalf("taking the size off: %+v", got)
	}
}

func TestApplyAddsAndRemovesAMentionAndReadsBackTheSame(t *testing.T) {
	t.Parallel()
	server := roomHolding(t, paragraphDoc(para("b1", "Ask now")), 1)
	anna := docfmt.Inline{User: "anna.lee", Name: "Anna Lee"}
	withMention := docfmt.Block{ID: "b1", Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: "Ask "}, anna, {Text: " now"}}}
	change(t, server, paragraphDoc(withMention))
	got := read(t, server).Tabs[0].Blocks[0].Content
	want := []docfmt.Inline{{Text: "Ask "}, anna, {Text: " now"}}
	if !slices.EqualFunc(got, want, func(a, b docfmt.Inline) bool { return a.Text == b.Text && a.User == b.User && a.Name == b.Name }) {
		t.Errorf("got %+v, want the mention in the middle", got)
	}
	change(t, server, paragraphDoc(para("b1", "Ask now")))
	if got := read(t, server).Tabs[0].Blocks[0].Content; len(got) != 1 || got[0].Text != "Ask now" {
		t.Errorf("after the mention is removed: %+v", got)
	}
}
