package designroom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

// The pages and frames of a design are not one entry but one entry per value,
// so that people restructuring at the same time merge instead of replacing each
// other (docs/design-room.md):
//
//	s|page|<id>|order   the page exists; its place among the pages
//	s|page|<id>|name
//	s|row|<id>|page     the row exists; the page it is on
//	s|row|<id>|order    its place among the page's rows
//	s|frame|<id>|file   the frame exists; the frame's file
//	s|frame|<id>|name, |w, |h
//	s|frame|<id>|row    the row it is in
//	s|frame|<id>|order  its place in the row
//
// Orders are the fractional keys of the node tree (`between`). design.json is
// the projection of these entries: pages, rows and frames by order, a row with
// no frame left is not drawn, and what has lost the thing it hangs on (a frame
// whose row or page was deleted, a frame whose content is gone) is not part of
// the design.
const structPrefix = "s|"

func pageKey(id, field string) string  { return "s|page|" + id + "|" + field }
func rowKey(id, field string) string   { return "s|row|" + id + "|" + field }
func frameKey(id, field string) string { return "s|frame|" + id + "|" + field }

type sPage struct {
	id, name, order string
	live            bool
}

type sRow struct {
	id, page, order string
	live            bool
}

type sFrame struct {
	id, name, file, w, h, row, order string
	live                             bool
}

// projection is the structure the entries hold, read once.
type projection struct {
	pages []pPage
	// orphans are the files of frames that still exist but hang on a row or page that was deleted: not part of the design.
	orphans map[string]bool
	// problems are what a single entry gets wrong, whatever the others say.
	problems []string
}

type pPage struct {
	id, name string
	rows     [][]pFrame
}

type pFrame struct {
	id, name, file string
	w, h           int
}

func (p *projection) hasFile(file string) bool {
	return !p.orphans[file]
}

// entriesOf are the structure entries of a room, by what they describe.
type entriesOf struct {
	pages  map[string]*sPage
	rows   map[string]*sRow
	frames map[string]*sFrame
}

// readStructureEntries sorts the room's structure entries by page, row and
// frame. False when there are none.
func readStructureEntries(e map[string]any) (entriesOf, bool) {
	in := entriesOf{pages: map[string]*sPage{}, rows: map[string]*sRow{}, frames: map[string]*sFrame{}}
	found := false
	for k, v := range e {
		rest, ok := strings.CutPrefix(k, structPrefix)
		if !ok {
			continue
		}
		kindName, rest, ok := strings.Cut(rest, "|")
		if !ok {
			continue
		}
		id, field, ok := strings.Cut(rest, "|")
		if !ok {
			continue
		}
		found = true
		switch kindName {
		case "page":
			in.page(id).set(field, str(v))
		case "row":
			in.row(id).set(field, str(v))
		case "frame":
			in.frame(id).set(field, str(v))
		}
	}
	return in, found
}

func (in entriesOf) page(id string) *sPage {
	if in.pages[id] == nil {
		in.pages[id] = &sPage{id: id}
	}
	return in.pages[id]
}

func (in entriesOf) row(id string) *sRow {
	if in.rows[id] == nil {
		in.rows[id] = &sRow{id: id}
	}
	return in.rows[id]
}

func (in entriesOf) frame(id string) *sFrame {
	if in.frames[id] == nil {
		in.frames[id] = &sFrame{id: id}
	}
	return in.frames[id]
}

func (p *sPage) set(field, v string) {
	switch field {
	case "order":
		p.order, p.live = v, true
	case "name":
		p.name = v
	}
}

func (r *sRow) set(field, v string) {
	switch field {
	case "page":
		r.page, r.live = v, true
	case "order":
		r.order = v
	}
}

func (f *sFrame) set(field, v string) {
	switch field {
	case "file":
		f.file, f.live = v, true
	case "name":
		f.name = v
	case "w":
		f.w = v
	case "h":
		f.h = v
	case "row":
		f.row = v
	case "order":
		f.order = v
	}
}

// projectStructure reads the structure of a room. False when the room keeps
// none per value (a room that has only the older `manifest` entry, or neither).
func projectStructure(e map[string]any) (*projection, bool) {
	in, found := readStructureEntries(e)
	if !found {
		return nil, false
	}
	out := &projection{orphans: map[string]bool{}}
	byRow := in.framesByRow(e, out)
	var pageList []*sPage
	for _, p := range in.pages {
		if p.live {
			out.problems = append(out.problems, pageProblems(p)...)
			pageList = append(pageList, p)
		}
	}
	sort.Slice(pageList, func(i, j int) bool { return less(pageList[i].order, pageList[i].id, pageList[j].order, pageList[j].id) })
	rowsOf := map[string][]*sRow{}
	for _, r := range in.rows {
		if r.live {
			rowsOf[r.page] = append(rowsOf[r.page], r)
		}
	}
	for _, p := range pageList {
		if pp := assemblePage(p, rowsOf[p.id], byRow); len(pp.rows) > 0 {
			out.pages = append(out.pages, pp)
		}
	}
	sort.Strings(out.problems)
	return out, true
}

