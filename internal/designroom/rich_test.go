package designroom_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

// frameOf is a normalised frame whose body is body.
func frameOf(t *testing.T, body string) string { return normal(t, `<body>`+body+`</body>`) }

func oneFrame(t *testing.T, body string) designroom.Design {
	return designroom.Design{Manifest: manifest, Frames: map[string]string{"frames/hero.html": frameOf(t, body)}}
}

// richOf is the formatted text of the element with the given pid, or nil.
func richOf(doc *crdt.Doc, pid string) *crdt.YText {
	v, _ := doc.GetMap(designroom.Map).Get(heroFile + "|n|" + pid + "|rich")
	t, _ := v.(*crdt.YText)
	return t
}

// roomKeys are the entries of the room whose key contains part.
func roomKeys(doc *crdt.Doc, part string) []string {
	var keys []string
	for k := range doc.GetMap(designroom.Map).Entries() {
		if strings.Contains(k, part) {
			keys = append(keys, k)
		}
	}
	return keys
}

func TestFormattedTextRoundTripsThroughTheRoomAsOneTextAndComesBackAsTheSameHTML(t *testing.T) {
	t.Parallel()
	d := oneFrame(t, `<div>Hello <b>bold</b><br>line2</div>`)
	doc := peer(t, 1, seeded(t, d))
	if got := read(t, doc).Frames[heroFile]; got != d.Frames[heroFile] {
		t.Fatalf("the HTML changed in the room:\nwant %q\ngot  %q", d.Frames[heroFile], got)
	}
	pid := pidOf(t, d.Frames[heroFile], "<div")
	text := richOf(doc, pid)
	if text == nil {
		t.Fatalf("the element %s has no formatted text; entries: %v", pid, roomKeys(doc, "|n|"))
	}
	if got := text.ToString(); got != "Hello bold"+"line2" {
		t.Errorf("the characters of the text are %q", got)
	}
	// Its content is that one text: the words and the <b> are not nodes of the tree.
	for _, k := range roomKeys(doc, "|parent") {
		if v, _ := doc.GetMap(designroom.Map).Get(k); v == pid {
			t.Errorf("%s: the element still has child nodes", k)
		}
	}
	// A peer that only has the encoded state reads the same.
	other := peer(t, 2, crdt.EncodeStateAsUpdateV1(doc, nil))
	if got := read(t, other).Frames[heroFile]; got != d.Frames[heroFile] {
		t.Errorf("another peer reads %q", got)
	}
}

func TestInlineElementsOfEveryKindNestAndNeighbourAsTheyWereWritten(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"two bolds next to each other":   `<p><b>a</b><b>b</b> c</p>`,
		"both ways of nesting":           `<p><b><i>x</i></b> and <i><b>y</b></i> end</p>`,
		"a link with a style and a span": `<p>Go <a href="#top" class="l" target="_blank"><span style="color:red;font-weight:700">there</span> now</a>.</p>`,
		"strong, em, u and code":         `<p><strong>s</strong> <em>e</em> <u>u</u> <code>c &lt; d</code></p>`,
		"a bold that holds a break":      `<p><b>one<br class="k">two</b> three</p>`,
		"emoji and CJK around a format":  `<p>😀 <b>世界</b> 𠮷野家<br>x</p>`,
		"a preformatted text":            "<pre>line <b>1</b>\n  line 2</pre>",
		"entities":                       `<p>a &amp; b <b>&lt;c&gt;</b> &nbsp; d</p>`,
	} {
		d := oneFrame(t, body)
		doc := peer(t, 1, seeded(t, d))
		if got := read(t, doc).Frames[heroFile]; got != d.Frames[heroFile] {
			t.Errorf("%s:\nwant %q\ngot  %q", name, d.Frames[heroFile], got)
		}
		if richOf(doc, pidOf(t, d.Frames[heroFile], "<p")) == nil && richOf(doc, pidOf(t, d.Frames[heroFile], "<pre")) == nil {
			t.Errorf("%s: no formatted text was made", name)
		}
	}
}

