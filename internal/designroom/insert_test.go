package designroom_test

import (
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

// The editor inserts, duplicates and groups by writing new nodes straight into
// the room's map (web/src/design/ops), with the keys these tests spell. What it
// writes must read as the element it meant, and a room holding what a frame may
// not have must be refused whoever wrote it.

func rootOf(t *testing.T, doc *crdt.Doc) string {
	t.Helper()
	v, _ := doc.GetMap(designroom.Map).Get(heroFile + "|shell|root")
	root, _ := v.(string)
	if root == "" {
		t.Fatal("the room has no body")
	}
	return root
}

// put writes the entries of one element of the body, as the editor does.
func put(doc *crdt.Doc, id, parent, order, tag string, extra map[string]string) {
	m := doc.GetMap(designroom.Map)
	key := func(field string) string { return heroFile + "|n|" + id + "|" + field }
	doc.Transact(func(txn *crdt.Transaction) {
		m.Set(txn, key("kind"), "el")
		m.Set(txn, key("tag"), tag)
		m.Set(txn, key("parent"), parent)
		m.Set(txn, key("order"), order)
		for field, v := range extra {
			m.Set(txn, key(field), v)
		}
	})
}

func TestAnInsertedTextBoxAndRectangleAreInTheSavedFileAfterWhatWasThereAndNothingElseMoves(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, heroDesign(t)))
	before := read(t, doc).Frames[heroFile]
	root := rootOf(t, doc)
	put(doc, "nbox", root, "zz", "div", map[string]string{"attrs": "[]", "s|position": "absolute", "s|left": "40px", "s|top": "50px", "s|width": "160px", "s|height": "120px", "s|background-color": "#d9d9d9"})
	put(doc, "ntext", root, "zzz", "div", map[string]string{"attrs": "[]", "s|position": "absolute", "s|left": "60px", "s|top": "70px", "s|font-size": "24px"})
	m := doc.GetMap(designroom.Map)
	doc.Transact(func(txn *crdt.Transaction) {
		m.Set(txn, heroFile+"|n|~tnew|kind", "text")
		m.Set(txn, heroFile+"|n|~tnew|parent", "ntext")
		m.Set(txn, heroFile+"|n|~tnew|order", "V")
		m.Set(txn, heroFile+"|n|~tnew|text", "Text")
	})
	got := read(t, doc).Frames[heroFile]
	added := `<div style="background-color:#d9d9d9;height:120px;left:40px;position:absolute;top:50px;width:160px" data-pid="nbox"></div>` +
		`<div style="font-size:24px;left:60px;position:absolute;top:70px" data-pid="ntext">Text</div>`
	if want := replaceOnce(t, before, "</body>", added+"</body>"); got != want {
		t.Errorf("want %s\ngot  %s", want, got)
	}
}

func TestAnInsertedImageReadsAsAnImgWithItsAltAndAFillAsAnOwnedBackground(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, heroDesign(t)))
	before := read(t, doc).Frames[heroFile]
	root := rootOf(t, doc)
	put(doc, "nimg", root, "zz", "img", map[string]string{
		"attrs":        `[["","src","/files/abc/photo.png"],["","alt","A photo"]]`,
		"s|object-fit": "cover",
		"s|width":      "200px",
	})
	put(doc, "nfill", root, "zzz", "div", map[string]string{
		"attrs":                 `[]`,
		"s|background-image":    "url(/files/abc/photo.png)",
		"s|background-size":     "contain",
		"s|background-repeat":   "no-repeat",
		"s|background-position": "center",
	})
	if err := designroom.Check(doc); err != nil {
		t.Fatalf("a picture of the artifact was refused: %v", err)
	}
	got := read(t, doc).Frames[heroFile]
	added := `<img src="/files/abc/photo.png" alt="A photo" style="object-fit:cover;width:200px" data-pid="nimg"/>` +
		`<div style="background-image:url(/files/abc/photo.png);background-position:center;background-repeat:no-repeat;background-size:contain" data-pid="nfill"></div>`
	if want := replaceOnce(t, before, "</body>", added+"</body>"); got != want {
		t.Errorf("want %s\ngot  %s", want, got)
	}
}

func TestAnElementThatAFrameMayNotHaveIsRefusedWhoeverWroteItAndAnImageOfTheArtifactIsNot(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		tag   string
		attrs string
	}{
		"a script":                    {"script", `[]`},
		"an iframe":                   {"iframe", `[["","src","/files/a/b.html"]]`},
		"a picture from elsewhere":    {"img", `[["","src","https://x.test/a.png"]]`},
		"a picture from another host": {"img", `[["","src","//x.test/a.png"]]`},
		"an event handler":            {"div", `[["","onclick","alert(1)"]]`},
		"a script address":            {"a", `[["","href","javascript:alert(1)"]]`},
		"a tag that is not a name":    {"div x=1", `[]`},
	}
	for name, c := range cases {
		doc := peer(t, 1, seeded(t, heroDesign(t)))
		put(doc, "bad", rootOf(t, doc), "zz", c.tag, map[string]string{"attrs": c.attrs})
		if err := designroom.Check(doc); err == nil {
			t.Errorf("%s: the room was accepted", name)
		}
	}
	doc := peer(t, 1, seeded(t, heroDesign(t)))
	put(doc, "ok", rootOf(t, doc), "zz", "img", map[string]string{"attrs": `[["","src","/files/abc/p.png"],["","alt",""]]`})
	put(doc, "data", rootOf(t, doc), "zzz", "img", map[string]string{"attrs": `[["","src","data:image/png;base64,iVBORw0KGgo="],["","alt","x"]]`})
	if err := designroom.Check(doc); err != nil {
		t.Errorf("an image of the artifact and a small picture were refused: %v", err)
	}
}

