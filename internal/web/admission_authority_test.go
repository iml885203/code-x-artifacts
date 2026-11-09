package web

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestDeleteAndVisibilityChangesExcludeAnOverlappingOldPeerWrite(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"delete", "visibility"} {
		t.Run(mode, func(t *testing.T) { t.Parallel(); exerciseAuthorityOverlap(t, mode) })
	}
}

func exerciseAuthorityOverlap(t *testing.T, mode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	h, probe, barrier := newAdmissionHarness(t)
	id := ownDesignForAdmission(t, h)
	if mode == "visibility" {
		if _, err := h.app.svc.SetVisibility(ctx, h.app.svc.Viewer("alice"), id, store.VisibilityOrganisation); err != nil {
			t.Fatal(err)
		}
		enabled := true
		if _, err := h.app.svc.Share(ctx, h.app.svc.Viewer("alice"), id, service.ShareInput{RemoveEditors: []string{"bob"}, OrganisationEdit: &enabled}); err != nil {
			t.Fatal(err)
		}
	}
	peer, err := h.app.live.hub.Join(ctx, id, realtime.Identity{Username: "bob"}, "")
	if err != nil {
		t.Fatal(err)
	}
	message := peerDesignMessage(t, h, id)
	rows, _ := h.st.RealtimeRows(ctx, id)
	barrier.active.Store(true)
	changed := make(chan error, 1)
	go func() {
		if mode == "delete" {
			changed <- h.app.svc.Delete(ctx, h.app.svc.Viewer("alice"), id)
			return
		}
		_, err := h.app.svc.SetVisibility(ctx, h.app.svc.Viewer("alice"), id, store.VisibilityPrivate)
		changed <- err
	}()
	awaitSignal(ctx, t, barrier.entered, "authority change did not reach watcher barrier")
	probe.enabled.Store(true)
	received := make(chan error, 1)
	go func() { received <- peer.Receive(ctx, message) }()
	awaitAdmission(ctx, t, probe, id, false)
	close(barrier.resume)
	if err := awaitError(ctx, t, changed); err != nil {
		t.Fatal(err)
	}
	if err := awaitError(ctx, t, received); !errors.Is(err, realtime.ErrPeerClosed) {
		t.Fatalf("old peer survived authority change: %v", err)
	}
	after, _ := h.st.RealtimeRows(ctx, id)
	if mode == "delete" {
		rows = 0
	}
	if after != rows {
		t.Fatal("old peer appended after authority change")
	}
	probe.enabled.Store(false)
	if _, err := h.app.svc.Update(ctx, h.app.svc.Viewer("bob"), id, service.PublishInput{}); service.CodeOf(err) != service.NotFound {
		t.Fatal("former editor still has source authority")
	}
}
