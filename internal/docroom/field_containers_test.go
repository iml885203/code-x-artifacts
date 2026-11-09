package docroom_test

import (
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/reearth/ygo/crdt"
)

func containerFields(kind, container string, next, peer bool) docfmt.Doc {
	value := "2026-10-24"
	if next {
		value = "2026-10-25"
	}
	field := docfmt.Inline{Date: value}
	if kind == "dropdown" {
		value = "A"
		if next {
			value = "B"
		}
		options := []string{"A", "B"}
		if next {
			options = append(options, "C")
		}
		field = docfmt.Inline{Dropdown: &docfmt.Dropdown{Options: options, Value: value}}
	}
	prefix := "Due "
	if peer {
		prefix += "peer "
	}
	runs := []docfmt.Inline{{Text: prefix}, field, {Text: " now"}}
	block := docfmt.Block{ID: "b1", Type: container, Content: runs}
	switch container {
	case docfmt.Heading:
		block.Level = 2
	case docfmt.BulletList, docfmt.NumberedList, docfmt.TaskList, "nested":
		block.Content = nil
		block.Items = []docfmt.Item{{Content: runs}, {Content: runs}, {Content: []docfmt.Inline{{Text: "unchanged item"}}}}
		if container == docfmt.TaskList {
			checked := false
			for i := range block.Items {
				block.Items[i].Checked = &checked
			}
		}
		if container == "nested" {
			block.Type = docfmt.BulletList
			block.Items[1].Items = []docfmt.Item{{Content: runs, Items: []docfmt.Item{{Content: runs}}}}
		}
	case docfmt.Table:
		block.Content = nil
		block.Header = true
		block.Rows = []docfmt.Row{{Cells: []docfmt.Cell{{Content: runs}, {Align: "right", Content: runs}}}, {Cells: []docfmt.Cell{{Content: runs}, {Content: runs}}}}
	}
	return paragraphDoc(block, para("b2", "untouched"))
}

func containerNodes(t *testing.T, doc *crdt.Doc) []any {
	t.Helper()
	tab, ok := docroom.Open(doc).TabElement("t1")
	if !ok {
		t.Fatal("missing tab t1")
	}
	var nodes []any
	var visit func(*crdt.YXmlElement)
	visit = func(e *crdt.YXmlElement) {
		nodes = append(nodes, e)
		for _, child := range e.Children() {
			if el, ok := child.(*crdt.YXmlElement); ok {
				visit(el)
			} else {
				nodes = append(nodes, child)
			}
		}
	}
	visit(tab)
	return nodes
}

func TestFieldChangesInContainersPreserveConcurrentTypingAndSharedNodes(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"date", "dropdown"} {
		for _, container := range []string{docfmt.Paragraph, docfmt.Heading, docfmt.BulletList, docfmt.NumberedList, docfmt.TaskList, "nested", docfmt.Table} {
			t.Run(kind+"/"+container, func(t *testing.T) {
				t.Parallel()
				server := roomHolding(t, containerFields(kind, container, false, false), 1)
				client := overTheWire(t, server)
				before := containerNodes(t, server)
				for _, node := range containerNodes(t, client) {
					if text, ok := node.(*crdt.YXmlText); ok && text.ToString() == "Due " {
						typeAt(client, text, 4, "peer ")
					}
				}
				change(t, server, containerFields(kind, container, true, false))
				after := containerNodes(t, server)
				if len(before) != len(after) {
					t.Fatalf("shared node count changed: %d to %d", len(before), len(after))
				}
				for i := range before {
					if before[i] != after[i] {
						t.Errorf("shared node %d replaced", i)
					}
				}
				merge(t, server, client)
				want := jsonOf(t, docroom.Canon(containerFields(kind, container, true, true)))
				for _, doc := range []*crdt.Doc{server, client} {
					if got := jsonOf(t, read(t, doc)); got != want {
						t.Errorf("merged document:\n got %s\nwant %s", got, want)
					}
				}
			})
		}
	}
}

func TestFieldContainerChangesFallBackForLaterStructuralEdits(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"date", "dropdown"} {
		for _, container := range []string{docfmt.BulletList, "nested", docfmt.Table} {
			t.Run(kind+"/"+container, func(t *testing.T) {
				t.Parallel()
				server := roomHolding(t, containerFields(kind, container, false, false), 1)
				next := containerFields(kind, container, true, false)
				block := &next.Tabs[0].Blocks[0]
				switch container {
				case docfmt.BulletList:
					block.Items[1].Content = []docfmt.Inline{{Text: "replacement"}}
				case "nested":
					block.Items[1].Items[0].Items = append(block.Items[1].Items[0].Items, docfmt.Item{Content: []docfmt.Inline{{Text: "added"}}})
				case docfmt.Table:
					block.Rows[1].Cells[1].Align = "left"
					block.Rows[1].Cells[1].Content = []docfmt.Inline{{Text: "replacement"}}
				}
				if err := docfmt.Validate(jsonOf(t, next)); err != nil {
					t.Fatal(err)
				}
				change(t, server, next)
				want := jsonOf(t, docroom.Canon(next))
				for _, doc := range []*crdt.Doc{server, overTheWire(t, server)} {
					if got := jsonOf(t, read(t, doc)); got != want {
						t.Errorf("structural edit:\n got %s\nwant %s", got, want)
					}
				}
			})
		}
	}
}
