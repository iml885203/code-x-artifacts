package designroom_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

// An agent reads a design, changes it, and sends it back; the room has moved on
// since. These tests are about what the room holds afterwards: the base is the
// design as the agent read it, next is what it made of it, and "a person" is a
// change made to the room in the meantime.

// agentMerges merges next into the room in doc, as an agent that started from base.
func agentMerges(t *testing.T, doc *crdt.Doc, base, next designroom.Design) designroom.Touched {
	t.Helper()
	touched, err := tryMerge(doc, base, next)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	return touched
}

func tryMerge(doc *crdt.Doc, base, next designroom.Design) (designroom.Touched, error) {
	write, touched, err := designroom.Open(doc).Merge(base, next)
	if err != nil {
		return nil, err
	}
	doc.Transact(write)
	return touched, nil
}

// changed is a design whose hero frame is the base's with each old text in it replaced by the new one.
func changed(t *testing.T, base designroom.Design, pairs ...string) designroom.Design {
	t.Helper()
	frame := base.Frames[heroFile]
	for i := 0; i+1 < len(pairs); i += 2 {
		frame = replaceOnce(t, frame, pairs[i], pairs[i+1])
	}
	frames := map[string]string{}
	for file, text := range base.Frames {
		frames[file] = text
	}
	frames[heroFile] = frame
	return designroom.Design{Manifest: base.Manifest, Frames: frames}
}

func heroOf(t *testing.T, doc *crdt.Doc) string {
	t.Helper()
	return read(t, doc).Frames[heroFile]
}

func TestAnAgentsPublishChangesOnlyTheDifferingNodesAndPropertiesOfTheRoom(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	frame := base.Frames[heroFile]
	box, title := pidOf(t, frame, `id="box"`), pidOf(t, frame, "<h1")

	next := changed(t, base, "color:red", "color:blue", ">Title<", ">New title<", "width:30px;", "")
	touched := agentMerges(t, doc, base, next)

	if got := heroOf(t, doc); got != next.Frames[heroFile] {
		t.Errorf("the room holds\n%s\nwant\n%s", got, next.Frames[heroFile])
	}
	if got := strings.Join(touched[heroFile], ","); got != strings.Join(sorted(box, title), ",") {
		t.Errorf("touched %v, want %s and %s", touched, box, title)
	}
	if len(touched) != 1 {
		t.Errorf("touched frames %v", touched)
	}
}

func sorted(ids ...string) []string {
	out := append([]string(nil), ids...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestAPersonsChangeToAnotherPropertyOfTheSameElementSurvivesAnAgentsPublish(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	state := seeded(t, base)
	person, agentSide := peer(t, 11, state), peer(t, 22, state)
	box := pidOf(t, base.Frames[heroFile], `id="box"`)

	edit(t, person, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, box, "left", "100px")
	})
	sync(t, person, agentSide) // the person's edit reached the room the agent publishes into
	agentMerges(t, agentSide, base, changed(t, base, "color:red", "color:blue"))
	sync(t, person, agentSide)

	for name, doc := range map[string]*crdt.Doc{"person": person, "agent": agentSide} {
		got := heroOf(t, doc)
		if !strings.Contains(got, "position:absolute;left:100px;top:20px;width:30px;height:40px;color:blue") {
			t.Errorf("%s sees %s", name, got)
		}
	}
}

func TestAPersonsChangeMadeWhileTheAgentPublishesIsKeptToo(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	state := seeded(t, base)
	person, agentSide := peer(t, 11, state), peer(t, 22, state)
	box := pidOf(t, base.Frames[heroFile], `id="box"`)

	// Neither has seen the other's change when it is made.
	edit(t, person, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, box, "top", "55px")
	})
	agentMerges(t, agentSide, base, changed(t, base, "color:red", "color:blue", ">Title<", ">Other<"))
	sync(t, person, agentSide)

	if a, p := heroOf(t, agentSide), heroOf(t, person); a != p || !strings.Contains(a, "top:55px;width:30px;height:40px;color:blue") || !strings.Contains(a, ">Other<") {
		t.Errorf("the two do not agree on both changes:\nagent  %s\nperson %s", a, p)
	}
}

