package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

func TestRevocationInFlightExcludesPeerPublishAndSocketJoinUntilTheWatcherClosesOldRights(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	h, probe, barrier := newAdmissionHarness(t)
	id := ownDesignForAdmission(t, h)
	bobBrowser := h.browser("bob", "pw-b")
	ticket := h.mustTicket(bobBrowser, id)
	peer, err := h.app.live.hub.Join(ctx, id, realtime.Identity{Username: "bob"}, "")
	if err != nil {
		t.Fatal(err)
	}
	observer, err := h.app.live.hub.Join(ctx, id, realtime.Identity{Username: "alice"}, "")
	if err != nil {
		t.Fatal(err)
	}
	message := peerDesignMessage(t, h, id)
	read, err := h.app.svc.ReadSourceFile(ctx, h.app.svc.Viewer("bob"), id, "frames/hero.html", 0)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(read.Text, "red", "green", 1)
	bobToken := h.token("bob")
	provenance, err := h.st.AuthenticateToken(ctx, bobToken)
	if err != nil {
		t.Fatal(err)
	}
	provenance.Transport = "mcp"
	before, _ := h.st.RealtimeRows(ctx, id)
	activityBefore := len(activityPage(t, h.harness, bobToken, "").Items)
	for len(observer.Out()) > 0 {
		<-observer.Out()
	}
	barrier.active.Store(true)
	revoked := make(chan error, 1)
	go func() {
		_, err := h.app.svc.Share(ctx, h.app.svc.Viewer("alice"), id, service.ShareInput{RemoveEditors: []string{"bob"}})
		revoked <- err
	}()
	awaitSignal(ctx, t, barrier.entered, "revocation did not reach watcher barrier")
	probe.enabled.Store(true)
	received := make(chan error, 1)
	go func() { received <- peer.Receive(ctx, message) }()
	select {
	case attempt := <-probe.attempts:
		if attempt.id != id || attempt.exclusive {
			t.Fatal("incorrect receive admission")
		}
	case err := <-received:
		rows, _ := h.st.RealtimeRows(ctx, id)
		t.Fatalf("peer completed inside revocation before watcher: err=%v rows=%d before=%d broadcasts=%d", err, rows, before, len(observer.Out()))
	case <-ctx.Done():
		t.Fatal("peer did not attempt admission")
	}
	published := make(chan error, 1)
	go func() {
		_, err := h.app.svc.Update(service.WithTokenProvenance(ctx, provenance), h.app.svc.Viewer("bob"), id, service.PublishInput{Title: "Landing", Content: designManifest, BaseVersion: 1, Files: map[string]*string{"frames/hero.html": &changed}, SourceBases: map[string]string{"frames/hero.html": read.SourceBase}})
		published <- err
	}()
	awaitAdmission(ctx, t, probe, id, false)
	joined := make(chan int, 1)
	go func() {
		conn, status, _ := h.dial(bobBrowser, id, ticket.Ticket, "", nil)
		if conn != nil {
			_ = conn.CloseNow()
		}
		joined <- status
	}()
	awaitAdmission(ctx, t, probe, id, false)
	close(barrier.resume)
	assertRevokedAdmissionResults(ctx, t, h, id, before, activityBefore, observer, revoked, received, published, joined)
}

func assertRevokedAdmissionResults(ctx context.Context, t *testing.T, h *liveHarness, id string, before, activityBefore int, observer *realtime.Peer, revoked, received, published <-chan error, joined <-chan int) {
	t.Helper()
	if err := awaitError(ctx, t, revoked); err != nil {
		t.Fatal(err)
	}
	if err := awaitError(ctx, t, received); !errors.Is(err, realtime.ErrPeerClosed) {
		t.Fatalf("revoked peer write admitted: %v", err)
	}
	if err := awaitError(ctx, t, published); service.CodeOf(err) != service.NotFound {
		t.Fatalf("revoked publisher admitted: %v", err)
	}
	select {
	case status := <-joined:
		if status != http.StatusNotFound {
			t.Fatal("revoked join acquired stale rights")
		}
	case <-ctx.Done():
		t.Fatal("join did not finish")
	}
	rows, _ := h.st.RealtimeRows(ctx, id)
	if rows != before || len(observer.Out()) != 0 {
		t.Fatal("revoked operations appended, broadcast or recorded activity")
	}
	assertNoRevokedActivityOrSource(ctx, t, h, id, activityBefore)
}

func assertNoRevokedActivityOrSource(ctx context.Context, t *testing.T, h *liveHarness, id string, before int) {
	t.Helper()
	page, err := h.app.svc.AgentActivity(ctx, h.app.svc.Viewer("bob"), service.ActivityQuery{})
	if err != nil || len(page.Items) != before {
		t.Fatal("revoked operation recorded activity")
	}
	held, err := h.app.live.snap.Content(ctx, id, "design")
	if err != nil || strings.Contains(held.Files["frames/hero.html"], "blue") {
		t.Fatal("revoked peer source survived")
	}
}
