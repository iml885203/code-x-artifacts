// Package designroom is how a design lives in a realtime room: its manifest and
// every frame as a flat tree of nodes in one Y.Map, and back to the frame files
// of docs/design-format.md, byte for byte. Editors write the room, the server
// reads and writes it, and a version is the files the room serialises to
// (docs/design-room.md).
//
// The tree is flat on purpose: a node is a handful of entries keyed by its
// id, and its place is two of them (parent, order), so moving an element writes
// nothing the element's own edits write, and two people editing one element
// change different entries unless they change the same thing.
package designroom

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/reearth/ygo/crdt"
)

// Map is the name of the shared type that holds the design.
const Map = "design"

// ErrReadWhileWriting is what a read made from inside a write returns; in ygo
// such a read waits for a lock the write holds and never comes back.
var ErrReadWhileWriting = errors.New("designroom: the room was read from inside a write, which would never return")

// ErrCycle is what a move that would make an element its own ancestor is
// refused with.
var ErrCycle = errors.New("designroom: the move would put an element inside itself")

// Room is a design's room. Open it in the plan, never inside a transaction.
type Room struct {
	m       *crdt.YMap
	writing atomic.Bool
}

// Open looks up the shared type that holds the design.
func Open(doc *crdt.Doc) *Room { return &Room{m: doc.GetMap(Map)} }

// Keys: a frame's entries start with its file, a node's with the file and its id.
func shellKey(file, field string) string { return file + "|shell|" + field }

func nodeKey(file, id, field string) string {
	return file + "|n|" + id + "|" + field
}
func styleKey(file, id, prop string) string { return nodeKey(file, id, "s|"+prop) }

const manifestKey = "manifest"

// Design is what a room holds as the content of a version: design.json and the
// text of each frame file.
type Design struct {
	Manifest string
	Frames   map[string]string
}

// entries is every entry of the map, read once.
func (r *Room) entries() map[string]any { return r.m.Entries() }

func str(v any) string {
	s, _ := v.(string)
	return s
}

// Read is the design the room holds. A tree that cannot be a frame (a cycle, a
// node of a kind frames do not have) is an error naming the frame.
func (r *Room) Read() (Design, error) {
	if r.writing.Load() {
		return Design{}, ErrReadWhileWriting
	}
	e := r.entries()
	text := manifestOf(e)
	d := Design{Manifest: text, Frames: map[string]string{}}
	files := map[string]bool{}
	for k := range e {
		if file, rest, ok := strings.Cut(k, "|"); ok && strings.HasPrefix(rest, "shell|") {
			files[file] = true
		}
	}
	if p, ok := projectStructure(e); ok {
		// What the design does not include (a frame whose page was deleted as it was moved there) is not a file of it.
		for file := range files {
			if !p.hasFile(file) {
				delete(files, file)
			}
		}
	}
	for file := range files {
		m, err := r.model(e, file)
		if err != nil {
			return Design{}, fmt.Errorf("%s: %w", file, err)
		}
		text, err := build(m)
		if err != nil {
			return Design{}, fmt.Errorf("%s: %w", file, err)
		}
		d.Frames[file] = text
	}
	return d, nil
}

// model reads one frame's entries into its model.
func (r *Room) model(e map[string]any, file string) (frameModel, error) {
	var m frameModel
	m.docBefore, m.docAfter = str(e[shellKey(file, "docBefore")]), str(e[shellKey(file, "docAfter")])
	m.pre, m.post = str(e[shellKey(file, "pre")]), str(e[shellKey(file, "post")])
	m.root = str(e[shellKey(file, "root")])
	var err error
	if m.htmlAttrs, err = parseAttrs(str(e[shellKey(file, "htmlAttrs")])); err != nil {
		return m, err
	}
	byID, err := nodesOf(e, file)
	if err != nil {
		return m, err
	}
	for id, n := range byID {
		if !n.hasRich {
			continue
		}
		v, _ := r.m.Get(nodeKey(file, id, "rich"))
		t, ok := v.(*crdt.YText)
		if !ok {
			return m, fmt.Errorf("node %s: its formatted text is not a text", id)
		}
		n.rich = readRich(t)
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if n := byID[id]; n.kind != "" {
			m.nodes = append(m.nodes, *n)
		}
	}
	return settle(m)
}