func TestOnlyAPictureOfTheArtifactOrASmallOneWrittenInIsAValueOfBackgroundImage(t *testing.T) {
	t.Parallel()
	for _, v := range []string{
		"none", "url(/files/abc/photo.png)", `url("/files/abc/photo.png")`, `url( '/files/abc/photo.png' )`,
		"url(/files/a/b.png), url(/files/a/c.png)", "url(data:image/png;base64,iVBORw0KGgo=)", "inherit", "",
	} {
		if err := designroom.ValidateStyleValue("background-image", v); err != nil {
			t.Errorf("%q was refused: %v", v, err)
		}
	}
	for _, v := range []string{
		"url(https://x.test/a.png)", "url(//x.test/a.png)", "url(/api/artifacts)", "url(javascript:alert(1))", "url(data:text/html;base64,AAAA)",
		"url(/files/a/b.png);color:red", "url(/files/a/b.png) }", "url(/files/a/b.png", `url("/files/a/b.png)`, "url(/files/a/b\\.png)",
		"linear-gradient(#fff,#000)", "image-set(x 1x)", "url(/files/a/b.png) no-repeat", strings.Repeat("url(/files/a/b.png),", 60) + "none",
	} {
		if err := designroom.ValidateStyleValue("background-image", v); err == nil {
			t.Errorf("%q was accepted", v)
		}
	}
	for prop, ok := range map[string][]string{
		"background-size":     {"cover", "contain", "auto", "50% auto", "120px"},
		"background-position": {"center", "left top", "50% 50%"},
		"background-repeat":   {"no-repeat", "repeat-x"},
		"object-fit":          {"cover", "contain", "none", "scale-down", "fill"},
	} {
		for _, v := range ok {
			if err := designroom.ValidateStyleValue(prop, v); err != nil {
				t.Errorf("%s: %q was refused: %v", prop, v, err)
			}
		}
	}
	for _, bad := range [][2]string{{"object-fit", "cover;x"}, {"background-repeat", "sideways"}, {"background-size", "url(/files/a/b.png)"}} {
		if err := designroom.ValidateStyleValue(bad[0], bad[1]); err == nil {
			t.Errorf("%s: %q was accepted", bad[0], bad[1])
		}
	}
}

const filled = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">` +
	`<div id="photo" style="width:100px;background-image:url(/files/abc/photo.png);background-size:cover"></div>` +
	`<div id="sky" style="width:100px;background-image:linear-gradient(#fff,#000)"></div></body></html>`

func TestAPictureFillTheEditorCanHoldIsAnEntryAndAnAuthoredGradientStaysWhereItIsAndComesBackExactly(t *testing.T) {
	t.Parallel()
	d := designroom.Design{Manifest: manifest, Frames: map[string]string{heroFile: normal(t, filled)}}
	doc := peer(t, 1, seeded(t, d))
	before := read(t, doc).Frames[heroFile]
	if before != d.Frames[heroFile] {
		t.Fatalf("the frame did not come back as it went in:\n%s\n%s", d.Frames[heroFile], before)
	}
	photo, sky := pidOf(t, before, `id="photo"`), pidOf(t, before, `id="sky"`)
	m := doc.GetMap(designroom.Map)
	if v, _ := m.Get(heroFile + "|n|" + photo + "|s|background-image"); v != "url(/files/abc/photo.png)" {
		t.Errorf("the fill of the editor is not its entry: %v", v)
	}
	if v, _ := m.Get(heroFile + "|n|" + sky + "|s|background-image"); v != nil {
		t.Errorf("a gradient the editor cannot hold became an entry: %v", v)
	}
	edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
		return r.SetStyle(heroFile, photo, "background-size", "contain")
	})
	if got, want := read(t, doc).Frames[heroFile], replaceOnce(t, before, "background-size:cover", "background-size:contain"); got != want {
		t.Errorf("want %s\ngot  %s", want, got)
	}
}

func TestAGroupTheEditorWritesIsABoxAroundTheElementsInTheSavedFileAndTakingItAwayGivesTheFileBack(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, heroDesign(t)))
	before := read(t, doc).Frames[heroFile]
	box, section := pidOf(t, before, `id="box"`), pidOf(t, before, "<section")
	root := rootOf(t, doc)
	// The box takes the place of the first element; the elements keep their order and change only their parent.
	m := doc.GetMap(designroom.Map)
	var boxOrder any
	boxOrder, _ = m.Get(heroFile + "|n|" + box + "|order")
	put(doc, "grp", root, boxOrder.(string), "div", map[string]string{"attrs": `[["","data-name","Group"]]`, "s|position": "absolute", "s|left": "5px"})
	setEntry(doc, heroFile+"|n|"+box+"|parent", "grp")
	setEntry(doc, heroFile+"|n|"+section+"|parent", "grp")
	grouped := read(t, doc).Frames[heroFile]
	if !strings.Contains(grouped, `data-name="Group"`) || !strings.Contains(grouped, `<div data-name="Group" style="left:5px;position:absolute" data-pid="grp"><div id="box"`) || !strings.Contains(grouped, `</section></div></body>`) {
		t.Errorf("the elements are not inside the box in the file:\n%s", grouped)
	}
	// Taking the box away: the elements go back to the body in their order.
	setEntry(doc, heroFile+"|n|"+box+"|parent", root)
	setEntry(doc, heroFile+"|n|"+section+"|parent", root)
	for _, field := range []string{"kind", "tag", "parent", "order", "attrs", "s|position", "s|left"} {
		dropEntry(doc, heroFile+"|n|grp|"+field)
	}
	if got := read(t, doc).Frames[heroFile]; got != before {
		t.Errorf("ungrouping did not give the file back:\nwant %s\ngot  %s", before, got)
	}
}
