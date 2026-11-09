package docedit

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// Tab operations (docs/doc-edit.md). A tab op names its tab with Tab, never
// with ID, and has no IfHash: the guard is for blocks.
const (
	TabAdd    = "tab_add"
	TabRename = "tab_rename"
	TabDelete = "tab_delete"
	TabMove   = "tab_move"
)

// TabRef is a tab as a read lists it: its id and its title.
type TabRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Tabs lists the tabs of a doc, in order.
func Tabs(d docfmt.Doc) []TabView {
	out := make([]TabView, len(d.Tabs))
	for i, t := range d.Tabs {
		out[i] = TabView{ID: t.ID, Title: t.Title}
	}
	return out
}

func tabRefs(d docfmt.Doc) []TabRef {
	out := make([]TabRef, len(d.Tabs))
	for i, t := range d.Tabs {
		out[i] = TabRef{ID: t.ID, Title: t.Title}
	}
	return out
}

// MarshalJSON leaves "blocks" out of a tab that was listed without them, and
// keeps it (empty or not) for a tab that was read as blocks.
func (t TabView) MarshalJSON() ([]byte, error) {
	out := struct {
		ID     string    `json:"id"`
		Title  string    `json:"title"`
		Blocks *[]Hashed `json:"blocks,omitempty"`
	}{ID: t.ID, Title: t.Title}
	if t.Blocks != nil {
		out.Blocks = &t.Blocks
	}
	return json.Marshal(out)
}

func isTabOp(op string) bool {
	return op == TabAdd || op == TabRename || op == TabDelete || op == TabMove
}

// doTab does one tab operation on the doc.
func doTab(doc *docfmt.Doc, op Op, i int, newID func(string) string, res *Result, written map[string]bool) error {
	if op.ID != "" {
		return refuse(i, "%s names its tab with tab, not id (a block id is not a tab id)", op.Op)
	}
	if op.IfHash != "" {
		return refuse(i, "%s has no ifHash: the guard is for blocks", op.Op)
	}
	switch op.Op {
	case TabAdd:
		return addTab(doc, op, i, newID, written)
	case TabRename:
		at, err := tabAt(doc, op.Tab, i)
		if err != nil {
			return err
		}
		title, err := tabTitle(op.Title, i)
		if err != nil {
			return err
		}
		doc.Tabs[at].Title = title
	case TabDelete:
		at, err := tabAt(doc, op.Tab, i)
		if err != nil {
			return err
		}
		if len(doc.Tabs) == 1 {
			return refuse(i, "tab %q is the last tab, and a doc needs at least one; add another tab first or delete the doc", op.Tab)
		}
		res.Deleted = append(res.Deleted, tabBlockIDs(doc.Tabs[at])...)
		doc.Tabs = slices.Delete(doc.Tabs, at, at+1)
	case TabMove:
		return moveTab(doc, op, i)
	}
	return nil
}

func tabAt(doc *docfmt.Doc, id string, i int) (int, error) {
	if id == "" {
		return 0, refuse(i, "name the tab with tab (its id, from the read)")
	}
	at := slices.IndexFunc(doc.Tabs, func(t docfmt.Tab) bool { return t.ID == id })
	if at < 0 {
		return 0, refuse(i, "there is no tab %q; the tabs are in the read", id)
	}
	return at, nil
}

func tabTitle(raw string, i int) (string, error) {
	title := strings.TrimSpace(raw)
	switch {
	case title == "":
		return "", refuse(i, "a tab needs a title")
	case strings.ContainsAny(title, "\r\n"):
		return "", refuse(i, "a tab title is one line")
	}
	return title, nil
}

// placeFor says at which index a tab goes, from before, after or at: the
// index is in the list as it is after the moved tab (if any) is taken out.
func placeFor(doc *docfmt.Doc, op Op, i int, moving string) (int, error) {
	n := 0
	for _, set := range []bool{op.Before != "", op.After != "", op.At != ""} {
		if set {
			n++
		}
	}
	if n > 1 || (n == 0 && moving != "") {
		return 0, refuse(i, "%s says where with exactly one of before (a tab id), after (a tab id) or at (start or end)", op.Op)
	}
	rest := slices.DeleteFunc(slices.Clone(doc.Tabs), func(t docfmt.Tab) bool { return t.ID == moving })
	indexOf := func(id string) (int, error) {
		if id == moving {
			return 0, refuse(i, "a tab cannot be moved next to itself")
		}
		at := slices.IndexFunc(rest, func(t docfmt.Tab) bool { return t.ID == id })
		if at < 0 {
			return 0, refuse(i, "there is no tab %q; the tabs are in the read", id)
		}
		return at, nil
	}
	switch {
	case op.Before != "":
		return indexOf(op.Before)
	case op.After != "":
		at, err := indexOf(op.After)
		return at + 1, err
	case op.At == "start":
		return 0, nil
	case op.At == "end", n == 0:
		return len(rest), nil
	}
	return 0, refuse(i, "at is %q or %q", "start", "end")
}

func addTab(doc *docfmt.Doc, op Op, i int, newID func(string) string, written map[string]bool) error {
	title, err := tabTitle(op.Title, i)
	if err != nil {
		return err
	}
	at, err := placeFor(doc, op, i, "")
	if err != nil {
		return err
	}
	tab := docfmt.Tab{ID: newID("t_"), Title: title}
	if op.Markdown == "" && (len(op.Blocks) == 0 || string(op.Blocks) == "null") {
		// What a new tab starts with: an empty H1 to type the title of the page in.
		tab.Blocks = []docfmt.Block{{ID: newID("b_"), Type: docfmt.Heading, Level: 1}}
	} else if tab.Blocks, err = newBlocks(op, i, "", newID); err != nil {
		return err
	}
	for _, b := range tab.Blocks {
		written[b.ID] = true
	}
	doc.Tabs = slices.Insert(doc.Tabs, at, tab)
	return nil
}

func moveTab(doc *docfmt.Doc, op Op, i int) error {
	from, err := tabAt(doc, op.Tab, i)
	if err != nil {
		return err
	}
	to, err := placeFor(doc, op, i, op.Tab)
	if err != nil {
		return err
	}
	tab := doc.Tabs[from]
	doc.Tabs = slices.Insert(slices.Delete(doc.Tabs, from, from+1), to, tab)
	return nil
}

// tabBlockIDs are the ids of every block in a tab, nested ones too.
func tabBlockIDs(t docfmt.Tab) []string {
	var ids []string
	var walk func(blocks []docfmt.Block)
	walk = func(blocks []docfmt.Block) {
		for _, b := range blocks {
			ids = append(ids, b.ID)
			walk(b.Blocks)
		}
	}
	walk(t.Blocks)
	return ids
}
