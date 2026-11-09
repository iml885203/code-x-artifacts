package sourcecodec_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designroom/sourcecodec"
	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
	"github.com/reearth/ygo/crdt"
)

func TestValidatedSourceWritesKeepPendingLiteralCoedits(t *testing.T) {
	t.Parallel()
	input := fixture(t)
	state, err := sourcecodec.State(input)
	if err != nil {
		t.Fatal(err)
	}
	server, peer := crdt.New(), crdt.New()
	defer server.Destroy()
	defer peer.Destroy()
	for _, doc := range []*crdt.Doc{server, peer} {
		if err := crdt.ApplyUpdateV1(doc, state, nil); err != nil {
			t.Fatal(err)
		}
	}
	literalKey := sourcecodec.SlotKey("frames/source.html", "literal", "label", 0, 0, "")
	value, _ := peer.GetMap("design").Get(literalKey)
	text := value.(*crdt.YText)
	peer.Transact(func(txn *crdt.Transaction) { text.Insert(txn, 0, "朋友😀 ", nil) })
	pending := crdt.EncodeStateAsUpdateV1(peer, server.StateVector())
	current, err := sourcecodec.Read(server)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := designsourceedit.Apply(current, []designsourceedit.Op{{Kind: "set_prop", File: "frames/source.html", Key: "title", Expected: designsourceedit.ValueDigest(json.RawMessage(`"Cards"`)), Value: json.RawMessage(`"Changed"`)}})
	if err != nil {
		t.Fatal(err)
	}
	write, err := sourcecodec.Write(server, applied.Input)
	if err != nil {
		t.Fatal(err)
	}
	server.Transact(write)
	if err := crdt.ApplyUpdateV1(server, pending, nil); err != nil {
		t.Fatal(err)
	}
	update := crdt.EncodeStateAsUpdateV1(server, peer.StateVector())
	if err := crdt.ApplyUpdateV1(peer, update, nil); err != nil {
		t.Fatal(err)
	}
	for _, doc := range []*crdt.Doc{server, peer} {
		got, err := sourcecodec.Read(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Files["frames/source.html"], "朋友😀 Hello &amp; ") || !strings.Contains(got.Files["frames/source.html"], `"title":"Changed"`) {
			t.Fatal("pending coedit or protected source change was lost")
		}
	}
}

func TestInvalidSourceWritePlanningLeavesEveryRoomByteUnchanged(t *testing.T) {
	t.Parallel()
	input := fixture(t)
	state, err := sourcecodec.State(input)
	if err != nil {
		t.Fatal(err)
	}
	server := crdt.New()
	defer server.Destroy()
	if err := crdt.ApplyUpdateV1(server, state, nil); err != nil {
		t.Fatal(err)
	}
	before := string(crdt.EncodeStateAsUpdateV1(server, nil))
	invalid := input
	invalid.Files = map[string]string{"frames/source.html": "<script>unsafe</script>"}
	if write, err := sourcecodec.Write(server, invalid); err == nil || write != nil {
		t.Fatal("invalid source returned a partial write")
	}
	if string(crdt.EncodeStateAsUpdateV1(server, nil)) != before {
		t.Fatal("planning mutated room before validation")
	}
}