func TestWhenAPersonAndTheAgentChangeTheSamePropertyTheAgentsValueIsTheOneThatStays(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	box := pidOf(t, base.Frames[heroFile], `id="box"`)
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, box, "color", "green")
	})
	agentMerges(t, doc, base, changed(t, base, "color:red", "color:blue"))
	if got := heroOf(t, doc); !strings.Contains(got, "color:blue") || strings.Contains(got, "green") {
		t.Errorf("%s", got)
	}
}

func TestWhatTheAgentLeavesAsItReadItKeepsItsIdsAndItsContentEvenWhenPeopleChangedIt(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	base.Frames["frames/foot.html"] = normal(t, `<p>Footer</p>`)
	base.Manifest = `{"version":1,"pages":[{"id":"p","name":"P","rows":[[{"id":"hero","name":"Hero","file":"frames/hero.html","w":800,"h":600},{"id":"foot","name":"Foot","file":"frames/foot.html","w":800,"h":200}]]}]}`
	doc := peer(t, 1, seeded(t, base))
	foot := pidOf(t, base.Frames["frames/foot.html"], "<p")
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle("frames/foot.html", foot, "color", "purple")
	})
	before := read(t, doc)

	// The agent changes the hero and sends the foot back as it read it.
	touched := agentMerges(t, doc, base, changed(t, base, ">Title<", ">Other<"))

	after := read(t, doc)
	if after.Frames["frames/foot.html"] != before.Frames["frames/foot.html"] || !strings.Contains(after.Frames["frames/foot.html"], "purple") {
		t.Errorf("the foot changed: %s", after.Frames["frames/foot.html"])
	}
	if _, ok := touched["frames/foot.html"]; ok {
		t.Errorf("the foot is reported as changed: %v", touched)
	}
	for _, id := range []string{`id="box"`, "<h1", "<section"} {
		if pidOf(t, after.Frames[heroFile], id) != pidOf(t, base.Frames[heroFile], id) {
			t.Errorf("%s did not keep its pid", id)
		}
	}
}

func TestAnElementTheAgentAddsGetsAnIdThatNoElementOfTheRoomHas(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	state := seeded(t, base)
	a, b := peer(t, 11, state), peer(t, 22, state)

	// Two agents add an element each, neither giving it an id; the second publishes into a room that has the first's.
	agentMerges(t, a, base, changed(t, base, "</section>", "</section><aside>one</aside>"))
	sync(t, a, b)
	agentMerges(t, b, base, changed(t, base, "</section>", "</section><footer>two</footer>"))
	sync(t, a, b)

	got := heroOf(t, a)
	ids := regexp.MustCompile(`data-pid="([^"]+)"`).FindAllStringSubmatch(got, -1)
	seen := map[string]bool{}
	for _, m := range ids {
		if seen[m[1]] {
			t.Errorf("the id %s is on two elements: %s", m[1], got)
		}
		seen[m[1]] = true
	}
	if !strings.Contains(got, "one</aside>") || !strings.Contains(got, "two</footer>") {
		t.Errorf("an added element is missing: %s", got)
	}
}

func TestAnElementTheAgentTakesOutGoesWithEverythingInsideItAndNothingElse(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	frame := base.Frames[heroFile]
	box, body := pidOf(t, frame, `id="box"`), pidOf(t, frame, "<body")
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, box, "left", "100px") // the room's own change to what is about to go
	})

	i, j := strings.Index(frame, "<div"), strings.Index(frame, "</div>")+len("</div>")
	touched := agentMerges(t, doc, base, changed(t, base, frame[i:j], ""))

	got := heroOf(t, doc)
	if want := frame[:i] + frame[j:]; got != want {
		t.Errorf("the room holds\n%s\nwant\n%s", got, want)
	}
	if strings.Join(touched[heroFile], ",") != body {
		t.Errorf("touched %v, want the body, which lost a child", touched)
	}
	if strings.Contains(got, "Hello") {
		t.Errorf("what was inside the element is still there: %s", got)
	}
}

