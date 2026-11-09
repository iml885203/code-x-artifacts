package kind

import (
	"errors"
	"strings"
	"testing"
)

func TestLookupFindsHTMLAndRefusesAnythingElseNamingWhatExists(t *testing.T) {
	k, err := Lookup(Default)
	if err != nil || k.Name() != "html" {
		t.Fatalf("Lookup(%q) = %v, %v", Default, k, err)
	}
	_, err = Lookup("video")
	var unknown *UnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("want UnknownError, got %v", err)
	}
	if !strings.Contains(err.Error(), `"video"`) || !strings.Contains(err.Error(), "html") {
		t.Errorf("the error must name the bad kind and the valid ones: %v", err)
	}
}

func TestHTMLValidatesContentAndItsSizeLimit(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		ok      bool
	}{
		"empty":          {"", false},
		"fragment":       {"<p>hi", true},
		"at the limit":   {strings.Repeat("a", MaxHTMLBytes), true},
		"over the limit": {strings.Repeat("a", MaxHTMLBytes+1), false},
	} {
		if err := (HTML{}).Validate(tc.content, ""); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok=%v", name, err, tc.ok)
		}
	}
}

func TestHTMLRendersItsContentUnchangedAndExportsTheOriginal(t *testing.T) {
	const page = "<!doctype html><title>t</title>"
	if got, err := (HTML{}).Render(page, ""); err != nil || got != page {
		t.Errorf("Render = %q, %v", got, err)
	}
	d, err := (HTML{}).Export(page, "", "html", "")
	if err != nil || string(d.Body) != page || d.Extension != "html" {
		t.Errorf("Export = %+v, %v", d, err)
	}
	if _, err := (HTML{}).Export(page, "", "pdf", ""); err == nil {
		t.Error("exported a format the kind does not list")
	}
}

func TestFormatVersionsNameWhatABuildCanRenderAndNewArtifactsGetTheNewest(t *testing.T) {
	if got := CurrentFormat(HTML{}); got != "" || !HasFormat(HTML{}, "") || HasFormat(HTML{}, "v1") {
		t.Errorf("html is unversioned: current %q, has \"\" %v, has v1 %v", got, HasFormat(HTML{}, ""), HasFormat(HTML{}, "v1"))
	}
	k := versioned{"v1", "v2"}
	if got := CurrentFormat(k); got != "v2" {
		t.Errorf("CurrentFormat = %q, want the newest (v2)", got)
	}
	if !HasFormat(k, "v1") || !HasFormat(k, "v2") || HasFormat(k, "v3") || HasFormat(k, "") {
		t.Error("HasFormat must accept exactly the listed versions")
	}
}

type versioned []string

func (versioned) Name() string                       { return "versioned" }
func (v versioned) FormatVersions() []string         { return v }
func (versioned) Validate(_, _ string) error         { return nil }
func (versioned) Render(c, _ string) (string, error) { return c, nil }
func (versioned) Guide() string                      { return "g" }

func TestEveryKindTeachesItsAuthoringRules(t *testing.T) {
	for _, name := range Names() {
		k, err := Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(k.Guide()) == "" {
			t.Errorf("kind %s has no authoring guide: agents learn how to write it from Guide()", name)
		}
	}
}

func TestTheHTMLGuideCoversTheSandboxAndTheBridge(t *testing.T) {
	g := (HTML{}).Guide()
	for _, want := range []string{"sandbox", "localStorage", "window.pictor", "pictor.db.set", "visibility", "organisation"} {
		if !strings.Contains(g, want) {
			t.Errorf("the html guide never mentions %q", want)
		}
	}
}

func TestTheHTMLGuideTellsAgentsToLinkTheKitAndKeepTheContract(t *testing.T) {
	g := (HTML{}).Guide()
	for _, want := range []string{
		`href="/kit/v5/pictor.css"`, "viewport", "prefers-color-scheme", "390 px", "pk-callout", "pk-table-wrap",
		"var(--chart-1)", "gradient", "emoji", "running text only", "unless the person asks",
	} {
		if !strings.Contains(g, want) {
			t.Errorf("the html guide never mentions %q", want)
		}
	}
}

func TestTheGuidesNameTheCurrentKitAndNotTheFrozenOne(t *testing.T) {
	for name, g := range map[string]string{"html": (HTML{}).Guide(), "react": (React{}).Guide()} {
		if !strings.Contains(g, "/kit/v5/") || strings.Contains(g, "/kit/v1/") {
			t.Errorf("the %s guide must name /kit/v5/ and not /kit/v1/", name)
		}
	}
}

