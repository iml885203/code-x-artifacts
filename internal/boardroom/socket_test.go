package boardroom

import (
	"context"
	"errors"
	"testing"

	"github.com/reearth/ygo/crdt"
	"github.com/reearth/ygo/encoding"
	ygsync "github.com/reearth/ygo/sync"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/iml885203/code-x-artifacts/internal/store/storetest"
)

// peer is an in-process client of a room speaking y-websocket through a
// realtime.Peer, standing in for a browser's binding.
type peer struct {
	t    *testing.T
	doc  *crdt.Doc
	conn *realtime.Peer
}

func join(t *testing.T, hub *realtime.Hub, room string) *peer {
	t.Helper()
	conn, err := hub.Join(t.Context(), room, realtime.Identity{Username: "alice", Label: "Alice"}, "")
	if err != nil {
		t.Fatal(err)
	}
	p := &peer{t: t, doc: crdt.New(), conn: conn}
	p.doc.OnUpdate(func(update []byte, origin any) {
		if origin == "remote" {
			return
		}
		enc := encoding.NewEncoder()
		enc.WriteVarUint(0)
		_ = conn.Receive(context.Background(), append(enc.Bytes(), ygsync.EncodeUpdate(update)...)) // a refusal ends the peer, which the test reads
	})
	p.pump()
	return p
}

// pump applies what the room has queued for the peer, answering as a provider does.
func (p *peer) pump() {
	for {
		select {
		case msg, open := <-p.conn.Out():
			if !open {
				return
			}
			dec := encoding.NewDecoder(msg)
			if kind, err := dec.ReadVarUint(); err != nil || kind != 0 {
				continue // awareness is not what these tests are about
			}
			reply, err := ygsync.ApplySyncMessage(p.doc, dec.RemainingBytes(), "remote")
			if err != nil {
				p.t.Fatalf("apply a sync message from the room: %v", err)
			}
			if reply != nil {
				enc := encoding.NewEncoder()
				enc.WriteVarUint(0)
				_ = p.conn.Receive(context.Background(), append(enc.Bytes(), reply...)) // see join
			}
		default:
			return
		}
	}
}

func (p *peer) set(entries map[string]string) {
	m := p.doc.GetMap(Map)
	p.doc.Transact(func(txn *crdt.Transaction) {
		for k, v := range entries {
			m.Set(txn, k, v)
		}
	})
	p.pump()
}

// A browser can write what it likes into its own copy of the room; the hub
// holds the board to its rules before it keeps an update, so what the board may
// not hold never reaches the log or the other people in the room.
func TestWhatABoardMayNotHoldIsRefusedOverTheSocketAndTheRoomKeepsWhatItHad(t *testing.T) {
	t.Parallel()
	refused := map[string]map[string]string{
		"an embedded web page": {"e1|type": `"embeddable"`, "e1|link": `"https://example.com"`},
		"a javascript link":    {"r1|link": `"javascript:alert(1)"`},
		"a picture written in": {"i1|type": `"image"`, "i1|fileId": `"data:image/png;base64,iVBORw0KGgo="`},
	}
	for name, bad := range refused {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			st := storetest.Open(t)
			a, err := st.CreateArtifact(t.Context(), "A board", "doc", "", "", "alice", store.VisibilityPrivate)
			if err != nil {
				t.Fatal(err)
			}
			hub, err := realtime.New(realtime.Config{
				Log:      st.RealtimeLog(),
				Validate: func(_ string, doc *crdt.Doc) error { return Check(doc) },
			})
			if err != nil {
				t.Fatal(err)
			}
			alice := join(t, hub, a.ID)
			alice.set(rectangle())
			if err := alice.conn.Err(); err != nil {
				t.Fatalf("a fine rectangle was refused: %v", err)
			}

			mallory := join(t, hub, a.ID)
			mallory.set(bad)
			if err := mallory.conn.Err(); !errors.Is(err, realtime.ErrRoomReset) {
				t.Fatalf("the connection ended with %v, want a room reset", err)
			}
			alice.pump()

			var held map[string]any
			if err := hub.Read(t.Context(), a.ID, func(doc *crdt.Doc) error {
				held = doc.GetMap(Map).Entries()
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(held) != len(rectangle()) || held["r1|link"] != "null" {
				t.Errorf("the room does not hold what it had: %v", held)
			}
			for k := range bad {
				if v, ok := alice.doc.GetMap(Map).Get(k); ok && v == bad[k] {
					t.Errorf("the refused entry %s reached another person in the room", k)
				}
			}
		})
	}
}
