package web

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/reearth/ygo/crdt"
)

type blockedMerges struct {
	*roomkind.Snapshotter
	entered chan struct{}
	resume  chan struct{}
}

func (g *blockedMerges) MergeExpect(ctx context.Context, id, name string, base, next kind.Bundle, who string, expect kind.SourceExpectation) (map[string][]string, error) {
	g.entered <- struct{}{}
	select {
	case <-g.resume:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return g.Snapshotter.MergeExpect(ctx, id, name, base, next, who, expect)
}

func blockSourceMerges(h *liveHarness) *blockedMerges {
	gate := &blockedMerges{Snapshotter: h.app.live.snap, entered: make(chan struct{}, 4), resume: make(chan struct{})}
	h.app.svc.SetLiveRooms(gate)
	return gate
}

func TestTwoAdmittedSourceWritersCoexistAndPreserveIndependentChanges(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	h, _, _ := newAdmissionHarness(t)
	id := ownDesignForAdmission(t, h)
	_ = liveDesignFrame(t, h, id, "frames/hero.html")
	read, err := h.app.svc.ReadSourceFile(ctx, h.app.svc.Viewer("alice"), id, "frames/hero.html", 0)
	if err != nil {
		t.Fatal(err)
	}
	gate := blockSourceMerges(h)
	result := make(chan error, 2)
	for _, text := range []string{strings.Replace(read.Text, "red", "blue", 1), strings.Replace(read.Text, "Text", "Second", 1)} {
		go func() {
			_, err := h.app.svc.Update(ctx, h.app.svc.Viewer("alice"), id, service.PublishInput{Title: "Landing", Content: designManifest, BaseVersion: 1, Files: map[string]*string{"frames/hero.html": &text}, SourceBases: map[string]string{"frames/hero.html": read.SourceBase}})
			result <- err
		}()
	}
	awaitSignal(ctx, t, gate.entered, "first writer did not reach room")
	awaitSignal(ctx, t, gate.entered, "shared admission serialized independent writers")
	close(gate.resume)
	for range 2 {
		if err := awaitError(ctx, t, result); err != nil {
			t.Fatal(err)
		}
	}
	source := liveDesignFrame(t, h, id, "frames/hero.html")
	if !strings.Contains(source, "blue") || !strings.Contains(source, "Second") {
		t.Fatal("independent source changes lost")
	}
}

func TestCommitAdmittedBeforeRevocationFinishesAndIsAttributedBeforeOldPeerCloses(t *testing.T) {
	t.Parallel()
	h, probe, _ := newAdmissionHarness(t)
	id := ownDesignForAdmission(t, h)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	peer, err := h.app.live.hub.Join(ctx, id, realtime.Identity{Username: "bob"}, "")
	if err != nil {
		t.Fatal(err)
	}
	read, err := h.app.svc.ReadSourceFile(ctx, h.app.svc.Viewer("bob"), id, "frames/hero.html", 0)
	if err != nil {
		t.Fatal(err)
	}
	token := h.token("bob")
	p, err := h.st.AuthenticateToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	p.Transport = "mcp"
	gate := blockSourceMerges(h)
	text := strings.Replace(read.Text, "Text", "Committed", 1)
	probe.enabled.Store(true)
	published := make(chan error, 1)
	go func() {
		_, err := h.app.svc.Update(service.WithTokenProvenance(ctx, p), h.app.svc.Viewer("bob"), id, service.PublishInput{Title: "Landing", Content: designManifest, BaseVersion: 1, Files: map[string]*string{"frames/hero.html": &text}, SourceBases: map[string]string{"frames/hero.html": read.SourceBase}})
		published <- err
	}()
	awaitAdmission(ctx, t, probe, id, false)
	awaitSignal(ctx, t, gate.entered, "publish did not reach mutation barrier")
	revoked := make(chan error, 1)
	go func() {
		_, err := h.app.svc.Share(ctx, h.app.svc.Viewer("alice"), id, service.ShareInput{RemoveEditors: []string{"bob"}})
		revoked <- err
	}()
	awaitAdmission(ctx, t, probe, id, true)
	close(gate.resume)
	if err := awaitError(ctx, t, published); err != nil {
		t.Fatal(err)
	}
	if err := awaitError(ctx, t, revoked); err != nil {
		t.Fatal(err)
	}
	awaitSignal(ctx, t, peer.Done(), "old writable peer not closed")
	if !strings.Contains(liveDesignFrame(t, h, id, "frames/hero.html"), "Committed") {
		t.Fatal("admitted source commit lost")
	}
	entries := activityPage(t, h.harness, token, "").Items
	if len(entries) != 1 || entries[0].Operation != "publish" {
		t.Fatal("admitted source activity missing or duplicated")
	}
}

func TestEpochReplacementInsideTheReadMutationGapRefusesWithoutAppendOrBroadcast(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	h, _, _ := newAdmissionHarness(t)
	id := ownDesignForAdmission(t, h)
	_ = liveDesignFrame(t, h, id, "frames/hero.html")
	read, err := h.app.svc.ReadSourceFile(ctx, h.app.svc.Viewer("alice"), id, "frames/hero.html", 0)
	if err != nil {
		t.Fatal(err)
	}
	gate := blockSourceMerges(h)
	text := strings.Replace(read.Text, "Text", "Stale epoch", 1)
	result := make(chan error, 1)
	go func() {
		_, err := h.app.svc.Update(ctx, h.app.svc.Viewer("alice"), id, service.PublishInput{Title: "Landing", Content: designManifest, BaseVersion: 1, Files: map[string]*string{"frames/hero.html": &text}, SourceBases: map[string]string{"frames/hero.html": read.SourceBase}})
		result <- err
	}()
	awaitSignal(ctx, t, gate.entered, "publish did not reach mutation barrier")
	var state []byte
	if err := h.app.live.hub.Read(ctx, id, func(doc *crdt.Doc) error { state = crdt.EncodeStateAsUpdateV1(doc, nil); return nil }); err != nil {
		t.Fatal(err)
	}
	// Native log replacement also defends against a protocol caller with stale history.
	if err := h.app.live.hub.Replace(ctx, id, func(doc *crdt.Doc) error { return crdt.ApplyUpdateV1(doc, state, nil) }); err != nil {
		t.Fatal(err)
	}
	observer, err := h.app.live.hub.Join(ctx, id, realtime.Identity{Username: "alice"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for len(observer.Out()) > 0 {
		<-observer.Out()
	}
	before, _ := h.st.RealtimeRows(ctx, id)
	close(gate.resume)
	if err := awaitError(ctx, t, result); service.CodeOf(err) != service.Conflict {
		t.Fatalf("old epoch source admitted: %v", err)
	}
	after, _ := h.st.RealtimeRows(ctx, id)
	if before != after || len(observer.Out()) != 0 || strings.Contains(liveDesignFrame(t, h, id, "frames/hero.html"), "Stale epoch") {
		t.Fatal("stale epoch appended or broadcast")
	}
}
