package docroom

import (
	"slices"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// Canon is d in the form that survives the room unchanged: what the room
// cannot tell apart is made the same. A run's marks are a set in a fixed order,
// neighbouring runs with the same marks are one run, a numbered list that
// starts at 1 says nothing about it, and a line break has the marks of the
// text before it. Reading a room gives Canon of what was written, so two docs
// with the same Canon are the same document in the room.
func Canon(d docfmt.Doc) docfmt.Doc {
	out := docfmt.Doc{Tabs: make([]docfmt.Tab, len(d.Tabs))}
	for i, t := range d.Tabs {
		out.Tabs[i] = docfmt.Tab{ID: t.ID, Title: t.Title, Children: t.Children, Blocks: canonBlocks(t.Blocks)}
	}
	return out
}

func canonBlocks(blocks []docfmt.Block) []docfmt.Block {
	out := make([]docfmt.Block, len(blocks))
	for i, b := range blocks {
		b.Content = canonRuns(b.Content)
		if len(b.Blocks) > 0 {
			b.Blocks = canonBlocks(b.Blocks)
		} else {
			b.Blocks = nil
		}
		if b.Type == docfmt.NumberedList && b.Start == 1 {
			b.Start = 0
		}
		b.Items = canonItems(b.Items)
		if len(b.Rows) > 0 {
			rows := make([]docfmt.Row, len(b.Rows))
			for j, r := range b.Rows {
				rows[j].Cells = make([]docfmt.Cell, len(r.Cells))
				for k, c := range r.Cells {
					rows[j].Cells[k] = docfmt.Cell{Align: c.Align, Content: canonRuns(c.Content)}
				}
			}
			b.Rows = rows
		} else {
			b.Rows = nil
		}
		out[i] = b
	}
	return out
}

func canonItems(items []docfmt.Item) []docfmt.Item {
	if len(items) == 0 {
		return nil
	}
	out := make([]docfmt.Item, len(items))
	for i, it := range items {
		out[i] = docfmt.Item{Content: canonRuns(it.Content), Checked: it.Checked, Items: canonItems(it.Items)}
	}
	return out
}

// canonRuns puts the marks of every run in order, lets a line break take the
// marks of the text before it (after it, at the start of a line), and merges
// neighbours that now have the same marks.
func canonRuns(runs []docfmt.Inline) []docfmt.Inline {
	var pieces []docfmt.Inline
	var breaks []int
	for _, r := range runs {
		if r.Dropdown != nil {
			pieces = append(pieces, docfmt.Inline{Dropdown: &docfmt.Dropdown{Options: slices.Clone(r.Dropdown.Options), Value: r.Dropdown.Value}})
			continue
		}
		if r.Mention() || r.Date != "" {
			pieces = append(pieces, docfmt.Inline{User: r.User, Name: r.Name, Date: r.Date})
			continue
		}
		var marks []string
		for _, m := range markOrder {
			if slices.Contains(r.Marks, m) {
				marks = append(marks, m)
			}
		}
		href := ""
		if slices.Contains(marks, docfmt.MarkLink) {
			href = r.Href
		}
		for i, part := range strings.Split(r.Text, "\n") {
			if i > 0 {
				breaks = append(breaks, len(pieces))
				pieces = append(pieces, docfmt.Inline{Text: "\n"})
			}
			if part != "" {
				pieces = append(pieces, docfmt.Inline{Text: part, Marks: marks, Href: href})
			}
		}
	}
	if len(pieces) == 0 {
		return nil
	}
	return settle(pieces, breaks)
}

// FirstChanged is the id of the first top-level block of after that is new or
// different from the block with that id in before, or "" when there is none.
func FirstChanged(before, after docfmt.Doc) string {
	old := map[string]string{}
	for _, t := range before.Tabs {
		for _, b := range t.Blocks {
			old[b.ID] = keyOf(b)
		}
	}
	for _, t := range after.Tabs {
		for _, b := range t.Blocks {
			if was, ok := old[b.ID]; !ok || was != keyOf(b) {
				return b.ID
			}
		}
	}
	return ""
}
