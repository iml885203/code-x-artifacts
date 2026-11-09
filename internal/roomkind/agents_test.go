package roomkind_test

import (
	"context"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

func TestAnAgentsDocEditShowsItsNameAtTheBlockItWroteAndItLeavesWhenIdle(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.editableDoc()
	watcher := e.connect(id, bob)

	ctx := service.WithAgent(t.Context(), "Claude Code")
	res, err := e.svc.DocEdit(ctx, e.svc.Viewer("alice"), id, service.DocEditInput{Ops: []docedit.Op{replace("p2", "agent was here")}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Saved {
		t.Fatal("the edit should be in the live room")
	}
	if got := watcher.agents(); len(got) != 1 || got["alice 的 Claude Code"] != "p2" {
		t.Fatalf("the roster has %v, want alice 的 Claude Code at p2", got)
	}

	// It goes after the idle time, as the server's sweep sees it.
	e.clock.Advance(14 * time.Second)
	e.hub.Sweep(t.Context())
	if len(watcher.agents()) != 1 {
		t.Fatal("the agent left too soon")
	}
	e.clock.Advance(time.Second)
	e.hub.Sweep(t.Context())
	if got := watcher.agents(); len(got) != 0 {
		t.Errorf("the agent is still in the roster: %v", got)
	}
}

func TestAnUnknownClientIsShownAsAgentAndAPersonIsNotShownAtAll(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.editableDoc()
	watcher := e.connect(id, bob)
	alice := e.svc.Viewer("alice")

	if _, err := e.svc.DocEdit(t.Context(), alice, id, service.DocEditInput{Ops: []docedit.Op{replace("p1", "a person's call")}}); err != nil {
		t.Fatal(err)
	}
	if got := watcher.agents(); len(got) != 0 {
		t.Errorf("a call that is not an agent's shows %v", got)
	}
	if _, err := e.svc.DocEdit(service.WithAgent(t.Context(), ""), alice, id, service.DocEditInput{Ops: []docedit.Op{replace("p1", "an unknown client")}}); err != nil {
		t.Fatal(err)
	}
	if got := watcher.agents(); len(got) != 1 || got["alice 的 agent"] != "p1" {
		t.Errorf("the roster has %v, want alice 的 agent", got)
	}
}

func TestAnAgentsWholeDocPublishShowsItAtTheFirstBlockThatChanged(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	watcher := e.connect(id, bob)
	blocks := e.roomBlocks(id)

	ctx := service.WithAgent(t.Context(), "Codex")
	if _, err := e.svc.Update(ctx, e.svc.Viewer("alice"), id, publishInput("# Plan\n\nfirst paragraph changed\n", 1)); err != nil {
		t.Fatal(err)
	}
	if got := watcher.agents(); len(got) != 1 || got["alice 的 Codex"] != blocks[1].ID {
		t.Errorf("the roster has %v, want alice 的 Codex at %s", got, blocks[1].ID)
	}
}

func TestTwoAgentsOfOneAccountAreTwoEntries(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.editableDoc()
	watcher := e.connect(id, bob)
	for _, client := range []string{"Claude Code", "Cursor"} {
		ctx := service.WithAgent(context.Background(), client)
		if _, err := e.svc.DocEdit(ctx, e.svc.Viewer("alice"), id, service.DocEditInput{Ops: []docedit.Op{replace("p1", "by "+client)}}); err != nil {
			t.Fatal(err)
		}
	}
	if got := watcher.agents(); len(got) != 2 {
		t.Errorf("the roster has %v", got)
	}
}
