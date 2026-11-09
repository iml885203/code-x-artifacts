package roomkind_test

import (
	"maps"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestIndividualAgentEditsAreDurableBeforeRoomSnapshotsAndDoNotDuplicateAtSnapshot(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "activity.db")
	st := openActivityStore(t, path)
	disk, err := assets.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, st: st, svc: service.New(st, disk, false), clock: &clock{t: time.Now().UTC().Truncate(time.Millisecond)}, logs: &logRecorder{}}
	e.svc.SetClock(e.clock.Now)
	e.start()
	t.Cleanup(func() { e.hub.Close(t.Context()) })
	expected := map[time.Time]string{}
	ctx := t.Context()
	p := activityProvenance(t, e.st)
	p.Transport, p.Client = "mcp", "Codex"
	agentCtx := service.WithAgent(service.WithTokenProvenance(ctx, p), "Codex")
	doc := e.editableDoc()
	e.connect(doc, bob)
	for _, text := range []string{"first agent edit", "second agent edit"} {
		e.clock.Advance(time.Millisecond)
		expected[e.clock.Now()] = "doc_edit:" + doc
		if _, err := e.svc.DocEdit(agentCtx, e.svc.Viewer("alice"), doc, service.DocEditInput{Ops: []docedit.Op{replace("p1", text)}}); err != nil {
			t.Fatal(err)
		}
	}
	design := e.design()
	e.roomDesign(design)
	first := e.versionFiles(design, 1)
	for _, color := range []string{"blue", "green"} {
		e.clock.Advance(time.Millisecond)
		expected[e.clock.Now()] = "publish:" + design
		if _, err := e.publishHero(agentCtx, design, strings.Replace(first[heroPath], "color:red", "color:"+color, 1)); err != nil {
			t.Fatal(err)
		}
	}
	before := assertAgentActivity(t, e.st, p.TokenID, expected)
	// Restart the live side before a snapshot; acknowledged edits must come from the update log.
	e.hub.Close(ctx)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openActivityStore(t, path)
	e.st = reopened
	e.svc = service.New(reopened, disk, false)
	e.svc.SetClock(e.clock.Now)
	e.start()
	after := assertAgentActivity(t, e.st, p.TokenID, expected)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("live activity changed across store/service restart: %+v -> %+v", before, after)
	}
	read, err := e.svc.ReadDoc(ctx, e.svc.Viewer("alice"), doc)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := docfmt.Canonical(read.Doc)
	if err != nil || !strings.Contains(canonical, "second agent edit") {
		t.Fatalf("restarted Doc: %s %v", canonical, err)
	}
	if got := e.roomDesign(design).Frames[heroPath]; !strings.Contains(got, "color:green") {
		t.Fatalf("restarted Design: %s", got)
	}
	for _, id := range []string{doc, design} {
		if err := e.snap.Flush(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	assertAgentActivity(t, e.st, p.TokenID, expected)
}

func activityProvenance(t *testing.T, st *store.Store) store.TokenProvenance {
	t.Helper()
	token, err := st.CreateToken(t.Context(), "alice", "Laptop")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.AuthenticateToken(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func assertAgentActivity(t *testing.T, st *store.Store, token string, expected map[time.Time]string) []store.ActivityEntry {
	t.Helper()
	rows, err := st.Activity(t.Context(), "alice", store.ActivityFilter{Limit: 100}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("want four individual durable edits: %+v", rows)
	}
	actual := map[time.Time]string{}
	for _, item := range rows {
		if item.TokenID != token || item.Client != "Codex" {
			t.Fatalf("wrong attribution: %+v", item)
		}
		actual[item.OccurredAt] = item.Operation + ":" + item.ArtifactID
	}
	if !maps.Equal(actual, expected) {
		t.Fatalf("individual operation, target and occurrence times: %+v, want %+v", actual, expected)
	}
	foreign, err := st.Activity(t.Context(), "bob", store.ActivityFilter{Limit: 100}, time.Now())
	if err != nil || len(foreign) != 0 {
		t.Fatalf("owner-scoped activity: %+v %v", foreign, err)
	}
	return rows
}

func openActivityStore(t *testing.T, path string) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
