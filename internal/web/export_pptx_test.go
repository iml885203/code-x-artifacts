package web

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// The agent path: an agent learns from kinds that a deck downloads as pptx,
// publishes one over MCP, and downloads it with the same token.
func TestAnAgentPublishesADeckAndDownloadsItAsAPowerPointFileNamedAfterItsTitle(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	ag := h.agent(tok)
	for _, k := range ag.ok("kinds", nil)["kinds"].([]any) {
		if m := k.(map[string]any); m["name"] == "slides" && fmt.Sprint(m["exportFormats"]) != "[pptx]" {
			t.Errorf("kinds says slides export as %v", m["exportFormats"])
		}
	}
	id, _ := ag.ok("publish", map[string]any{"title": "季報 Deck", "kind": "slides", "content": threeSlides})["id"].(string)

	res, body := api(h, "GET", "/api/artifacts/"+id+"/export/pptx", "", tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("export: %d %s", res.StatusCode, body)
	}
	if got := res.Header.Get("Content-Type"); got != "application/vnd.openxmlformats-officedocument.presentationml.presentation" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := res.Header.Get("Content-Disposition"); !strings.Contains(got, "attachment") || !strings.Contains(got, "%E5%AD%A3%E5%A0%B1%20Deck.pptx") {
		t.Errorf("Content-Disposition = %q", got)
	}
	zr, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	if err != nil {
		t.Fatalf("the download is not a zip: %v", err)
	}
	parts := map[string]bool{}
	for _, f := range zr.File {
		parts[f.Name] = true
	}
	for _, want := range []string{"ppt/presentation.xml", "ppt/slides/slide1.xml", "ppt/slides/slide3.xml", "ppt/notesSlides/notesSlide3.xml"} {
		if !parts[want] {
			t.Errorf("the file has no %s", want)
		}
	}
	if parts["ppt/notesSlides/notesSlide2.xml"] {
		t.Error("the slide without notes has a notes slide")
	}

	// Someone who may not see the deck gets the same answer as for a deck that is not there.
	if res, _ := api(h, "GET", "/api/artifacts/"+id+"/export/pptx", "", h.token("bob")); res.StatusCode != http.StatusNotFound {
		t.Errorf("another person's export of a private deck: %d", res.StatusCode)
	}
	if res, _ := api(h, "GET", "/api/artifacts/"+id+"/export/docx", "", tok); res.StatusCode != http.StatusNotFound {
		t.Errorf("a deck exported as Word: %d", res.StatusCode)
	}
}
