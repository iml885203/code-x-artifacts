package designfmt

import (
	"strings"
	"testing"
)

const oneFrame = `{"version":1,"pages":[{"id":"home","name":"Home","rows":[[{"id":"hero","name":"Hero","file":"frames/hero.html","w":1440,"h":900}]]}]}`

// frameDoc is a frame that is a body only, so the first element of body is element 2.
func frameDoc(body string) string { return `<body>` + body + `</body>` }

func validateHero(body string) error {
	return Validate(oneFrame, map[string]string{"frames/hero.html": frameDoc(body)})
}

func TestADesignThatFollowsTheRulesValidates(t *testing.T) {
	if err := validateHero(`<h1 style="font-size:40px">Hello</h1><img alt="" src="data:image/png;base64,AAAA"><a href="#x">x</a>`); err != nil {
		t.Fatal(err)
	}
}

// Each of these is one mistake an agent makes; the message must start with where it is.
func TestEveryRuleIsReportedWithItsPath(t *testing.T) {
	const where = `Page "Home" frame "Hero" element `
	for name, tc := range map[string]struct{ body, want string }{
		"a script tag":          {`<p>a</p><script>alert(1)</script>`, where + `3: tag script is not allowed`},
		"an iframe":             {`<iframe src="#x"></iframe>`, where + `2: tag iframe is not allowed`},
		"a link element":        {`<link rel="stylesheet" href="https://x.test/a.css">`, where + `2: tag link is not allowed`},
		"an event attribute":    {`<p onclick="x()">a</p>`, where + `2: attribute onclick is not allowed`},
		"an upper case event":   {`<p ONMOUSEOVER="x()">a</p>`, where + `2: attribute onmouseover is not allowed`},
		"an external image":     {`<img alt="" src="https://x.test/a.png">`, where + `2: attribute src: external address`},
		"an ftp link":           {`<a href="ftp://x.test/">x</a>`, where + `2: attribute href: external address`},
		"a link on a non-link":  {`<area href="https://x.test/">`, where + `2: attribute href: external address`},
		"a link in a form":      {`<form action="mailto:a@x.test"><button>go</button></form>`, where + `2: attribute action: external address`},
		"an https image":        {`<img alt="" src="https://x.test/a.png">`, where + `2: attribute src: external address`},
		"an https css url":      {`<div style="background:url(https://x.test/a.png)">x</div>`, where + `2: style attribute: css property "background": external address`},
		"a script url":          {`<a href="java&#x09;script:alert(1)">x</a>`, where + `2: attribute href: external address`},
		"a relative file":       {`<img alt="" src="logo.png">`, where + `2: attribute src: external address`},
		"a srcset candidate":    {`<img alt="" src="#a" srcset="data:image/png;base64,AA 1x, https://x.test/b.png 2x">`, where + `2: attribute srcset: external address`},
		"a css url in style":    {`<div style="background:url(https://x.test/a.png)">x</div>`, where + `2: style attribute: css property "background": external address`},
		"a css url in a sheet":  {`<style>.a { background-image: url("//x.test/a.png") }</style>`, where + `2: style element: css property "background-image": external address`},
		"an import":             {`<style>@import url(https://x.test/a.css);</style>`, where + `2: style element: at-rule @import is not allowed`},
		"a css escape":          {`<div style="background:u\72l(https://x.test/a.png)">x</div>`, where + `2: style attribute: css property "background": a backslash escape`},
		"an expression":         {`<div style="width:expression(alert(1))">x</div>`, where + `2: style attribute: css property "width": expression()`},
		"a css property":        {`<div style="behavior:url(#x)">x</div>`, where + `2: style attribute: css property "behavior" is not allowed`},
		"an svg url attribute":  {`<svg><rect fill="url(https://x.test/#a)"/></svg>`, where + `3: attribute fill: external address`},
		"a meta refresh":        {`<meta http-equiv="refresh" content="0;url=https://x.test">`, where + `2: attribute http-equiv is not allowed`},
		"srcdoc":                {`<div srcdoc="x">a</div>`, where + `2: attribute srcdoc is not allowed`},
		"a font face from far":  {`<style>@font-face { font-family: X; src: url(https://x.test/f.woff2) }</style>`, where + `2: style element: css property "src": external address`},
		"an unknown at-rule":    {`<style>@counter-style x { system: cyclic }</style>`, where + `2: style element: at-rule @counter-style is not allowed`},
		"a form action outside": {`<form action="https://x.test"><button>go</button></form>`, where + `2: attribute action: external address`},
	} {
		err := validateHero(tc.body)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want a problem starting %q", name, err, tc.want)
		}
	}
}

func TestEveryProblemIsListedNotJustTheFirst(t *testing.T) {
	err := validateHero(`<script>1</script><p onclick="x()">a</p><img alt="" src="https://x.test/a.png">`)
	if err == nil {
		t.Fatal("a frame with three mistakes validated")
	}
	for _, want := range []string{"tag script", "attribute onclick", "attribute src", "3 problems", "nothing was stored"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not say %q", err, want)
		}
	}
}