func TestAnElementAPersonDeletedStaysDeletedAndTheRestOfTheAgentsChangeIsMade(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	box := pidOf(t, base.Frames[heroFile], `id="box"`)
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Delete(heroFile, box) })

	agentMerges(t, doc, base, changed(t, base, "color:red", "color:blue", "<p ", "<p lang=\"en\" ", ">Title<", ">Other<"))

	got := heroOf(t, doc)
	if strings.Contains(got, `id="box"`) || strings.Contains(got, "blue") || !strings.Contains(got, ">Other<") {
		t.Errorf("%s", got)
	}
}

func TestTheAgentMovingElementsAndAPersonMovingAnotherAtTheSameTimeKeepBoth(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	state := seeded(t, base)
	person, agentSide := peer(t, 11, state), peer(t, 22, state)
	frame := base.Frames[heroFile]
	p, section := pidOf(t, frame, "<p "), pidOf(t, frame, "<section")

	edit(t, person, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Move(heroFile, p, section, 1) })
	// The agent puts the section before the div.
	i, j := strings.Index(frame, "<div"), strings.Index(frame, "</div>")+len("</div>")
	k, l := strings.Index(frame, "<section"), strings.Index(frame, "</section>")+len("</section>")
	reordered := frame[:i] + frame[k:l] + frame[i:j] + frame[l:]
	next := base
	next.Frames = map[string]string{heroFile: reordered}
	agentMerges(t, agentSide, base, next)
	sync(t, person, agentSide)

	a, b := heroOf(t, agentSide), heroOf(t, person)
	if a != b {
		t.Fatalf("they disagree:\n%s\n%s", a, b)
	}
	if strings.Index(a, "<section") > strings.Index(a, `id="box"`) || !strings.Contains(a, "Title</h1><p ") {
		t.Errorf("the agent's order or the person's move is missing: %s", a)
	}
}

func TestAnElementTheAgentMovesIntoAnElementItAddsIsMoved(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	frame := base.Frames[heroFile]
	k, l := strings.Index(frame, "<section"), strings.Index(frame, "</section>")+len("</section>")
	next := base
	next.Frames = map[string]string{heroFile: frame[:k] + `<main>` + frame[k:l] + `</main>` + frame[l:]}
	agentMerges(t, doc, base, next)
	got := heroOf(t, doc)
	if !regexp.MustCompile(`<main data-pid="[^"]+"><section data-pid="[^"]+"><h1 data-pid="[^"]+">Title</h1></section></main>`).MatchString(got) {
		t.Errorf("%s", got)
	}
	if pidOf(t, got, "<section") != pidOf(t, frame, "<section") {
		t.Error("the section lost its id when it was wrapped")
	}
}

const wordsFrame = `<!doctype html><html><head><meta charset="utf-8"></head><body><h1>Welcome to the show</h1></body></html>`

func wordsDesign(t *testing.T) designroom.Design {
	return designroom.Design{Manifest: manifest, Frames: map[string]string{heroFile: normal(t, wordsFrame)}}
}

func TestAnAgentsWordsAndAPersonsWordsInTheSameTextBothStayWhenTheyDoNotOverlap(t *testing.T) {
	t.Parallel()
	base := wordsDesign(t)
	state := seeded(t, base)
	person, agentSide := peer(t, 11, state), peer(t, 22, state)
	textID := textNodeWith(t, person, "Welcome to the show")
	edit(t, person, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.EditText(heroFile, textID, "Welcome to the show!")
	})
	sync(t, person, agentSide)
	agentMerges(t, agentSide, base, changed(t, base, "the show", "the big show"))
	sync(t, person, agentSide)
	for name, doc := range map[string]*crdt.Doc{"person": person, "agent": agentSide} {
		if got := heroOf(t, doc); !strings.Contains(got, ">Welcome to the big show!<") {
			t.Errorf("%s sees %s", name, got)
		}
	}
}

func TestWordsAPersonTypedBeforeTheAgentsChangeAreKeptAndTheChangeLandsWhereItWasMeant(t *testing.T) {
	t.Parallel()
	base := wordsDesign(t)
	doc := peer(t, 1, seeded(t, base))
	textID := textNodeWith(t, doc, "Welcome to the show")
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.EditText(heroFile, textID, "Hello, Welcome to the show")
	})
	agentMerges(t, doc, base, changed(t, base, "the show", "the big show"))
	if got := heroOf(t, doc); !strings.Contains(got, ">Hello, Welcome to the big show<") {
		t.Errorf("%s", got)
	}
}

