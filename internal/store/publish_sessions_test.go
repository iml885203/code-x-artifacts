package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func newSession(user string, at time.Time) PublishSession {
	return PublishSession{
		ID: NewSessionID(), Username: user, Title: "t", Kind: "html", Visibility: VisibilityPrivate,
		Files: []SessionFile{{Path: "content", SHA256: strings.Repeat("a", 64), Size: 5}}, CreatedAt: at, ExpiresAt: at.Add(10 * time.Minute),
	}
}

func TestAnUploadSecretIsSpentExactlyOnceAndOnlyWhileItsSessionLives(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	sess := newSession("alice", at)
	secrets, err := st.CreatePublishSession(ctx, sess, sess.Files, 10)
	if err != nil {
		t.Fatal(err)
	}
	secret := secrets[sess.Files[0].SHA256]
	if !strings.HasPrefix(secret, "pup_") || len(secret) != len("pup_")+64 {
		t.Fatalf("secret = %q", secret)
	}

	if _, err := st.OpenUpload(ctx, "pup_"+strings.Repeat("0", 64), at); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown secret opened: %v", err)
	}
	slot, err := st.OpenUpload(ctx, secret, at.Add(time.Minute))
	if err != nil || slot.SessionID != sess.ID || slot.SHA256 != sess.Files[0].SHA256 || slot.Size != 5 {
		t.Fatalf("slot = %+v, %v", slot, err)
	}
	if err := st.UseUpload(ctx, secret, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := st.UseUpload(ctx, secret, at.Add(time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Errorf("a secret was spent twice: %v", err)
	}
	if _, err := st.OpenUpload(ctx, secret, at.Add(time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Errorf("a spent secret still opens: %v", err)
	}
	if got, err := st.UploadedBlobs(ctx, sess.ID); err != nil || !got[sess.Files[0].SHA256] {
		t.Errorf("uploaded = %v, %v", got, err)
	}
}

func TestAnUploadSecretStopsWorkingWhenItsSessionExpires(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	other := newSession("alice", at)
	secrets, err := st.CreatePublishSession(ctx, other, other.Files, 10)
	if err != nil {
		t.Fatal(err)
	}
	late := at.Add(10 * time.Minute)
	if _, err := st.OpenUpload(ctx, secrets[other.Files[0].SHA256], late); !errors.Is(err, ErrNotFound) {
		t.Errorf("a secret opened when its session expired: %v", err)
	}
	if err := st.UseUpload(ctx, secrets[other.Files[0].SHA256], late); !errors.Is(err, ErrNotFound) {
		t.Errorf("a secret was spent when its session expired: %v", err)
	}
}

func TestASessionIsClaimedOnceLoadedOnlyByItsOwnerAndGoneWhenItExpires(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	sess := newSession("alice", at)
	sess.Removed = []string{"old.js"}
	if _, err := st.CreatePublishSession(ctx, sess, nil, 10); err != nil {
		t.Fatal(err)
	}

	if _, err := st.PublishSession(ctx, sess.ID, "bob", at); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob loaded alice's session: %v", err)
	}
	got, err := st.PublishSession(ctx, sess.ID, "alice", at)
	if err != nil || len(got.Files) != 1 || got.Files[0].Path != "content" || len(got.Removed) != 1 || got.Removed[0] != "old.js" || !got.ExpiresAt.Equal(at.Add(10*time.Minute)) {
		t.Fatalf("session = %+v, %v", got, err)
	}
	if err := st.ClaimPublishSession(ctx, sess.ID, "bob", at); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob claimed alice's session: %v", err)
	}
	if err := st.ClaimPublishSession(ctx, sess.ID, "alice", at); err != nil {
		t.Fatal(err)
	}
	if err := st.ClaimPublishSession(ctx, sess.ID, "alice", at); !errors.Is(err, ErrNotFound) {
		t.Errorf("a session was claimed twice: %v", err)
	}
	if err := st.ReleasePublishSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.ClaimPublishSession(ctx, sess.ID, "alice", at); err != nil {
		t.Errorf("a released session cannot be claimed again: %v", err)
	}
}

func TestAnExpiredSessionIsNotLoadedAndIsDeletedByTheSweep(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	sess := newSession("alice", at)
	if _, err := st.CreatePublishSession(ctx, sess, nil, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PublishSession(ctx, sess.ID, "alice", at.Add(10*time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Errorf("an expired session loaded: %v", err)
	}
	ids, err := st.ExpirePublishSessions(ctx, at.Add(10*time.Minute))
	if err != nil || len(ids) != 1 || ids[0] != sess.ID {
		t.Fatalf("expired = %v, %v", ids, err)
	}
	if all, err := st.PublishSessionIDs(ctx); err != nil || len(all) != 0 {
		t.Errorf("sessions after expiry = %v, %v", all, err)
	}
}

func TestAPersonCannotHoldMoreOpenSessionsThanTheLimitAndExpiredOnesDoNotCount(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	for range 2 {
		if _, err := st.CreatePublishSession(ctx, newSession("alice", at), nil, 2); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.CreatePublishSession(ctx, newSession("alice", at), nil, 2); !errors.Is(err, ErrTooManySessions) {
		t.Errorf("a third session: %v", err)
	}
	if _, err := st.CreatePublishSession(ctx, newSession("bob", at), nil, 2); err != nil {
		t.Errorf("bob is limited by alice's sessions: %v", err)
	}
	if _, err := st.CreatePublishSession(ctx, newSession("alice", at.Add(11*time.Minute)), nil, 2); err != nil {
		t.Errorf("expired sessions still count: %v", err)
	}
}
