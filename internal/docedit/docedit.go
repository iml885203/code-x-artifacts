// Package docedit is how an agent changes part of a doc: a list of operations
// on blocks, found by id, each optionally guarded by the hash of the block as
// the agent read it. It is a function of the document, so it works the same
// with or without a live room; the service writes the result where the doc
// lives (docs/doc-edit.md).
package docedit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// Operations.
const (
	Replace = "replace"
	Insert  = "insert"
	Delete  = "delete"
)

// MaxOps is how many operations one call may carry.
const MaxOps = 200

// Op is one operation. The four tab operations (tabs.go) name a tab with Tab and
// place it with Before or After (a tab id) or At; Replace and Delete name their block by ID; Insert says
// where with Before or After (a block id) or with Tab and At ("start" or
// "end"). Replace and Insert bring the new blocks as Markdown or as a list of
// blocks (docs/doc-format.md; an id they leave out is made up, except that the
// first block of a replace keeps the id of the block it replaces). IfHash is
// the hash the read gave for the block the operation names, or for the block
// it is next to: when the block is different now, the whole call is refused.
type Op struct {
	Op       string          `json:"op"`
	ID       string          `json:"id,omitempty"`
	IfHash   string          `json:"ifHash,omitempty"`
	Before   string          `json:"before,omitempty"`
	After    string          `json:"after,omitempty"`
	Tab      string          `json:"tab,omitempty"`
	At       string          `json:"at,omitempty"`
	Markdown string          `json:"markdown,omitempty"`
	Blocks   json.RawMessage `json:"blocks,omitempty"`
	// Title is a tab's title, for tab_add and tab_rename.
	Title string `json:"title,omitempty"`
}

// Hash is what a block is now, in a few characters: it changes when anything in
// the block does, nested blocks and ids included.
func Hash(b docfmt.Block) string {
	raw, _ := json.Marshal(b)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:12]
}

// State is a block as it is, with its hash.
type State struct {
	Hashed
	Tab string `json:"tab"`
}

// Hashed is a block that carries its hash, as a read shows it.
type Hashed struct {
	docfmt.Block
	Hash string `json:"hash"`
}

// TabView is a tab of a read as blocks.
type TabView struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Blocks []Hashed `json:"blocks"`
}

// View is a doc as an agent reads it to edit it: each tab with its blocks,
// each block with its id, type, hash and content.
func View(d docfmt.Doc) []TabView {
	out := make([]TabView, len(d.Tabs))
	for i, t := range d.Tabs {
		out[i] = TabView{ID: t.ID, Title: t.Title, Blocks: make([]Hashed, len(t.Blocks))}
		for j, b := range t.Blocks {
			out[i].Blocks[j] = Hashed{Block: b, Hash: Hash(b)}
		}
	}
	return out
}

// Outline is the order of a doc's top-level blocks with a hint of each, for a
// reader that has the Markdown and needs the ids.
type Outline struct {
	Tab     string `json:"tab"`
	ID      string `json:"id"`
	Type    string `json:"type"`
	Hash    string `json:"hash"`
	Preview string `json:"preview,omitempty"`
}

// Outlines lists every top-level block of the doc.
func Outlines(d docfmt.Doc) []Outline {
	var out []Outline
	for _, t := range d.Tabs {
		for _, b := range t.Blocks {
			out = append(out, Outline{Tab: t.ID, ID: b.ID, Type: b.Type, Hash: Hash(b), Preview: preview(b)})
		}
	}
	return out
}

func preview(b docfmt.Block) string {
	s := strings.Join(strings.Fields(docfmt.PlainText(b.Content)+b.Intent+b.Alt), " ")
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

// Refusal is an operation that cannot be done; nothing was changed.
type Refusal struct{ Msg string }

func (e *Refusal) Error() string { return e.Msg }

func refuse(i int, format string, args ...any) error {
	return &Refusal{Msg: fmt.Sprintf("ops[%d]: %s", i, fmt.Sprintf(format, args...))}
}

// Stale is a call refused because blocks are not what the caller read: each
// with the state it is in now.
type Stale struct {
	Msg    string
	Blocks []State
}

func (e *Stale) Error() string { return e.Msg }

// Result is what a call did.
type Result struct {
	// Blocks are the blocks the call wrote (new and replacement), in the order
	// of the doc, with their ids and hashes.
	Blocks []Written `json:"blocks"`
	// Tabs are the tabs of the doc after a call that added, renamed, deleted or
	// moved one, with the ids the new ones were given.
	Tabs []TabRef `json:"tabs,omitempty"`
	// Deleted are the ids of the blocks that are gone.
	Deleted []string `json:"deleted"`
	// Review is what the caller should look at again.
	Review []string `json:"review,omitempty"`
}

// Written is a block the call wrote.
type Written struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Hash string `json:"hash"`
}
