package roomkind_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// An agent changes a frame of a design people are editing: it reads the frame,
// changes it, and publishes it with the version it started from.

const heroPath = "frames/hero.html"

// publishHero is an agent's publish of a changed hero frame, on top of version 1.
func (e *env) publishHero(ctx context.Context, id, frame string, proof ...string) (service.Published, error) {
	e.t.Helper()
	base := ""
	if len(proof) > 0 {
		base = proof[0]
	} else {
		read, err := e.svc.ReadSourceFile(ctx, e.svc.Viewer("alice"), id, heroPath, 0)
		if err != nil {
			e.t.Fatal(err)
		}
		base = read.SourceBase
	}
	return e.svc.Update(ctx, e.svc.Viewer("alice"), id, service.PublishInput{
		Title: "Landing", Content: designManifest, BaseVersion: 1, Files: map[string]*string{heroPath: &frame}, SourceBases: map[string]string{heroPath: base},
	})
}

func (c *client) designRoom() designroom.Design {
	c.t.Helper()
	c.pump()
	d, err := designroom.Open(c.doc).Read()
	if err != nil {
		c.t.Fatal(err)
	}
	return d
}

// touchedByAgents is what the agents in the roster say they changed: name to {frame id: pids}.
func (c *client) touchedByAgents() map[string]map[string][]any {
	c.t.Helper()
	c.pump()
	out := map[string]map[string][]any{}
	for _, st := range c.aw.GetStates() {
		user, _ := st.State["user"].(map[string]any)
		if user["agent"] != true {
			continue
		}
		frames := map[string][]any{}
		touched, _ := st.State["touched"].(map[string]any)
		for frame, pids := range touched {
			frames[frame], _ = pids.([]any)
		}
		out[user["name"].(string)] = frames
	}
	return out
}