func TestTheSameWordsChangedByAPersonAndTheAgentIsAConflictThatChangesNothing(t *testing.T) {
	t.Parallel()
	base := wordsDesign(t)
	doc := peer(t, 1, seeded(t, base))
	textID := textNodeWith(t, doc, "Welcome to the show")
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.EditText(heroFile, textID, "Welcome to the party")
	})
	before := heroOf(t, doc)
	_, err := tryMerge(doc, base, changed(t, base, "the show", "the gala"))
	var merge *designroom.MergeError
	if !errors.As(err, &merge) || !merge.Conflict || !strings.Contains(merge.Msg, "read the design again") {
		t.Fatalf("got %v, want a conflict that says to read again", err)
	}
	if heroOf(t, doc) != before {
		t.Error("a refused merge changed the room")
	}
}

func TestAnAgentChangingFormattedTextChangesItAndKeepsTheFormattingItDidNotTouch(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	next := changed(t, base, ">big<", ">huge<", "Hello ", "Hi ")
	agentMerges(t, doc, base, next)
	if got := heroOf(t, doc); got != next.Frames[heroFile] {
		t.Errorf("the room holds\n%s\nwant\n%s", got, next.Frames[heroFile])
	}
}

func TestATextTheAgentAddsAndOneItTakesOutAreAddedAndTakenOut(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	next := changed(t, base, "</h1></section>", "</h1>after</section>")
	agentMerges(t, doc, base, next)
	if got := heroOf(t, doc); got != next.Frames[heroFile] {
		t.Fatalf("added: the room holds\n%s\nwant\n%s", got, next.Frames[heroFile])
	}
	again := changed(t, designroom.Design{Manifest: base.Manifest, Frames: next.Frames}, ">Title</h1>after<", "></h1><")
	agentMerges(t, doc, designroom.Design{Manifest: base.Manifest, Frames: next.Frames}, again)
	if got := heroOf(t, doc); got != again.Frames[heroFile] {
		t.Errorf("taken out: the room holds\n%s\nwant\n%s", got, again.Frames[heroFile])
	}
}

func TestAnAgentThatSendsAFrameWithoutIdsReplacesTheFrameWithoutLosingAnythingItSaid(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	stripped := regexp.MustCompile(` data-pid="[^"]*"`).ReplaceAllString(base.Frames[heroFile], "")
	next := base
	next.Frames = map[string]string{heroFile: strings.Replace(stripped, ">Title<", ">Rewritten<", 1)}
	agentMerges(t, doc, base, next)
	got := heroOf(t, doc)
	if strip := regexp.MustCompile(` data-pid="[^"]*"`).ReplaceAllString(got, ""); strip != next.Frames[heroFile] {
		t.Errorf("the room holds\n%s\nwant the agent's frame", got)
	}
	if strings.Count(got, "data-pid") != strings.Count(base.Frames[heroFile], "data-pid") {
		t.Errorf("an element has no id or has two: %s", got)
	}
}

func TestAValueTheEditorDoesNotAcceptIsRefusedAndTheRoomIsUnchanged(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	before := heroOf(t, doc)
	_, err := tryMerge(doc, base, changed(t, base, "color:red", "color:url(http://x.test/a.png)"))
	var merge *designroom.MergeError
	if !errors.As(err, &merge) || merge.Conflict || !strings.Contains(merge.Msg, "color") {
		t.Fatalf("got %v, want a refusal that names the property", err)
	}
	if heroOf(t, doc) != before {
		t.Error("a refused merge changed the room")
	}
}

func TestAMoveThatWouldMakeACycleWithAPersonsMoveIsAConflict(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	frame := base.Frames[heroFile]
	box, section := pidOf(t, frame, `id="box"`), pidOf(t, frame, "<section")
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Move(heroFile, section, box, 0) })
	before := heroOf(t, doc)

	// The agent puts the div into the section.
	i, j := strings.Index(frame, "<div"), strings.Index(frame, "</div>")+len("</div>")
	k := strings.Index(frame, "</section>")
	next := base
	next.Frames = map[string]string{heroFile: frame[:i] + frame[j:k] + frame[i:j] + frame[k:]}
	_, err := tryMerge(doc, base, next)
	var merge *designroom.MergeError
	if !errors.As(err, &merge) || !merge.Conflict {
		t.Fatalf("got %v, want a conflict", err)
	}
	if heroOf(t, doc) != before {
		t.Error("a refused merge changed the room")
	}
}

