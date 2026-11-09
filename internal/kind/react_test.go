package kind

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/jsruntime"
)

func mustFailToCompile(t *testing.T, src string, want ...string) {
	t.Helper()
	err := React{}.Validate(src, CurrentFormat(React{}))
	if err == nil {
		t.Fatalf("source compiled, want an error mentioning %q", want)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("the error does not mention %q:\n%v", w, err)
		}
	}
}

func renderOK(t *testing.T, src string) Page {
	t.Helper()
	p, err := React{}.RenderPage(src, CurrentFormat(React{}))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReactCompileFailuresTellTheAgentWhatToFix(t *testing.T) {
	for name, tc := range map[string]struct {
		src  string
		want []string
	}{
		"syntax error names the line and column": {
			"export default function App() {\n  return <div>\n}\n",
			[]string{"does not compile", "line 3:"},
		},
		"unknown package lists what the server provides": {
			`import _ from "lodash"; export default () => <p>{_.toString(1)}</p>`,
			[]string{`cannot import "lodash"`, "recharts", "lucide-react", "@/components/ui/button"},
		},
		"a relative import with no files to import says to publish the file": {
			`import x from "./other"; export default () => <p>{x}</p>`,
			[]string{`cannot import "./other"`, "no files", "in files"},
		},
		"a URL import is refused and says what an import may be": {
			`import x from "https://esm.sh/x"; export default () => <p>{x}</p>`,
			[]string{`cannot import "https://esm.sh/x"`, "relative path", "recharts"},
		},
		"an icon that does not exist names the package and the name": {
			`import { Fire } from "lucide-react"; export default () => <Fire />`,
			[]string{`"lucide-react"`, `"Fire"`, "line 1:"},
		},
		"a component that does not exist names the package and the name": {
			`import { Gauge } from "@/components/ui/progress"; export default () => <Gauge />`,
			[]string{`"@/components/ui/progress"`, `"Gauge"`},
		},
		"no default export says what to write": {
			`export function App() { return <p/> }`,
			[]string{"export default function App()"},
		},
		"every problem is reported, not only the first": {
			`import { Fire } from "lucide-react";
import { Nope } from "recharts";
export default () => <><Fire /><Nope /></>;
`,
			[]string{"2 problem(s)", `"Fire"`, `"Nope"`},
		},
	} {
		t.Run(name, func(t *testing.T) { mustFailToCompile(t, tc.src, tc.want...) })
	}
}

func TestReactRefusesEmptyAndOversizedSource(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"empty":      {"", "needs content"},
		"blank":      {" \n\t", "needs content"},
		"oversized":  {"//" + strings.Repeat("a", MaxReactBytes), "at most 1 MB"},
		"exactly at": {"export default () => <p/>;" + strings.Repeat(" ", MaxReactBytes-len("export default () => <p/>;")), ""},
	} {
		err := React{}.Validate(tc.src, CurrentFormat(React{}))
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: refused: %v", name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: error %v, want one mentioning %q", name, err, tc.want)
		}
	}
}

func TestReactStripsTypesAndDoesNotCheckThem(t *testing.T) {
	src := `import { useState } from "react"; type P = { n: number };
export default function App({ n }: P) { const [x] = useState<string>(1 as any); return <p>{n}{x}</p> }`
	p := renderOK(t, src)
	if strings.Contains(p.InlineScripts[2], "type P") || strings.Contains(p.InlineScripts[2], "as any") {
		t.Errorf("types survived compilation: %s", p.InlineScripts[2])
	}
}

func TestReactAScriptCloserInTheSourceCannotEndThePagesScript(t *testing.T) {
	for name, src := range map[string]string{
		"string":     `export default () => <p>{"</script><script>alert(1)</script>"}</p>`,
		"upper case": `export default () => <p>{"</SCRIPT><script>alert(1)</script>"}</p>`,
		"jsx text":   "export default () => <p>{`</script>`}</p>",
		"comment":    `export default () => <p>{"<!--<script>"}</p>`,
	} {
		p := renderOK(t, src)
		for i, s := range p.InlineScripts {
			if low := strings.ToLower(s); strings.Contains(low, "</script") || strings.Contains(s, "<!--") {
				t.Errorf("%s: inline script %d can end or confuse the script element: %.200s", name, i, s)
			}
		}
	}
}

func TestReactTheSampleCompilesAndRuntimeModulesStayImports(t *testing.T) {
	src, err := os.ReadFile("testdata/sprint.tsx")
	if err != nil {
		t.Fatal(err)
	}
	p := renderOK(t, string(src))
	js := p.InlineScripts[2]
	if strings.Contains(js, "require(") {
		t.Error("the compiled component calls require, which a module in the browser cannot")
	}
	for _, spec := range []string{"recharts", "lucide-react", "@/components/ui/tabs", "react/jsx-runtime", "react-dom/client"} {
		if !strings.Contains(js, `"`+spec+`"`) {
			t.Errorf("the component's script does not import %q; the import map must resolve it", spec)
		}
	}
}

