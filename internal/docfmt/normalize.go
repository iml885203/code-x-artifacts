package docfmt

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// NewID makes the id of a block (prefix "b_") or a tab ("t_"): random, so two
// agents writing at once never collide, and short enough to read in a message.
func NewID(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return prefix + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]))
}

// Normalize turns what a publisher sent as `blocks` into the stored content:
// either a whole doc ({"tabs": [...]}) or a list of blocks, which becomes the
// only tab. Tabs and blocks sent without an id get one from newID; ids that
// were sent are kept. The result is validated and in canonical form (fixed
// field order, no insignificant spaces), so equal docs are equal bytes.
func Normalize(raw []byte, newID func(prefix string) string) (string, error) {
	root, err := decode(raw)
	if err != nil {
		return "", err
	}
	switch r := root.(type) {
	case []any:
		root = map[string]any{"tabs": []any{map[string]any{"title": DefaultTabTitle, "blocks": r}}}
	case map[string]any:
	default:
		return "", errors.New("blocks must be a list of blocks or {\"tabs\": [...]}")
	}
	fillIDs(root, newID)
	filled, err := marshal(root)
	if err != nil {
		return "", fmt.Errorf("document: %w", err)
	}
	doc, err := Parse(filled)
	if err != nil {
		return "", err
	}
	return Canonical(doc)
}

// DefaultTabTitle names the only tab of a doc that was sent as a list of blocks.
const DefaultTabTitle = "Document"

// Canonical is the stored form of a doc.
func Canonical(doc Doc) (string, error) {
	out, err := marshal(doc)
	if err != nil {
		return "", fmt.Errorf("document: %w", err)
	}
	if len(out) > MaxBytes {
		return "", fmt.Errorf("a doc can be at most %d bytes of JSON, this one is %d bytes", MaxBytes, len(out))
	}
	return out, nil
}

// marshal is json.Marshal without HTML escaping, so a link's & stays one.
func marshal(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// fillIDs gives every tab and block of a decoded tree that has no id one. It
// reads whatever shape it finds and leaves the checking to Parse.
func fillIDs(root any, newID func(prefix string) string) {
	used := map[string]bool{}
	collect(root, used)
	fresh := func(prefix string) string {
		for {
			if id := newID(prefix); !used[id] {
				used[id] = true
				return id
			}
		}
	}
	obj, _ := root.(map[string]any)
	tabs, _ := obj["tabs"].([]any)
	for _, t := range tabs {
		tab, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if _, has := tab["id"]; !has {
			tab["id"] = fresh("t_")
		}
		blocks, _ := tab["blocks"].([]any)
		fillBlocks(blocks, fresh)
	}
}

func fillBlocks(blocks []any, fresh func(prefix string) string) {
	for _, raw := range blocks {
		b, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, has := b["id"]; !has {
			b["id"] = fresh("b_")
		}
		kids, _ := b["blocks"].([]any)
		fillBlocks(kids, fresh)
	}
}

// collect notes every id already in the tree.
func collect(node any, used map[string]bool) {
	switch n := node.(type) {
	case map[string]any:
		if id, ok := n["id"].(string); ok {
			used[id] = true
		}
		for _, v := range n {
			collect(v, used)
		}
	case []any:
		for _, v := range n {
			collect(v, used)
		}
	}
}
