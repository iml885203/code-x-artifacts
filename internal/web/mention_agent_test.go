package web

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
)

// typedMentions reads the editor's recorded document of mentions, as a person
// typing them in the Docs editor leaves it in the room.
func typedMentions(t *testing.T) []docfmt.Block {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "docroom", "testdata", "yjs", "mentions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Update string `json:"update"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	update, err := hex.DecodeString(fx.Update)
	if err != nil {
		t.Fatal(err)
	}
	doc := crdt.New()
	if err := crdt.ApplyUpdateV1(doc, update, nil); err != nil {
		t.Fatal(err)
	}
	typed, err := docroom.Open(doc).Read()
	if err != nil {
		t.Fatal(err)
	}
	return typed.Tabs[0].Blocks
}

func TestAMentionAnAgentInsertsWithDocEditIsStoredAsTheSameBlocksAsTypingIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	id := h.editableDoc(tok)
	typed := typedMentions(t)

	var sent []any
	if err := json.Unmarshal(mustJSON(t, typed), &sent); err != nil {
		t.Fatal(err)
	}
	agent.ok("doc_edit", map[string]any{"id": id, "ops": []any{map[string]any{"op": "insert", "after": "p2", "blocks": sent}}})

	read := agent.ok("read", map[string]any{"id": id, "view": "blocks"})
	var stored struct {
		Tabs []struct {
			Blocks []docfmt.Block `json:"blocks"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(mustJSON(t, read), &stored); err != nil {
		t.Fatal(err)
	}
	blocks := stored.Tabs[0].Blocks
	if len(blocks) != 3+len(typed) {
		t.Fatalf("read returns %d blocks, want the 3 there were and the %d inserted", len(blocks), len(typed))
	}
	if got, want := string(mustJSON(t, blocks[3:])), string(mustJSON(t, typed)); got != want {
		t.Errorf("the blocks doc_edit stored differ from the typed ones:\n got: %s\nwant: %s", got, want)
	}
}
