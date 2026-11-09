package designsource

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

func bundle(files map[string]string) ParseInput {
	var frames []string
	for _, f := range sortedKeys(files) {
		id := strings.TrimSuffix(strings.TrimPrefix(f, "frames/"), ".html")
		frames = append(frames, fmt.Sprintf(`{"id":%q,"name":%q,"file":%q,"w":400,"h":300}`, id, id, f))
	}
	return ParseInput{"2", `{"version":2,"runtime":"pictor-design/2.0.0","pages":[{"id":"page","name":"Page","rows":[[` + strings.Join(frames, ",") + `]]}]}`, files}
}

func TestBundleChecksVersionOwnershipImportsAndStableDiagnostics(t *testing.T) {
	input := bundle(map[string]string{"frames/a.html": `<p-import src="frames/b.html"></p-import>`, "frames/b.html": `<p>Child</p>`})
	p, e := ParseBundle(input)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Dependencies["frames/a.html"]) != 1 || len(p.Sources) != 2 || p.Manifest != input.Manifest {
		t.Fatalf("bad bundle: %#v", p)
	}
	for _, change := range []func(*ParseInput){
		func(i *ParseInput) { i.Format = "1" },
		func(i *ParseInput) { i.Manifest = strings.Replace(i.Manifest, `"version":2`, `"version":1`, 1) },
		func(i *ParseInput) { i.Manifest = strings.Replace(i.Manifest, RuntimeID, "pictor-design/9.0.0", 1) },
		func(i *ParseInput) { delete(i.Files, "frames/b.html") },
		func(i *ParseInput) { i.Files["frames/extra.html"] = "<p>extra</p>" },
		func(i *ParseInput) { i.Files["frames/b.html"] = `<p-import src="frames/a.html"></p-import>` },
	} {
		i := bundle(map[string]string{"frames/a.html": input.Files["frames/a.html"], "frames/b.html": input.Files["frames/b.html"]})
		change(&i)
		a, e := ParseBundle(i)
		if e == nil {
			t.Fatal("invalid bundle accepted")
		}
		b, again := ParseBundle(i)
		if again == nil || fmt.Sprint(a.Diagnostics) != fmt.Sprint(b.Diagnostics) {
			t.Fatal("unstable diagnostics")
		}
	}
}

func TestLogicSourceIsInertAndSyntaxDoesNotAdmitExecution(t *testing.T) {
	source := `class Frame extends PictorLogic { render() { return {title:"Hello"}; } }`
	p, e := ParseFrame("frames/a.html", `<script type="text/x-pictor">`+source+`</script><p>{{title}}</p>`)
	if e != nil {
		t.Fatal(e)
	}
	if p.Logic == nil || !p.Logic.SyntaxChecked || p.Logic.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(source))) {
		t.Fatalf("logic: %#v", p.Logic)
	}
	if e := p.RequireExecutable(); e == nil {
		t.Fatal("syntax-only source was admitted for execution")
	}
	for _, s := range []string{`<script type="text/x-pictor">class {</script>`, `<script type="text/x-pictor" src="a.js"></script>`, `<script type="text/x-pictor"></script><script type="text/x-pictor"></script>`, `<script type="application/x-pictor-props+json">{}</script>`} {
		if _, e := ParseFrame("frames/a.html", s); e == nil {
			t.Fatalf("accepted invalid logic/props: %s", s)
		}
	}
}

