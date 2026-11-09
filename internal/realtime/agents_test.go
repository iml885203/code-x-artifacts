package realtime_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/reearth/ygo/encoding"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
)

// agentsIn lists the roster entries a client sees that are agents: name and block.
func (c *client) agentsIn() map[string]string {
	out := map[string]string{}
	for _, st := range c.aw.GetStates() {
		user, _ := st.State["user"].(map[string]any)
		if user["agent"] == true {
			block, _ := st.State["block"].(string)
			out[user["name"].(string)] = block
		}
	}
	return out
}

func TestAnAnnouncedAgentIsInTheRosterAtItsBlockAndLeavesAfterTheIdleTime(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	b := e.connect(room, bob)

	if err := e.hub.Announce(t.Context(), room, "alice/claude", "alice の Claude Code", "b_one"); err != nil {
		t.Fatal(err)
	}
	settle(t, b)
	if got := b.agentsIn(); got["alice の Claude Code"] != "b_one" || len(got) != 1 {
		t.Fatalf("bob sees agents %v, want the agent at b_one", got)
	}
	var at float64
	for _, st := range b.aw.GetStates() {
		if v, ok := st.State["at"].(float64); ok {
			at = v
		}
	}
	if int64(at) != e.clock.Now().UnixMilli() {
		t.Errorf("at = %v, want the time of the announcement", at)
	}

	// A later joiner sees it too, and an announcement at another block moves it and extends its stay.
	c := e.connect(room, alice)
	e.clock.Advance(10 * time.Second)
	if err := e.hub.Announce(t.Context(), room, "alice/claude", "alice の Claude Code", "b_two"); err != nil {
		t.Fatal(err)
	}
	settle(t, b, c)
	if b.agentsIn()["alice の Claude Code"] != "b_two" || c.agentsIn()["alice の Claude Code"] != "b_two" {
		t.Errorf("the agent is at %v and %v, want b_two for both", b.agentsIn(), c.agentsIn())
	}
	e.clock.Advance(14 * time.Second) // 24 s in, 14 s after the last announcement
	e.hub.Sweep(t.Context())
	settle(t, b, c)
	if len(b.agentsIn()) != 1 {
		t.Fatalf("the agent left after %v, before its idle time: %v", 14*time.Second, b.agentsIn())
	}
	e.clock.Advance(time.Second)
	e.hub.Sweep(t.Context())
	settle(t, b, c)
	if len(b.agentsIn()) != 0 || len(c.agentsIn()) != 0 {
		t.Errorf("the agent is still shown after the idle time: %v %v", b.agentsIn(), c.agentsIn())
	}
	if b.names()["alice の Claude Code"] {
		t.Error("its name is still in the roster")
	}
}

func TestTwoAgentsAreTwoEntriesAndAnAnnouncedAgentKeepsTheRoomLoaded(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	if err := e.hub.Announce(t.Context(), room, "a", "A の agent", "b1"); err != nil {
		t.Fatal(err)
	}
	if err := e.hub.Announce(t.Context(), room, "b", "B の Codex", "b2"); err != nil {
		t.Fatal(err)
	}
	c := e.connect(room, alice)
	settle(t, c)
	if got := c.agentsIn(); len(got) != 2 || got["A の agent"] != "b1" || got["B の Codex"] != "b2" {
		t.Errorf("agents = %v", got)
	}
	c.peer.Close()
	e.clock.Advance(10 * time.Minute) // past the room's idle time, and the agents' too
	e.hub.Sweep(t.Context())
	e.hub.Sweep(t.Context()) // the agents go first, then the empty room
	if e.hub.Loaded() != 0 {
		t.Errorf("%d rooms still loaded once the agents went", e.hub.Loaded())
	}
}

func TestAConnectedBrowserCannotBeAnAgentOrSpeakForOne(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	if err := e.hub.Announce(t.Context(), room, "alice/claude", "alice の Claude Code", "b_real"); err != nil {
		t.Fatal(err)
	}
	mallory := e.connect(room, bob)
	watcher := e.connect(room, alice)
	settle(t, mallory, watcher)

	// It says it is an agent, with another name, at a block of its choosing.
	mallory.aw.SetLocalState(map[string]any{"user": map[string]any{"name": "Fake の Claude Code", "agent": true}, "block": "b_fake", "agent": true})
	if err := mallory.send(frame(1, mallory.aw.EncodeUpdate([]uint64{uint64(mallory.doc.ClientID())}))); err != nil {
		t.Fatal(err)
	}
	settle(t, mallory, watcher)
	got := watcher.agentsIn()
	if len(got) != 1 || got["alice の Claude Code"] != "b_real" {
		t.Errorf("agents = %v: a browser became an agent", got)
	}
	if watcher.names()["Fake の Claude Code"] {
		t.Error("the name it chose is in the roster")
	}

	// It tries to overwrite the agent's own entry, and to remove it.
	var agentID uint64
	for id, st := range watcher.aw.GetStates() {
		if user, _ := st.State["user"].(map[string]any); user["agent"] == true {
			agentID = id
		}
	}
	for _, state := range []string{`{"user":{"name":"hijacked","agent":true},"block":"b_fake"}`, `null`} {
		enc := encoding.NewEncoder()
		enc.WriteVarUint(1)
		enc.WriteVarUint(agentID)
		enc.WriteVarUint(1 << 20)
		enc.WriteVarBytes([]byte(state))
		if err := mallory.send(frame(1, enc.Bytes())); err == nil {
			t.Errorf("an awareness update for the agent's entry (%s) was accepted", state)
		}
	}
	settle(t, mallory, watcher)
	if got := watcher.agentsIn(); len(got) != 1 || got["alice の Claude Code"] != "b_real" {
		t.Errorf("agents = %v after the attempts", got)
	}
}

