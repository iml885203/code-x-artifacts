package web

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

type firstMergeBarrier struct {
	*roomkind.Snapshotter
	paused          atomic.Bool
	entered, resume chan struct{}
}

func (g *firstMergeBarrier) MergeExpect(ctx context.Context, id, name string, base, next kind.Bundle, who string, expect kind.SourceExpectation) (map[string][]string, error) {
	if g.paused.CompareAndSwap(false, true) {
		close(g.entered)
		select {
		case <-g.resume:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return g.Snapshotter.MergeExpect(ctx, id, name, base, next, who, expect)
}

func TestConcurrentCreationCannotBeOverwrittenByAnAlreadyResolvedAbsentProof(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	h, _, _ := newAdmissionHarness(t)
	id := ownDesignForAdmission(t, h)
	_ = liveDesignFrame(t, h, id, "frames/hero.html")
	manifest, err := h.app.svc.ReadSource(ctx, h.app.svc.Viewer("alice"), id)
	if err != nil {
		t.Fatal(err)
	}
	absent, err := h.app.svc.ReadSourceFile(ctx, h.app.svc.Viewer("alice"), id, "frames/new.html", 0)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Replace(manifest.Artifact.Content, "frames/footer.html", "frames/new.html", 1)
	first, second := "<p>First pending</p>", "<p>Other creation</p>"
	makeInput := func(text *string) service.PublishInput {
		return service.PublishInput{Title: "Landing", Content: content, BaseVersion: 1, Files: map[string]*string{"frames/new.html": text}, SourceBases: map[string]string{"design.json": manifest.SourceBases["design.json"], "frames/new.html": absent.SourceBase}}
	}
	gate := &firstMergeBarrier{Snapshotter: h.app.live.snap, entered: make(chan struct{}), resume: make(chan struct{})}
	h.app.svc.SetLiveRooms(gate)
	result := make(chan error, 1)
	go func() {
		_, err := h.app.svc.Update(ctx, h.app.svc.Viewer("alice"), id, makeInput(&first))
		result <- err
	}()
	awaitSignal(ctx, t, gate.entered, "first creation did not reach mutation gap")
	if _, err := h.app.svc.Update(ctx, h.app.svc.Viewer("alice"), id, makeInput(&second)); err != nil {
		t.Fatal(err)
	}
	before := liveDesignFrame(t, h, id, "frames/new.html")
	rows, _ := h.st.RealtimeRows(ctx, id)
	close(gate.resume)
	if err := awaitError(ctx, t, result); service.CodeOf(err) != service.Conflict {
		t.Fatalf("resolved absence overwrote creation: %v", err)
	}
	afterRows, _ := h.st.RealtimeRows(ctx, id)
	if before != liveDesignFrame(t, h, id, "frames/new.html") || rows != afterRows {
		t.Fatal("conflicting creation mutated source/log")
	}
}

func TestConcurrentPeerEditSurvivesAnAlreadyResolvedWholeFrameDeletion(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	h, _, _ := newAdmissionHarness(t)
	id := ownDesignForAdmission(t, h)
	peer, err := h.app.live.hub.Join(ctx, id, realtime.Identity{Username: "alice"}, "")
	if err != nil {
		t.Fatal(err)
	}
	message := peerDesignMessage(t, h, id)
	read, err := h.app.svc.ReadSourceFile(ctx, h.app.svc.Viewer("alice"), id, "frames/hero.html", 0)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := h.app.svc.ReadSource(ctx, h.app.svc.Viewer("alice"), id)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Replace(manifest.Artifact.Content, `{"id":"hero","name":"Hero","file":"frames/hero.html","w":1440,"h":900},`, "", 1)
	if content == manifest.Artifact.Content {
		t.Fatal("fixture did not remove hero reference")
	}
	gate := blockSourceMerges(h)
	result := make(chan error, 1)
	go func() {
		_, err := h.app.svc.Update(ctx, h.app.svc.Viewer("alice"), id, service.PublishInput{Title: "Landing", Content: content, BaseVersion: 1, Files: map[string]*string{"frames/hero.html": nil}, SourceBases: map[string]string{"design.json": manifest.SourceBases["design.json"], "frames/hero.html": read.SourceBase}})
		result <- err
	}()
	awaitSignal(ctx, t, gate.entered, "deletion did not reach mutation gap")
	if err := peer.Receive(ctx, message); err != nil {
		t.Fatal(err)
	}
	before := liveDesignFrame(t, h, id, "frames/hero.html")
	rows, _ := h.st.RealtimeRows(ctx, id)
	close(gate.resume)
	if err := awaitError(ctx, t, result); service.CodeOf(err) != service.Conflict {
		t.Fatalf("peer source deleted: %v", err)
	}
	afterRows, _ := h.st.RealtimeRows(ctx, id)
	if before != liveDesignFrame(t, h, id, "frames/hero.html") || rows != afterRows {
		t.Fatal("refused deletion changed peer source/log")
	}
}