func TestAnUnsupportedInlineElementInsideTextSurvivesARoundTripUnchanged(t *testing.T) {
	t.Parallel()
	body := `<p>a <mark class="m" data-x="1">hi <sub>2</sub></mark> b <!-- kept --> <small>s</small> <b>x</b> <span></span></p>`
	d := oneFrame(t, body)
	doc := peer(t, 1, seeded(t, d))
	if got := read(t, doc).Frames[heroFile]; got != d.Frames[heroFile] {
		t.Fatalf("the markup changed:\nwant %q\ngot  %q", d.Frames[heroFile], got)
	}
	// It is in the text as markup that is kept and not edited: an embed, one position each.
	text := richOf(doc, pidOf(t, d.Frames[heroFile], "<p"))
	var raws int
	for _, d := range text.ToDelta() {
		if m, ok := d.Insert.(map[string]any); ok && m["raw"] != nil {
			raws++
		}
	}
	if raws != 4 {
		t.Errorf("the mark, the comment, the small and the empty span are %d embeds, want 4", raws)
	}
}

func TestAnElementThatIsABoxOfTheDesignStaysANodeEvenInsideText(t *testing.T) {
	t.Parallel()
	body := `<div>one <span style="position:absolute;left:5px;top:6px">two</span> three <b>four</b></div><p>plain</p><p>x<img alt="" src="data:image/png;base64,AA">y</p>`
	d := oneFrame(t, body)
	doc := peer(t, 1, seeded(t, d))
	if got := read(t, doc).Frames[heroFile]; got != d.Frames[heroFile] {
		t.Fatalf("the markup changed:\nwant %q\ngot  %q", d.Frames[heroFile], got)
	}
	frame := d.Frames[heroFile]
	for _, marker := range []string{"<div", "<p"} {
		if richOf(doc, pidOf(t, frame, marker)) != nil {
			t.Errorf("%s holds a box of the design, so it is not formatted text", marker)
		}
	}
	// The positioned span is still an element the editor can move.
	span := pidOf(t, frame, `<span style="position:absolute`)
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, span, "left", "50px")
	})
	if got := read(t, doc).Frames[heroFile]; !strings.Contains(got, "left:50px") {
		t.Errorf("the span did not move: %s", got)
	}
	// A text of plain words is a text node as before: no formatted text for what has no formatting.
	if keys := roomKeys(doc, "|rich"); len(keys) != 0 {
		t.Errorf("formatted text was made for markup that has none: %v", keys)
	}
}

func TestTwoPeopleChangingDifferentWordsOfOneElementKeepBoth(t *testing.T) {
	t.Parallel()
	d := oneFrame(t, `<p>Hello brave <b>new</b> world today</p>`)
	state := seeded(t, d)
	a, b := peer(t, 11, state), peer(t, 22, state)
	pid := pidOf(t, d.Frames[heroFile], "<p")
	ta, tb := richOf(a, pid), richOf(b, pid)
	// Alice changes "brave" to "kind"; Bob makes "today" bold and adds a word at the end.
	// (a text is read outside a transaction, never inside one)
	brave, today, end := strings.Index(ta.ToString(), "brave"), strings.Index(tb.ToString(), "today"), tb.Len()
	a.Transact(func(txn *crdt.Transaction) {
		ta.Delete(txn, brave, len("brave"))
		ta.Insert(txn, brave, "kind", nil)
	})
	b.Transact(func(txn *crdt.Transaction) {
		tb.Format(txn, today, len("today"), crdt.Attributes{"f:e90": `{"t":"b","d":0,"a":[["","data-pid","e90"]]}`})
		tb.Insert(txn, end, " again", crdt.Attributes{"f:e90": nil})
	})
	sync(t, a, b)
	gotA, gotB := read(t, a).Frames[heroFile], read(t, b).Frames[heroFile]
	if gotA != gotB {
		t.Fatalf("the peers did not converge:\n%s\n%s", gotA, gotB)
	}
	for _, want := range []string{"Hello kind ", `<b data-pid="e3">new</b>`, `<b data-pid="e90">today</b> again`} {
		if !strings.Contains(gotA, want) {
			t.Errorf("%q is missing from %s", want, gotA)
		}
	}
	if err := designfmt.Validate(manifest, map[string]string{heroFile: gotA}); err != nil {
		t.Errorf("what the two people made is not a frame: %v", err)
	}
}

