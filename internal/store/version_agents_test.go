package store

import (
	"slices"
	"testing"
)

func TestAVersionNamesTheAgentsThatMadeItAndOtherVersionsNameNone(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	a := artifactBy(t, st, "alice")

	rev := Revision{Title: "t", Content: "<p>2", Author: "alice", Agents: []VersionAgent{{By: "alice", Client: "Claude Code"}, {By: "bob", Client: "agent"}}}
	if err := st.UpdateArtifact(ctx, a.ID, 1, rev, ""); err != nil {
		t.Fatal(err)
	}
	list, err := st.Versions(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int][]VersionAgent{1: {}, 2: {{By: "alice", Client: "Claude Code"}, {By: "bob", Client: "agent"}}}
	for _, v := range list {
		if !slices.Equal(v.Agents, want[v.Number]) || v.Agents == nil {
			t.Errorf("version %d agents = %#v, want %#v and never nil", v.Number, v.Agents, want[v.Number])
		}
		one, err := st.Version(ctx, a.ID, v.Number)
		if err != nil || !slices.Equal(one.Agents, want[v.Number]) || one.Agents == nil {
			t.Errorf("version %d read alone: agents = %#v, %v", v.Number, one.Agents, err)
		}
	}
}

func TestAPendingRoomRemembersWhichAgentsWroteInOrderAndForgetsThemWithTheRoom(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	a := artifactBy(t, st, "alice")
	for _, ag := range []VersionAgent{{"alice", "Claude Code"}, {"bob", "agent"}, {"alice", "Claude Code"}} {
		if err := st.MarkPendingAgent(ctx, a.ID, ag.By, ag.Client); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.PendingAgents(ctx, a.ID)
	if err != nil || !slices.Equal(got, []VersionAgent{{"alice", "Claude Code"}, {"bob", "agent"}}) {
		t.Fatalf("pending agents = %#v, %v, want alice's then bob's, once each", got, err)
	}
	if err := st.ClearPending(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.PendingAgents(ctx, a.ID); len(got) != 0 {
		t.Errorf("pending agents after the version = %#v", got)
	}
}
