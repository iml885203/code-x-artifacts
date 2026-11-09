package realtime_test

import (
	"testing"
	"time"

	"github.com/reearth/ygo/crdt"
)

// Edit splits a change into a plan that reads and a write that only writes
// because of this: reading a shared type from inside a transaction never
// returns. If this test fails, ygo has fixed it, and the split in Hub.Edit
// can be relaxed.
func TestReadingASharedTypeInsideATransactionNeverReturns(t *testing.T) {
	t.Parallel()
	doc := crdt.New()
	returned := make(chan struct{})
	go func() {
		// The goroutine is left blocked on purpose, on a document nothing else uses.
		doc.Transact(func(*crdt.Transaction) { doc.GetText("t") })
		close(returned)
	}()
	select {
	case <-returned:
		t.Fatal("ygo no longer blocks a read inside a transaction; revisit the plan/write split of Hub.Edit")
	case <-time.After(250 * time.Millisecond):
	}
}
