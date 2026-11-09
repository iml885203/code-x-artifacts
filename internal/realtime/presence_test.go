package realtime_test

import (
	"errors"
	"testing"

	"github.com/reearth/ygo/encoding"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
)

// awarenessOf builds an awareness message by hand, as a client (or an attacker) could.
func awarenessOf(clientID, clock uint64, stateJSON string) []byte {
	enc := encoding.NewEncoder()
	enc.WriteVarUint(1)
	enc.WriteVarUint(clientID)
	enc.WriteVarUint(clock)
	enc.WriteVarString(stateJSON)
	return frame(1, enc.Bytes())
}

func TestTheRosterShowsTheAuthenticatedNameNotTheReportedOne(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a, b := e.connect(room, alice), e.connect(room, bob)

	// Alice claims to be someone else and picks a colour.
	err := a.send(awarenessOf(a.aw.ClientID(), 1, `{"user":{"name":"Carol (admin)","color":"#ff0066"},"cursor":{"anchor":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	settle(t, a, b)

	names := b.names()
	if !names["Alice"] || names["Carol (admin)"] {
		t.Errorf("bob sees %v, want Alice's authenticated name and not the one she reported", names)
	}
	var kept bool
	for _, st := range b.aw.GetStates() {
		if user, ok := st.State["user"].(map[string]any); ok && user["color"] == "#ff0066" {
			kept = st.State["cursor"] != nil
			// Whatever it calls itself, the entry is the account the server authorised: the id the viewers list carries too.
			if user["id"] != alice.Username {
				t.Errorf("the roster entry of alice has id %v, want her account %q", user["id"], alice.Username)
			}
		}
	}
	if !kept {
		t.Error("the colour and the cursor the client chose were not passed on")
	}

	// Someone who joins later is shown the roster as it is.
	c := e.connect(room, realtime.Identity{Username: "dave", Label: "Dave"})
	settle(t, a, b, c)
	if !c.names()["Alice"] {
		t.Errorf("a late joiner sees %v", c.names())
	}
}

func TestAConnectionCannotSpeakForAnotherConnectionsAwareness(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a, b := e.connect(room, alice), e.connect(room, bob)
	if err := a.send(awarenessOf(a.aw.ClientID(), 1, `{"user":{"name":"x"}}`)); err != nil {
		t.Fatal(err)
	}
	settle(t, a, b)

	// Bob publishes under Alice's awareness id, to hide her cursor or take it over.
	if err := b.send(awarenessOf(a.aw.ClientID(), 2, `null`)); err == nil {
		t.Error("a connection removed another connection's awareness entry")
	}
	if err := b.send(awarenessOf(a.aw.ClientID(), 3, `{"user":{"name":"Alice"}}`)); err == nil {
		t.Error("a connection replaced another connection's awareness entry")
	}
	settle(t, a, b)
	if !b.names()["Alice"] {
		t.Errorf("alice left the roster: %v", b.names())
	}
}

func TestAConnectionThatLeavesIsRemovedFromTheRoster(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a, b := e.connect(room, alice), e.connect(room, bob)
	if err := a.send(awarenessOf(a.aw.ClientID(), 1, `{"user":{"name":"x"}}`)); err != nil {
		t.Fatal(err)
	}
	settle(t, a, b)
	if !b.names()["Alice"] {
		t.Fatalf("alice is not in the roster: %v", b.names())
	}
	a.peer.Close()
	settle(t, b)
	if b.names()["Alice"] {
		t.Errorf("alice is still in the roster after leaving: %v", b.names())
	}
}

func TestAnAgentTheServerSpeaksForShowsInTheRosterUntilItLeaves(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	b := e.connect(room, bob)

	leave, err := e.hub.Present(t.Context(), room, "alice の claude")
	if err != nil {
		t.Fatal(err)
	}
	settle(t, b)
	if !b.names()["alice の claude"] {
		t.Fatalf("bob sees %v, want the agent", b.names())
	}
	var isAgent bool
	for _, st := range b.aw.GetStates() {
		if user, ok := st.State["user"].(map[string]any); ok && user["name"] == "alice の claude" {
			isAgent = user["agent"] == true
			if user["id"] != "alice の claude" {
				t.Errorf("the agent's roster entry has id %v, want the key it was announced under", user["id"])
			}
		}
	}
	if !isAgent {
		t.Error("the roster does not say it is an agent")
	}

	// Someone who joins while the agent is there sees it too.
	c := e.connect(room, alice)
	settle(t, b, c)
	if !c.names()["alice の claude"] {
		t.Errorf("a late joiner sees %v", c.names())
	}

	leave()
	leave() // leaving twice is harmless
	settle(t, b, c)
	if b.names()["alice の claude"] || c.names()["alice の claude"] {
		t.Errorf("the agent is still shown after it left: %v, %v", b.names(), c.names())
	}
}

func TestAnAwarenessMessageThatIsNotAnObjectIsRefused(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a := e.connect(room, alice)
	err := a.send(awarenessOf(a.aw.ClientID(), 1, `"just a string"`))
	if err == nil {
		t.Fatal("a state that is not an object was accepted")
	}
	var le *realtime.LimitError
	if errors.As(err, &le) {
		t.Errorf("a malformed state is not a limit: %v", err)
	}
}

func TestAClientHearsItsOwnAwarenessBackSoAQuietConnectionIsNeverSilent(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a := e.connect(room, alice)
	if err := a.send(awarenessOf(a.aw.ClientID(), 1, `{"user":{"name":"x"}}`)); err != nil {
		t.Fatal(err)
	}
	// Alone in the room, the client still receives a message for its heartbeat:
	// the provider drops a connection that heard nothing for 30 seconds.
	if got := a.pump(); got != 1 {
		t.Errorf("a lone client received %d message(s) after its awareness heartbeat, want 1", got)
	}
	if !a.names()["Alice"] {
		t.Errorf("the echo does not carry the authenticated name: %v", a.names())
	}
}

func TestTheRoomListsWhoIsConnectedAndACloseCarriesItsReason(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	if got := e.hub.Connected(room); len(got) != 0 {
		t.Fatalf("connected before anyone joined: %v", got)
	}
	a, r := e.connect(room, alice), e.connect(room, reader)
	got := map[string]bool{}
	for _, who := range e.hub.Connected(room) {
		got[who.Username] = who.ReadOnly
	}
	if len(got) != 2 || got["alice"] || !got["carol"] {
		t.Errorf("connected = %v, want alice (writes) and carol (read only)", got)
	}

	reason := errors.New("lifetime reached")
	a.peer.CloseBecause(reason)
	if !errors.Is(a.peer.Err(), reason) {
		t.Errorf("Err = %v", a.peer.Err())
	}
	if got := e.hub.Connected(room); len(got) != 1 || got[0].Username != "carol" {
		t.Errorf("connected after alice's close = %v", got)
	}
	r.peer.Close()
	if r.peer.Err() != nil {
		t.Errorf("a client that left has Err = %v, want nil", r.peer.Err())
	}
	if e.hub.Limits().DocBytes != realtime.DefaultLimits.DocBytes {
		t.Errorf("limits = %+v", e.hub.Limits())
	}
}

// A design's canvas puts the selected elements in awareness next to the user
// (web/src/design/presence.ts). The server passes that field on untouched and
// writes the name itself, so a forged name anywhere in the message is never shown.
func TestASelectionInAwarenessReachesOthersUnderTheNameTheServerKnows(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a, b := e.connect(room, alice), e.connect(room, bob)

	err := a.send(awarenessOf(a.aw.ClientID(), 1, `{"user":{"name":"Carol (admin)","color":"c3"},"selection":{"hero":["title","cta"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	settle(t, a, b)

	var seen bool
	for _, st := range b.aw.GetStates() {
		user, _ := st.State["user"].(map[string]any)
		if user["name"] != "Alice" {
			continue
		}
		seen = true
		sel, _ := st.State["selection"].(map[string]any)
		if pids, _ := sel["hero"].([]any); len(pids) != 2 || pids[0] != "title" || pids[1] != "cta" {
			t.Errorf("bob sees the selection %v", st.State["selection"])
		}
		if user["color"] != "c3" {
			t.Errorf("the colour was not kept: %v", user)
		}
	}
	if !seen || b.names()["Carol (admin)"] {
		t.Errorf("bob sees %v, want Alice's authenticated name and not the forged one", b.names())
	}

	// Letting go of the selection is a state without the field, and that reaches bob too.
	if err := a.send(awarenessOf(a.aw.ClientID(), 2, `{"user":{"name":"Carol (admin)","color":"c3"},"selection":null}`)); err != nil {
		t.Fatal(err)
	}
	settle(t, a, b)
	for _, st := range b.aw.GetStates() {
		if user, _ := st.State["user"].(map[string]any); user["name"] == "Alice" && st.State["selection"] != nil {
			t.Errorf("a cleared selection is still shown: %v", st.State["selection"])
		}
	}
}