func TestAnAgentAddsAFrameAndAPageSomeoneElseAddedMeanwhileStays(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	state := seeded(t, base)
	person, agentSide := peer(t, 11, state), peer(t, 22, state)
	withFrame := func(id string) designroom.Design {
		return designroom.Design{
			Manifest: `{"version":1,"pages":[{"id":"p","name":"P","rows":[[{"id":"hero","name":"Hero","file":"frames/hero.html","w":800,"h":600},{"id":"` + id + `","name":"` + id + `","file":"frames/` + id + `.html","w":400,"h":300}]]}]}`,
			Frames:   map[string]string{heroFile: base.Frames[heroFile], "frames/" + id + ".html": normal(t, `<p>`+id+`</p>`)},
		}
	}
	// Both add a frame to the design the room started as, neither seeing the other's.
	agentMerges(t, person, base, withFrame("mobile"))
	agentMerges(t, agentSide, base, withFrame("tablet"))
	sync(t, person, agentSide)

	for name, doc := range map[string]*crdt.Doc{"person": person, "agent": agentSide} {
		d := read(t, doc)
		for _, id := range []string{"mobile", "tablet"} {
			if !strings.Contains(d.Manifest, `"id":"`+id+`"`) || d.Frames["frames/"+id+".html"] == "" {
				t.Errorf("%s: the frame %s is missing:\n%s\n%v", name, id, d.Manifest, d.Frames)
			}
		}
	}
}

func TestAFrameTheAgentTakesOutOfTheDesignIsTakenOutOfTheRoom(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	base.Frames["frames/old.html"] = normal(t, `<p>old</p>`)
	base.Manifest = `{"version":1,"pages":[{"id":"p","name":"P","rows":[[{"id":"hero","name":"Hero","file":"frames/hero.html","w":800,"h":600},{"id":"old","name":"Old","file":"frames/old.html","w":400,"h":300}]]}]}`
	doc := peer(t, 1, seeded(t, base))
	next := designroom.Design{Manifest: manifest, Frames: map[string]string{heroFile: base.Frames[heroFile]}}
	agentMerges(t, doc, base, next)
	d := read(t, doc)
	if strings.Contains(d.Manifest, "old") || len(d.Frames) != 1 {
		t.Errorf("%s %v", d.Manifest, d.Frames)
	}
}

func TestAMergeThatChangesNothingWritesNothing(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	before := crdt.EncodeStateAsUpdateV1(doc, nil)
	if touched := agentMerges(t, doc, base, base); len(touched) != 0 {
		t.Errorf("touched %v", touched)
	}
	if string(crdt.EncodeStateAsUpdateV1(doc, nil)) != string(before) {
		t.Error("the room changed")
	}
}

// typeIntoFormattedText is a person typing into the formatted text of the hero's paragraph: text is inserted at the
// position of the first occurrence of after in what the text holds, right after it.
func typeIntoFormattedText(t *testing.T, doc *crdt.Doc, after, text string) {
	t.Helper()
	m := doc.GetMap(designroom.Map)
	for k := range m.Entries() {
		if !strings.HasPrefix(k, heroFile+"|n|") || !strings.HasSuffix(k, "|rich") {
			continue
		}
		v, _ := m.Get(k)
		y, ok := v.(*crdt.YText)
		if !ok {
			continue
		}
		at := strings.Index(y.ToString(), after)
		if at < 0 {
			continue
		}
		pos := at + len(after) // the text is ASCII: a byte is a unit
		doc.Transact(func(txn *crdt.Transaction) { y.Insert(txn, pos, text, nil) })
		return
	}
	t.Fatalf("no formatted text with %q", after)
}

