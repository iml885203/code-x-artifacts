package designsourceedit_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
)

func TestExplicitReadBasesSurviveIndependentReadsAndRefuseExpiryEvictionAndRestart(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	book, err := designsourceedit.NewReadBases(time.Minute, 2, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	basis := designsourceedit.Basis{ArtifactID: "a", File: "frames/a.html", Format: "1", SavedVersion: 1, LiveEpoch: "epoch", Principal: "alice"}
	first, err := book.Remember(basis, "First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := book.Remember(basis, "Second")
	if err != nil {
		t.Fatal(err)
	}
	scope := designsourceedit.ScopeOf(basis)
	for _, c := range []struct{ token, source string }{{first, "First"}, {second, "Second"}} {
		got, err := book.Load(c.token, scope)
		if err != nil || got.Source != c.source || got.Basis != basis {
			t.Fatalf("read replaced: %+v %v", got, err)
		}
	}
	now = now.Add(time.Second)
	third, err := book.Remember(basis, "Third")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := book.Load(first, scope); !errors.Is(err, designsourceedit.ErrSourceBase) {
		t.Fatalf("eviction guessed a base: %v", err)
	}
	if got, err := book.Load(third, scope); err != nil || got.Source != "Third" {
		t.Fatalf("current: %v", err)
	}
	now = now.Add(time.Minute)
	if _, err := book.Load(second, scope); !errors.Is(err, designsourceedit.ErrSourceBase) {
		t.Fatalf("expiry guessed a base: %v", err)
	}
	restarted, err := designsourceedit.NewReadBases(time.Minute, 2, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Load(third, scope); !errors.Is(err, designsourceedit.ErrSourceBase) {
		t.Fatalf("restart guessed a base: %v", err)
	}
}

func TestReadBaseLookupIsScopedAndDoesNotMistakeASaveForALiveEpochChange(t *testing.T) {
	t.Parallel()
	book, err := designsourceedit.NewReadBases(time.Minute, 4, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	basis := designsourceedit.Basis{ArtifactID: "a", File: "frames/a.html", Format: "2", SavedVersion: 3, LiveEpoch: "live", Principal: "alice"}
	token, err := book.Remember(basis, "<p>base</p>")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"artifact", "file", "format", "epoch", "principal"} {
		changed := changedBasis(basis, field)
		if _, err := book.Load(token, designsourceedit.ScopeOf(changed)); !errors.Is(err, designsourceedit.ErrSourceBase) {
			t.Fatalf("%s: %v", field, err)
		}
	}
	// The original saved version stays authenticated in the returned basis. An
	// ordinary snapshot does not retire the live edit epoch or overwrite the read.
	afterSave := basis
	afterSave.SavedVersion++
	read, err := book.Load(token, designsourceedit.ScopeOf(afterSave))
	if err != nil || read.Basis.SavedVersion != 3 {
		t.Fatalf("saved version binding lost: %+v %v", read, err)
	}
	if _, err := book.Load("", designsourceedit.ScopeOf(basis)); !errors.Is(err, designsourceedit.ErrSourceBase) {
		t.Fatal("missing proof accepted")
	}
}

func TestConcurrentReadsOfOneAccountKeepTheirOwnExactBaselines(t *testing.T) {
	t.Parallel()
	book, err := designsourceedit.NewReadBases(time.Minute, 64, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	basis := designsourceedit.Basis{ArtifactID: "a", File: "frames/a.html", Format: "1", SavedVersion: 1, LiveEpoch: "e", Principal: "alice"}
	var readers sync.WaitGroup
	for i := range 32 {
		readers.Go(func() {
			source := fmt.Sprintf("source-%d", i)
			token, err := book.Remember(basis, source)
			if err != nil {
				t.Error(err)
				return
			}
			got, err := book.Load(token, designsourceedit.ScopeOf(basis))
			if err != nil || got.Source != source {
				t.Errorf("another read replaced baseline: %v", err)
			}
		})
	}
	readers.Wait()
}

func TestReadBaseRegistryRejectsInvalidBoundsOversizedSourcesAndUnboundContexts(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		ttl   time.Duration
		limit int
	}{{0, 1}, {time.Minute, 0}, {-time.Second, 2}} {
		if _, err := designsourceedit.NewReadBases(c.ttl, c.limit, nil); err == nil {
			t.Fatal("invalid bounds accepted")
		}
	}
	book, err := designsourceedit.NewReadBases(time.Minute, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	basis := designsourceedit.Basis{ArtifactID: "a", File: "frames/a.html", Format: "1", SavedVersion: 1, LiveEpoch: "e", Principal: "alice"}
	if _, err := book.Remember(basis, strings.Repeat("x", (512<<10)+1)); !errors.Is(err, designsourceedit.ErrSourceBase) {
		t.Fatal("oversized frame retained")
	}
	basis.File = "design.json"
	if _, err := book.Remember(basis, strings.Repeat("x", (256<<10)+1)); !errors.Is(err, designsourceedit.ErrSourceBase) {
		t.Fatal("oversized manifest retained")
	}
	basis.LiveEpoch = ""
	if _, err := book.Remember(basis, "source"); !errors.Is(err, designsourceedit.ErrSourceBase) {
		t.Fatal("unbound base retained")
	}
}

func TestRetiredProofsCannotReviveAfterTheSameSourceIsReadAgain(t *testing.T) {
	t.Parallel()
	book, err := designsourceedit.NewReadBases(time.Minute, 8, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	basis := designsourceedit.Basis{ArtifactID: "a", File: "frames/a.html", Format: "1", SavedVersion: 1, LiveEpoch: "e", Principal: "alice"}
	old, err := book.Remember(basis, "same source")
	if err != nil {
		t.Fatal(err)
	}
	book.ForgetArtifact("a")
	fresh, err := book.Remember(basis, "same source")
	if err != nil {
		t.Fatal(err)
	}
	if old == fresh {
		t.Fatal("permission retirement can revive stale input")
	}
	if _, err := book.Load(old, designsourceedit.ScopeOf(basis)); !errors.Is(err, designsourceedit.ErrSourceBase) {
		t.Fatal("retired input accepted")
	}
	if _, err := book.Load(fresh, designsourceedit.ScopeOf(basis)); err != nil {
		t.Fatal(err)
	}
}

func TestAbsentReadBasesAreDistinctFromPresentEmptySources(t *testing.T) {
	t.Parallel()
	book, err := designsourceedit.NewReadBases(time.Minute, 8, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	basis := designsourceedit.Basis{ArtifactID: "a", File: "frames/new.html", Format: "1", SavedVersion: 1, LiveEpoch: "e", Principal: "alice"}
	absent, err := book.RememberAbsent(basis)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := book.Remember(basis, "")
	if err != nil {
		t.Fatal(err)
	}
	missing, err := book.Load(absent, designsourceedit.ScopeOf(basis))
	if err != nil || !missing.Absent {
		t.Fatal("absence lost")
	}
	present, err := book.Load(empty, designsourceedit.ScopeOf(basis))
	if err != nil || present.Absent || present.Source != "" {
		t.Fatal("empty source became absent")
	}
	basis.File = "design.json"
	if _, err := book.RememberAbsent(basis); !errors.Is(err, designsourceedit.ErrSourceBase) {
		t.Fatal("absent manifest accepted")
	}
	basis.File = "frames/new.html"
	book.ForgetArtifact("a")
	if _, err := book.Load(absent, designsourceedit.ScopeOf(basis)); !errors.Is(err, designsourceedit.ErrSourceBase) {
		t.Fatal("retired absence revived")
	}
}
