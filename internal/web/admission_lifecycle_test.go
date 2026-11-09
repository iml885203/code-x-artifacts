package web

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestPublishMetadataHelpersDoNotReenterAdmissionWhileAnAuthorityWriterWaits(t *testing.T) {
	t.Parallel()
	for _, visibility := range []store.Visibility{"", store.VisibilityOrganisation} {
		t.Run(string(visibility), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			h, probe, _ := newAdmissionHarness(t)
			id := ownDesignForAdmission(t, h)
			_ = liveDesignFrame(t, h, id, "frames/hero.html")
			read, err := h.app.svc.ReadSourceFile(ctx, h.app.svc.Viewer("alice"), id, "frames/hero.html", 0)
			if err != nil {
				t.Fatal(err)
			}
			gate := blockSourceMerges(h)
			text := strings.Replace(read.Text, "Text", "Updated", 1)
			probe.enabled.Store(true)
			result := make(chan error, 1)
			go func() {
				_, err := h.app.svc.Update(ctx, h.app.svc.Viewer("alice"), id, service.PublishInput{Title: "Renamed", Visibility: visibility, Content: designManifest, BaseVersion: 1, Files: map[string]*string{"frames/hero.html": &text}, SourceBases: map[string]string{"frames/hero.html": read.SourceBase}})
				result <- err
			}()
			awaitAdmission(ctx, t, probe, id, visibility != "")
			awaitSignal(ctx, t, gate.entered, "publish did not reach room barrier")
			shared := make(chan error, 1)
			go func() {
				_, err := h.app.svc.Share(ctx, h.app.svc.Viewer("alice"), id, service.ShareInput{AddEditors: []string{"carol"}})
				shared <- err
			}()
			awaitAdmission(ctx, t, probe, id, true)
			close(gate.resume)
			if err := awaitError(ctx, t, result); err != nil {
				t.Fatal(err)
			}
			if err := awaitError(ctx, t, shared); err != nil {
				t.Fatal(err)
			}
			after, err := h.st.Artifact(ctx, id)
			if err != nil || after.Title != "Renamed" {
				t.Fatal("metadata publish failed or reentered admission")
			}
		})
	}
}

type blockedRead struct {
	once sync.Once
	*roomkind.Snapshotter
	entered, resume chan struct{}
}

func (g *blockedRead) SourceFile(ctx context.Context, id, name, path string) (kind.SourceFile, error) {
	source, err := g.Snapshotter.SourceFile(ctx, id, name, path)
	g.once.Do(func() { close(g.entered) })
	select {
	case <-g.resume:
	case <-ctx.Done():
		return kind.SourceFile{}, ctx.Err()
	}
	return source, err
}

func TestRestoreCannotRetireWhileAnAdmittedSourceReadHasNotRememberedItsProof(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	h, probe, _ := newAdmissionHarness(t)
	id := ownDesignForAdmission(t, h)
	_ = liveDesignFrame(t, h, id, "frames/hero.html")
	gate := &blockedRead{Snapshotter: h.app.live.snap, entered: make(chan struct{}), resume: make(chan struct{})}
	h.app.svc.SetLiveRooms(gate)
	probe.enabled.Store(true)
	reads := make(chan service.SourceFileRead, 1)
	failed := make(chan error, 1)
	go func() {
		read, err := h.app.svc.ReadSourceFile(ctx, h.app.svc.Viewer("alice"), id, "frames/hero.html", 0)
		reads <- read
		failed <- err
	}()
	awaitAdmission(ctx, t, probe, id, false)
	awaitSignal(ctx, t, gate.entered, "source read did not reach snapshot barrier")
	restored := make(chan error, 1)
	go func() { _, err := h.app.svc.Restore(ctx, h.app.svc.Viewer("alice"), id, 1, 1); restored <- err }()
	awaitAdmission(ctx, t, probe, id, true)
	close(gate.resume)
	if err := awaitError(ctx, t, failed); err != nil {
		t.Fatal(err)
	}
	if err := awaitError(ctx, t, restored); err != nil {
		t.Fatal(err)
	}
	read := <-reads
	probe.enabled.Store(false)
	now, err := h.st.Artifact(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(read.Text, "Text", "Retired", 1)
	_, err = h.app.svc.Update(ctx, h.app.svc.Viewer("alice"), id, service.PublishInput{Title: "Landing", Content: designManifest, BaseVersion: now.Version, Files: map[string]*string{"frames/hero.html": &text}, SourceBases: map[string]string{"frames/hero.html": read.SourceBase}})
	if service.CodeOf(err) != service.Conflict {
		t.Fatalf("read proof escaped restore retirement: %v", err)
	}
}

type blockedApply struct {
	*roomkind.Snapshotter
	entered, resume chan struct{}
}

func (g *blockedApply) Apply(ctx context.Context, id, name string, bundle kind.Bundle, who string) error {
	close(g.entered)
	select {
	case <-g.resume:
	case <-ctx.Done():
		return ctx.Err()
	}
	return g.Snapshotter.Apply(ctx, id, name, bundle, who)
}

func TestReadAttemptDuringRestoreReturnsOnlyPostRestoreSourceAndAUsableNewProof(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	h, probe, _ := newAdmissionHarness(t)
	id := ownDesignForAdmission(t, h)
	_ = liveDesignFrame(t, h, id, "frames/hero.html")
	if err := h.app.live.hub.Edit(ctx, id, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return designroom.Open(doc).SetStyle("frames/hero.html", "box", "color", "green")
	}); err != nil {
		t.Fatal(err)
	}
	gate := &blockedApply{Snapshotter: h.app.live.snap, entered: make(chan struct{}), resume: make(chan struct{})}
	h.app.svc.SetLiveRooms(gate)
	probe.enabled.Store(true)
	restored := make(chan error, 1)
	go func() { _, err := h.app.svc.Restore(ctx, h.app.svc.Viewer("alice"), id, 1, 1); restored <- err }()
	awaitAdmission(ctx, t, probe, id, true)
	awaitSignal(ctx, t, gate.entered, "restore did not reach apply barrier")
	reads := make(chan service.SourceFileRead, 1)
	failed := make(chan error, 1)
	go func() {
		read, err := h.app.svc.ReadSourceFile(ctx, h.app.svc.Viewer("alice"), id, "frames/hero.html", 0)
		reads <- read
		failed <- err
	}()
	awaitAdmission(ctx, t, probe, id, false)
	close(gate.resume)
	if err := awaitError(ctx, t, restored); err != nil {
		t.Fatal(err)
	}
	if err := awaitError(ctx, t, failed); err != nil {
		t.Fatal(err)
	}
	read := <-reads
	if strings.Contains(read.Text, "green") || !strings.Contains(read.Text, "red") {
		t.Fatal("read escaped restore with pre-restore source")
	}
	probe.enabled.Store(false)
	now, err := h.st.Artifact(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(read.Text, "Text", "Fresh", 1)
	if _, err := h.app.svc.Update(ctx, h.app.svc.Viewer("alice"), id, service.PublishInput{Title: "Landing", Content: designManifest, BaseVersion: now.Version, Files: map[string]*string{"frames/hero.html": &text}, SourceBases: map[string]string{"frames/hero.html": read.SourceBase}}); err != nil {
		t.Fatal(err)
	}
}