func TestFormatsThatCrossAreWrittenAsMarkupThatSaysTheSame(t *testing.T) {
	t.Parallel()
	d := oneFrame(t, `<p>one two three</p><p>x <i>y</i></p>`)
	doc := peer(t, 1, seeded(t, d))
	text := richOf(doc, pidOf(t, d.Frames[heroFile], `<p data-pid="e3"`))
	if text == nil {
		t.Fatal("the second paragraph has no formatted text")
	}
	// Bold over "x " and underline over " y" cross each other, and the <i> is inside the second.
	doc.Transact(func(txn *crdt.Transaction) {
		text.Format(txn, 0, 2, crdt.Attributes{"f:e50": `{"t":"b","d":0,"a":[["","data-pid","e50"]]}`})
		text.Format(txn, 1, 2, crdt.Attributes{"f:e51": `{"t":"u","d":0,"a":[["","data-pid","e51"]]}`})
	})
	got := read(t, doc).Frames[heroFile]
	if !strings.Contains(got, `<b data-pid="e50">x<u data-pid="e51"> </u></b>`) || !strings.Contains(got, `<u data-pid="e51">y</u>`) {
		t.Errorf("crossing formats are not closed and opened again: %s", got)
	}
	// What the room reads is a frame, and saving it makes each element's pid its own.
	if err := designfmt.Validate(manifest, map[string]string{heroFile: got}); err != nil {
		t.Errorf("the markup is not a frame: %v", err)
	}
	saved := normal(t, got)
	if n := strings.Count(saved, `data-pid="e51"`); n != 1 {
		t.Errorf("the pid e51 is on %d elements after saving, want 1: %s", n, saved)
	}
}

func TestDeletingAnElementTakesAwayItsFormattedTextToo(t *testing.T) {
	t.Parallel()
	d := oneFrame(t, `<div><p>one <b>two</b></p></div><p>keep <i>me</i></p>`)
	doc := peer(t, 1, seeded(t, d))
	gone := pidOf(t, d.Frames[heroFile], "<p")
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) { return r.Delete(heroFile, gone) })
	if keys := roomKeys(doc, "|"+gone+"|"); len(keys) != 0 {
		t.Errorf("entries of the deleted element are left: %v", keys)
	}
	got := read(t, doc).Frames[heroFile]
	if strings.Contains(got, "two") || !strings.Contains(got, "keep <i") {
		t.Errorf("deleted the wrong thing: %s", got)
	}
}

// The room refuses what a frame could not say, whoever writes it (the hub runs Check).
func TestMarkupAFrameMayNotHaveIsRefusedWhenSomeoneWritesItIntoFormattedText(t *testing.T) {
	t.Parallel()
	d := oneFrame(t, `<p>Hello <b>bold</b></p>`)
	pid := pidOf(t, d.Frames[heroFile], "<p")
	for name, tc := range map[string]struct {
		attrs crdtAttrs
		embed map[string]any
	}{
		"an event attribute":        {attrs: bold(`["","onclick","x()"]`)},
		"a script address":          {attrs: link(`["","href","javascript:alert(1)"]`)},
		"an external address":       {attrs: link(`["","href","ftp://x.test/"]`)},
		"a positioned span":         {attrs: map[string]any{"f:e70": `{"t":"span","d":0,"a":[["","style","position:absolute;left:1px"]]}`}},
		"a style with an address":   {attrs: map[string]any{"f:e70": `{"t":"span","d":0,"a":[["","style","background:url(https://x.test/a.png)"]]}`}},
		"an element text may not":   {attrs: map[string]any{"f:e70": `{"t":"div","d":0,"a":[]}`}},
		"a script in markup":        {embed: map[string]any{"raw": `<script>alert(1)</script>`}},
		"an event in markup":        {embed: map[string]any{"raw": `<mark onclick="x()">x</mark>`}},
		"a break with an event":     {embed: map[string]any{"br": `[["","onmouseover","x()"]]`}},
		"a format that is not JSON": {attrs: map[string]any{"f:e70": `not json`}},
	} {
		doc := peer(t, 1, seeded(t, d))
		text := richOf(doc, pid)
		doc.Transact(func(txn *crdt.Transaction) {
			if tc.embed != nil {
				text.InsertEmbed(txn, 2, tc.embed, nil)
				return
			}
			text.Format(txn, 0, 3, crdt.Attributes(tc.attrs))
		})
		if err := designroom.Check(doc); err == nil {
			t.Errorf("%s: the room accepted it", name)
		}
	}
	// And what a frame may have is not refused.
	doc := peer(t, 1, seeded(t, d))
	text := richOf(doc, pid)
	doc.Transact(func(txn *crdt.Transaction) {
		text.Format(txn, 0, 3, crdt.Attributes(link(`["","href","#top"]`, `["","style","color:red"]`)))
		text.InsertEmbed(txn, 2, map[string]any{"raw": `<mark class="m">x</mark>`}, nil)
	})
	if err := designroom.Check(doc); err != nil {
		t.Errorf("a link to a fragment, a colour and a mark are refused: %v", err)
	}
}