func TestAnAgentsPublishIntoALiveDesignMergesWithWhatPeopleChangedAndSavesNoVersionBesideTheRoom(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	first := e.versionFiles(id, 1)
	box := pid(t, first[heroPath], `id="box"`)
	e.roomDesign(id)
	watcher := e.connect(id, bob)
	basis, err := e.svc.ReadSourceFile(t.Context(), e.svc.Viewer("alice"), id, heroPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	e.setStyle(id, alice, box, "left", "99px") // a person's edit, not in any version yet

	ctx := service.WithAgent(t.Context(), "Claude Code")
	published, err := e.publishHero(ctx, id, strings.Replace(first[heroPath], "color:red", "color:blue", 1), basis.SourceBase)
	if err != nil {
		t.Fatal(err)
	}

	// The room holds both changes, and the browser connected to it sees them.
	want := "position:absolute;left:99px;top:20px;color:blue"
	for name, got := range map[string]string{"room": e.roomDesign(id).Frames[heroPath], "browser": watcher.designRoom().Frames[heroPath]} {
		if !strings.Contains(got, want) {
			t.Errorf("the %s holds %s, want the person's left and the agent's color", name, got)
		}
	}
	// Nothing was written beside the room: no version, and the saved frames are what they were.
	if got := len(e.versions(id)); got != 1 || published.Version != 1 {
		t.Errorf("%d versions and the publish says version %d, want one saved version", got, published.Version)
	}
	for path, text := range e.versionFiles(id, 1) {
		if first[path] != text {
			t.Errorf("%s of the saved version changed", path)
		}
	}
	// The agent is told how its frame was stored, ids included, and reads what the room holds.
	if !strings.Contains(published.Rewritten[heroPath], "data-pid=") {
		t.Errorf("rewritten %v", published.Rewritten)
	}
	_, text, err := e.svc.ReadFile(t.Context(), e.svc.Viewer("alice"), id, heroPath, 0)
	if err != nil || !strings.Contains(text, want) {
		t.Errorf("read of the frame: %v %s", err, text)
	}
}

func TestWhatAnAgentMergedIntoALiveDesignBecomesAVersionOnceTheRoomIsQuietWithTheAgentAmongItsMakers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	first := e.versionFiles(id, 1)
	box := pid(t, first[heroPath], `id="box"`)
	e.roomDesign(id)
	basis, err := e.svc.ReadSourceFile(t.Context(), e.svc.Viewer("alice"), id, heroPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	e.setStyle(id, alice, box, "left", "99px")
	if _, err := e.publishHero(service.WithAgent(t.Context(), "Claude Code"), id, strings.Replace(first[heroPath], "color:red", "color:blue", 1), basis.SourceBase); err != nil {
		t.Fatal(err)
	}
	want := "position:absolute;left:99px;top:20px;color:blue"

	e.clock.Advance(2 * roomkind.Quiet)
	e.sweep()
	versions := e.versions(id)
	if len(versions) != 2 || !strings.Contains(e.versionFiles(id, 2)[heroPath], want) {
		t.Fatalf("%d versions after the pause; version 2: %s", len(versions), e.versionFiles(id, 2)[heroPath])
	}
	for _, v := range versions {
		if v.Number == 2 && (len(v.Agents) != 1 || v.Agents[0] != (store.VersionAgent{By: "alice", Client: "Claude Code"})) {
			t.Errorf("the agents of version 2 = %v", v.Agents)
		}
	}
}

func TestAnAgentsPublishIntoALiveDesignOnAStaleVersionIsRefusedWithTheLatestAndWritesNothing(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	first := e.versionFiles(id, 1)
	box := pid(t, first[heroPath], `id="box"`)
	e.roomDesign(id)
	e.setStyle(id, alice, box, "left", "99px")
	e.clock.Advance(2 * roomkind.Quiet)
	e.sweep() // version 2 now holds the person's edit

	_, err := e.publishHero(t.Context(), id, strings.Replace(first[heroPath], "color:red", "color:blue", 1))
	refusal, ok := errors.AsType[*service.Error](err)
	if !ok || refusal.Code != service.Conflict || refusal.Current != 2 || refusal.Latest == nil || refusal.Latest.Version != 2 {
		t.Fatalf("got %v, want a conflict that names version 2 and carries it", err)
	}
	if got := e.roomDesign(id).Frames[heroPath]; strings.Contains(got, "color:blue") {
		t.Errorf("a refused publish reached the room: %s", got)
	}
}

func TestAPublishIntoALiveDesignThatCannotBeMergedSaysWhyAndLeavesTheRoomAsItWas(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		code  service.Code
		say   string
		frame func(first string) string
	}{
		{"someone changed the same words", service.Conflict, "read the design again", func(first string) string { return strings.Replace(first, ">Title<", ">Headline<", 1) }},
		{"a value the editor does not take", service.Invalid, "color", func(first string) string {
			return strings.Replace(first, "color:red", "color:url(http://x.test/a.png)", 1)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			id := e.design()
			first := e.versionFiles(id, 1)
			e.roomDesign(id)
			basis, err := e.svc.ReadSourceFile(t.Context(), e.svc.Viewer("alice"), id, heroPath, 0)
			if err != nil {
				t.Fatal(err)
			}
			if c.code == service.Conflict { // a person has changed the title in the room
				title := textNodeOf(t, e, id, "Title")
				err := e.hub.Edit(t.Context(), id, alice, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
					return designroom.Open(doc).EditText(heroPath, title, "Heading")
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			before := e.roomDesign(id)
			_, err = e.publishHero(t.Context(), id, c.frame(first[heroPath]), basis.SourceBase)
			refusal, ok := errors.AsType[*service.Error](err)
			if !ok || refusal.Code != c.code || !strings.Contains(refusal.Msg, c.say) {
				t.Fatalf("got %v, want a refusal with code %v that says %q", err, c.code, c.say)
			}
			if after := e.roomDesign(id); after.Frames[heroPath] != before.Frames[heroPath] {
				t.Errorf("a refused publish changed the room: %s", after.Frames[heroPath])
			}
		})
	}
}

func TestAnAgentThatReadAFrameWhilePeopleHadChangedItPublishesOnWhatItReadAndIsNotInConflictWithIt(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	e.roomDesign(id)
	title := textNodeOf(t, e, id, "Title")
	err := e.hub.Edit(t.Context(), id, alice, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return designroom.Open(doc).EditText(heroPath, title, "Title!") // not in any version yet
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := service.WithAgent(t.Context(), "Claude Code")

	// The agent reads the frame as the room holds it, puts words in front of the title and sends the frame back.
	_, read, err := e.svc.ReadFile(ctx, e.svc.Viewer("alice"), id, heroPath, 0)
	if err != nil || !strings.Contains(read, ">Title!<") {
		t.Fatalf("the agent read %v: %s", err, read)
	}
	if _, err := e.publishHero(ctx, id, strings.Replace(read, ">Title!<", ">Big Title!<", 1)); err != nil {
		t.Fatalf("a change built on what the agent read was refused: %v", err)
	}
	if got := e.roomDesign(id).Frames[heroPath]; !strings.Contains(got, ">Big Title!<") {
		t.Errorf("the room holds %s", got)
	}

	// A publisher without an explicit read cannot guess a saved baseline for live work.
	other := service.WithAgent(t.Context(), "Codex")
	saved := e.versionFiles(id, 1)[heroPath]
	if _, err := e.publishHero(other, id, strings.Replace(saved, ">Title<", ">Headline<", 1), ""); service.CodeOf(err) != service.Conflict {
		t.Errorf("got %v, want a conflict", err)
	}
}

func TestAnAgentsPublishShowsItsNameOverTheElementsItChangedToEveryoneAndItLeavesWhenIdle(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	first := e.versionFiles(id, 1)
	box, title := pid(t, first[heroPath], `id="box"`), pid(t, first[heroPath], "<h1")
	e.roomDesign(id)
	watcher := e.connect(id, bob)

	changed := strings.Replace(strings.Replace(first[heroPath], "color:red", "color:blue", 1), ">Title<", ">New<", 1)
	if _, err := e.publishHero(service.WithAgent(t.Context(), "Claude Code"), id, changed); err != nil {
		t.Fatal(err)
	}
	got := watcher.touchedByAgents()["alice 的 Claude Code"]
	if len(got["hero"]) != 2 || !contains(got["hero"], box) || !contains(got["hero"], title) || len(got) != 1 {
		t.Fatalf("the roster says %v, want alice 的 Claude Code at %s and %s of the hero frame", watcher.touchedByAgents(), box, title)
	}
	e.clock.Advance(14 * time.Second)
	e.hub.Sweep(t.Context())
	if len(watcher.touchedByAgents()) != 1 {
		t.Fatal("the agent left too soon")
	}
	e.clock.Advance(time.Second)
	e.hub.Sweep(t.Context())
	if got := watcher.touchedByAgents(); len(got) != 0 {
		t.Errorf("the agent is still in the roster: %v", got)
	}
}

func TestAPersonsPublishIntoALiveDesignIsMergedTooAndShowsNobodyInTheRoster(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.design()
	first := e.versionFiles(id, 1)
	e.roomDesign(id)
	watcher := e.connect(id, bob)
	if _, err := e.publishHero(t.Context(), id, strings.Replace(first[heroPath], "color:red", "color:blue", 1)); err != nil {
		t.Fatal(err)
	}
	if got := e.roomDesign(id).Frames[heroPath]; !strings.Contains(got, "color:blue") {
		t.Errorf("the room holds %s", got)
	}
	if got := watcher.touchedByAgents(); len(got) != 0 {
		t.Errorf("a call that is not an agent's shows %v", got)
	}
}

func contains(list []any, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// textNodeOf is the id of the text node of the hero frame whose text is text.
func textNodeOf(t *testing.T, e *env, id, text string) string {
	t.Helper()
	var found string
	err := e.hub.Read(t.Context(), id, func(doc *crdt.Doc) error {
		m := doc.GetMap(designroom.Map)
		for k := range m.Entries() {
			if !strings.HasPrefix(k, heroPath+"|n|") || !strings.HasSuffix(k, "|text") {
				continue
			}
			if v, ok := m.Get(k); ok {
				if y, isText := v.(*crdt.YText); isText && y.ToString() == text {
					found = strings.Split(k, "|")[2]
				}
			}
		}
		return nil
	})
	if err != nil || found == "" {
		t.Fatalf("no text %q in the room: %v", text, err)
	}
	return found
}
