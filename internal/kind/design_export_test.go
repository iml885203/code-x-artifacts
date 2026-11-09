package kind

import (
	"archive/zip"
	"bytes"
	"io"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/files"
)

const exportManifest = `{"version":1,"pages":[{"id":"a","name":"Home <1>","rows":[[` +
	`{"id":"hero","name":"Hero","file":"frames/hero.html","w":1440,"h":900},` +
	`{"id":"mob","name":"Mobile","file":"frames/mobile.html","w":390,"h":844}]]}]}`

func exportFrames() files.Set {
	return files.Set{
		"frames/hero.html": `<!doctype html><html><head><style>@font-face{font-family:F;src:url("/static/fonts/F.woff2")}` +
			`.b{background:url(/files/ART/bg.png)}</style></head><body>` +
			`<img src="/files/ART/logo.png" srcset="/files/ART/logo.png 1x, https://pictor.test/files/ART/logo2.png 2x">` +
			`<img src="data:image/png;base64,AAAA"><a href="#top">top</a><img src="/files/OTHER/theirs.png"><img src="/files/ART/gone.png"></body></html>`,
		"frames/mobile.html": `<!doctype html><html><body><img src="/files/ART/logo.png"></body></html>`,
	}
}

func fakeOpen(src string) (File, bool) {
	src = strings.TrimPrefix(src, "https://pictor.test")
	switch src {
	case "/files/ART/logo.png", "/files/ART/logo2.png", "/files/ART/bg.png":
		return File{Data: []byte("png:" + path.Base(src)), ContentType: "image/png"}, true
	case "/static/fonts/F.woff2":
		return File{Data: []byte("font"), ContentType: "font/woff2"}, true
	}
	return File{}, false
}

func unzipAll(t *testing.T, body []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(b)
	}
	return out
}

var (
	attrRef = regexp.MustCompile(`(?i)\b(?:src|href|poster)="([^"]*)"`)
	setRef  = regexp.MustCompile(`(?i)\bsrcset="([^"]*)"`)
	urlRef  = regexp.MustCompile(`(?i)url\(\s*["']?([^"')]*)`)
)

// references lists every address a document in the zip asks for.
func references(doc string) []string {
	var refs []string
	for _, m := range attrRef.FindAllStringSubmatch(doc, -1) {
		refs = append(refs, m[1])
	}
	for _, m := range setRef.FindAllStringSubmatch(doc, -1) {
		for _, candidate := range strings.Split(m[1], ",") {
			if f := strings.Fields(candidate); len(f) > 0 {
				refs = append(refs, f[0])
			}
		}
	}
	for _, m := range urlRef.FindAllStringSubmatch(doc, -1) {
		refs = append(refs, m[1])
	}
	return refs
}

func TestEveryAddressInADesignZipResolvesInsideTheZip(t *testing.T) {
	d, err := (Design{}).ExportBundle("Landing", exportManifest, designFormat, "zip", "https://pictor.test", exportFrames(), fakeOpen)
	if err != nil {
		t.Fatal(err)
	}
	if d.Extension != "zip" || d.ContentType != "application/zip" {
		t.Errorf("download = %s .%s", d.ContentType, d.Extension)
	}
	entries := unzipAll(t, d.Body)
	for _, want := range []string{"index.html", "design.json", "frames/hero.html", "frames/mobile.html", "assets/logo.png", "assets/logo2.png", "assets/bg.png", "fonts/F.woff2"} {
		if _, ok := entries[want]; !ok {
			t.Errorf("the zip lacks %s; it has %v", want, keys(entries))
		}
	}
	if entries["assets/logo.png"] != "png:logo.png" || entries["design.json"] != exportManifest {
		t.Error("an entry is not the bytes it came from")
	}
	problems, checked := unresolved(entries)
	for _, p := range problems {
		t.Error(p)
	}
	if checked < 10 {
		t.Errorf("the check read %d references; it is not looking at the zip", checked)
	}
}

// unresolved lists the references in the zip's pages that do not land on a file of
// the zip, and says how many references it read.
func unresolved(entries map[string]string) (problems []string, checked int) {
	for name, doc := range entries {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		for _, ref := range references(doc) {
			checked++
			if ref == "" || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "data:") {
				continue
			}
			target := path.Clean(path.Join(path.Dir(name), ref))
			if _, ok := entries[target]; !ok || strings.Contains(ref, "://") || strings.HasPrefix(ref, "/") {
				problems = append(problems, name+" points at "+ref+", which is not a file of the zip")
			}
		}
		if strings.Contains(doc, "/files/") || strings.Contains(doc, "/static/") || strings.Contains(doc, "pictor.test") || strings.Contains(doc, "@import") {
			problems = append(problems, name+" still names a server")
		}
	}
	return problems, checked
}

func TestAFileThatCannotBeFoundBecomesNothingInsteadOfARequest(t *testing.T) {
	d, err := (Design{}).ExportBundle("Landing", exportManifest, designFormat, "zip", "https://pictor.test", exportFrames(), fakeOpen)
	if err != nil {
		t.Fatal(err)
	}
	hero := unzipAll(t, d.Body)["frames/hero.html"]
	if strings.Count(hero, `src="data:,"`) != 2 {
		t.Errorf("another artifact's picture and a deleted upload should both be empty data: addresses: %s", hero)
	}
	// Without any way to open files, nothing is left pointing at a server either.
	d, err = (Design{}).ExportBundle("Landing", exportManifest, designFormat, "zip", "", exportFrames(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, doc := range unzipAll(t, d.Body) {
		if strings.Contains(doc, "/files/") || strings.Contains(doc, "/static/") {
			t.Errorf("%s still names a server: %.200s", name, doc)
		}
	}
}

func TestTheZipIndexShowsEveryFrameFromItsOwnFileAndLoadsNothingElse(t *testing.T) {
	d, err := (Design{}).ExportBundle("A <b>title</b>", exportManifest, designFormat, "zip", "", exportFrames(), fakeOpen)
	if err != nil {
		t.Fatal(err)
	}
	index := unzipAll(t, d.Body)["index.html"]
	for _, want := range []string{`src="frames/hero.html"`, `src="frames/mobile.html"`, `<title>A &lt;b&gt;title&lt;/b&gt;</title>`, `<h2>Home &lt;1&gt;</h2>`, `aspect-ratio: 1440 / 900`} {
		if !strings.Contains(index, want) {
			t.Errorf("the index lacks %q", want)
		}
	}
	if strings.Contains(index, "<script") || strings.Contains(index, "@import") || strings.Contains(index, "sandbox=") {
		t.Errorf("the index runs a script, imports a sheet or sandboxes its frames: %.300s", index)
	}
}

func TestTheSameDesignIsTheSameZipAndOnlyZipIsAFormat(t *testing.T) {
	a, _ := (Design{}).ExportBundle("T", exportManifest, designFormat, "zip", "", exportFrames(), fakeOpen)
	b, _ := (Design{}).ExportBundle("T", exportManifest, designFormat, "zip", "", exportFrames(), fakeOpen)
	if !bytes.Equal(a.Body, b.Body) {
		t.Error("two exports of the same design differ")
	}
	if _, err := (Design{}).ExportBundle("T", exportManifest, designFormat, "pdf", "", exportFrames(), fakeOpen); err == nil {
		t.Error("exported a design as a format the server does not make")
	}
	if _, err := (Design{}).ExportBundle("T", `{"version":1,"pages":[]}`, designFormat, "zip", "", nil, nil); err == nil {
		t.Error("exported a design.json that does not parse")
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
