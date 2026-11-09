package roomkind_test

import (
	"context"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

type pausedMerge struct {
	*roomkind.Snapshotter
	entered chan struct{}
	resume  chan struct{}
}

func (p *pausedMerge) MergeExpect(ctx context.Context, id, name string, base, next kind.Bundle, who string, expect kind.SourceExpectation) (map[string][]string, error) {
	close(p.entered)
	select {
	case <-p.resume:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return p.Snapshotter.MergeExpect(ctx, id, name, base, next, who, expect)
}

func TestConcurrentHumanEditCannotBeLostBetweenRemovalReadAndRoomMutation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	e := newEnv(t)
	id := e.design()
	e.roomDesign(id)
	read, err := e.svc.ReadSourceFile(ctx, e.svc.Viewer("alice"), id, heroPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := e.svc.ReadSource(ctx, e.svc.Viewer("alice"), id)
	if err != nil {
		t.Fatal(err)
	}
	gate := &pausedMerge{Snapshotter: e.snap, entered: make(chan struct{}), resume: make(chan struct{})}
	e.svc.SetLiveRooms(gate)
	result := make(chan error, 1)
	go func() {
		_, err := e.svc.Update(ctx, e.svc.Viewer("alice"), id, service.PublishInput{Title: "Landing", Content: `{"version":1,"pages":[{"id":"p","name":"P","rows":[[{"id":"foot","name":"Foot","file":"frames/foot.html","w":800,"h":200}]]}]}`, BaseVersion: 1, Files: map[string]*string{heroPath: nil}, SourceBases: map[string]string{heroPath: read.SourceBase, "design.json": manifest.SourceBases["design.json"]}})
		result <- err
	}()
	select {
	case <-gate.entered:
	case err := <-result:
		t.Fatalf("publish refused before barrier: %v", err)
	case <-ctx.Done():
		t.Fatal("publish did not reach pre-mutation barrier")
	}
	box := pid(t, read.Text, `id="box"`)
	e.setStyle(id, alice, box, "left", "123px")
	before := e.roomDesign(id).Frames[heroPath]
	rows, _ := e.st.RealtimeRows(ctx, id)
	close(gate.resume)
	select {
	case err := <-result:
		if service.CodeOf(err) != service.Conflict {
			t.Fatalf("concurrent human source was removed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("publish did not finish")
	}
	afterRows, _ := e.st.RealtimeRows(ctx, id)
	if afterRows != rows || e.roomDesign(id).Frames[heroPath] != before {
		t.Fatal("refused removal changed room or log")
	}
}
