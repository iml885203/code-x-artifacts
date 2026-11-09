package files

import (
	"strings"
	"testing"
)

func TestPathsThatEscapeCollideOrAreMalformedAreRefused(t *testing.T) {
	t.Parallel()
	bad := []struct{ path, want string }{
		{"", "empty"},
		{"/app.js", "relative"},
		{"app.js/", "empty"},
		{"a//b.js", "empty"},
		{"../app.js", ".."},
		{"a/../b.js", ".."},
		{"./app.js", "start with a dot"},
		{".hidden.js", "start with a dot"},
		{"a/.b/c.js", "start with a dot"},
		{"my file.js", "letters, digits"},
		{`dir\app.js`, "letters, digits"},
		{"é.js", "letters, digits"},
		{"a/b/c/d/e/f.js", "at most 5"},
		{strings.Repeat("a", 201) + ".js", "at most 200"},
		{"noext", "extension"},
	}
	for _, c := range bad {
		if err := CheckPath(c.path, Types()); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("CheckPath(%q) = %v, want a refusal mentioning %q", c.path, err, c.want)
		}
	}
	good := []string{"app.js", "styles/site.css", "data/rows.json", "a/b/c/d/e.mjs", "lib/other.HTML", "v1.2-final_x.js"}
	for _, p := range good {
		if err := CheckPath(p, Types()); err != nil {
			t.Errorf("CheckPath(%q) = %v, want it accepted", p, err)
		}
	}
}

func TestIndexHTMLCannotBeAFileName(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"index.html", "INDEX.HTML", "Index.Html"} {
		err := CheckPath(p, Types())
		if err == nil || !strings.Contains(err.Error(), "content") {
			t.Errorf("CheckPath(%q) = %v, want a refusal that says the entry page is content", p, err)
		}
	}
	if err := CheckPath("sub/index.html", Types()); err != nil {
		t.Errorf("a nested index.html is an ordinary page: %v", err)
	}
}

func TestFilesOfOtherTypesAreRefusedWithTheTypesThatAreAccepted(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"logo.png", "x.svg", "x.ts", "x.txt", "x.wasm", "x.map"} {
		err := CheckPath(p, Types())
		if err == nil || !strings.Contains(err.Error(), ".js") || !strings.Contains(err.Error(), ".json") {
			t.Errorf("CheckPath(%q) = %v, want a refusal that lists the accepted types", p, err)
		}
	}
}

func TestJSONFilesMustParse(t *testing.T) {
	t.Parallel()
	if _, err := Check("data/rows.json", `{"a": [1, 2`, Types()); err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Errorf("broken JSON: %v", err)
	}
	if ct, err := Check("data/rows.json", `{"a": [1, 2]}`, Types()); err != nil || ct != "application/json; charset=utf-8" {
		t.Errorf("good JSON: %q, %v", ct, err)
	}
}

func TestAFilesContentIsTextWithoutNULAndCarriesItsTypeFromTheExtension(t *testing.T) {
	t.Parallel()
	if _, err := Check("a.js", "x\x00y", Types()); err == nil || !strings.Contains(err.Error(), "NUL") {
		t.Errorf("NUL: %v", err)
	}
	if _, err := Check("a.js", "\xff\xfe", Types()); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("not UTF-8: %v", err)
	}
	want := map[string]string{
		"a.js": "text/javascript; charset=utf-8", "a.mjs": "text/javascript; charset=utf-8", "a.css": "text/css; charset=utf-8",
		"a.html": "text/html; charset=utf-8", "a.JSON": "application/json; charset=utf-8",
	}
	for p, ct := range want {
		content := "x"
		if strings.HasSuffix(strings.ToLower(p), ".json") {
			content = "1"
		}
		if got, err := Check(p, content, Types()); err != nil || got != ct {
			t.Errorf("Check(%q) = %q, %v; want %q", p, got, err, ct)
		}
	}
}

func str(s string) *string { return &s }

