package designroom_test

import (
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/reearth/ygo/crdt"
)

func TestWrapAndBorderSidesAcceptSingleValuesAndRefuseUnsafeOrMultipleValues(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ good, bad []string }{
		"flex-wrap": {[]string{"nowrap", "wrap", "wrap-reverse", "inherit", "revert-layer", "WRAP !important", ""}, []string{"wrap nowrap", "diagonal", "wrap; color:red", "url(x)", "wrap\nnowrap"}},
	}
	for _, side := range []string{"top", "right", "bottom", "left"} {
		cases["border-"+side+"-width"] = struct{ good, bad []string }{[]string{"0", "2px", "thin", "medium", "thick", "var(--edge)", "calc(1px + 2px)", "inherit", ""}, []string{"1px 2px", "auto", "banana", "/", "url(x)", "1px; color:red", "2px}"}}
		cases["border-"+side+"-style"] = struct{ good, bad []string }{[]string{"none", "hidden", "solid", "dashed", "double", "groove", "ridge", "inset", "outset", "dotted", "unset", ""}, []string{"solid dashed", "banana", "solid/*x*/", "solid; color:red"}}
		cases["border-"+side+"-color"] = struct{ good, bad []string }{[]string{"#1234", "red", "currentcolor", "transparent", "rgb(1 2 3 / 50%)", "var(--ink)", "revert", ""}, []string{"red blue", "#12", "url(x)", "red; color:blue", "red}"}}
	}
	for prop, c := range cases {
		for _, value := range c.good {
			if err := designroom.ValidateStyleValue(prop, value); err != nil {
				t.Errorf("%s=%q refused: %v", prop, value, err)
			}
		}
		for _, value := range c.bad {
			if err := designroom.ValidateStyleValue(prop, value); err == nil {
				t.Errorf("%s=%q accepted", prop, value)
			}
		}
	}
}

func TestWrapAndBorderSideEditsPreserveOtherDeclarationsAndUnsupportedAuthoredValues(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ prop, before, after string }{
		{"flex-wrap", "wrap", "nowrap"},
		{"border-top-width", "2px", "3px"},
		{"border-right-style", "dotted", "solid"},
		{"border-bottom-color", "red", "blue"},
		{"border-left-width", "4px", "5px"},
		{"border-left-color", "", "purple"},
	} {
		t.Run(c.prop, func(t *testing.T) {
			frame := normal(t, `<html><head><style>.card{background:red}</style></head><body><div data-pid="card" style="border:1px solid black;border-top-width:2px;border-right-style:dotted;border-bottom-color:red;border-left-width:4px;flex-wrap:wrap;transform:rotate(2deg)"><span>Keep</span></div></body></html>`)
			doc := peer(t, 1, seeded(t, designroom.Design{Manifest: manifest, Frames: map[string]string{heroFile: frame}}))
			before := read(t, doc).Frames[heroFile]
			edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
				return r.SetStyle(heroFile, "card", c.prop, c.after)
			})
			want := strings.Replace(before, c.prop+":"+c.before, c.prop+":"+c.after, 1)
			if c.before == "" {
				want = strings.Replace(before, "transform:rotate(2deg)", "transform:rotate(2deg);"+c.prop+":"+c.after, 1)
			}
			if got := read(t, doc).Frames[heroFile]; got != want {
				t.Fatalf("want %s\ngot %s", want, got)
			}
		})
	}
	frame := normal(t, `<html><body><div data-pid="card" style="flex-wrap:var(--wrap);border-top-style:var(--line);color:red">Keep</div></body></html>`)
	doc := peer(t, 1, seeded(t, designroom.Design{Manifest: manifest, Frames: map[string]string{heroFile: frame}}))
	if err := designroom.Check(doc); err != nil {
		t.Fatalf("unsupported authored values must remain readable: %v", err)
	}
	if got := read(t, doc).Frames[heroFile]; got != frame {
		t.Fatalf("authored values changed: %s", got)
	}
}

func TestNoFillLonghandOverridesAndResetRevealAuthoredBackgroundWithoutNormalizingIt(t *testing.T) {
	t.Parallel()
	frame := normal(t, `<html><head><style>.card{background:blue}</style></head><body><div class="card" data-pid="card" style="background:linear-gradient(red,blue);transform:rotate(2deg)">Keep</div></body></html>`)
	doc := peer(t, 1, seeded(t, designroom.Design{Manifest: manifest, Frames: map[string]string{heroFile: frame}}))
	for prop, value := range map[string]string{"background-color": "transparent", "background-image": "none"} {
		edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
			return r.SetStyle(heroFile, "card", prop, value)
		})
	}
	want := strings.Replace(frame, "transform:rotate(2deg)", "transform:rotate(2deg);background-color:transparent;background-image:none", 1)
	if got := read(t, doc).Frames[heroFile]; got != want {
		t.Fatalf("No fill overrides must preserve authored shorthand: %s", got)
	}
	for _, prop := range []string{"background-color", "background-image"} {
		edit(t, doc, func(r *designroom.Room) (func(*crdt.Transaction), error) {
			return r.SetStyle(heroFile, "card", prop, "")
		})
	}
	if got := read(t, doc).Frames[heroFile]; got != frame {
		t.Fatalf("reset must remove overrides and retain authored shorthand: %s", got)
	}
}

func TestInvalidWrapAndBorderRoomEntriesAreRefusedWithoutChangingSource(t *testing.T) {
	t.Parallel()
	for prop, value := range map[string]string{"flex-wrap": "wrap nowrap", "border-top-width": "auto", "border-right-style": "solid dashed", "border-bottom-color": "red blue", "border-left-color": "url(x)"} {
		doc := peer(t, 1, seeded(t, propertiesDesign(t)))
		before := read(t, doc).Frames[heroFile]
		id := pidOf(t, before, `id="card"`)
		if _, err := designroom.Open(doc).SetStyle(heroFile, id, prop, value); err == nil {
			t.Fatalf("%s accepted bad value", prop)
		}
		if got := read(t, doc).Frames[heroFile]; got != before {
			t.Fatalf("%s refusal changed source", prop)
		}
		setEntry(doc, heroFile+"|n|"+id+"|s|"+prop, value)
		if err := designroom.Check(doc); err == nil {
			t.Fatalf("raw %s room entry accepted", prop)
		}
	}
}
