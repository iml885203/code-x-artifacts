package service

import (
	"testing"
	"time"
)

type fakeClock struct{ at time.Time }

func (c *fakeClock) now() time.Time { return c.at }

func names(list []ViewerEntry) []string {
	out := []string{}
	for _, e := range list {
		out = append(out, e.Name)
	}
	return out
}

func TestAPersonStaysInTheListAMinuteAfterTheirLastHeartbeatAndNoLonger(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)
	clock := &fakeClock{at: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)}
	svc.now = clock.now
	alice, bob := svc.Viewer("alice"), svc.Viewer("bob")
	p, err := svc.Create(t.Context(), alice, PublishInput{Title: "Page", Content: "<p>x", Visibility: "organisation"})
	if err != nil {
		t.Fatal(err)
	}
	beat := func(v Viewer) {
		t.Helper()
		if _, err := svc.Heartbeat(t.Context(), v, p.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	seenByAlice := func() []string {
		t.Helper()
		list, err := svc.Viewers(t.Context(), alice, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		return names(list)
	}

	beat(bob)
	if got := seenByAlice(); len(got) != 1 || got[0] != "bob" {
		t.Fatalf("right after the heartbeat: %v", got)
	}
	// Heartbeats every 20 s keep bob for as long as they last.
	for range 6 {
		clock.at = clock.at.Add(HeartbeatEvery(PersonStays))
		beat(bob)
	}
	if got := seenByAlice(); len(got) != 1 {
		t.Fatalf("two minutes of heartbeats: %v", got)
	}
	clock.at = clock.at.Add(PersonStays - time.Second)
	if got := seenByAlice(); len(got) != 1 {
		t.Errorf("just under a minute after the last heartbeat: %v", got)
	}
	clock.at = clock.at.Add(time.Second)
	if got := seenByAlice(); len(got) != 0 {
		t.Errorf("a minute after the last heartbeat: %v", got)
	}
}

func TestSeveralTabsOfOnePersonAreOneEntryAndYouAreFirst(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)
	alice, bob := svc.Viewer("alice"), svc.Viewer("bob")
	p, err := svc.Create(t.Context(), alice, PublishInput{Title: "Page", Content: "<p>x", Visibility: "organisation"})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 { // three tabs
		if _, err := svc.Heartbeat(t.Context(), bob, p.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Heartbeat(t.Context(), alice, p.ID, ""); err != nil {
		t.Fatal(err)
	}
	list, err := svc.Viewers(t.Context(), bob, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "bob" || !list[0].Self || list[1].Name != "alice" || list[1].Self {
		t.Errorf("the list for bob = %+v, want himself then alice, once each", list)
	}
}

func TestAnAgentIsAViewerForItsIdleTimeAfterReadingWithItsNameAndFlagAndNotInItsOwnAnswer(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)
	clock := &fakeClock{at: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)}
	svc.now = clock.now
	alice, bob := svc.Viewer("alice"), svc.Viewer("bob")
	p, err := svc.Create(t.Context(), alice, PublishInput{Title: "Page", Content: "<p>x", Visibility: "organisation"})
	if err != nil {
		t.Fatal(err)
	}
	agentCtx := WithAgent(t.Context(), "Claude Code")
	own, err := svc.Viewers(agentCtx, bob, p.ID)
	if err != nil || len(own) != 0 {
		t.Fatalf("the agent's own first read = %v, %v", own, err)
	}
	if own, _ = svc.Viewers(agentCtx, bob, p.ID); len(own) != 0 {
		t.Errorf("the agent sees itself: %v", own)
	}
	list, err := svc.Viewers(t.Context(), alice, p.ID)
	if err != nil || len(list) != 1 || list[0].Name != "bob 的 Claude Code" || !list[0].Agent || list[0].Self {
		t.Fatalf("what alice sees = %+v, %v", list, err)
	}
	clock.at = clock.at.Add(AgentStays)
	if list, _ = svc.Viewers(t.Context(), alice, p.ID); len(list) != 0 {
		t.Errorf("an agent that stopped reading is still shown after its idle time: %+v", list)
	}
}
