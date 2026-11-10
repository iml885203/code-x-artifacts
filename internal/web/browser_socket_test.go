package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestRevokedBrowserTicketsCannotUpgradeButIndependentTicketsStillCan(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t)
	old := w.h.mustTicket(w.alice, w.private.ID)
	unaffected := w.h.mustTicket(w.bob, w.private.ID)
	r, _ := w.h.do(w.alice, "POST", "/logout", "", map[string]string{"Origin": w.h.srv.URL})
	if r.StatusCode != 303 {
		t.Fatal(r.StatusCode)
	}
	c, status, err := w.h.dial(w.alice, w.private.ID, old.Ticket, "", nil)
	if c != nil {
		_ = c.CloseNow()
	}
	if err == nil || status != 401 {
		t.Fatalf("revoked ticket upgraded: %d %v", status, err)
	}
	for _, ticket := range []ticketReply{unaffected} {
		c, status, err = w.h.dial(w.bob, w.private.ID, ticket.Ticket, "", nil)
		if err != nil {
			t.Fatalf("independent ticket refused: %d %v", status, err)
		}
		_ = c.CloseNow()
	}
}

func TestBrowserSocketJoinHoldsRevocationAuthorityThroughUpgrade(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	w := newLiveWorld(t)
	ticket := w.h.mustTicket(w.alice, w.private.ID)
	entered, resume := make(chan struct{}), make(chan struct{})
	handler := w.h.app.Handler()
	server := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/ws/") {
			handler.ServeHTTP(&blockedUpgrade{rw, ctx, entered, resume}, r)
			return
		}
		handler.ServeHTTP(rw, r)
	}))
	defer server.Close()
	joined := make(chan int, 1)
	go func() {
		c, r, _ := websocket.Dial(ctx, server.URL+"/ws/"+w.private.ID+"?ticket="+ticket.Ticket, &websocket.DialOptions{HTTPClient: server.Client(), HTTPHeader: http.Header{"Origin": {server.URL}}})
		if c != nil {
			_ = c.CloseNow()
		}
		if r != nil {
			if r.Body != nil {
				_ = r.Body.Close()
			}
			joined <- r.StatusCode
		} else {
			joined <- 0
		}
	}()
	awaitSignal(ctx, t, entered, "socket did not reach upgrade")
	// At this externally observed handshake boundary, revocation must not be
	// able to acquire authority until Join finishes. No scheduling sleep needed.
	if w.h.app.browserAdmission.TryLock() {
		w.h.app.browserAdmission.Unlock()
		close(resume)
		t.Fatal("revocation can straddle browser validation and Join")
	}
	close(resume)
	select {
	case status := <-joined:
		if status != 101 {
			t.Fatal(status)
		}
	case <-ctx.Done():
		t.Fatal("upgrade stuck")
	}
	r, _ := w.h.do(w.alice, "POST", "/logout", "", map[string]string{"Origin": w.h.srv.URL})
	if r.StatusCode != 303 {
		t.Fatal(r.StatusCode)
	}
}
