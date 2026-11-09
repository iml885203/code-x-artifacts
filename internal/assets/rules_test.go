package assets

import (
	"bytes"
	"strings"
	"testing"
)

var (
	png  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")
	jpeg = []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00")
	gif  = []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!")
	webp = []byte("RIFF\x1a\x00\x00\x00WEBPVP8 ")
	pdf  = []byte("%PDF-1.7\n1 0 obj\n")
	woff = []byte("wOFF\x00\x01\x00\x00\x00\x00\x00\x10")
	svg  = []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"><rect width="1" height="1"/></svg>`)
	csv  = []byte("name,count\nalice,3\nbob,5\n")
)

func TestAFileIsWhatItsBytesSayAndItsNameAgrees(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"chart.png", png, "image/png"},
		{"photo.JPG", jpeg, "image/jpeg"},
		{"photo.jpeg", jpeg, "image/jpeg"},
		{"anim.gif", gif, "image/gif"},
		{"pic.webp", webp, "image/webp"},
		{"logo.svg", svg, "image/svg+xml"},
		{"paper.pdf", pdf, "application/pdf"},
		{"font.woff", woff, "font/woff"},
		{"data.csv", csv, "text/csv; charset=utf-8"},
	}
	for _, c := range cases {
		got, err := Detect(c.name, c.data)
		if err != nil || got.ContentType != c.want {
			t.Errorf("%s: %v, %v; want %s", c.name, got.ContentType, err, c.want)
		}
	}
}

func TestFilesThatAreNotOnTheAllowListAreRefusedWithAReason(t *testing.T) {
	cases := []struct {
		label string
		name  string
		data  []byte
		want  string
	}{
		{"an empty file", "a.png", nil, "empty"},
		{"HTML named as a picture", "a.png", []byte("<html><script>alert(1)</script>"), "not an accepted type"},
		{"a script named as a picture", "a.png", []byte("alert(1)"), "not an accepted type"},
		{"HTML", "a.html", []byte("<!doctype html><p>hi"), "not an accepted type"},
		{"a binary program", "a.exe", []byte("MZ\x90\x00\x03\x00\x00\x00"), "not an accepted type"},
		{"plain text that is not named csv", "notes.txt", []byte("hello"), "not an accepted type"},
		{"a picture whose name says another type", "chart.pdf", png, "image/png"},
		{"a pdf named as csv", "a.csv", pdf, "application/pdf"},
		{"no extension", "chart", png, "name it with .png"},
		{"binary data named csv", "a.csv", []byte("a,b\x00\x01\x02"), "not an accepted type"},
		{"an SVG that declares entities", "a.svg", []byte(`<?xml version="1.0"?><!DOCTYPE svg [<!ENTITY x "y">]><svg xmlns="http://www.w3.org/2000/svg"/>`), "not an accepted type"},
		{"XML that is not an SVG", "a.svg", []byte(`<?xml version="1.0"?><note>hi</note>`), "not an accepted type"},
		{"an SVG without its namespace", "a.svg", []byte(`<svg><rect/></svg>`), "not an accepted type"},
	}
	for _, c := range cases {
		_, err := Detect(c.name, c.data)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", c.label, err, c.want)
		}
	}
}

func TestOnlyActiveTypesAreServedUnderASandboxPolicy(t *testing.T) {
	for _, c := range []struct {
		contentType string
		active      bool
	}{{"image/svg+xml", true}, {"image/png", false}, {"application/pdf", false}, {"text/csv; charset=utf-8", false}} {
		if got, ok := TypeOf(c.contentType); !ok || got.Active != c.active {
			t.Errorf("%s: %+v %v, want active=%v", c.contentType, got, ok, c.active)
		}
	}
}

func TestANameKeepsOnlyWhatIsSafeInAPathAndInAnAttribute(t *testing.T) {
	long := strings.Repeat("a", 300) + ".png"
	cases := []struct{ given, want string }{
		{"chart.png", "chart.png"},
		{"Q3 Revenue (final).PNG", "Q3-Revenue-final-.PNG"},
		{"../../etc/passwd.png", "passwd.png"},
		{`C:\Users\me\pic.jpg`, "pic.jpg"},
		{"/abs/path/pic.gif", "pic.gif"},
		{".hidden.png", "hidden.png"},
		{`a"onerror="x.png`, "a-onerror-x.png"},
		{"a<b>.png", "a-b-.png"},
	}
	for _, c := range cases {
		if got, err := Name(c.given); err != nil || got != c.want {
			t.Errorf("Name(%q) = %q, %v; want %q", c.given, got, err, c.want)
		}
	}
	got, err := Name(long)
	if err != nil || len(got) != MaxNameLength || !strings.HasSuffix(got, ".png") {
		t.Errorf("a long name became %q (%d), %v", got, len(got), err)
	}
	for _, bad := range []string{"", ".", "..", "/", "noextension", "報告.", "報告圖表.png", "...png-"} {
		if got, err := Name(bad); err == nil {
			t.Errorf("Name(%q) = %q, want a refusal", bad, got)
		}
	}
}

func TestADiskKeepsEachArtifactsFilesApartAndRefusesAnyPathOutsideIt(t *testing.T) {
	d, err := OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Write("art1", "k1", []byte("one")); err != nil {
		t.Fatal(err)
	}
	f, err := d.Open("art1", "k1")
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if _, err := got.ReadFrom(f); err != nil || got.String() != "one" {
		t.Errorf("read back %q, %v", got.String(), err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][2]string{{"..", "k"}, {"art1", "../k1"}, {"a/b", "k"}, {"", "k"}, {"art1", ""}} {
		if err := d.Write(bad[0], bad[1], []byte("x")); err == nil {
			t.Errorf("Write(%q, %q) succeeded", bad[0], bad[1])
		}
	}
	if err := d.RemoveArtifact("art1"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Open("art1", "k1"); err == nil {
		t.Error("a removed artifact's file is still there")
	}
	if err := d.Remove("art1", "k1"); err != nil {
		t.Errorf("removing a file that is gone: %v", err)
	}
}