func TestReactThePageMapsEveryImportToTheRuntimeItWasPublishedWith(t *testing.T) {
	p := renderOK(t, `export default () => <p>hi</p>`)
	rt, err := jsruntime.Load(jsruntime.V1)
	if err != nil {
		t.Fatal(err)
	}
	var im struct {
		Imports map[string]string `json:"imports"`
	}
	if err := json.Unmarshal([]byte(p.InlineScripts[0]), &im); err != nil {
		t.Fatalf("the import map is not JSON: %v", err)
	}
	if len(im.Imports) != len(rt.Manifest.Specifiers) {
		t.Errorf("import map has %d entries, runtime v1 has %d specifiers", len(im.Imports), len(rt.Manifest.Specifiers))
	}
	for spec, m := range rt.Manifest.Specifiers {
		if got, want := im.Imports[spec], "/vendor/v1/"+m.File; got != want {
			t.Errorf("%s maps to %q, want %q", spec, got, want)
		}
	}
	tailwind := regexp.MustCompile(`<script src="(/vendor/v1/tailwind-browser-[0-9a-f]+\.js)"></script>`).FindStringSubmatch(p.HTML)
	if tailwind == nil || !strings.HasSuffix(tailwind[1], rt.Manifest.Scripts["tailwind"]) {
		t.Errorf("the page does not load the runtime's Tailwind script %q", rt.Manifest.Scripts["tailwind"])
	}
	if !strings.Contains(p.HTML, `@import url("/kit/v5/pictor.css") layer(kit)`) || !strings.Contains(p.HTML, "@theme inline") {
		t.Error("the page does not link the kit in its layer and map the theme")
	}
}

func TestReactDeclaresExactlyTheInlineScriptsOfThePage(t *testing.T) {
	p := renderOK(t, `export default () => <p>hi</p>`)
	for _, s := range p.InlineScripts {
		if !strings.Contains(p.HTML, ">"+s+"</script>") {
			t.Errorf("declared inline script is not in the page verbatim: %.80s", s)
		}
	}
	// The HTML parser sees as many inline scripts as are declared: the import map,
	// the error script and the module. The Tailwind script has a src, no body.
	inline := regexp.MustCompile(`(?s)<script(?: type="[a-z]+")?>`).FindAllString(p.HTML, -1)
	if len(inline) != len(p.InlineScripts) {
		t.Errorf("the page has %d inline scripts, the kind declares %d", len(inline), len(p.InlineScripts))
	}
}

func TestReactRenderIsMemoisedByContentAndRuntime(t *testing.T) {
	src := "export default () => <p>memo " + t.Name() + "</p>"
	before := reactCompiles.Load()
	if err := (React{}).Validate(src, "v1"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := (React{}).Render(src, "v1"); err != nil {
			t.Fatal(err)
		}
	}
	if got := reactCompiles.Load() - before; got != 1 {
		t.Errorf("publish and three views compiled %d times, want once", got)
	}
	if _, err := (React{}).Render(src+" ", "v1"); err != nil {
		t.Fatal(err)
	}
	if got := reactCompiles.Load() - before; got != 2 {
		t.Errorf("different content did not compile: %d compiles", got)
	}
}

func TestReactAFormatThisServerLacksIsRefused(t *testing.T) {
	if err := (React{}).Validate("export default () => <p/>", "v9"); err == nil || !strings.Contains(err.Error(), `"v9"`) || !strings.Contains(err.Error(), "v1") {
		t.Errorf("error %v, want one naming the format asked for and the ones available", err)
	}
	if _, err := (React{}).Render("export default () => <p/>", ""); err == nil {
		t.Error("an empty format rendered")
	}
}

func TestReactEveryFormatVersionIsARuntimeThisBinaryCarries(t *testing.T) {
	for _, v := range (React{}).FormatVersions() {
		if _, err := jsruntime.Load(jsruntime.Version(v)); err != nil {
			t.Errorf("format %s has no embedded runtime: %v", v, err)
		}
		if _, err := (React{}).Render("export default () => <p/>", v); err != nil {
			t.Errorf("format %s does not render: %v", v, err)
		}
	}
}

