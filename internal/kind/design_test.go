package kind

import (
	"regexp"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/files"
)

func TestADesignTakesOnlyHTMLFilesAndTheirPagesAreFramedByThePlatform(t *testing.T) {
	var k Kind = Design{}
	fk, ok := k.(FileKind)
	if !ok || strings.Join(fk.AcceptedFileTypes(), ",") != ".html" {
		t.Fatalf("a design's files are frames, which are .html: %v", fk)
	}
	if _, ok := k.(FrameKind); !ok {
		t.Error("a design's HTML files are frames, framed by the platform, not pages of their own")
	}
}

func TestTheInspectorIsTheOnlyScriptTheFrameDocumentGetsAndItIsFirstInTheHead(t *testing.T) {
	for name, in := range map[string]string{
		"a document":      `<!doctype html><html><head><meta charset="utf-8"></head><body><p>x</p></body></html>`,
		"no head":         `<!doctype html><body><p>x</p></body>`,
		"a bare fragment": `<p>x</p>`,
	} {
		page, err := (Design{}).RenderFrame("frames/a.html", in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(page.InlineScripts) != 1 || page.InlineScripts[0] != designInspector {
			t.Errorf("%s: scripts %d, want the inspector alone", name, len(page.InlineScripts))
		}
		if strings.Count(page.HTML, "<script>"+designInspector+"</script>") != 1 {
			t.Errorf("%s: the inspector is not in the document exactly once: %.200s", name, page.HTML)
		}
		if !strings.Contains(page.HTML, "<p>x</p>") {
			t.Errorf("%s: the frame's own markup was changed: %.200s", name, page.HTML)
		}
	}
	page, _ := (Design{}).RenderFrame("f.html", `<!doctype html><html><head></head><body></body></html>`)
	if !strings.HasPrefix(page.HTML, "<!doctype html><html><head><script>") {
		t.Errorf("the doctype must stay first and the inspector follow the head: %.80s", page.HTML)
	}
}

func TestTheInspectorOnlyListensToItsParent(t *testing.T) {
	if !regexp.MustCompile(`\bif\s*\(\s*e\.source\s*!==\s*parent\s*\)\s*return\b`).MatchString(designInspector) {
		t.Error("the inspector answers messages from any window, a frame's author has no script but this check must stay for whoever loosens script-src")
	}
}

func TestADesignIsJudgedWithItsFilesAndStoresThemNormalisedButIsNotBundled(t *testing.T) {
	var k Kind = Design{}
	if _, ok := k.(FileValidator); !ok {
		t.Error("a design's manifest cannot be judged without its frames: it must be a FileValidator")
	}
	if _, ok := k.(Bundler); ok {
		t.Error("a design's frames are served on their own, not compiled into the page; it must not be a Bundler")
	}
	if _, ok := k.(FileNormalizer); !ok {
		t.Error("frames are stored in one normal form: Design must be a FileNormalizer")
	}
	if _, ok := k.(FileReviewer); !ok {
		t.Error("what a publisher cannot see about its frames is for a FileReviewer to say")
	}
	if err := k.Validate(`{"version":1,"pages":[]}`, "2"); err == nil {
		t.Error("a content format the kind lacks was accepted")
	}
	if err := k.Validate(`{"version":1,"pages":[{"id":"a","name":"A","rows":[[{"id":"h","name":"H","file":"frames/h.html","w":10,"h":10}]]}]}`, "1"); err == nil || !strings.Contains(err.Error(), "frames/h.html is not among this design's files") {
		t.Errorf("a manifest whose frame has no file: %v", err)
	}
}

var guideFence = regexp.MustCompile("(?s)```(\\w+)([^\\n]*)\\n(.*?)```")

// guideExamples returns the guide's fenced blocks: design.json, the frames by
// file name, and the unnamed html and css pieces.
func guideExamples(t *testing.T) (manifest string, frames files.Set, pieces, sheets []string) {
	t.Helper()
	frames = files.Set{}
	for _, m := range guideFence.FindAllStringSubmatch((Design{}).Guide(), -1) {
		lang, label, body := m[1], strings.TrimSpace(m[2]), m[3]
		switch {
		case lang == "json" && label == "design.json":
			manifest = body
		case lang == "html" && strings.HasPrefix(label, "frames/"):
			frames[label] = body
		case lang == "html":
			pieces = append(pieces, body)
		case lang == "css":
			sheets = append(sheets, body)
		case lang == "text":
		default:
			t.Errorf("a guide example of kind %q is not checked: label it json design.json, html frames/<name>.html, html, css or text", lang)
		}
	}
	return manifest, frames, pieces, sheets
}

func TestEveryExampleInTheDesignGuideValidates(t *testing.T) {
	manifest, frames, pieces, sheets := guideExamples(t)
	if manifest == "" || len(frames) != 3 {
		t.Fatalf("the guide must hold one design.json and three frames, it has %q and %d frames", manifest, len(frames))
	}
	if err := (Design{}).ValidateFiles(manifest, designFormat, frames); err != nil {
		t.Errorf("the guide's example design does not validate: %v", err)
	}
	for i, piece := range pieces {
		if err := (Design{}).ValidateFiles(oneFrameDesign, designFormat, files.Set{"frames/a.html": piece}); err != nil {
			t.Errorf("html example %d of the guide breaks the rules it teaches: %v", i+1, err)
		}
	}
	for i, sheet := range sheets {
		doc := files.Set{"frames/a.html": "<style>" + sheet + "</style><p>x"}
		if err := (Design{}).ValidateFiles(oneFrameDesign, designFormat, doc); err != nil {
			t.Errorf("css example %d of the guide breaks the rules it teaches: %v", i+1, err)
		}
	}
}

func TestTheGuidesExampleDesignIsAlreadyInNormalFormAfterOnePassAndNeedsNoReview(t *testing.T) {
	manifest, frames, _, _ := guideExamples(t)
	stored := files.Set{}
	var changed []string
	for p, text := range frames {
		out, err := (Design{}).NormalizeFile(designFormat, p, text)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		again, _ := (Design{}).NormalizeFile(designFormat, p, out)
		if again != out {
			t.Errorf("%s: normalising twice changed it", p)
		}
		stored[p], changed = out, append(changed, p)
	}
	if notes := (Design{}).ReviewFiles(manifest, designFormat, stored, changed); len(notes) != 0 {
		t.Errorf("the guide's own example earns review notes: %v", notes)
	}
}

func TestTheDesignGuideSaysWhatAnAgentNeedsToPublishAndEdit(t *testing.T) {
	g := (Design{}).Guide()
	for _, want := range []string{
		"design.json", "frames/", "files", "rewritten", "data-pid", "null", "baseVersion", "review", "upload_asset",
		"1 to 8192", "20 pages", "60 frames", "256 KB", "512 KB", "script", "@import", "#fragment", "element 3", "read",
	} {
		if !strings.Contains(g, want) {
			t.Errorf("the design guide never mentions %q", want)
		}
	}
}

const oneFrameDesign = `{"version":1,"pages":[{"id":"a","name":"A","rows":[[{"id":"a","name":"A","file":"frames/a.html","w":100,"h":100}]]}]}`

func TestTheOverviewDrawsEveryFrameAsAThumbnailOfItsOwnFileScaledByCSSAndRunsNoScript(t *testing.T) {
	manifest, _, _, _ := guideExamples(t)
	page, err := (Design{}).RenderPage(manifest, designFormat)
	if err != nil {
		t.Fatal(err)
	}
	html := page.Titled("Landing <b>").HTML
	for _, want := range []string{
		`src="frames/hero.html"`, `src="frames/mobile.html"`, `src="frames/footer.html"`, `data-w="1440"`, `data-w="390"`,
		`aspect-ratio: 1440 / 900`, `<h2>Home</h2>`, `Hero mobile`, `<title>Landing &lt;b&gt;</title>`, `sandbox="allow-scripts"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the overview lacks %q", want)
		}
	}
	if len(page.InlineScripts) != 0 || strings.Contains(html, "<script") {
		t.Errorf("the overview must run no script, so a preview without allow-scripts shows it: %d declared", len(page.InlineScripts))
	}
	if !strings.Contains(html, "scale(tan(atan2(100cqw, 1440px)))") {
		t.Error("the thumbnails are not scaled to their cell by CSS")
	}
	if _, err := (Design{}).RenderPage(`{"version":1,"pages":[]}`, designFormat); err == nil {
		t.Error("drew a design.json that does not validate")
	}
}

func TestAFrameNameCannotBreakOutOfTheOverview(t *testing.T) {
	const evil = `{"version":1,"pages":[{"id":"a","name":"<img src=x onerror=1>","rows":[[{"id":"a","name":"\"><script>1</script>","file":"frames/a.html","w":10,"h":10}]]}]}`
	page, err := (Design{}).RenderPage(evil, designFormat)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page.HTML, "<img src=x") || strings.Contains(page.HTML, "<script") {
		t.Errorf("a name was written into the overview as markup: %.300s", page.HTML)
	}
}

func TestTheInspectorStaysSmallBecauseItIsInEveryFrame(t *testing.T) {
	if got := len(designInspector); got > 12<<10 {
		t.Errorf("the inspector is %d bytes; it is in every frame and is at most 12 KB", got)
	}
}

// A frame that is not loaded when the browser prints is a blank page on paper, and a
// lazy frame is not loaded until it comes near the screen.
func TestEveryFrameOfTheOverviewAndOfTheZipIndexLoadsAtOnceSoNoPrintedPageIsBlank(t *testing.T) {
	manifest, _, _, _ := guideExamples(t)
	page, err := (Design{}).RenderPage(manifest, designFormat)
	if err != nil {
		t.Fatal(err)
	}
	zip, err := (Design{}).ExportBundle("T", exportManifest, designFormat, "zip", "", exportFrames(), fakeOpen)
	if err != nil {
		t.Fatal(err)
	}
	for name, html := range map[string]string{"overview": page.HTML, "zip index": unzipAll(t, zip.Body)["index.html"]} {
		if !strings.Contains(html, "<iframe") {
			t.Fatalf("the %s has no frame", name)
		}
		if strings.Contains(html, `loading="lazy"`) {
			t.Errorf("the %s holds a frame that loads only when it is near the screen", name)
		}
	}
}
