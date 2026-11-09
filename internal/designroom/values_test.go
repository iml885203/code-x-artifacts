package designroom_test

import (
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
)

func TestTheValuesThePanelWritesAndFramesUseAreAccepted(t *testing.T) {
	t.Parallel()
	for prop, values := range map[string][]string{
		"width":            {"120px", "100%", "fit-content", "auto", " 10px", "calc(100% - 20px)", "50vw", "0", "-8.5px", "clamp(1rem, 2vw, 3rem)", "100px !important", "min-content"},
		"padding":          {"8px 12px", "0 0 0 env(safe-area-inset-left)", "1px 2px 3px 4px", "var(--gap)"},
		"border-radius":    {"50%", "8px", "4px 8px 4px 8px", "9999px"},
		"gap":              {"12px", "normal", "1rem 2rem"},
		"line-height":      {"1.5", "24px", "normal"},
		"letter-spacing":   {"0.5px", "normal", "-0.02em"},
		"font-size":        {"44px", "clamp(1rem, 4vw, 3rem)", "inherit"},
		"opacity":          {"0.5", "1", "0", "50%"},
		"flex-grow":        {"1", "0"},
		"font-weight":      {"700", "bold", "400"},
		"position":         {"absolute", "relative", "static"},
		"display":          {"none", "flex", "revert", "inline-flex"},
		"flex-direction":   {"row", "column"},
		"align-items":      {"flex-start", "center", "stretch"},
		"justify-content":  {"space-between", "flex-end"},
		"text-align":       {"center", "left"},
		"font-style":       {"italic", "normal"},
		"border-style":     {"solid", "dashed none"},
		"color":            {"#112233", "#fa0", "#11223344", "rgba(1, 2, 3, 0.5)", "rgb(1 2 3 / 50%)", "var(--ink)", "currentcolor", "transparent", "red", "color-mix(in srgb, red 50%, blue)", "hsl(200 50% 40%)"},
		"background-color": {"#336699", "rgba(51, 102, 153, 0.5)", "transparent"},
		"border-color":     {"#ff0000", "red blue"},
		"box-shadow":       {"none", "0px 4px 8px rgba(0, 0, 0, 0.25)", "0 0 0 1px var(--line), 0 2px 6px #0003", "inset 0 1px 0 #fff"},
		"font-family":      {`Inter, sans-serif`, `"Helvetica Neue", Arial`, `'Noto Sans TC', "微軟正黑體"`, "var(--font)", "system-ui"},
	} {
		for _, v := range values {
			if err := designroom.ValidateStyleValue(prop, v); err != nil {
				t.Errorf("%s: %q was refused: %v", prop, v, err)
			}
		}
	}
	for _, prop := range []string{"width", "color", "font-family", "box-shadow", "display", "padding"} {
		for _, empty := range []string{"", "   "} {
			if err := designroom.ValidateStyleValue(prop, empty); err != nil {
				t.Errorf("%s: taking the declaration away was refused: %v", prop, err)
			}
		}
	}
}

func TestAValueThatCouldEndTheDeclarationOrTheRuleOrLoadSomethingIsRefused(t *testing.T) {
	t.Parallel()
	for prop, values := range map[string][]string{
		"color":            {"red; background: url(x)", "red}", "{red", "red\\", "</style>", "url(https://x.test/a.png)", "expression(alert(1))", "red /* x */", "@import", "javascript:x", "red\nblue", "rgb(1,2,3", "rgb(1,2,3))"},
		"width":            {"10px; height: 1px", "wide!", "calc(1px + url(x))", "calc(1px;2px)", "10px 20px 30px 40px 50px 60px", "1e3px"},
		"font-family":      {"Arial; color: red", `"Arial`, `Ari"al`, "a,,b", "Arial, ", `url("x")`, "Arial{"},
		"box-shadow":       {"0 0 1px red; x", "0 0 1px url(x)", "0 0 1px <red>"},
		"background-color": {"url(x.png)", "image-set(x 1x)", "#12", "#gggggg", "red green blue black white"},
		"opacity":          {"half", "1px", "0.5; x"},
		"position":         {"sticky;", "middle", "absolute fixed relative static sticky"},
		"display":          {"flex; x", "none !import"},
		"font-weight":      {"heavy", "7 0 0"},
		"padding":          {"1px 2px 3px 4px 5px 6px"},
		"flex-direction":   {"diagonal"},
	} {
		for _, v := range values {
			if err := designroom.ValidateStyleValue(prop, v); err == nil {
				t.Errorf("%s: %q was accepted", prop, v)
			}
		}
	}
	if err := designroom.ValidateStyleValue("width", strings.Repeat("1", 2000)); err == nil {
		t.Error("a value of two thousand bytes was accepted")
	}
}

func TestOnlyThePropertiesTheEditorOwnsHaveValues(t *testing.T) {
	t.Parallel()
	for _, prop := range []string{"background", "transform", "z-index", "content", "--x", "behavior", "", "Color"} {
		if err := designroom.ValidateStyleValue(prop, "1px"); err == nil {
			t.Errorf("a value for %q was accepted", prop)
		}
	}
}

// What a person writes straight into the room's map is held to the same rules
// as what SetStyle writes: the hub runs Check before it keeps an update.
func TestARoomHoldingABadStyleOrVisibilityEntryIsRefusedByCheck(t *testing.T) {
	t.Parallel()
	for name, entry := range map[string][2]string{
		"a declaration that ends":      {"s|color", "red;position:fixed"},
		"an address":                   {"s|background-color", "url(https://x.test/a.png)"},
		"a property that is not owned": {"s|transform", "none"},
		"a picture from elsewhere":     {"s|background-image", "url(https://x.test/a.png)"},
		"a font that breaks the rule":  {"s|font-family", "a}body{display:none"},
		"a visibility that is not one": {"hidden", "yes"},
	} {
		doc := peer(t, 1, seeded(t, propertiesDesign(t)))
		id := pidOf(t, read(t, doc).Frames[heroFile], `id="card"`)
		setEntry(doc, heroFile+"|n|"+id+"|"+entry[0], entry[1])
		if err := designroom.Check(doc); err == nil {
			t.Errorf("%s: the room was accepted", name)
		}
	}
	doc := peer(t, 1, seeded(t, propertiesDesign(t)))
	if err := designroom.Check(doc); err != nil {
		t.Errorf("a frame with its own declarations was refused: %v", err)
	}
}

func TestSetStyleRefusesABadValueAndWritesNothing(t *testing.T) {
	t.Parallel()
	doc := peer(t, 1, seeded(t, propertiesDesign(t)))
	before := read(t, doc).Frames[heroFile]
	id := pidOf(t, before, `id="card"`)
	for _, v := range []string{"red;left:0", "url(x)"} {
		if _, err := designroom.Open(doc).SetStyle(heroFile, id, "color", v); err == nil {
			t.Errorf("%q was accepted", v)
		}
	}
	if got := read(t, doc).Frames[heroFile]; got != before {
		t.Error("the frame changed")
	}
}
