package sourcecodec

import (
	"fmt"
	"sort"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

// Write plans a validated source replacement under a caller-held room lock.
// Existing literal Y.Text objects survive unchanged units and receive minimal
// code-point-safe edits; replacing them would orphan a peer's pending coedit.
// Only the authorized conditional service path may execute protected writes.
// Raw peers must still pass CheckTransition before persistence/broadcast.
func Write(doc *crdt.Doc, input designsource.ParseInput) (func(*crdt.Transaction), error) {
	if _, err := Read(doc); err != nil {
		return nil, err
	}
	state, err := State(input)
	if err != nil {
		return nil, err
	}
	next := crdt.New()
	defer next.Destroy()
	if err := crdt.ApplyUpdateV1(next, state, nil); err != nil {
		return nil, err
	}
	if _, err := Read(next); err != nil {
		return nil, err
	}
	currentMap, nextMap := doc.GetMap("design"), next.GetMap("design")
	currentEntries, nextEntries := currentMap.Entries(), nextMap.Entries()
	keys := make([]string, 0, len(currentEntries)+len(nextEntries))
	for key := range currentEntries {
		keys = append(keys, key)
	}
	for key := range nextEntries {
		if _, exists := currentEntries[key]; !exists {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var writes []func(*crdt.Transaction)
	for _, key := range keys {
		write, err := writeKey(currentMap, nextMap, key)
		if err != nil {
			return nil, err
		}
		if write != nil {
			writes = append(writes, write)
		}
	}
	if len(writes) == 0 {
		return nil, nil
	}
	return func(txn *crdt.Transaction) {
		for _, write := range writes {
			write(txn)
		}
	}, nil
}

func writeKey(currentMap, nextMap *crdt.YMap, key string) (func(*crdt.Transaction), error) {
	desired, exists := nextMap.Get(key)
	if !exists {
		return func(txn *crdt.Transaction) { currentMap.Delete(txn, key) }, nil
	}
	current, exists := currentMap.Get(key)
	if desiredText, ok := desired.(*crdt.YText); ok {
		return literalWrite(currentMap, key, current, exists, desiredText.ToString())
	}
	value, ok := desired.(string)
	if !ok {
		return nil, fmt.Errorf("protected source slot has incompatible type")
	}
	if exists && current == value {
		return nil, nil
	}
	return func(txn *crdt.Transaction) { currentMap.Set(txn, key, value) }, nil
}

func literalWrite(m *crdt.YMap, key string, current any, exists bool, wanted string) (func(*crdt.Transaction), error) {
	if !exists {
		return func(txn *crdt.Transaction) {
			text := crdt.NewTextPrelim()
			m.Set(txn, key, text)
			text.Insert(txn, 0, wanted, nil)
		}, nil
	}
	text, ok := current.(*crdt.YText)
	if !ok {
		return nil, fmt.Errorf("literal source slot has incompatible type")
	}
	if text.ToString() == wanted {
		return nil, nil
	}
	return textWrite(text, wanted), nil
}

func textWrite(text *crdt.YText, wanted string) func(*crdt.Transaction) {
	before, after := []rune(text.ToString()), []rune(wanted)
	prefix := 0
	for prefix < len(before) && prefix < len(after) && before[prefix] == after[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(before)-prefix && suffix < len(after)-prefix && before[len(before)-1-suffix] == after[len(after)-1-suffix] {
		suffix++
	}
	start := utf16Length(before[:prefix])
	remove := utf16Length(before[prefix : len(before)-suffix])
	insert := string(after[prefix : len(after)-suffix])
	return func(txn *crdt.Transaction) {
		if remove > 0 {
			text.Delete(txn, start, remove)
		}
		if insert != "" {
			text.Insert(txn, start, insert, nil)
		}
	}
}

func utf16Length(text []rune) int {
	length := 0
	for _, r := range text {
		length++
		if r > 0xffff {
			length++
		}
	}
	return length
}