// touchedBy is what each agent in the room's roster says it changed: {frame: [pid…]}, by the agent's name.
func (c *client) touchedBy() map[string]map[string][]any {
	out := map[string]map[string][]any{}
	for _, st := range c.aw.GetStates() {
		user, _ := st.State["user"].(map[string]any)
		if user["agent"] != true {
			continue
		}
		frames := map[string][]any{}
		touched, _ := st.State["touched"].(map[string]any)
		for frame, pids := range touched {
			frames[frame], _ = pids.([]any)
		}
		out[user["name"].(string)] = frames
	}
	return out
}

func TestAnAgentAnnouncedWithTheElementsItChangedShowsThemToEveryoneAndLeavesAfterTheIdleTime(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	b := e.connect(room, bob)

	if err := e.hub.AnnounceTouched(t.Context(), room, "alice/claude", "alice 的 Claude Code", map[string][]string{"hero": {"e3", "e5"}, "foot": {"e1"}}); err != nil {
		t.Fatal(err)
	}
	settle(t, b)
	got := b.touchedBy()["alice 的 Claude Code"]
	if len(got["hero"]) != 2 || got["hero"][0] != "e3" || got["hero"][1] != "e5" || len(got["foot"]) != 1 {
		t.Fatalf("bob sees %v", b.touchedBy())
	}

	// The same agent again replaces what it touched and stays; after the idle time it is gone.
	e.clock.Advance(10 * time.Second)
	if err := e.hub.AnnounceTouched(t.Context(), room, "alice/claude", "alice 的 Claude Code", map[string][]string{"hero": {"e9"}}); err != nil {
		t.Fatal(err)
	}
	settle(t, b)
	if got := b.touchedBy()["alice 的 Claude Code"]; len(got["hero"]) != 1 || got["hero"][0] != "e9" || len(got) != 1 {
		t.Fatalf("bob sees %v after the second announcement", got)
	}
	e.clock.Advance(14 * time.Second)
	e.hub.Sweep(t.Context())
	settle(t, b)
	if len(b.touchedBy()) != 1 {
		t.Fatal("the agent left before its idle time")
	}
	e.clock.Advance(time.Second)
	e.hub.Sweep(t.Context())
	settle(t, b)
	if got := b.touchedBy(); len(got) != 0 {
		t.Errorf("the agent is still shown after the idle time: %v", got)
	}
}

func TestAnAnnouncementNamesNoMoreElementsThanTheLimit(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	b := e.connect(room, bob)
	many := make([]string, 0, realtime.MaxTouched+30)
	for i := range realtime.MaxTouched + 30 {
		many = append(many, fmt.Sprintf("e%d", i))
	}
	if err := e.hub.AnnounceTouched(t.Context(), room, "a", "A 的 agent", map[string][]string{"hero": many, "foot": {"x"}}); err != nil {
		t.Fatal(err)
	}
	settle(t, b)
	got := b.touchedBy()["A 的 agent"]
	if total := len(got["hero"]) + len(got["foot"]); total != realtime.MaxTouched {
		t.Errorf("the announcement names %d elements, want %d", total, realtime.MaxTouched)
	}
}

func TestWhatABrowserSaysItTouchedDoesNotMakeItAnAgent(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	mallory, watcher := e.connect(room, bob), e.connect(room, alice)
	settle(t, mallory, watcher)
	mallory.aw.SetLocalState(map[string]any{"user": map[string]any{"name": "Fake 的 Claude Code", "agent": true}, "touched": map[string]any{"hero": []string{"e1"}}})
	if err := mallory.send(frame(1, mallory.aw.EncodeUpdate([]uint64{uint64(mallory.doc.ClientID())}))); err != nil {
		t.Fatal(err)
	}
	settle(t, mallory, watcher)
	if got := watcher.touchedBy(); len(got) != 0 {
		t.Errorf("a browser is shown as an agent that changed elements: %v", got)
	}
}