// framesByRow groups the live frames that have content and a row on a live page,
// reports what is wrong with each frame's own values, and notes the frames whose
// row or page is gone.
func (in entriesOf) framesByRow(e map[string]any, out *projection) map[string][]*sFrame {
	byRow := map[string][]*sFrame{}
	files := map[string]string{}
	for _, f := range in.frames {
		if !f.live {
			continue // what someone wrote to a frame that was deleted
		}
		out.problems = append(out.problems, frameProblems(f, files)...)
		if _, has := e[shellKey(f.file, "root")]; !has {
			continue // a frame with nothing in it cannot be a frame of the design
		}
		if r := in.rows[f.row]; r != nil && r.live && in.pages[r.page] != nil && in.pages[r.page].live {
			byRow[f.row] = append(byRow[f.row], f)
		} else {
			out.orphans[f.file] = true
		}
	}
	return byRow
}

func pageProblems(p *sPage) []string {
	var out []string
	if m := designfmt.IDProblem(p.id); m != "" {
		out = append(out, fmt.Sprintf("page %q: %s", p.id, m))
	}
	if m := designfmt.NameProblem(p.name); m != "" {
		out = append(out, fmt.Sprintf("page %q: %s", p.id, m))
	}
	return out
}

// assemblePage is the page with its rows and frames in order; a row with no frame is left out.
func assemblePage(p *sPage, rows []*sRow, byRow map[string][]*sFrame) pPage {
	pp := pPage{id: p.id, name: p.name}
	sort.Slice(rows, func(i, j int) bool { return less(rows[i].order, rows[i].id, rows[j].order, rows[j].id) })
	for _, r := range rows {
		fs := byRow[r.id]
		sort.Slice(fs, func(i, j int) bool { return less(fs[i].order, fs[i].id, fs[j].order, fs[j].id) })
		var row []pFrame
		for _, f := range fs {
			w, errW := strconv.Atoi(f.w)
			h, errH := strconv.Atoi(f.h)
			if f.name == "" || errW != nil || errH != nil {
				continue // reported by frameProblems when it is wrong, absent while it is still being written
			}
			row = append(row, pFrame{id: f.id, name: f.name, file: f.file, w: w, h: h})
		}
		if len(row) > 0 {
			pp.rows = append(pp.rows, row)
		}
	}
	return pp
}

func less(orderA, idA, orderB, idB string) bool {
	if orderA != orderB {
		return orderA < orderB
	}
	return idA < idB
}

// frameProblems are what is wrong with one frame's own values.
func frameProblems(f *sFrame, files map[string]string) []string {
	var out []string
	add := func(msg string) { out = append(out, fmt.Sprintf("frame %q: %s", f.id, msg)) }
	if m := designfmt.IDProblem(f.id); m != "" {
		add(m)
	}
	if m := designfmt.NameProblem(f.name); m != "" {
		add(m)
	}
	if err := designfmt.CheckFramePath(f.file); err != nil {
		add(err.Error())
	} else if other, dup := files[f.file]; dup {
		add(fmt.Sprintf("file %s is already the file of frame %q", f.file, other))
	} else {
		files[f.file] = f.id
	}
	for _, side := range []struct{ name, text string }{{"w", f.w}, {"h", f.h}} {
		n, err := strconv.Atoi(side.text)
		if m := designfmt.SideProblem(side.name, n, err == nil); m != "" {
			add(m)
		}
	}
	return out
}

// manifest is design.json for the projection, in the form a publisher writes it.
func (p *projection) manifest(tokens map[string]string) string {
	type frame struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		File string `json:"file"`
		W    int    `json:"w"`
		H    int    `json:"h"`
	}
	type page struct {
		ID   string    `json:"id"`
		Name string    `json:"name"`
		Rows [][]frame `json:"rows"`
	}
	doc := struct {
		Version int               `json:"version"`
		Pages   []page            `json:"pages"`
		Tokens  map[string]string `json:"tokens,omitempty"`
	}{Version: designfmt.Version, Pages: []page{}, Tokens: tokens}
	for _, pg := range p.pages {
		out := page{ID: pg.id, Name: pg.name}
		for _, row := range pg.rows {
			var fr []frame
			for _, f := range row {
				fr = append(fr, frame{f.id, f.name, f.file, f.w, f.h})
			}
			out.Rows = append(out.Rows, fr)
		}
		doc.Pages = append(doc.Pages, out)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(doc) // plain strings and numbers: it cannot fail
	return strings.TrimSuffix(buf.String(), "\n")
}