func TestApplyAddsReplacesRemovesAndCarriesTheRest(t *testing.T) {
	t.Parallel()
	base := map[string]int64{"app.js": 1, "styles/site.css": 1}
	got, err := Apply(base, map[string]*string{"app.js": str("1"), "data/rows.json": str("[]"), "styles/site.css": nil}, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Remove) != 1 || got.Remove[0] != "styles/site.css" {
		t.Errorf("removes = %v", got.Remove)
	}
	if len(got.Put) != 2 || got.Put["app.js"] == "" || got.Put["data/rows.json"] == "" {
		t.Errorf("puts = %v", got.Put)
	}
	if want := []string{"app.js", "data/rows.json"}; strings.Join(got.Paths, ",") != strings.Join(want, ",") {
		t.Errorf("the version's paths = %v, want %v", got.Paths, want)
	}
}

func TestApplyRefusesRemovingAFileThatIsNotThereAndNamesTheFilesThatAre(t *testing.T) {
	t.Parallel()
	base := map[string]int64{"app.js": 1}
	_, err := Apply(base, map[string]*string{"old.js": nil}, DefaultLimits)
	if err == nil || !strings.Contains(err.Error(), "old.js") || !strings.Contains(err.Error(), "app.js") {
		t.Errorf("removing a missing file: %v", err)
	}
}

func TestTwoPathsThatDifferOnlyInCaseCollide(t *testing.T) {
	t.Parallel()
	_, err := Apply(nil, map[string]*string{"App.js": str("1"), "app.js": str("2")}, DefaultLimits)
	if err == nil || !strings.Contains(err.Error(), "case") {
		t.Errorf("both in one publish: %v", err)
	}
	_, err = Apply(map[string]int64{"app.js": 1}, map[string]*string{"APP.JS": str("2")}, DefaultLimits)
	if err == nil || !strings.Contains(err.Error(), "case") {
		t.Errorf("against a carried file: %v", err)
	}
	// Replacing by exactly the same path is a replacement, not a collision.
	if _, err := Apply(map[string]int64{"app.js": 1}, map[string]*string{"app.js": str("2")}, DefaultLimits); err != nil {
		t.Errorf("replace: %v", err)
	}
}

func TestTheLimitsOnFilesAreEnforcedAndNameTheLimitHit(t *testing.T) {
	t.Parallel()
	lim := Limits{FileBytes: 10, PerPublish: 2, PerVersion: 3, TotalBytes: 12}
	cases := []struct {
		label string
		base  map[string]int64
		patch map[string]*string
		want  string
	}{
		{"one file too large", nil, map[string]*string{"a.js": str(strings.Repeat("x", 11))}, "at most 10 bytes"},
		{"too many in one publish", nil, map[string]*string{"a.js": str("1"), "b.js": str("1"), "c.js": str("1")}, "at most 2 files in one publish"},
		{"too many in a version", map[string]int64{"a.js": 1, "b.js": 1, "c.js": 1}, map[string]*string{"d.js": str("1")}, "at most 3 files"},
		{"too many bytes in all", nil, map[string]*string{"a.js": str(strings.Repeat("x", 7)), "b.js": str(strings.Repeat("x", 7))}, "12 bytes in all"},
	}
	for _, c := range cases {
		_, err := Apply(c.base, c.patch, lim)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want a refusal mentioning %q", c.label, err, c.want)
		}
	}
}

func TestThePublishLimitsMatchTheDocumentedNumbers(t *testing.T) {
	t.Parallel()
	l := DefaultLimits
	if l.FileBytes != 16<<20 || l.PerPublish != 255 || l.PerVersion != 511 || l.TotalBytes != 256<<20 || MaxRequestBytes != 64<<20 {
		t.Errorf("limits = %+v, request %d; docs/multi-file.md documents 16 MB, 255, 511, 256 MB and 64 MB", l, MaxRequestBytes)
	}
}

func TestSourcesAreAcceptedOnlyByAKindThatBundlesAndAreNeverServed(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"Chart.tsx", "lib/format.ts", "x.jsx"} {
		if err := CheckPath(p, Types()); err == nil {
			t.Errorf("%s accepted by a kind that does not compile sources", p)
		}
		if err := CheckPath(p, append(Types(), SourceTypes()...)); err != nil {
			t.Errorf("%s refused by a kind that compiles sources: %v", p, err)
		}
		if Servable(p) {
			t.Errorf("%s is servable: a source is only ever imported", p)
		}
	}
	for _, p := range []string{"app.js", "a.mjs", "s.css", "d.json", "o.html"} {
		if !Servable(p) {
			t.Errorf("%s is not servable", p)
		}
	}
}
