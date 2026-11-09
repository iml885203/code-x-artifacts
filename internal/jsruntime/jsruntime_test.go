package jsruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

func loadV1(t *testing.T) *Runtime {
	t.Helper()
	rt, err := Load(V1)
	if err != nil {
		t.Fatalf("Load(V1): %v", err)
	}
	return rt
}

func read(t *testing.T, rt *Runtime, name string) string {
	t.Helper()
	b, err := fs.ReadFile(rt.FS, name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestEveryEmbeddedFileIsInTheManifestWithItsHash(t *testing.T) {
	rt := loadV1(t)
	entries, err := fs.ReadDir(rt.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	var onDisk []string
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("%s is a folder; the runtime is flat", e.Name())
			continue
		}
		if e.Name() == "manifest.json" {
			continue
		}
		onDisk = append(onDisk, e.Name())
		sum := sha256.Sum256([]byte(read(t, rt, e.Name())))
		if got, want := hex.EncodeToString(sum[:]), rt.Manifest.Files[e.Name()]; got != want {
			t.Errorf("%s hashes to %s, the manifest says %s", e.Name(), got, want)
		}
	}
	var listed []string
	for name := range rt.Manifest.Files {
		listed = append(listed, name)
	}
	sort.Strings(onDisk)
	sort.Strings(listed)
	if !slices.Equal(onDisk, listed) {
		t.Errorf("embedded files %v differ from the manifest's %v", onDisk, listed)
	}
}

func TestEveryFileNameCarriesAHashOfItsContent(t *testing.T) {
	rt := loadV1(t)
	named := regexp.MustCompile(`-[0-9A-Za-z]{8}\.js$`)
	for name := range rt.Manifest.Files {
		if name == rt.Manifest.Notices {
			continue
		}
		if !named.MatchString(name) {
			t.Errorf("%s has no content hash in its name, so it could not be served immutable", name)
		}
	}
	tailwind := rt.Manifest.Scripts["tailwind"]
	sum := sha256.Sum256([]byte(read(t, rt, tailwind)))
	if want := "-" + hex.EncodeToString(sum[:])[:8] + ".js"; !strings.HasSuffix(tailwind, want) {
		t.Errorf("tailwind script %s is not named by its SHA-256 (%s)", tailwind, want)
	}
}

func TestRuntimeV1OffersExactlyTheLibrariesOfTheIssue(t *testing.T) {
	rt := loadV1(t)
	want := []string{
		"@/components/ui/badge", "@/components/ui/button", "@/components/ui/card", "@/components/ui/dialog",
		"@/components/ui/progress", "@/components/ui/tabs", "@/components/ui/tooltip", "@/lib/utils",
		"lucide-react", "react", "react-dom", "react-dom/client", "react/jsx-runtime", "recharts",
	}
	var got []string
	for s := range rt.Manifest.Specifiers {
		got = append(got, s)
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Errorf("v1 specifiers = %v, want %v; adding a library to v1 changes what published pages can rely on", got, want)
	}
}

func TestEverySpecifierIsAFileThatExportsWhatTheManifestSays(t *testing.T) {
	rt := loadV1(t)
	for specifier, m := range rt.Manifest.Specifiers {
		if _, ok := rt.Manifest.Files[m.File]; !ok {
			t.Errorf("%s points at %s, which is not an embedded file", specifier, m.File)
		}
		if len(m.Exports) == 0 {
			t.Errorf("%s exports nothing", specifier)
		}
	}
	wantNames := map[string][]string{
		"react":                  {"useState", "createElement", "default"},
		"react/jsx-runtime":      {"jsx", "jsxs", "Fragment"},
		"react-dom/client":       {"createRoot"},
		"recharts":               {"LineChart", "ResponsiveContainer"},
		"lucide-react":           {"Flame"},
		"@/lib/utils":            {"cn"},
		"@/components/ui/tabs":   {"Tabs", "TabsList", "TabsTrigger", "TabsContent"},
		"@/components/ui/card":   {"Card", "CardContent"},
		"@/components/ui/button": {"Button"},
	}
	for specifier, names := range wantNames {
		for _, name := range names {
			if !slices.Contains(rt.Manifest.Specifiers[specifier].Exports, name) {
				t.Errorf("%s does not export %s", specifier, name)
			}
		}
	}
}

// A page gets one React: the import map points "react" at a single file, so
// every other module has to import it by name instead of carrying its own.
var importsOf = regexp.MustCompile(`(?:^|;)(?:import|export)[^;"']*?(?:from)?"([^"]+)"`)

func TestNoModuleBundlesItsOwnReactOrReachesOutsideTheRuntime(t *testing.T) {
	rt := loadV1(t)
	external := map[string]bool{"react": true, "react/jsx-runtime": true, "react-dom": true, "react-dom/client": true}
	importsReact := 0
	for name := range rt.Manifest.Files {
		if !strings.HasSuffix(name, ".js") || name == rt.Manifest.Scripts["tailwind"] {
			continue
		}
		src := read(t, rt, name)
		if strings.Contains(src, "import(") {
			t.Errorf("%s loads code dynamically; every file the page needs must be named in the manifest", name)
		}
		for _, m := range importsOf.FindAllStringSubmatch(src, -1) {
			target := m[1]
			switch {
			case external[target]:
				importsReact++
			case strings.HasPrefix(target, "./") && rt.Manifest.Files[strings.TrimPrefix(target, "./")] != "":
			default:
				t.Errorf("%s imports %q, which is neither a runtime file nor one of the shared React specifiers", name, target)
			}
		}
		if strings.Contains(src, "process.env") {
			t.Errorf("%s still reads process.env: it was not built for production", name)
		}
	}
	if importsReact == 0 {
		t.Error("no module imports react by name: either React is bundled into each of them or the pattern no longer matches the output")
	}
	for _, specifier := range []string{"recharts", "lucide-react", "@/components/ui/tooltip", "@/components/ui/dialog"} {
		src := read(t, rt, rt.Manifest.Specifiers[specifier].File)
		if !regexp.MustCompile(`from"react"`).MatchString(src) {
			t.Errorf("%s does not import react by name: it carries its own copy, and the page would run two Reacts", specifier)
		}
	}
}

func TestTheNoticesCoverTheBundledPackages(t *testing.T) {
	rt := loadV1(t)
	notices := read(t, rt, rt.Manifest.Notices)
	for _, pkg := range []string{"react@", "react-dom@", "recharts@", "lucide-react@", "@tailwindcss/browser@", "@radix-ui/react-dialog@", "tailwind-merge@", "clsx@", "d3-scale@"} {
		if !strings.Contains(notices, "\n"+pkg) {
			t.Errorf("NOTICES.txt has no entry for %s", pkg)
		}
	}
}

func TestAVersionThisBinaryDoesNotCarryIsAnError(t *testing.T) {
	if _, err := Load("v0"); err == nil {
		t.Fatal("Load(v0) succeeded for a runtime that is not embedded")
	}
}
