package designfmt

import (
	"strings"
	"testing"
)

// reviewBody normalises a body and reviews it as the only frame of a 400x300 design.
func reviewBody(t *testing.T, body string) []string {
	t.Helper()
	const manifest = `{"version":1,"pages":[{"id":"p","name":"Home","rows":[[{"id":"a","name":"Hero","file":"frames/a.html","w":400,"h":300}]]}]}`
	frame, err := Normalize(`<body>` + body + `</body>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(manifest, map[string]string{"frames/a.html": frame}); err != nil {
		t.Fatalf("the frame under review does not validate: %v", err)
	}
	return Review(manifest, map[string]string{"frames/a.html": frame}, []string{"frames/a.html"})
}

// Each rule has a frame that trips it and a frame that is close to it and fine.
func TestEachReviewRuleFiresOnASmallFrameAndStaysSilentOnAFineOne(t *testing.T) {
	const where = `Page "Home" frame "Hero": `
	long := strings.Repeat("A long sentence that keeps going. ", 6)
	for name, tc := range map[string]struct{ bad, fine, want string }{
		"text taller than its box": {
			bad:  `<div style="width:200px;height:40px;font-size:20px">` + long + `</div>`,
			fine: `<div style="width:200px;height:400px;font-size:20px">` + long + `</div>`,
			want: where + `data-pid "e2" ("A long sentence that kee…") is likely to overflow its box`,
		},
		"a box with padding inside its size": {
			bad:  `<div style="box-sizing:border-box;width:100px;height:60px;padding:20px;font-size:16px">Some words that need three lines</div>`,
			fine: `<div style="width:100px;height:60px;padding:20px;font-size:16px">Some words that need three lines</div>`,
			want: `is likely to overflow its box`,
		},
		"a line that does not wrap and is wider than its box": {
			bad:  `<div style="width:100px;white-space:nowrap;font-size:20px">A heading that is far too long</div>`,
			fine: `<div style="width:100px;white-space:nowrap;text-overflow:ellipsis;overflow:hidden;font-size:20px">A heading that is far too long</div>`,
			want: `stays on one line and is about`,
		},
		"a line that does not wrap and is wider than the frame": {
			bad:  `<p style="white-space:nowrap;font-size:40px">` + long + `</p>`,
			fine: `<p style="white-space:nowrap;font-size:10px">A short line</p>`,
			want: `stays on one line and is about`,
		},
		"text with a line break counts both lines": {
			bad:  `<div style="width:300px;height:30px;font-size:20px">one<br>two<br>three</div>`,
			fine: `<div style="width:300px;height:100px;font-size:20px">one<br>two<br>three</div>`,
			want: `is likely to overflow its box`,
		},
		"an element past the edge": {
			bad:  `<p>hi</p><div style="position:absolute;left:380px;top:10px;width:50px">y</div>`,
			fine: `<p>hi</p><div style="position:absolute;left:300px;top:10px;width:50px">y</div>`,
			want: `reaches past the edge of the 400x300 frame`,
		},
		"an element outside": {
			bad:  `<p>hi</p><div style="position:absolute;left:500px;top:10px;width:50px">y</div>`,
			fine: `<p>hi</p><div style="position:absolute;left:100px;top:10px;width:50px">y</div>`,
			want: `is placed outside the 400x300 frame`,
		},
		"a property no browser applies, on an element": {
			bad:  `<p style="-ms-filter:blur(2px);color:red">hi</p>`,
			fine: `<p style="filter:blur(2px);-webkit-text-stroke:1px red;-moz-osx-font-smoothing:grayscale">hi</p>`,
			want: where + `data-pid "e2" sets -ms-filter, which no current browser applies`,
		},
		"a property no browser applies, in a stylesheet": {
			bad:  `<style>.a { -o-transition: all 1s; }</style><p>hi</p>`,
			fine: `<style>.a { transition: all 1s; }</style><p>hi</p>`,
			want: where + `a <style> rule sets -o-transition, which no current browser applies`,
		},
		"an image without alt text": {
			bad:  `<p>hi</p><img src="data:image/png;base64,AA">`,
			fine: `<p>hi</p><img alt="" src="data:image/png;base64,AA">`,
			want: `has no alt text`,
		},
		"no text at all": {
			bad:  `<div style="width:10px"></div>`,
			fine: `<div style="width:10px">x</div>`,
			want: `the frame has no text`,
		},
	} {
		if got := strings.Join(reviewBody(t, tc.bad), "\n"); !strings.Contains(got, tc.want) {
			t.Errorf("%s: the frame that should trip the rule gave %q, want a note containing %q", name, got, tc.want)
		}
		if got := reviewBody(t, tc.fine); len(got) != 0 {
			t.Errorf("%s: the fine frame was given notes: %v", name, got)
		}
	}
}

// The estimate has to say what it is about, so the agent can find the element.
func TestAnOverflowNoteNamesTheElementTheSizeItNeedsAndTheRoomItHas(t *testing.T) {
	got := reviewBody(t, `<h1 style="width:120px;height:30px;font-size:24px">Quarterly results for the whole region</h1>`)
	if len(got) != 1 {
		t.Fatalf("want one note, got %v", got)
	}
	for _, want := range []string{`data-pid "e2"`, `Quarterly results for`, "px of text for 30 px of height"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("the note %q does not say %q", got[0], want)
		}
	}
}

// What the estimate cannot know it does not guess at.
func TestTheOverflowEstimateKeepsQuietWhereItCannotKnowTheFontOrTheBox(t *testing.T) {
	for name, body := range map[string]string{
		"a font size in em":       `<div style="width:100px;height:20px;font-size:3em">` + strings.Repeat("word ", 30) + `</div>`,
		"a box with no height":    `<div style="width:100px;font-size:20px">` + strings.Repeat("word ", 30) + `</div>`,
		"a scrolling box":         `<div style="width:100px;height:20px;font-size:20px;overflow:auto">` + strings.Repeat("word ", 30) + `</div>`,
		"a clamped box":           `<div style="width:100px;height:20px;font-size:20px;-webkit-line-clamp:2">` + strings.Repeat("word ", 30) + `</div>`,
		"a box that holds blocks": `<div style="width:100px;height:20px;font-size:20px"><p>` + strings.Repeat("word ", 30) + `</p></div>`,
		"text inside a drawing":   `<svg width="10" height="10"><text style="width:5px;height:5px;font-size:30px">` + strings.Repeat("word ", 30) + `</text></svg>`,
	} {
		for _, note := range reviewBody(t, body) {
			if strings.Contains(note, "overflow") || strings.Contains(note, "one line") {
				t.Errorf("%s: a note was given: %s", name, note)
			}
		}
	}
}

// A property a frame uses is refused by name when it is outside the allow-list,
// and is kept, written as it was, when it is only one nothing applies.
func TestAFrameUsingAPropertyNoBrowserAppliesIsToldItsNameAndKeepsItAsWritten(t *testing.T) {
	frame := `<body><p style="-ms-filter: blur(2px);color:red">hi</p></body>`
	normal, err := Normalize(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(normal, `style="-ms-filter: blur(2px);color:red"`) {
		t.Errorf("the declaration was changed: %s", normal)
	}
	notes := reviewBody(t, `<p style="-ms-filter: blur(2px);color:red">hi</p>`)
	if len(notes) != 1 || !strings.Contains(notes[0], "-ms-filter") {
		t.Errorf("want one note that names -ms-filter, got %v", notes)
	}
	if err := validateHero(`<p style="made-up-property: 1">hi</p>`); err == nil || !strings.Contains(err.Error(), `css property "made-up-property" is not allowed`) {
		t.Errorf("a property outside the list is refused by name: %v", err)
	}
}

// What real pages used and the list refused (docs/design-format.md, "The width
// of the allow-list") is accepted, and what it could be abused for still is not.
func TestThePropertiesRealPagesUsedAreAcceptedAndTheOnesThatBindBehaviourAreNot(t *testing.T) {
	for _, accepted := range []string{
		`scroll-margin-top:80px`, `scroll-padding-top:80px`, `scroll-padding-block:4px`, `overscroll-behavior:contain`, `overscroll-behavior-x:none`,
		`scrollbar-width:thin`, `scrollbar-color:#888 #eee`, `break-inside:avoid`, `page-break-inside:avoid`, `grid-gap:8px`, `rx:4px`,
	} {
		if err := validateHero(`<div style="` + accepted + `">x</div>`); err != nil {
			t.Errorf("%s is refused: %v", accepted, err)
		}
	}
	for _, accepted := range []string{`<dialog open>hi</dialog>`, `<style>@page { margin: 0 }</style><p>x</p>`, `<style>@property --x { syntax: "<length>"; inherits: false; initial-value: 0px }</style><p>x</p>`} {
		if err := validateHero(accepted); err != nil {
			t.Errorf("%s is refused: %v", accepted, err)
		}
	}
	for _, refused := range []string{`<div style="behavior:url(#x)">x</div>`, `<div style="-moz-binding:url(#x)">x</div>`, `<template>x</template>`, `<style>@import "x.css";</style>`} {
		if err := validateHero(refused); err == nil {
			t.Errorf("%s is accepted", refused)
		}
	}
}
