package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestOnlyDesignPreviewFramesReceiveTheCaptureProtocolAndItsHash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishDesign(h.token("alice"), designFrames(), store.VisibilityOrganisation)
	address := h.pageAddress(h.browser("bob", "pw-b"), a.ID, "") + "frames/hero.html"
	full, fullBody := h.do(nil, "GET", address, "", nil)
	preview, previewBody := h.do(nil, "GET", address+"?preview", "", nil)
	if full.StatusCode != http.StatusOK || preview.StatusCode != http.StatusOK {
		t.Fatal("frame not served")
	}
	if strings.Contains(fullBody, previewCaptureJS) || strings.Contains(full.Header.Get("Content-Security-Policy"), scriptHash(testThumbnailScript(t))) {
		t.Fatal("full design frame was changed")
	}
	if !strings.Contains(previewBody, previewCaptureJS) || !strings.Contains(preview.Header.Get("Content-Security-Policy"), scriptHash(testThumbnailScript(t))) {
		t.Fatal("preview capture not installed and pinned")
	}
	if strings.Contains(full.Header.Get("Content-Security-Policy"), "connect-src") || !strings.Contains(preview.Header.Get("Content-Security-Policy"), "connect-src ") {
		t.Fatal("capture resource reads changed full frame policy or were not scoped to previews")
	}
	if strings.Contains(previewBody, bridgeJS) || strings.Contains(previewBody, previewStubJS) || strings.Contains(preview.Header.Get("Content-Security-Policy"), "allow-same-origin") {
		t.Fatal("preview frame acquired API bridge or same-origin authority")
	}
}