func TestFormatOneRemainsStaticAndItsBytesAreUntouched(t *testing.T) {
	source, e := os.ReadFile("testdata/static-v1.html")
	if e != nil {
		t.Fatal(e)
	}
	manifest, e := os.ReadFile("testdata/static-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	frozen, e := os.ReadFile("testdata/static-v1.normalized.html")
	if e != nil {
		t.Fatal(e)
	}
	files := map[string]string{"frames/static.html": string(source)}
	if e := designfmt.Validate(string(manifest), files); e != nil {
		t.Fatal(e)
	}
	normalized, e := designfmt.Normalize(string(source))
	if e != nil || normalized != string(frozen) {
		t.Fatalf("static drawing contract changed: %v", e)
	}
	if _, e := ParseBundle(ParseInput{"1", string(manifest), files}); e == nil {
		t.Fatal("format 1 was admitted by format 2 parser")
	}
	if e := designfmt.Validate(string(manifest), map[string]string{"frames/static.html": `<p-if value="{{x}}"></p-if>`}); e == nil {
		t.Fatal("static format accepted runnable directives")
	}
	if _, e := ParseFrame("frames/static.html", string(source)); e != nil {
		t.Fatalf("shared token/kit/SVG/pen source rules diverged: %v", e)
	}
}

func TestRepresentativeSourceFixtureHasOneAuthoritativePlan(t *testing.T) {
	manifest, e := os.ReadFile("testdata/design.json")
	if e != nil {
		t.Fatal(e)
	}
	source, e := os.ReadFile("testdata/source.html")
	if e != nil {
		t.Fatal(e)
	}
	plan, e := ParseBundle(ParseInput{"2", string(manifest), map[string]string{"frames/source.html": string(source)}})
	if e != nil {
		t.Fatal(e)
	}
	frame := plan.Frames["frames/source.html"]
	if frame.Rewritten != string(source) || frame.Logic == nil || frame.Props == nil || len(plan.Sources) != 11 {
		t.Fatalf("fixture plan lost source: %#v", frame)
	}
}

func TestImportPropsNavigationAndDependencyDepthStayInOneSnapshot(t *testing.T) {
	input := bundle(map[string]string{"frames/a.html": `<p-import src="frames/b.html" props="{{props.child}}"></p-import><a href="b.html">Next</a>`, "frames/b.html": `<p>child</p>`})
	p, e := ParseBundle(input)
	if e != nil {
		t.Fatal(e)
	}
	if p.Frames["frames/a.html"].Nodes[0].ImportProps != "props.child" || p.Frames["frames/a.html"].Nodes[1].Navigation != "frames/b.html" {
		t.Fatal("lost import props/navigation")
	}
	input.Files["frames/a.html"] = `<a href="missing.html">Missing</a>`
	if _, e := ParseBundle(input); e == nil {
		t.Fatal("accepted missing navigation target")
	}
	files := map[string]string{}
	for i := 0; i < 10; i++ {
		source := "<p>end</p>"
		if i < 9 {
			source = fmt.Sprintf(`<p-import src="frames/f%d.html"></p-import>`, i+1)
		}
		files[fmt.Sprintf("frames/f%d.html", i)] = source
	}
	if _, e := ParseBundle(bundle(files)); e == nil {
		t.Fatal("accepted import depth >8")
	}
	for _, manifest := range []string{"null", `{"version":2,"version":2}`, strings.Repeat("x", (256<<10)+1)} {
		if _, e := ParseBundle(ParseInput{"2", manifest, nil}); e == nil {
			t.Fatal("accepted malformed manifest")
		}
	}
}

func TestRendererJSONFixtureIsTheExactServerFramePlan(t *testing.T) {
	manifest, e := os.ReadFile("testdata/design.json")
	if e != nil {
		t.Fatal(e)
	}
	source, e := os.ReadFile("testdata/source.html")
	if e != nil {
		t.Fatal(e)
	}
	bundle, e := ParseBundle(ParseInput{"2", string(manifest), map[string]string{"frames/source.html": string(source)}})
	if e != nil {
		t.Fatal(e)
	}
	encoded, e := json.MarshalIndent(bundle.Frames["frames/source.html"], "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	fixture, e := os.ReadFile("testdata/source.plan.json")
	if e != nil {
		t.Fatal(e)
	}
	if string(encoded)+"\n" != string(fixture) {
		t.Fatal("renderer IR fixture differs from authoritative server plan")
	}
}
