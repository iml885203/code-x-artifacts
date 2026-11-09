package docroom_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
)

func TestBlocksToTheRoomAndBackGiveTheSameJSONForEveryCase(t *testing.T) {
	t.Parallel()
	for name, d := range cases(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			doc := roomHolding(t, d, 1)
			if got, want := jsonOf(t, read(t, doc)), jsonOf(t, d); got != want {
				t.Errorf("read back:\n got: %s\nwant: %s", got, want)
			}
			// What a peer holds after the sync is the same document.
			if got, want := jsonOf(t, read(t, overTheWire(t, doc))), jsonOf(t, d); got != want {
				t.Errorf("a peer reads:\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

// The cases cover every block type and every mark; if a type is added to
// docfmt without a case, this says so.
func TestTheCasesHoldEveryBlockTypeAndMark(t *testing.T) {
	t.Parallel()
	types := map[string]bool{}
	marks := map[string]bool{}
	mentions := 0
	var walk func(blocks []docfmt.Block)
	runs := func(list []docfmt.Inline) {
		for _, r := range list {
			if r.Mention() {
				mentions++
			}
			for _, m := range r.Marks {
				marks[m] = true
			}
		}
	}
	walk = func(blocks []docfmt.Block) {
		for _, b := range blocks {
			types[b.Type] = true
			runs(b.Content)
			walk(b.Blocks)
			var items func([]docfmt.Item)
			items = func(list []docfmt.Item) {
				for _, it := range list {
					runs(it.Content)
					items(it.Items)
				}
			}
			items(b.Items)
			for _, r := range b.Rows {
				for _, c := range r.Cells {
					runs(c.Content)
				}
			}
		}
	}
	for _, d := range cases(t) {
		for _, tab := range d.Tabs {
			walk(tab.Blocks)
		}
	}
	for _, typ := range []string{docfmt.Paragraph, docfmt.Heading, docfmt.BulletList, docfmt.NumberedList, docfmt.TaskList, docfmt.Quote, docfmt.Code, docfmt.Rule, docfmt.Table, docfmt.Image, docfmt.Pending} {
		if !types[typ] {
			t.Errorf("no case holds a %s block", typ)
		}
	}
	if mentions < 11 {
		t.Errorf("the cases hold %d mentions; they should hold one in a paragraph, a heading, a list item, a nested item and a cell, next to marked text and after a line break", mentions)
	}
	for _, m := range []string{docfmt.MarkBold, docfmt.MarkItalic, docfmt.MarkUnderline, docfmt.MarkStrike, docfmt.MarkCode, docfmt.MarkHighlight, docfmt.MarkLink} {
		if !marks[m] {
			t.Errorf("no case holds the %s mark", m)
		}
	}
}

func TestWhatTheRoomCannotTellApartIsReadBackAsOneForm(t *testing.T) {
	t.Parallel()
	loose := docfmt.Doc{Tabs: []docfmt.Tab{{ID: "t1", Title: "T", Blocks: []docfmt.Block{
		{ID: "b1", Type: docfmt.Paragraph, Content: []docfmt.Inline{
			{Text: "a ", Marks: []string{docfmt.MarkItalic, docfmt.MarkBold}},
			{Text: "b", Marks: []string{docfmt.MarkBold, docfmt.MarkItalic}},
			{Text: "\nc", Marks: []string{docfmt.MarkStrike}},
		}},
		{ID: "b2", Type: docfmt.NumberedList, Start: 1, Items: []docfmt.Item{{Content: []docfmt.Inline{{Text: "x"}}}}},
	}}}}
	got := read(t, roomHolding(t, loose, 1))
	canon := docroom.Canon(loose)
	if g, w := jsonOf(t, got), jsonOf(t, canon); g != w {
		t.Fatalf("read back:\n got: %s\nwant: %s", g, w)
	}
	runs := got.Tabs[0].Blocks[0].Content
	bi := []string{docfmt.MarkBold, docfmt.MarkItalic}
	if len(runs) != 2 || runs[0].Text != "a b\n" || !slices.Equal(runs[0].Marks, bi) || runs[1].Text != "c" || !slices.Equal(runs[1].Marks, []string{docfmt.MarkStrike}) {
		t.Errorf("marks are a set in a fixed order, a break has the marks of the text before it and runs that match are one: %+v", runs)
	}
	if got.Tabs[0].Blocks[1].Start != 0 {
		t.Errorf("a list that starts at 1 says nothing about it, got start %d", got.Tabs[0].Blocks[1].Start)
	}
}

func TestCanonDoesNotChangeWhatIsAlreadyCanonicalAndIsStable(t *testing.T) {
	t.Parallel()
	for name, d := range cases(t) {
		once := docroom.Canon(d)
		if jsonOf(t, once) != jsonOf(t, d) {
			t.Errorf("%s: the case is not canonical", name)
		}
		if jsonOf(t, docroom.Canon(once)) != jsonOf(t, once) {
			t.Errorf("%s: Canon is not stable", name)
		}
	}
}

func TestTextWithEmojiAndCJKKeepsItsPlaceWhenEditedInTheMiddle(t *testing.T) {
	t.Parallel()
	d := docfmt.Doc{Tabs: []docfmt.Tab{{ID: "t1", Title: "T", Blocks: []docfmt.Block{
		{ID: "b1", Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: "😀 世界 "}, {Text: "𠮷野家", Marks: []string{docfmt.MarkBold}}, {Text: " end"}}},
	}}}}
	if got, want := jsonOf(t, read(t, roomHolding(t, d, 1))), jsonOf(t, d); got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestReadingARoomThatHoldsSomethingTheSchemaHasNotGotNamesWhere(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		build func(*roomBuilder)
		want  string
	}{
		"an unknown block":      {func(b *roomBuilder) { b.tab().child("video", "b1") }, "tabs[0].blocks[0]: <video> is not a block of a doc"},
		"a block without an id": {func(b *roomBuilder) { b.tab().child("paragraph", "") }, "tabs[0].blocks[0]: <paragraph> has no id"},
		"an unknown mark":       {func(b *roomBuilder) { b.tab().child("paragraph", "b1").text("x", "blink") }, "tabs[0].blocks[0]: \"blink\" is not a mark of a doc"},
		"a link without a href": {func(b *roomBuilder) { b.tab().child("paragraph", "b1").text("x", "link") }, "tabs[0].blocks[0]: a link has no href"},
		"text outside a block":  {func(b *roomBuilder) { b.tab().stray() }, "tabs[0].blocks"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := newRoomBuilder()
			c.build(b)
			_, err := docroom.Open(b.doc).Read()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want an error with %q", err, c.want)
			}
		})
	}
}
