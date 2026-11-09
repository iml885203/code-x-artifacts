package designfmt

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func readMessy(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/messy.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var messySamples = map[string]string{
	"a fragment":               `<p>one<p>two`,
	"no doctype":               `<html><body><div>x</div></body></html>`,
	"a table without ends":     `<table><tr><td>a<td>b<tr><td>c</table>`,
	"a pre with a first break": "<pre>\n\nkept</pre>",
	"unquoted attributes":      `<div class=a id=b data-x=1>x</div>`,
	"misnested tags":           `<b><i>bold italic</b> italic</i>`,
	"entities":                 `<p>&lt;tag&gt; &amp; &nbsp; &copy; 你好 😀</p>`,
	"an svg":                   `<svg viewBox="0 0 10 10"><path d="M0 0L10 10"/><text x="1" y="2">t</text></svg>`,
	"a style element":          `<style>a > b { color: red } /* c */</style><p>x`,
	"a select":                 `<select><option>a<option selected>b</select>`,
	"nested lists":             `<ul><li>a<ul><li>b</ul><li>c</ul>`,
	"messy":                    "",
}

func TestNormalizingTwiceGivesTheSameBytes(t *testing.T) {
	messySamples["messy"] = readMessy(t)
	for name, in := range messySamples {
		once, err := Normalize(in)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		twice, err := Normalize(once)
		if err != nil || twice != once {
			t.Errorf("%s: normalising the normal form changed it (err %v):\nonce:  %q\ntwice: %q", name, err, once, twice)
		}
	}
}

func TestNormalizingAddsTheDoctypeAndAPIDToEveryElementOfTheBody(t *testing.T) {
	out, err := Normalize(`<p>one<span>two</span></p>`)
	if err != nil {
		t.Fatal(err)
	}
	want := `<!DOCTYPE html><html><head></head><body data-pid="e1"><p data-pid="e2">one<span data-pid="e3">two</span></p></body></html>`
	if out != want {
		t.Errorf("got  %s\nwant %s", out, want)
	}
	if strings.Contains(out, `<html data-pid`) || strings.Contains(out, `<head data-pid`) {
		t.Error("html and head are not part of the design and get no id")
	}
}

func TestAPIDThatIsThereIsKeptAndANewOneNeverReusesANumberInTheFrame(t *testing.T) {
	out, err := Normalize(`<body><p data-pid="e7">a</p><p>b</p><p data-pid="hero">c</p></body>`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<p data-pid="e7">a</p>`, `<p data-pid="hero">c</p>`, `<p data-pid="e9">b</p>`, `<body data-pid="e8">`} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is not in %s", want, out)
		}
	}
}

func TestAnElementThatRepeatsAnIDOrHasAnInvalidOneGetsANewOne(t *testing.T) {
	out, err := Normalize(`<body data-pid="b"><p data-pid="e1">a</p><p data-pid="e1">b</p><p data-pid="has space">c</p></body>`)
	if err != nil {
		t.Fatal(err)
	}
	ids := regexp.MustCompile(`data-pid="([^"]*)"`).FindAllStringSubmatch(out, -1)
	seen := map[string]bool{}
	for _, m := range ids {
		if seen[m[1]] {
			t.Errorf("the id %q is on two elements in %s", m[1], out)
		}
		seen[m[1]] = true
	}
	if len(ids) != 4 || !strings.Contains(out, `<p data-pid="e1">a</p>`) || strings.Contains(out, "has space") {
		t.Errorf("got %s", out)
	}
}

// The editor changes only what it has controls for, so what an agent wrote that
// the editor does not know has to come out of a publish as it went in.
func TestUnknownCSSAndAttributesInsideAnAllowedElementSurviveUnchanged(t *testing.T) {
	in := readMessy(t)
	if err := Validate(oneFrame, map[string]string{"frames/hero.html": in}); err != nil {
		t.Fatalf("the messy frame does not validate: %v", err)
	}
	out, err := Normalize(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{
		`--x: {a:b;c}`,
		`background: linear-gradient(135deg, #fff 0%, #e8e4da 100%)`,
		`grid-template-columns: 1fr 2fr`,
		`@media (max-width: 600px) { .card { grid-template-columns: 1fr } }`,
		`@supports (display: grid) { .grid { display: grid } }`,
		`@keyframes pulse { from { opacity: .6 } 50% { opacity: 1 } to { opacity: .6 } }`,
		`:root { --accent: #0f766e; --pad: 12px;`,
		`-webkit-foo: 1`,
		`width: 220px !important`,
		`display:-webkit-box;display:flex;--local: 7px`,
		`content: "\201C"`,
		`box-shadow: 0 2px 8px rgb(0 0 0 / 20%)`,
		`data-owner="design-team"`,
		`data-x-unknown="1"`,
		`<!-- a comment the author left -->`,
		`名前 テスト 😀`,
		`<linearGradient id="g"`,
	} {
		if !strings.Contains(out, kept) {
			t.Errorf("%q did not survive", kept)
		}
	}
}

func TestADesignWithTheMessyFrameValidatesAndNormalizedItStillDoes(t *testing.T) {
	out, err := Normalize(readMessy(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(oneFrame, map[string]string{"frames/hero.html": out}); err != nil {
		t.Errorf("the normal form of a valid frame does not validate: %v", err)
	}
}

func TestReviewNotesWhatAPublisherCannotSee(t *testing.T) {
	const manifest = `{"version":1,"pages":[{"id":"p","name":"Home","rows":[[{"id":"a","name":"Hero","file":"frames/a.html","w":400,"h":300},{"id":"b","name":"Empty","file":"frames/b.html","w":400,"h":300}]]}]}`
	a, err := Normalize(`<body><p>hi</p><img src="data:image/png;base64,AA"><div style="position:absolute;left:500px;top:10px;width:50px">x</div><div style="position:absolute;left:380px;top:10px;width:50px">y</div></body>`)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Normalize(`<body><div style="width:10px"></div></body>`)
	orphan, _ := Normalize(`<p>x</p>`)
	fs := map[string]string{"frames/a.html": a, "frames/b.html": b, "frames/c.html": orphan}
	notes := strings.Join(Review(manifest, fs, []string{"frames/a.html", "frames/b.html", "frames/c.html"}), "\n")
	for _, want := range []string{
		`Page "Home" frame "Hero": img data-pid "e3" has no alt text`,
		`Page "Home" frame "Hero": data-pid "e4" is placed outside the 400x300 frame`,
		`Page "Home" frame "Hero": data-pid "e5" reaches past the edge of the 400x300 frame`,
		`Page "Home" frame "Empty": the frame has no text`,
		`frames/c.html is not in design.json`,
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("no note %q in:\n%s", want, notes)
		}
	}
	if got := Review(manifest, fs, []string{"frames/b.html"}); len(got) != 1 || !strings.Contains(got[0], "Empty") {
		t.Errorf("a publish of one frame is answered with notes about it only: %v", got)
	}
}

func TestAFrameWithTextAndAltTextHasNothingToReview(t *testing.T) {
	a, _ := Normalize(`<body><h1>Title</h1><img alt="" src="data:image/png;base64,AA"></body>`)
	if got := Review(oneFrame, map[string]string{"frames/hero.html": a}, []string{"frames/hero.html"}); len(got) != 0 {
		t.Errorf("notes for a sound frame: %v", got)
	}
}
