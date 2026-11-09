package designsource

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestLogicRequiresARealClassASTAndDeclaredInstanceEvents(t *testing.T) {
	valid := `// retained comment
class Frame extends PictorLogic { renderVals() { return {text:"class Fake extends Wrong {}"}; } pick(e) { const x = [1,2].map(n => n*2); return x; } }`
	c, e := ValidateClassicClass(valid, "Frame", "PictorLogic")
	if e != nil || strings.Join(c.Methods, ",") != "pick,renderVals" {
		t.Fatalf("%v %v", c, e)
	}
	for _, source := range []string{
		`class Fake extends PictorLogic {}`,
		`class Frame extends Wrong {}`,
		`class Frame extends PictorLogic {} alert(1)`,
		`const Frame = class extends PictorLogic {}`,
		`export class Frame extends PictorLogic {}`,
		`class Frame extends PictorLogic { static pick() {} }`,
		`class Frame extends PictorLogic { ["pick"]() {} }`,
		`class Frame extends PictorLogic { get pick() {} }`,
		`class Frame extends PictorLogic { pick() {} pick() {} }`,
		`class Frame extends PictorLogic { pick() { return document.body; } }`,
		`class Frame extends PictorLogic { pick() { return fetch("/"); } }`,
		`class Frame extends PictorLogic { pick() { return eval("1"); } }`,
		`class Frame extends PictorLogic { pick() { return new Function("return 1"); } }`,
		`class Frame extends PictorLogic { pick() { return ({}).constructor; } }`,
	} {
		if _, e := ValidateClassicClass(source, "Frame", "PictorLogic"); e == nil {
			t.Fatalf("accepted %s", source)
		}
	}
}

func TestImporterClassHeaderSpansPreserveBodiesAndExposeRenameRisks(t *testing.T) {
	source := `/* Component in comment */ class /* comment */ Component extends DCLogic { renderVals(){ return {text:"Component/DCLogic"}; } }`
	c, e := ValidateClassicClass(source, "Component", "DCLogic")
	if e != nil {
		t.Fatal(e)
	}
	if source[c.ClassNameSpan.StartByte:c.ClassNameSpan.EndByte] != "Component" || source[c.BaseNameSpan.StartByte:c.BaseNameSpan.EndByte] != "DCLogic" || c.ReferencesClass || c.ReferencesBase {
		t.Fatalf("header contract: %#v", c)
	}
	risky, e := ValidateClassicClass(`class Component extends DCLogic { value(){ return Component; } other(){ return DCLogic; } }`, "Component", "DCLogic")
	if e != nil || !risky.ReferencesClass || !risky.ReferencesBase {
		t.Fatalf("rename risks: %#v %v", risky, e)
	}
}

func TestUnsupportedLogicAndExcessiveNestingAreRefused(t *testing.T) {
	for _, source := range []string{
		`class Frame extends PictorLogic { pick(){ return import("/bad.js"); } }`,
		`class Frame extends PictorLogic { pick(){ return ({})["constructor"]; } }`,
		`class Frame extends PictorLogic { pick(){ return ({})['constr\u0075ctor']; } }`,
		`class Frame extends PictorLogic { #hidden(){} }`,
		`class Frame extends PictorLogic { data = 1; }`,
		`class Frame extends PictorLogic { static { throw 1; } }`,
		`class Frame extends PictorLogic { pick(){ with({}) {} } }`,
		`class Frame extends PictorLogic { pick(){ return ` + strings.Repeat("[", 150) + `0` + strings.Repeat("]", 150) + `; } }`,
	} {
		if _, e := ValidateClassicClass(source, "Frame", "PictorLogic"); e == nil {
			t.Fatalf("accepted unsupported logic %s", source)
		}
	}
	for _, source := range []string{`<button p-on:click="{{pick}}"></button>`, `<script type="text/x-pictor">class Frame extends PictorLogic { other() {} }</script><button p-on:click="{{pick}}"></button>`} {
		if _, e := ParseFrame("frames/a.html", source); e == nil {
			t.Fatalf("accepted undeclared event method: %s", source)
		}
	}
}

func TestLogicHashUsesExactOriginalCRLFBytes(t *testing.T) {
	logic := "class Frame extends PictorLogic {\r\n renderVals() { return {text:'Hello'}; }\r\n}"
	source := `<script type="text/x-pictor">` + logic + `</script><p data-pid="p">Hello</p>`
	p, e := ParseFrame("frames/a.html", source)
	if e != nil {
		t.Fatal(e)
	}
	if p.Logic.Source != logic || p.Logic.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(logic))) {
		t.Fatal("logic source or digest normalized line endings")
	}
	if p.Rewritten != source {
		t.Fatal("source bytes changed")
	}
}