// structureEntries are the entries that make the room hold the manifest m.
func structureEntries(m designfmt.Manifest) map[string]string {
	out := map[string]string{}
	tokenEntries(m, out)
	pageOrders := startKeys(len(m.Pages))
	for i, p := range m.Pages {
		out[pageKey(p.ID, "order")] = pageOrders[i]
		out[pageKey(p.ID, "name")] = p.Name
		rowOrders := startKeys(len(p.Rows))
		for r, row := range p.Rows {
			rid := fmt.Sprintf("%s.r%d", p.ID, r+1)
			out[rowKey(rid, "page")] = p.ID
			out[rowKey(rid, "order")] = rowOrders[r]
			frameOrders := startKeys(len(row))
			for f, fr := range row {
				out[frameKey(fr.ID, "file")] = fr.File
				out[frameKey(fr.ID, "name")] = fr.Name
				out[frameKey(fr.ID, "w")] = strconv.Itoa(fr.W)
				out[frameKey(fr.ID, "h")] = strconv.Itoa(fr.H)
				out[frameKey(fr.ID, "row")] = rid
				out[frameKey(fr.ID, "order")] = frameOrders[f]
			}
		}
	}
	return out
}

// setStructure makes the structure entries, and only them, those of the manifest text.
func (r *Room) setStructure(e map[string]any, text string) (func(*crdt.Transaction), error) {
	m, problems := designfmt.Parse(text)
	if len(problems) > 0 {
		return nil, fmt.Errorf("design.json: %s", strings.Join(problems, "; "))
	}
	want := structureEntries(m)
	var drop []string
	for k := range e {
		if _, kept := want[k]; (strings.HasPrefix(k, structPrefix) || strings.HasPrefix(k, tokenPrefix)) && !kept {
			drop = append(drop, k)
		}
	}
	_, legacy := e[manifestKey]
	var set []string
	for k, v := range want {
		if cur, ok := e[k]; !ok || str(cur) != v {
			set = append(set, k)
		}
	}
	sort.Strings(drop)
	sort.Strings(set)
	return func(txn *crdt.Transaction) {
		for _, k := range drop {
			r.m.Delete(txn, k)
		}
		if legacy {
			r.m.Delete(txn, manifestKey)
		}
		for _, k := range set {
			r.m.Set(txn, k, want[k])
		}
	}, nil
}

// manifestOf is design.json as the room holds it: the projection of the
// structure entries, or the older single `manifest` entry for a room that has
// none of them; empty when the room has neither.
func manifestOf(e map[string]any) string {
	if p, ok := projectStructure(e); ok {
		return p.manifest(tokensOf(e))
	}
	return str(e[manifestKey])
}

// maxStructureProblems is how many problems a refusal lists.
const maxStructureProblems = 5

// checkStructure refuses what one person's write can get wrong in the pages and
// frames: a side outside 1 to 8192, a name that is empty or too long, an id or
// file the format would not accept. What people doing valid things at the same
// time can add up to (more than 20 pages, a page whose frames were all deleted)
// is not refused: that is ordinary co-editing, the projection leaves out what
// is not a page, and a version of it is validated when it is saved. A room with
// the older `manifest` entry is held to the whole manifest and to its frames
// having content; a room with neither is not yet a design and passes.
func checkStructure(e map[string]any) error {
	var problems []string
	if p, ok := projectStructure(e); ok {
		problems = p.problems
	} else if text := str(e[manifestKey]); text != "" {
		m, found := designfmt.Parse(text)
		problems = found
		if len(problems) == 0 {
			for _, placed := range m.Frames() {
				if _, ok := e[shellKey(placed.Frame.File, "root")]; !ok {
					problems = append(problems, fmt.Sprintf("%s: the room holds nothing for %s", placed.Label(), placed.Frame.File))
				}
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	more := ""
	if len(problems) > maxStructureProblems {
		more = fmt.Sprintf(" (and %d more)", len(problems)-maxStructureProblems)
		problems = problems[:maxStructureProblems]
	}
	return fmt.Errorf("the design's pages and frames are not valid: %s%s", strings.Join(problems, "; "), more)
}
