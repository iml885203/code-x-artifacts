package roomkind_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

const designManifest = `{"version":1,"pages":[{"id":"p","name":"P","rows":[[` +
	`{"id":"hero","name":"Hero","file":"frames/hero.html","w":800,"h":600},` +
	`{"id":"foot","name":"Foot","file":"frames/foot.html","w":800,"h":200}]]}]}`

const (
	heroFrame = `<!doctype html><html><head><meta charset="utf-8"></head><body><div id="box" style="position:absolute;left:10px;top:20px;color:red"><p>Hello</p></div><section id="s"><h1>Title</h1></section></body></html>`
	footFrame = `<!doctype html><html><head><meta charset="utf-8"></head><body><p>Footer</p></body></html>`
)

// design publishes a two-frame design as alice and returns its id.
func (e *env) design() string {
	e.t.Helper()
	files := map[string]*string{"frames/hero.html": ptr(heroFrame), "frames/foot.html": ptr(footFrame)}
	p, err := e.svc.Create(e.t.Context(), e.svc.Viewer("alice"), service.PublishInput{Title: "Landing", Kind: "design", Content: designManifest, Files: files})
	if err != nil {
		e.t.Fatal(err)
	}
	return p.ID
}

func ptr(s string) *string { return &s }

// roomDesign is the design the room holds.
func (e *env) roomDesign(id string) designroom.Design {
	e.t.Helper()
	var d designroom.Design
	err := e.hub.Read(e.t.Context(), id, func(doc *crdt.Doc) (err error) {
		d, err = designroom.Open(doc).Read()
		return err
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}

// versionFiles is the text of the files of a saved version.
func (e *env) versionFiles(id string, version int) map[string]string {
	e.t.Helper()
	set, err := e.svc.FileTexts(e.t.Context(), id, version)
	if err != nil {
		e.t.Fatal(err)
	}
	return set
}

// pid is the data-pid of the first element in a frame whose start tag has marker.
func pid(t *testing.T, frame, marker string) string {
	t.Helper()
	i := strings.Index(frame, marker)
	if i < 0 {
		t.Fatalf("no %q in %s", marker, frame)
	}
	tag := frame[i : i+strings.Index(frame[i:], ">")]
	j := strings.Index(tag, `data-pid="`) + len(`data-pid="`)
	return tag[j : j+strings.Index(tag[j:], `"`)]
}

func (e *env) setStyle(id string, who realtime.Identity, element, prop, value string) {
	e.t.Helper()
	err := e.hub.Edit(e.t.Context(), id, who, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return designroom.Open(doc).SetStyle(heroPath, element, prop, value)
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

func TestADesignsRoomStartsFromTheFramesOfItsLatestVersion(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	room := e.roomDesign(id)
	want := e.versionFiles(id, 1)
	if len(room.Frames) != 2 || room.Manifest != designManifest {
		t.Fatalf("the room holds %d frames and manifest %q", len(room.Frames), room.Manifest)
	}
	for path, text := range want {
		if room.Frames[path] != text {
			t.Errorf("%s in the room is not the version's:\nroom    %s\nversion %s", path, room.Frames[path], text)
		}
	}
}

func TestAVersionFromADesignsRoomIsTheFramesTheRoomSerialisesAndOnlyAfterThePause(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	before := e.versionFiles(id, 1)
	box := pid(t, before["frames/hero.html"], `id="box"`)
	e.roomDesign(id)
	e.setStyle(id, alice, box, "left", "99px")
	e.setStyle(id, carol, box, "top", "7px")

	e.clock.Advance(roomkind.Quiet / 2)
	e.sweep()
	if got := len(e.versions(id)); got != 1 {
		t.Fatalf("a version was cut before the pause: %d versions", got)
	}
	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	versions := e.versions(id)
	if len(versions) != 2 {
		t.Fatalf("%d versions after the pause, want 2", len(versions))
	}
	latest := e.versionFiles(id, 2)
	inRoom := e.roomDesign(id)
	for path, text := range inRoom.Frames {
		if latest[path] != text {
			t.Errorf("version 2 of %s differs from what the room serialises:\nversion %s\nroom    %s", path, latest[path], text)
		}
	}
	if !strings.Contains(latest["frames/hero.html"], "position:absolute;left:99px;top:7px;color:red") {
		t.Errorf("the version does not hold both people's changes, in the style's own order: %s", latest["frames/hero.html"])
	}
	// The frame nobody touched is the very same file.
	if latest["frames/foot.html"] != before["frames/foot.html"] {
		t.Error("a frame that was not edited changed")
	}
	got, _ := e.st.Artifact(t.Context(), id)
	if got.Version != 2 || got.Content != designManifest {
		t.Errorf("the artifact is at %d with content %q", got.Version, got.Content)
	}
	// The authors are the people who edited; the version is not saved again.
	var authors []string
	for _, v := range versions {
		if v.Number == 2 {
			authors = v.Authors
		}
	}
	if strings.Join(authors, ",") != "alice,carol" {
		t.Errorf("authors of version 2 = %v", authors)
	}
	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	if len(e.versions(id)) != 2 {
		t.Error("a room that holds what the latest version holds was saved again")
	}
}

func TestRestoringADesignVersionWritesItsFramesIntoTheRoomAfterAProtectiveVersion(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	first := e.versionFiles(id, 1)
	box := pid(t, first["frames/hero.html"], `id="box"`)
	e.roomDesign(id)
	e.setStyle(id, bob, box, "left", "50px")

	saved, err := e.svc.Restore(t.Context(), e.svc.Viewer("alice"), id, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version != 3 {
		t.Fatalf("the restore is version %d, want 3 after the protective version 2", saved.Version)
	}
	if protective := e.versionFiles(id, 2); !strings.Contains(protective["frames/hero.html"], "left:50px") {
		t.Errorf("the protective version lost bob's work: %s", protective["frames/hero.html"])
	}
	room := e.roomDesign(id)
	for path, text := range e.versionFiles(id, 3) {
		if room.Frames[path] != text || first[path] != text {
			t.Errorf("%s: the room, the restore and version 1 should be the same file\nroom %s\nrestore %s", path, room.Frames[path], text)
		}
	}
}

func TestACycleMadeOfTwoPeoplesMovesIsRefusedByTheHubAndTheRoomKeepsWhatItHad(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	files := e.versionFiles(id, 1)
	section, box := pid(t, files["frames/hero.html"], `id="s"`), pid(t, files["frames/hero.html"], `id="box"`)
	a, b := e.connect(id, alice), e.connect(id, bob)
	a.pump()
	b.pump()
	move := func(c *client, el, into string) {
		write, err := designroom.Open(c.doc).Move("frames/hero.html", el, into, 0)
		if err != nil {
			t.Fatal(err)
		}
		c.doc.Transact(write)
	}
	// Each move is fine in the copy of the person who makes it.
	move(a, box, section)
	move(b, section, box)
	a.pump()
	b.pump()
	room := e.roomDesign(id) // the room is still a tree: Read fails on a cycle
	hero := room.Frames["frames/hero.html"]
	if !strings.Contains(hero, `id="s"`) || !strings.Contains(hero, `id="box"`) {
		t.Fatalf("an element was lost: %s", hero)
	}
	err := e.hub.Read(t.Context(), id, designroom.Check)
	if err != nil {
		t.Errorf("the room holds a cycle: %v", err)
	}
	// The second mover was told the room was reset, and its connection ended.
	if err := b.peer.Err(); !errors.Is(err, realtime.ErrRoomReset) {
		t.Errorf("the connection of the refused move ended with %v, want a room reset", err)
	}
}

// A browser (or any client of the socket) can write what it likes into its copy
// of the room; the hub holds every style value to the grammar before it keeps
// the update, so a value that would end the declaration never reaches the file.
func TestAStyleValueThatBreaksOutOfItsDeclarationIsRefusedOverTheSocketAndTheRoomKeepsWhatItHad(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	box := pid(t, e.versionFiles(id, 1)["frames/hero.html"], `id="box"`)
	a := e.connect(id, alice)
	a.pump()
	m := a.doc.GetMap(designroom.Map)
	key := "frames/hero.html|n|" + box + "|s|"
	a.doc.Transact(func(txn *crdt.Transaction) {
		m.Set(txn, key+"font-size", "44px") // fine
		m.Set(txn, key+"color", "red;} body{display:none")
	})
	a.pump()
	if err := a.peer.Err(); !errors.Is(err, realtime.ErrRoomReset) {
		t.Fatalf("the connection ended with %v, want a room reset", err)
	}
	hero := e.roomDesign(id).Frames["frames/hero.html"]
	if strings.Contains(hero, "body{display:none") || strings.Contains(hero, "44px") {
		t.Errorf("the refused update reached the room: %s", hero)
	}
	if !strings.Contains(hero, "color:red") || strings.Contains(hero, "font-size") {
		t.Errorf("the room is not what it was: %s", hero)
	}
}