// An agent only learns to upload a picture instead of inlining it if the guide
// of the kind it is writing says how and what the reference looks like.
func TestEveryKindThatShowsPicturesTeachesHowToReferenceAnUploadedFile(t *testing.T) {
	for _, c := range []struct {
		kind Kind
		want []string
	}{
		{HTML{}, []string{"upload_asset", "/assets/{name}", `<img src="`, "/files/"}},
		{Doc{}, []string{"upload_asset", "/assets/{name}", "![", "/files/"}},
	} {
		g := c.kind.Guide()
		for _, want := range c.want {
			if !strings.Contains(g, want) {
				t.Errorf("the %s guide never mentions %q", c.kind.Name(), want)
			}
		}
	}
}

func TestTheGuidesForPagesTeachTheSharedDbQueriesConditionalWritesAndBatches(t *testing.T) {
	for name, g := range map[string]string{"html": (HTML{}).Guide(), "react": (React{}).Guide()} {
		for _, want := range []string{"pictor.db.query", "pictor.db.update", "pictor.db.batch", "ifVersion", "version", "prefix", "all of them or none"} {
			if !strings.Contains(g, want) {
				t.Errorf("the %s guide never mentions %q", name, want)
			}
		}
	}
}

// A page that grows past one file is only organised if the agent that writes it
// knows the files exist, that they are relative, how one is changed or removed,
// and that browser storage is not there for the state they would keep.
func TestTheHTMLGuideTeachesFilesRelativePathsAndTheSharedDBInsteadOfBrowserStorage(t *testing.T) {
	g := (HTML{}).Guide()
	for _, want := range []string{
		"files", `<script src="app.js">`, `<link rel="stylesheet" href="styles/site.css">`, `fetch("data/rows.json")`,
		"relative", "Never start one with", "null", "index.html", ".mjs", ".json", "carried over", "16 MB", "255", "511", "256 MB",
		"restore", "upload_asset", "localStorage", "pictor.db",
	} {
		if !strings.Contains(g, want) {
			t.Errorf("the html guide never mentions %q", want)
		}
	}
	for _, want := range []string{"files", `import Chart from "./Chart"`, "relative", "null", "carried over", "read", "16 MB", "255", "511", "256 MB", ".css", ".json", "restore", "upload_asset"} {
		if !strings.Contains((React{}).Guide(), want) {
			t.Errorf("the react guide never mentions %q", want)
		}
	}
	for name, g := range map[string]string{"html": g, "react": (React{}).Guide()} {
		if !strings.Contains(g, "localStorage") || !strings.Contains(g, "pictor.db") {
			t.Errorf("the %s guide does not tell agents to keep state in pictor.db instead of browser storage", name)
		}
	}
}

func TestOnlyKindsThatTakeFilesSayWhichTypesTheyAccept(t *testing.T) {
	for _, k := range []Kind{HTML{}, React{}, Design{}} {
		if fk, ok := k.(FileKind); !ok || len(fk.AcceptedFileTypes()) == 0 {
			t.Errorf("%s takes files and must say which types", k.Name())
		}
	}
	for _, k := range []Kind{Doc{}, Slides{}} {
		if _, ok := k.(FileKind); ok {
			t.Errorf("kind %s says it takes files, but it is one piece of content", k.Name())
		}
	}
	if _, ok := Kind(HTML{}).(Bundler); ok {
		t.Error("html's files are loaded by the browser, not compiled into the page")
	}
	if _, ok := Kind(React{}).(Bundler); !ok {
		t.Error("react compiles its files into the page and must be a Bundler")
	}
}

func TestTheHTMLAndReactGuidesTeachCodeXArtifactsDownloadWithItsLimitsAndThatTheViewerMayRefuse(t *testing.T) {
	for name, g := range map[string]string{"html": (HTML{}).Guide(), "react": (React{}).Guide()} {
		for _, want := range []string{"pictor.download", "10 MB", "text/csv", "application/zip", "image/png", "refuse", "Cancel"} {
			if !strings.Contains(g, want) {
				t.Errorf("the %s guide never mentions %q", name, want)
			}
		}
		if strings.Contains(g, "scripts, forms, `alert` / `confirm`, downloads") {
			t.Errorf("the %s guide still says a page can download by itself", name)
		}
	}
}
