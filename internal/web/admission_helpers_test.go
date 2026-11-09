package web

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/artifactguard"
	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/reearth/ygo/crdt"
	"github.com/reearth/ygo/encoding"
	ygsync "github.com/reearth/ygo/sync"
)

type admissionAttempt struct {
	id        string
	exclusive bool
}

// probeAdmission substitutes scheduling only; every acquisition reaches one real registry.
type probeAdmission struct {
	real     artifactguard.Registry
	enabled  atomic.Bool
	attempts chan admissionAttempt
}

func (p *probeAdmission) Shared(id string) func()    { p.note(id, false); return p.real.Shared(id) }
func (p *probeAdmission) Exclusive(id string) func() { p.note(id, true); return p.real.Exclusive(id) }

func (p *probeAdmission) note(id string, exclusive bool) {
	if p.enabled.Load() {
		p.attempts <- admissionAttempt{id, exclusive}
	}
}

func awaitAdmission(ctx context.Context, t *testing.T, p *probeAdmission, id string, exclusive bool) {
	t.Helper()
	select {
	case a := <-p.attempts:
		if a.id != id || a.exclusive != exclusive {
			t.Fatal("unexpected admission attempt")
		}
	case <-ctx.Done():
		t.Fatal("operation never reached admission")
	}
}

type accessBarrier struct {
	active  atomic.Bool
	entered chan struct{}
	resume  chan struct{}
}

func newAdmissionHarness(t *testing.T) (*liveHarness, *probeAdmission, *accessBarrier) {
	t.Helper()
	probe := &probeAdmission{attempts: make(chan admissionAttempt, 32)}
	var cfg Config
	h := newLiveHarness(t, func(c *Config) { c.Admission = probe; cfg = *c })
	// Install a scheduling barrier before the actual production access watcher.
	// All business collaborators remain real and use the same store and registry.
	h.app.svc = newService(cfg)
	h.app.svc.SetClock(h.clock.Now)
	h.app.live = newLive(cfg, h.app.svc)
	barrier := &accessBarrier{entered: make(chan struct{}), resume: make(chan struct{})}
	h.app.svc.OnAccessChange(func(string) {
		if barrier.active.Load() {
			close(barrier.entered)
			select {
			case <-barrier.resume:
			case <-time.After(2 * time.Second):
				t.Error("access watcher barrier timed out before explicit release")
			case <-t.Context().Done():
				t.Error("access watcher barrier canceled before explicit release")
			}
		}
	})
	h.app.svc.OnAccessChange(h.app.liveAccessChanged)
	t.Cleanup(func() {
		probe.enabled.Store(false)
		select {
		case <-barrier.resume:
		default:
			close(barrier.resume)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		h.app.live.hub.Close(ctx)
	})
	return h, probe, barrier
}

func peerDesignMessage(t *testing.T, h *liveHarness, id string) []byte {
	t.Helper()
	var state []byte
	if err := h.app.live.hub.Read(t.Context(), id, func(doc *crdt.Doc) error { state = crdt.EncodeStateAsUpdateV1(doc, nil); return nil }); err != nil {
		t.Fatal(err)
	}
	doc := crdt.New()
	if err := crdt.ApplyUpdateV1(doc, state, nil); err != nil {
		t.Fatal(err)
	}
	var update []byte
	doc.OnUpdate(func(b []byte, _ any) { update = b })
	write, err := designroom.Open(doc).SetStyle("frames/hero.html", "box", "color", "blue")
	if err != nil {
		t.Fatal(err)
	}
	doc.Transact(write, nil)
	enc := encoding.NewEncoder()
	enc.WriteVarUint(0)
	return append(enc.Bytes(), ygsync.EncodeUpdate(update)...)
}

func awaitSignal(ctx context.Context, t *testing.T, ch <-chan struct{}, why string) {
	t.Helper()
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatal(why)
	}
}

func awaitError(ctx context.Context, t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		t.Fatal("operation did not finish")
		return ctx.Err()
	}
}

func ownDesignForAdmission(t *testing.T, h *liveHarness) string {
	t.Helper()
	tok := h.token("alice")
	frames := designFrames()
	frames["frames/hero.html"] = `<p data-pid="box" style="color:red">Text</p>`
	a := h.publishDesign(tok, frames, "")
	if _, err := h.app.svc.Share(t.Context(), h.app.svc.Viewer("alice"), a.ID, service.ShareInput{AddEditors: []string{"bob"}}); err != nil {
		t.Fatal(err)
	}
	return a.ID
}
