package service

import (
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/store/storetest"
)

func newService(t *testing.T) (*Service, *assets.Disk) {
	t.Helper()
	st := storetest.Open(t)
	disk, err := assets.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(st, disk, false), disk
}

func text(s string) *string { return &s }

func TestAFailedPublishLeavesNoManifestRowsAndOnlySweepableBlobs(t *testing.T) {
	t.Parallel()
	svc, disk := newService(t)
	alice := svc.Viewer("alice")
	a, err := svc.Create(t.Context(), alice, PublishInput{Title: "t", Content: "<p>x", Files: map[string]*string{"app.js": text("1")}})
	if err != nil {
		t.Fatal(err)
	}
	// A publish that wrote its bytes and then did not finish: a blob no manifest names.
	orphan := []byte("orphan")
	if err := disk.WriteBlob(a.ID, sha256Hex(orphan), orphan); err != nil {
		t.Fatal(err)
	}
	// Nothing is deleted while it could still belong to a publish in flight ...
	if removed, err := svc.SweepBlobs(t.Context(), time.Now()); err != nil || removed != 0 {
		t.Fatalf("the sweep removed %d, %v; a blob a publish may still be recording must stay", removed, err)
	}
	// ... and once it is older than any publish, only the orphan goes.
	if removed, err := svc.SweepBlobs(t.Context(), time.Now().Add(2*BlobGrace)); err != nil || removed != 1 {
		t.Fatalf("the sweep removed %d, %v; want exactly the orphan", removed, err)
	}
	if _, err := disk.ReadBlob(a.ID, sha256Hex(orphan)); err == nil {
		t.Error("the orphan is still there")
	}
	if _, err := disk.ReadBlob(a.ID, sha256Hex([]byte("1"))); err != nil {
		t.Errorf("the sweep removed a file a version names: %v", err)
	}
}

func TestAPageThatTakesNoFilesSavesNothingWhenFilesAreSent(t *testing.T) {
	t.Parallel()
	svc, disk := newService(t)
	_, err := svc.Create(t.Context(), svc.Viewer("alice"), PublishInput{Title: "t", Kind: "doc", Content: "# x", Files: map[string]*string{"app.js": text("1")}})
	if CodeOf(err) != Invalid {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if removed, err := disk.SweepBlobs(nil, time.Now().Add(time.Hour)); err != nil || removed != 0 {
		t.Errorf("a refused publish stored bytes: %d, %v", removed, err)
	}
}
