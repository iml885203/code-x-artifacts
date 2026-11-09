package roomkind_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/reearth/ygo/crdt"
)

func TestDesignSeedsAndLoadedRoomsUseTheSameBoundWithoutDestroyingAnOversizedLog(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	want := e.roomDesign(id)
	before, err := e.st.RealtimeLog().Load(t.Context(), id, "")
	if err != nil {
		t.Fatal(err)
	}
	bytes := 0
	for _, u := range before.Updates {
		bytes += len(u)
	}
	for _, limit := range []int{bytes, bytes - 1, bytes} {
		log := roomkind.SeededLog(e.st.RealtimeLog(), e.st, e.svc, roomkind.DefaultCodecs(), limit)
		hub, err := realtime.New(realtime.Config{Log: log})
		if err != nil {
			t.Fatal(err)
		}
		err = hub.Read(t.Context(), id, func(_ *crdt.Doc) error { return nil })
		if limit < bytes {
			var bound *realtime.LimitError
			if !errors.As(err, &bound) || bound.Max != limit || bound.Value != bytes {
				t.Fatalf("load: %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
		hub.Close(t.Context())
		after, err := e.st.RealtimeLog().Load(t.Context(), id, "")
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("refusal/recovery changed retained history")
		}
	}
	if got := e.roomDesign(id); !reflect.DeepEqual(got, want) {
		t.Fatal("recovery changed content")
	}
	assertOversizedSeedRetainsNoLog(t, e, e.design())
}

func TestDesignCapacityIsSeparateFromTheGenericDocumentLimit(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	log := roomkind.SeededLog(e.st.RealtimeLog(), e.st, e.svc, roomkind.DefaultCodecs(), 0)
	policy, ok := log.(interface {
		DocumentLimit(context.Context, string) (int, error)
	})
	if !ok {
		t.Fatal("seeded log does not expose its admission budget")
	}
	for _, tc := range []struct {
		id   string
		want int
	}{{e.design(), 8 << 20}, {e.doc(), 2 << 20}} {
		got, err := policy.DocumentLimit(t.Context(), tc.id)
		if err != nil || got != tc.want {
			t.Fatalf("limit=%d err=%v want=%d", got, err, tc.want)
		}
	}
}

func assertOversizedSeedRetainsNoLog(t *testing.T, e *env, id string) {
	t.Helper()
	newID := id
	log := roomkind.SeededLog(e.st.RealtimeLog(), e.st, e.svc, roomkind.DefaultCodecs(), 1)
	if _, err := log.Load(t.Context(), newID, "seed-epoch"); err == nil {
		t.Fatal("oversized seed accepted")
	}
	rows, err := e.st.RealtimeRows(t.Context(), newID)
	if err != nil || rows != 0 {
		t.Fatal("oversized seed persisted")
	}
}