func TestReactIsRegisteredAndTeachesTheRules(t *testing.T) {
	k, err := Lookup("react")
	if err != nil {
		t.Fatal(err)
	}
	g := k.Guide()
	for _, must := range []string{"export default", "@/components/ui/dialog", "recharts", "lucide-react", "window.pictor", "localStorage", "var(--chart-1)", "bg-background", "private"} {
		if !strings.Contains(g, must) {
			t.Errorf("the react guide never mentions %q", must)
		}
	}
	if _, ok := k.(PageRenderer); !ok {
		t.Error("react does not declare its inline scripts, so /raw cannot pin script-src for it")
	}
}

// reactFiles are the siblings of a component that imports every kind of file.
func reactFiles() files.Set {
	return files.Set{
		"Chart.tsx":      `import { fmt } from "./lib/format"; import rows from "./data/rows.json"; export default function Chart() { return <p>{fmt(rows.length)}</p>; }`,
		"lib/format.ts":  `import { clamp } from "../util.js"; export const fmt = (n: number): string => "n=" + clamp(n);`,
		"util.js":        `export const clamp = (n) => Math.min(n, 99);`,
		"data/rows.json": `[1, 2, 3]`,
		"styles.css":     `.chart { color: rgb(1, 2, 3); background: url(../img/dot.svg); }`,
	}
}

const reactWithSiblings = `import Chart from "./Chart"; import "./styles.css"; export default function App() { return <div className="chart"><Chart /></div>; }`

func TestAReactArtifactImportsItsSiblingFilesAndTheyAreBundledIntoOneScript(t *testing.T) {
	format := CurrentFormat(React{})
	if err := (React{}).ValidateFiles(reactWithSiblings, format, reactFiles()); err != nil {
		t.Fatalf("a component with its siblings was refused: %v", err)
	}
	p, err := React{}.RenderPageFiles(reactWithSiblings, format, reactFiles())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.InlineScripts) != 3 {
		t.Fatalf("the page has %d inline scripts, want the import map, the error script and one bundle", len(p.InlineScripts))
	}
	bundle := p.InlineScripts[2]
	for _, want := range []string{`"n="`, "Math.min", "[1,2,3]"} {
		if !strings.Contains(bundle, want) {
			t.Errorf("the bundle lacks %q, so a sibling was not compiled into it: %.300s", want, bundle)
		}
	}
	if strings.Contains(bundle, "from\"./") || strings.Contains(bundle, `from "./`) || strings.Contains(bundle, "import(") {
		t.Errorf("the bundle still imports a sibling by path, which the page could not resolve: %.300s", bundle)
	}
	// Without the files the same component does not compile: there is nothing to import.
	if err := (React{}).Validate(reactWithSiblings, format); err == nil {
		t.Error("the component compiled with no files")
	}
}

func TestAReactImportResolvesFromTheImportingFilesFolderAndTriesTheExtensions(t *testing.T) {
	fs := files.Set{
		"ui/Panel.tsx":  `import { x } from "../lib/x"; import Row from "./Row.jsx"; export default () => <Row n={x} />;`,
		"lib/x.mjs":     `export const x = 1;`,
		"ui/Row.jsx":    `export default ({ n }) => <p>{n}</p>;`,
		"ui/index.json": `{}`,
	}
	src := `import Panel from "./ui/Panel"; export default () => <Panel />;`
	if err := (React{}).ValidateFiles(src, CurrentFormat(React{}), fs); err != nil {
		t.Errorf("imports from a nested folder, by extension, with and without it: %v", err)
	}
}

func TestAReactImportOfAMissingFileNamesTheFilesThatExist(t *testing.T) {
	err := React{}.ValidateFiles(`import Nope from "./Nope"; export default () => <Nope />;`, CurrentFormat(React{}), reactFiles())
	if err == nil {
		t.Fatal("an import of a file that is not there compiled")
	}
	for _, want := range []string{`cannot import "./Nope"`, "Chart.tsx", "lib/format.ts", "data/rows.json", "styles.css", "line 1:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q:\n%v", want, err)
		}
	}
	// A problem inside a sibling names that file's own line.
	bad := files.Set{"Bad.tsx": "export default function Bad() {\n  return <p>\n}\n"}
	err = React{}.ValidateFiles(`import Bad from "./Bad"; export default () => <Bad />;`, CurrentFormat(React{}), bad)
	if err == nil || !strings.Contains(err.Error(), "line 3:") {
		t.Errorf("a syntax error in a sibling: %v", err)
	}
}