func TestAnAgentsChangeToFormattedTextKeepsWordsAPersonTypedInTheSameElement(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	state := seeded(t, base)
	person, agentSide := peer(t, 11, state), peer(t, 22, state)

	typeIntoFormattedText(t, person, "world", " friends")
	sync(t, person, agentSide)
	agentMerges(t, agentSide, base, changed(t, base, ">big<", ">huge<"))
	sync(t, person, agentSide)

	for name, doc := range map[string]*crdt.Doc{"person": person, "agent": agentSide} {
		if got := heroOf(t, doc); !strings.Contains(got, "Hello <b data-pid=") || !strings.Contains(got, ">huge</b> world friends</p>") {
			t.Errorf("%s sees %s", name, got)
		}
	}
}

func TestAnAgentsFormatAndAnInsertionInFormattedTextLandWhereTheAgentPutThemAndNothingElseChanges(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	doc := peer(t, 1, seeded(t, base))
	typeIntoFormattedText(t, doc, "Hello", "!") // a person's words already in the room, not in the version
	// The agent adds plain words right after the bold ones and makes "Hello" bold.
	next := changed(t, base, "</b> world", "</b> now world", "Hello <b", "<b>Hello</b> <b")
	agentMerges(t, doc, base, next)
	got := heroOf(t, doc)
	if !regexp.MustCompile(`<b data-pid="[^"]+">Hello</b>! <b data-pid="[^"]+">big</b> now world</p>`).MatchString(got) {
		t.Errorf("want Hello bold, the person's ! after it, the agent's plain words after the bold ones: %s", got)
	}
}

func TestTheSameWordsOfFormattedTextChangedByAPersonAndTheAgentIsAConflictThatChangesNothing(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	state := seeded(t, base)
	person, agentSide := peer(t, 11, state), peer(t, 22, state)
	// The person replaces "world" by typing after it and the agent replaces the word itself.
	m := person.GetMap(designroom.Map)
	for k := range m.Entries() {
		if v, _ := m.Get(k); strings.HasSuffix(k, "|rich") {
			if y, ok := v.(*crdt.YText); ok {
				at := strings.Index(y.ToString(), "world")
				person.Transact(func(txn *crdt.Transaction) {
					y.Delete(txn, at, len("world"))
					y.Insert(txn, at, "earth", nil)
				})
			}
		}
	}
	sync(t, person, agentSide)
	before := heroOf(t, agentSide)
	_, err := tryMerge(agentSide, base, changed(t, base, " world", " planet"))
	var merge *designroom.MergeError
	if !errors.As(err, &merge) || !merge.Conflict {
		t.Fatalf("got %v, want a conflict", err)
	}
	if heroOf(t, agentSide) != before {
		t.Error("a refused merge changed the room")
	}
}

func TestAFrameWithoutIdsReplacesTheFrameOnlyWhenNoOneChangedItSinceTheAgentsBase(t *testing.T) {
	t.Parallel()
	base := heroDesign(t)
	strip := func(text string) string { return regexp.MustCompile(` data-pid="[^"]*"`).ReplaceAllString(text, "") }
	next := designroom.Design{Manifest: base.Manifest, Frames: map[string]string{heroFile: strings.Replace(strip(base.Frames[heroFile]), ">Title<", ">Rewritten<", 1)}}

	untouched := peer(t, 1, seeded(t, base))
	agentMerges(t, untouched, base, next)
	if got := heroOf(t, untouched); !strings.Contains(got, ">Rewritten<") {
		t.Errorf("an unchanged frame was not replaced: %s", got)
	}

	changedByAPerson := peer(t, 2, seeded(t, base))
	box := pidOf(t, base.Frames[heroFile], `id="box"`)
	edit(t, changedByAPerson, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, box, "left", "77px")
	})
	before := heroOf(t, changedByAPerson)
	_, err := tryMerge(changedByAPerson, base, next)
	var merge *designroom.MergeError
	if !errors.As(err, &merge) || !merge.Conflict || !strings.Contains(merge.Msg, "data-pid") {
		t.Fatalf("got %v, want a conflict that says to publish with data-pids", err)
	}
	if heroOf(t, changedByAPerson) != before || !strings.Contains(before, "left:77px") {
		t.Error("the person's change was lost")
	}
}