// nodesOf reads the entries of a frame's nodes, by node id.
func nodesOf(e map[string]any, file string) (map[string]*node, error) {
	prefix := file + "|n|"
	byID := map[string]*node{}
	for k, v := range e {
		rest, ok := strings.CutPrefix(k, prefix)
		if !ok {
			continue
		}
		id, field, ok := strings.Cut(rest, "|")
		if !ok {
			continue
		}
		n := byID[id]
		if n == nil {
			n = &node{id: id, style: map[string]string{}}
			byID[id] = n
		}
		if err := n.set(field, v); err != nil {
			return nil, fmt.Errorf("node %s: %w", id, err)
		}
	}
	return byID, nil
}

// set takes one entry of a node.
func (n *node) set(field string, v any) error {
	switch field {
	case "kind":
		n.kind = str(v)
	case "tag":
		n.tag = str(v)
	case "ns":
		n.ns = str(v)
	case "parent":
		n.parent = str(v)
	case "order":
		n.order = str(v)
	case "rest":
		n.rest = str(v)
	case "data":
		n.text = str(v)
	case "rich":
		n.hasRich = true // the runs are read from the shared text itself (Room.model)
	case "attrs":
		list, err := parseAttrs(str(v))
		if err != nil {
			return err
		}
		n.attrs = list
	case "text":
		// Entries hands a nested text back as its string.
		switch t := v.(type) {
		case *crdt.YText:
			n.text = t.ToString()
		case string:
			n.text = t
		}
	default:
		n.setEditorField(field, v)
	}
	return nil
}

// setEditorField takes the entries only the editor's controls write: an owned
// style property and the hidden state.
func (n *node) setEditorField(field string, v any) {
	if prop, ok := strings.CutPrefix(field, "override|"); ok {
		if n.styleOverrides == nil {
			n.styleOverrides = map[string]string{}
		}
		n.styleOverrides[prop] = str(v)
	} else if field == "hidden" {
		n.hidden = str(v)
	} else if prop, ok := strings.CutPrefix(field, "s|"); ok {
		n.style[prop] = str(v)
	}
}

// settle makes a model of what was read: a node whose parent is gone (someone
// deleted it while another moved something into it) is put at the end of the body
// rather than lost, and a cycle is an error.
func settle(m frameModel) (frameModel, error) {
	parent := map[string]string{}
	have := map[string]bool{}
	for _, n := range m.nodes {
		have[n.id] = true
	}
	for i, n := range m.nodes {
		if n.id != m.root && !have[n.parent] {
			m.nodes[i].parent, m.nodes[i].order = m.root, "~"+n.id // after every key a person makes
		}
		parent[n.id] = m.nodes[i].parent
	}
	for id := range parent {
		if cyc := inCycle(parent, id, m.root); cyc {
			return m, fmt.Errorf("%w: %s is among its own ancestors", ErrCycle, id)
		}
	}
	return m, nil
}

// inCycle reports whether walking up from id never reaches the root.
func inCycle(parent map[string]string, id, root string) bool {
	for steps := 0; steps <= len(parent); steps++ {
		if id == root {
			return false
		}
		next, ok := parent[id]
		if !ok {
			return false
		}
		id = next
	}
	return true
}

// Check refuses a room whose tree has a cycle, or whose formatted text holds
// markup a frame may not have. It is what the hub runs before it keeps an update.
func Check(doc *crdt.Doc) error {
	r := Open(doc)
	e := r.entries()
	if err := checkTokenEntries(e); err != nil {
		return err
	}
	if err := checkStructure(e); err != nil {
		return err
	}
	files := map[string]bool{}
	for k := range e {
		if file, rest, ok := strings.Cut(k, "|"); ok && strings.HasPrefix(rest, "shell|") {
			files[file] = true
		}
	}
	if err := checkStyleEntries(e); err != nil {
		return err
	}
	for file := range files {
		m, err := r.model(e, file)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		if err := checkNodes(file, m); err != nil {
			return err
		}
		for _, n := range m.nodes {
			if err := checkRich(n.rich); err != nil {
				return fmt.Errorf("%s: the text of %s: %w", file, n.id, err)
			}
		}
	}
	return nil
}
