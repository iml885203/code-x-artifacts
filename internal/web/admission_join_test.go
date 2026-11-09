package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/iml885203/code-x-artifacts/internal/service"
)

type blockedUpgrade struct {
	http.ResponseWriter
	ctx             context.Context
	entered, resume chan struct{}
}

func (w *blockedUpgrade) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *blockedUpgrade) WriteHeader(status int) {
	if status == http.StatusSwitchingProtocols {
		close(w.entered)
		select {
		case <-w.resume:
		case <-w.ctx.Done():
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func TestSocketRightsAndJoinCannotStraddleTheRevocationWatcher(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	h, probe, _ := newAdmissionHarness(t)
	id := ownDesignForAdmission(t, h)
	browser := h.browser("bob", "pw-b")
	ticket := h.mustTicket(browser, id)
	entered, resume := make(chan struct{}), make(chan struct{})
	handler := h.app.Handler()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/ws/") {
			handler.ServeHTTP(&blockedUpgrade{w, ctx, entered, resume}, r)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	client := server.Client()
	client.Jar = browser.Jar
	probe.enabled.Store(true)
	joined := make(chan struct{})
	var status int
	go func() {
		conn, response, _ := websocket.Dial(ctx, server.URL+"/ws/"+id+"?ticket="+ticket.Ticket, &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": {server.URL}}})
		if response != nil {
			status = response.StatusCode
			if response.Body != nil {
				_ = response.Body.Close()
			}
		}
		if conn != nil {
			defer func() { _ = conn.CloseNow() }()
			for {
				if _, _, err := conn.Read(ctx); err != nil {
					break
				}
			}
		}
		close(joined)
	}()
	awaitAdmission(ctx, t, probe, id, false)
	awaitSignal(ctx, t, entered, "join did not pause after deciding rights")
	revoked := make(chan error, 1)
	go func() {
		_, err := h.app.svc.Share(ctx, h.app.svc.Viewer("alice"), id, service.ShareInput{RemoveEditors: []string{"bob"}})
		revoked <- err
	}()
	awaitAdmission(ctx, t, probe, id, true)
	close(resume)
	if err := awaitError(ctx, t, revoked); err != nil {
		t.Fatal(err)
	}
	awaitSignal(ctx, t, joined, "revocation did not close just-admitted socket")
	if status != http.StatusSwitchingProtocols {
		t.Fatal("join admitted before revocation should finish its handshake")
	}
	for _, who := range h.app.live.hub.Connected(id) {
		if who.Username == "bob" {
			t.Fatal("stale socket escaped the watcher snapshot")
		}
	}
}
