package roomkind_test

import (
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/service"
)

func TestLiveSourcePublishesRequireTheirOwnExplicitReadAndRefuseWithoutSideEffects(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	e.roomDesign(id)
	ctx := t.Context()
	read, err := e.svc.ReadSourceFile(ctx, e.svc.Viewer("alice"), id, heroPath, 0)
	if err != nil || read.SourceBase == "" {
		t.Fatalf("writable read = %+v, %v", read, err)
	}
	changed := strings.Replace(read.Text, "color:red", "color:blue", 1)
	input := service.PublishInput{Title: "Changed title", Content: designManifest, BaseVersion: 1, Files: map[string]*string{heroPath: &changed}}
	before := e.roomDesign(id).Frames[heroPath]
	if _, err := e.svc.Update(ctx, e.svc.Viewer("alice"), id, input); service.CodeOf(err) != service.Conflict {
		t.Fatalf("missing proof: %v", err)
	}
	a, _ := e.st.Artifact(ctx, id)
	if a.Title != "Landing" || a.Version != 1 || e.roomDesign(id).Frames[heroPath] != before {
		t.Fatal("refusal mutated title, version or room")
	}
	input.SourceBases = map[string]string{heroPath: read.SourceBase}
	if _, err := e.svc.Update(ctx, e.svc.Viewer("alice"), id, input); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.roomDesign(id).Frames[heroPath], "color:blue") {
		t.Fatal("explicit source change absent")
	}
	history, err := e.svc.ReadSourceFile(ctx, e.svc.Viewer("alice"), id, heroPath, 1)
	if err != nil || history.SourceBase != "" || history.Text != e.versionFiles(id, 1)[heroPath] {
		t.Fatalf("history leaked proof or live bytes: %+v %v", history, err)
	}
}
