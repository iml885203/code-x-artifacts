package designroom_test

import (
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

func TestSharedTokensMergePersistAndLeaveFrameBindingsUntouched(t *testing.T) {
	frame := normal(t, `<div style="color:var(--accent)">Hi</div>`)
	base := designroom.Design{Manifest: strings.Replace(manifest, `"version":1`, `"version":1,"tokens":{"--accent":"red","--space":"8px"}`, 1), Frames: map[string]string{"frames/hero.html": frame}}
	state := seeded(t, base)
	a, b := peer(t, 1, state), peer(t, 2, state)
	am := a.GetMap(designroom.Map)
	a.Transact(func(tx *crdt.Transaction) { am.Set(tx, "t|--accent", "blue") })
	next := base
	next.Manifest = strings.Replace(base.Manifest, "8px", "16px", 1)
	write, _, err := designroom.Open(b).Merge(base, next)
	if err != nil {
		t.Fatal(err)
	}
	b.Transact(write)
	sync(t, a, b)
	for _, doc := range []*crdt.Doc{a, b, peer(t, 3, crdt.EncodeStateAsUpdateV1(a, nil))} {
		if err := designroom.Check(doc); err != nil {
			t.Fatal(err)
		}
		got := read(t, doc)
		m, problems := designfmt.Parse(got.Manifest)
		if len(problems) > 0 || m.Tokens["--accent"] != "blue" || m.Tokens["--space"] != "16px" || got.Frames["frames/hero.html"] != frame {
			t.Fatalf("lost tokens or binding: %+v %v", got, problems)
		}
	}
}

func TestUnsafeSharedTokensAreRefusedBeforeTheyBecomeAFrame(t *testing.T) {
	for _, value := range []any{`red;}body{color:blue`, `url(/files/x/a)`, 12} {
		doc := peer(t, 1, seeded(t, designroom.Design{Manifest: manifest, Frames: map[string]string{"frames/hero.html": normal(t, "<p>Hi</p>")}}))
		m := doc.GetMap(designroom.Map)
		doc.Transact(func(tx *crdt.Transaction) { m.Set(tx, "t|--accent", value) })
		if err := designroom.Check(doc); err == nil {
			t.Fatalf("accepted %v", value)
		}
	}
}

func TestRemovingTokensAndRestoringADesignRestoresOnlyItsOwnTheme(t *testing.T) {
	frame := normal(t, `<p style="color:var(--accent, red)">Bound</p>`)
	base := designroom.Design{Manifest: strings.Replace(manifest, `"version":1`, `"version":1,"tokens":{"--accent":"blue"}`, 1), Frames: map[string]string{"frames/hero.html": frame}}
	doc := peer(t, 1, seeded(t, base))
	next := base
	next.Manifest = manifest
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Write(next) })
	got := read(t, doc)
	m, _ := designfmt.Parse(got.Manifest)
	if len(m.Tokens) != 0 || got.Frames["frames/hero.html"] != frame {
		t.Fatal(got)
	}
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Write(base) })
	m, _ = designfmt.Parse(read(t, doc).Manifest)
	if m.Tokens["--accent"] != "blue" {
		t.Fatal(m)
	}
}
