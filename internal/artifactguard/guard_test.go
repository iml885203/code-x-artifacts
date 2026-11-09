package artifactguard

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestSharedAdmissionCoexistsAndOtherArtifactsDoNotWaitForAnExclusiveHolder(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var g Registry
	exclusive := g.Exclusive("locked")
	first := g.Shared("a")
	done := make(chan struct{})
	go func() { defer close(done); g.Shared("a")(); g.Exclusive("b")() }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("shared operations or independent artifacts blocked")
	}
	first()
	exclusive()
	if len(g.entries) != 0 {
		t.Fatal("completed entries retained")
	}
}

func TestWaitingAdmissionsKeepOneEntryUntilEveryHolderAndWaiterLeaves(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var g Registry
	exclusive := g.Exclusive("a")
	waiting := make(chan struct{})
	done := make(chan struct{})
	go func() { close(waiting); release := g.Shared("a"); release(); close(done) }()
	<-waiting
	// The exclusive owner remains the same entry even while a reader arrives.
	var held *entry
	for held == nil {
		g.mu.Lock()
		if e := g.entries["a"]; e != nil && e.refs == 2 {
			held = e
		}
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			t.Fatal("reader never retained the contended entry")
		default:
			runtime.Gosched()
		}
	}
	exclusive()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("waiter did not finish")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if held == nil || len(g.entries) != 0 {
		t.Fatal("entry lifecycle leaked")
	}
}