func TestAReactImportCannotEscapeTheArtifactsFiles(t *testing.T) {
	for _, src := range []string{
		`import x from "../secret"; export default () => <p>{x}</p>;`,
		`import x from "./a/../../secret"; export default () => <p>{x}</p>;`,
		`import x from "/etc/passwd"; export default () => <p>{x}</p>;`,
		`import x from "file:///etc/passwd"; export default () => <p>{x}</p>;`,
	} {
		err := React{}.ValidateFiles(src, CurrentFormat(React{}), files.Set{"secret.ts": "export default 1;"})
		if err == nil || !strings.Contains(err.Error(), "cannot import") {
			t.Errorf("%s: %v, want a refusal", src, err)
		}
	}
	// A sibling may not reach out of the files either, however deep it sits.
	fs := files.Set{"a/b.ts": `import x from "../../c"; export default x;`, "c.ts": "export default 1;"}
	if err := (React{}).ValidateFiles(`import b from "./a/b"; export default () => <p>{b}</p>;`, CurrentFormat(React{}), fs); err == nil || !strings.Contains(err.Error(), "leaves the artifact's files") {
		t.Errorf("a sibling that climbs out of the files: %v", err)
	}
	// A bare package name is never a file: a sibling named like a package does not shadow it.
	shadow := files.Set{"recharts.tsx": `export const LineChart = 1;`}
	if err := (React{}).ValidateFiles(`import { LineChart } from "recharts"; export default () => <LineChart />;`, CurrentFormat(React{}), shadow); err != nil {
		t.Errorf("a runtime module was shadowed or refused: %v", err)
	}
}

func TestAReactCSSImportBecomesAStyleElementNotAScriptSource(t *testing.T) {
	p, err := React{}.RenderPageFiles(reactWithSiblings, CurrentFormat(React{}), reactFiles())
	if err != nil {
		t.Fatal(err)
	}
	// esbuild minifies the colour to #010203.
	style := regexp.MustCompile(`(?s)<style>\n(.*?)</style>`).FindStringSubmatch(p.HTML)
	if style == nil || !strings.Contains(style[1], "#010203") {
		t.Fatalf("the imported stylesheet is not in the page as a style element:\n%s", p.HTML)
	}
	if !strings.Contains(style[1], "../img/dot.svg") {
		t.Errorf("a url() in the stylesheet was rewritten; the browser resolves it against the page: %s", style[1])
	}
	for i, s := range p.InlineScripts {
		if strings.Contains(s, "#010203") {
			t.Errorf("the stylesheet reached inline script %d", i)
		}
	}
	// A stylesheet cannot end its style element and carry markup out.
	evil := files.Set{"x.css": "a{}</style><img src=x onerror=alert(1)><style>"}
	err = React{}.ValidateFiles(`import "./x.css"; export default () => <p/>;`, CurrentFormat(React{}), evil)
	if err == nil || !strings.Contains(err.Error(), "</style") {
		t.Errorf("a stylesheet with </style compiled: %v", err)
	}
}

func TestAReactImportOfAFileThatIsNotCodeIsRefused(t *testing.T) {
	err := React{}.ValidateFiles(`import x from "./page.html"; export default () => <p>{x}</p>;`, CurrentFormat(React{}), files.Set{"page.html": "<p>"})
	if err == nil || !strings.Contains(err.Error(), "can be imported") {
		t.Errorf("an import of an html file: %v", err)
	}
}

func TestTheReactCompileCacheKeyIncludesTheFiles(t *testing.T) {
	format := CurrentFormat(React{})
	one := files.Set{"x.ts": "export const x = 1;"}
	two := files.Set{"x.ts": "export const x = 2;"}
	src := `import { x } from "./x"; export default () => <p>{x}</p>;`
	a, err := React{}.RenderPageFiles(src, format, one)
	if err != nil {
		t.Fatal(err)
	}
	b, err := React{}.RenderPageFiles(src, format, two)
	if err != nil {
		t.Fatal(err)
	}
	if a.InlineScripts[2] == b.InlineScripts[2] {
		t.Error("two different files compiled to the same page: the cache ignored the files")
	}
	before := reactCompiles.Load()
	if _, err := (React{}).RenderPageFiles(src, format, one); err != nil {
		t.Fatal(err)
	}
	if reactCompiles.Load() != before {
		t.Error("the same component and files compiled again instead of being remembered")
	}
}

func TestAReactBundleTooLargeToCarryInlineIsRefusedWithWhatToDo(t *testing.T) {
	const limit = 2 << 20 // the production limit is 16 MB; the rule is the same at any size
	big := files.Set{"big.js": "export const x = `" + strings.Repeat("a", limit+1) + "`;"}
	err := React{MaxBundleBytes: limit}.ValidateFiles(`import { x } from "./big.js"; export default () => <p>{x}</p>;`, CurrentFormat(React{}), big)
	if err == nil || !strings.Contains(err.Error(), "2 MB") || !strings.Contains(err.Error(), ".json") {
		t.Errorf("a bundle over the limit: %v", err)
	}
}