// Links are normalised as published links are, so a link written through the room
// (by a person's editor or an agent) opens in a new tab that knows nothing of the page.
func TestALinkWrittenIntoFormattedTextOpensInANewTabWithoutAnOpener(t *testing.T) {
	t.Parallel()
	d := oneFrame(t, `<p>Hello <b>bold</b> world</p>`)
	pid := pidOf(t, d.Frames[heroFile], "<p")
	for name, tc := range map[string]struct {
		rows      []string
		want, not string
	}{
		"https":                  {rows: []string{`["","data-pid","e60"]`, `["","href","https://x.test/a?b=1"]`}, want: `href="https://x.test/a?b=1" target="_blank" rel="noopener noreferrer"`},
		"mailto":                 {rows: []string{`["","data-pid","e60"]`, `["","href","mailto:a@x.test"]`}, want: `target="_blank" rel="noopener noreferrer"`},
		"its own target and rel": {rows: []string{`["","data-pid","e60"]`, `["","href","https://x.test/"]`, `["","target","_self"]`, `["","rel","opener"]`}, want: `target="_blank" rel="noopener noreferrer"`, not: "opener\""},
		"a link inside the page": {rows: []string{`["","data-pid","e60"]`, `["","href","#top"]`}, want: `<a data-pid="e60" href="#top">`, not: "_blank"},
		"no address":             {rows: []string{`["","data-pid","e60"]`}, want: `<a data-pid="e60">`, not: "_blank"},
	} {
		doc := peer(t, 1, seeded(t, d))
		text := richOf(doc, pid)
		doc.Transact(func(txn *crdt.Transaction) {
			text.Format(txn, 6, 4, crdt.Attributes(map[string]any{"f:e60": `{"t":"a","d":0,"a":[` + strings.Join(tc.rows, ",") + `]}`}))
		})
		if err := designroom.Check(doc); err != nil {
			t.Fatalf("%s: refused: %v", name, err)
		}
		got := read(t, doc).Frames[heroFile]
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q is missing from %s", name, tc.want, got)
		}
		if tc.not != "" && strings.Contains(got, tc.not) {
			t.Errorf("%s: %q should not be in %s", name, tc.not, got)
		}
		if err := designfmt.Validate(manifest, map[string]string{heroFile: got}); err != nil {
			t.Errorf("%s: what the room reads is not a frame: %v", name, err)
		}
	}
}

type crdtAttrs = map[string]any

func bold(rows ...string) crdtAttrs {
	return map[string]any{"f:e70": fmt.Sprintf(`{"t":"b","d":0,"a":[["","data-pid","e70"],%s]}`, strings.Join(rows, ","))}
}

func link(rows ...string) crdtAttrs {
	return map[string]any{"f:e70": fmt.Sprintf(`{"t":"a","d":0,"a":[["","data-pid","e70"],%s]}`, strings.Join(rows, ","))}
}