func TestManifestProblemsNameWhereTheyAre(t *testing.T) {
	frames := map[string]string{"frames/hero.html": frameDoc("<p>x"), "frames/two.html": frameDoc("<p>y")}
	for name, tc := range map[string]struct{ manifest, want string }{
		"not json":               {`{"version":1,`, "design.json is not valid JSON"},
		"not an object":          {`[]`, "design.json must be an object"},
		"an unknown field":       {`{"version":1,"pages":[{"id":"a","name":"A","rows":[[{"id":"h","name":"H","file":"frames/hero.html","w":10,"h":10}]]}],"theme":1}`, `unknown field "theme"`},
		"the wrong version":      {`{"version":2,"pages":[]}`, "version must be 1"},
		"no pages":               {`{"version":1,"pages":[]}`, "pages must be a list with at least one page"},
		"a page without rows":    {`{"version":1,"pages":[{"id":"a","name":"A","rows":[]}]}`, `Page "A": rows must be a list`},
		"a frame without a name": {`{"version":1,"pages":[{"id":"a","name":"A","rows":[[{"id":"h","file":"frames/hero.html","w":10,"h":10}]]}]}`, `Page "A" rows[0][0]: name is required`},
		"a bad id":               {`{"version":1,"pages":[{"id":"a","name":"A","rows":[[{"id":"Hero One","name":"H","file":"frames/hero.html","w":10,"h":10}]]}]}`, `Page "A" frame "H": frame id must be`},
		"too wide":               {`{"version":1,"pages":[{"id":"a","name":"A","rows":[[{"id":"h","name":"H","file":"frames/hero.html","w":8193,"h":10}]]}]}`, `Page "A" frame "H": w must be a whole number of pixels from 1 to 8192`},
		"a fractional height":    {`{"version":1,"pages":[{"id":"a","name":"A","rows":[[{"id":"h","name":"H","file":"frames/hero.html","w":10,"h":10.5}]]}]}`, `Page "A" frame "H": h must be a whole number`},
		"a file elsewhere":       {`{"version":1,"pages":[{"id":"a","name":"A","rows":[[{"id":"h","name":"H","file":"hero.html","w":10,"h":10}]]}]}`, `Page "A" frame "H": file "hero.html" is not a frame's`},
		"a duplicate frame id":   {`{"version":1,"pages":[{"id":"a","name":"A","rows":[[{"id":"h","name":"H","file":"frames/hero.html","w":10,"h":10},{"id":"h","name":"Again","file":"frames/two.html","w":10,"h":10}]]}]}`, `Page "A" frame "Again": frame id "h" is already used by Page "A" frame "H"`},
		"a shared file":          {`{"version":1,"pages":[{"id":"a","name":"A","rows":[[{"id":"h","name":"H","file":"frames/hero.html","w":10,"h":10},{"id":"i","name":"I","file":"frames/hero.html","w":10,"h":10}]]}]}`, `file frames/hero.html is already the file of`},
		"a missing file":         {`{"version":1,"pages":[{"id":"a","name":"A","rows":[[{"id":"h","name":"H","file":"frames/nothing.html","w":10,"h":10}]]}]}`, `Page "A" frame "H": file frames/nothing.html is not among this design's files`},
	} {
		err := Validate(tc.manifest, frames)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
}

func TestFilesThatAreNotFramesAreRefused(t *testing.T) {
	err := Validate(oneFrame, map[string]string{"frames/hero.html": frameDoc("<p>x"), "notes/readme.html": "<p>x"})
	if err == nil || !strings.Contains(err.Error(), "file notes/readme.html: only frames are files of a design") {
		t.Errorf("got %v", err)
	}
}

func TestTheLimitsNameTheLimit(t *testing.T) {
	big := frameDoc(strings.Repeat("a", MaxFrameBytes))
	if err := Validate(oneFrame, map[string]string{"frames/hero.html": big}); err == nil || !strings.Contains(err.Error(), "512 KB") {
		t.Errorf("an oversized frame: %v", err)
	}
	if err := Validate(`{"version":1,"pages":[],"x":"`+strings.Repeat("a", MaxManifestBytes)+`"}`, nil); err == nil || !strings.Contains(err.Error(), "256 KB") {
		t.Errorf("an oversized design.json: %v", err)
	}
	var b strings.Builder
	b.WriteString(`{"version":1,"pages":[{"id":"p","name":"P","rows":[[`)
	fs := map[string]string{}
	for i := range MaxFrames + 1 {
		if i > 0 {
			b.WriteString(",")
		}
		name := "f" + strings.Repeat("a", i%5) + string(rune('a'+i/5)) + string(rune('a'+i%26))
		b.WriteString(`{"id":"` + name + `","name":"` + name + `","file":"frames/` + name + `.html","w":10,"h":10}`)
		fs["frames/"+name+".html"] = frameDoc("<p>x")
	}
	b.WriteString(`]]}]}`)
	if err := Validate(b.String(), fs); err == nil || !strings.Contains(err.Error(), "at most 60 frames") {
		t.Errorf("too many frames: %v", err)
	}
}
