package web

import (
	"archive/zip"
	"bytes"
	"io"
	"mime"
	"net/http"
	"strings"
	"testing"
)

func TestAWordExportEmbedsThePicturesUploadedToTheDocAndTheDownloadIsNamedAfterTheTitle(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	id := h.editableDoc(tok)
	if res, body := h.upload(tok, id, "chart.png", pngBytes); res.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d %s", res.StatusCode, body)
	}
	update := `{"title":"週報 Report","baseVersion":1,"blocks":[{"id":"h","type":"heading","level":1,"content":[{"text":"標題 😀"}]},` +
		`{"id":"i1","type":"image","src":"/files/` + id + `/chart.png","alt":"a chart"},` +
		`{"id":"i2","type":"image","src":"https://example.com/far.png","alt":"far"}]}`
	if res, body := api(h, "PUT", "/api/artifacts/"+id, update, tok); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, body)
	}

	res, body := api(h, "GET", "/api/artifacts/"+id+"/export/docx", "", tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("export: %d %s", res.StatusCode, body)
	}
	if got := res.Header.Get("Content-Type"); got != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := res.Header.Get("Content-Disposition"); !strings.Contains(got, "attachment") || !strings.Contains(got, ".docx") {
		t.Errorf("Content-Disposition = %q", got)
	}
	zr, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	if err != nil {
		t.Fatalf("the download is not a zip: %v", err)
	}
	var media, document string
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		switch {
		case strings.HasPrefix(f.Name, "word/media/"):
			media = string(b)
		case f.Name == "word/document.xml":
			document = string(b)
		}
	}
	if media != pngBytes {
		t.Error("the uploaded picture is not in the file")
	}
	if !strings.Contains(document, "標題 😀") || !strings.Contains(document, "[image: far]") {
		t.Errorf("the document holds %s", document)
	}

	// Someone who may not see the doc gets the same answer as for a doc that is not there.
	if res, _ := api(h, "GET", "/api/artifacts/"+id+"/export/docx", "", h.token("mallory")); res.StatusCode != http.StatusNotFound {
		t.Errorf("a stranger's export: %d", res.StatusCode)
	}
}

func TestADownloadsNameIsQuotedForPlainClientsAndEncodedForThoseThatReadUTF8(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, want string }{
		{"Landing.zip", `attachment; filename="Landing.zip"; filename*=UTF-8''Landing.zip`},
		{"Landing page.zip", `attachment; filename="Landing page.zip"; filename*=UTF-8''Landing%20page.zip`},
		{"週報 Report.docx", `attachment; filename="__ Report.docx"; filename*=UTF-8''%E9%80%B1%E5%A0%B1%20Report.docx`},
		{`say "hi".md`, `attachment; filename="say _hi_.md"; filename*=UTF-8''say%20%22hi%22.md`},
		{"it's (100%).md", `attachment; filename="it's (100_).md"; filename*=UTF-8''it%27s%20%28100%25%29.md`},
		{`back\slash.md`, `attachment; filename="back_slash.md"; filename*=UTF-8''back%5Cslash.md`},
	} {
		got := attachmentDisposition(c.name)
		if got != c.want {
			t.Errorf("%q: %s, want %s", c.name, got, c.want)
		}
		// A client that understands both gets the real name back, whatever the name holds.
		if _, params, err := mime.ParseMediaType(got); err != nil || params["filename"] != c.name {
			t.Errorf("%q: parsed as %q, %v", c.name, params["filename"], err)
		}
	}
	// Slashes never reach the name: exportFileName has already turned them into spaces.
	if got := attachmentDisposition(exportFileName("a/b", "id", "md")); !strings.Contains(got, `filename="a b.md"`) {
		t.Errorf("a slash in a title: %s", got)
	}
}
