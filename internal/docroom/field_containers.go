package docroom

import (
	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/reearth/ygo/crdt"
)

// Container candidates use a separate planner: a later structural difference
// must discard every tentative field update before the replacement path runs.
func (p *planner) fieldItems(list *crdt.YXmlElement, old, next []docfmt.Item) bool {
	els, err := elementsOf(&list.YXmlFragment, "items")
	if err != nil || len(old) != len(next) || len(els) != len(old) {
		return false
	}
	for i, was := range old {
		item := next[i]
		if keyOf(was.Checked) != keyOf(item.Checked) {
			return false
		}
		children, err := elementsOf(&els[i].YXmlFragment, "item")
		count := 1
		if len(was.Items) > 0 {
			count = 2
		}
		if err != nil || len(children) != count || children[0].NodeName != docfmt.Paragraph {
			return false
		}
		if !p.fieldContent(children[0], was.Content, item.Content) {
			return false
		}
		if len(was.Items) == 0 {
			if len(item.Items) != 0 {
				return false
			}
		} else if !p.fieldItems(children[1], was.Items, item.Items) {
			return false
		}
	}
	return true
}

func (p *planner) fieldRows(rows []*crdt.YXmlElement, old, next []docfmt.Row) bool {
	if len(old) != len(next) || len(rows) != len(old) {
		return false
	}
	for i, was := range old {
		cells, err := elementsOf(&rows[i].YXmlFragment, "cells")
		if err != nil || len(was.Cells) != len(next[i].Cells) || len(cells) != len(was.Cells) {
			return false
		}
		for j, cell := range was.Cells {
			target := next[i].Cells[j]
			if cell.Align != target.Align {
				return false
			}
			children, err := elementsOf(&cells[j].YXmlFragment, "cell")
			if err != nil || len(children) != 1 || children[0].NodeName != docfmt.Paragraph {
				return false
			}
			if !p.fieldContent(children[0], cell.Content, target.Content) {
				return false
			}
		}
	}
	return true
}

func (p *planner) fieldContent(e *crdt.YXmlElement, old, next []docfmt.Inline) bool {
	return keyOf(old) == keyOf(next) || p.fields(e, old, next)
}
