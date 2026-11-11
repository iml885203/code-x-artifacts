package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

const previewPage = "<!DOCTYPE html><html><head><meta charset=utf-8></head><body><p>on the card</p></body></html>"

// sandboxOf is the sandbox directive of a policy: what makes the page an
// opaque origin.
func sandboxOf(csp string) string { return strings.SplitN(csp, ";", 2)[0] }

func TestAPreviewIsTheSamePageInTheSameSandboxAndCannotReachTheBridge(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Report", previewPage)
	bob := h.browser("bob", "pw-b")

	full, fullBody := h.do(bob, "GET", "/raw/"+a.ID, "", nil)
	card, cardBody := h.do(bob, "GET", "/raw/"+a.ID+"?preview", "", nil)
	if card.StatusCode != http.StatusOK || !strings.Contains(cardBody, "on the card") {
		t.Fatalf("preview: %d %.200s", card.StatusCode, cardBody)
	}
	// An author's page has no script-src, so the whole policy is the same.
	if got, want := card.Header.Get("Content-Security-Policy"), full.Header.Get("Content-Security-Policy"); got != want || strings.Contains(got, "allow-same-origin") {
		t.Errorf("preview policy %q, the page's is %q", got, want)
	}
	if !strings.Contains(fullBody, "postMessage") {
		t.Fatal("the opened page has lost its bridge")
	}
	// Preview capture speaks only its own protocol; it never installs the viewer's API bridge.
	if strings.Contains(cardBody, bridgeJS) || !strings.Contains(cardBody, previewCaptureJS) {
		t.Error("preview capture replaced or exposed the viewer bridge")
	}
	if !strings.Contains(cardBody, "window.pictor") {
		t.Error("a page that reads window.pictor throws in a preview")
	}
}

func TestAPreviewShowsAPageDrawnByScriptsUnderTheSandboxOfThePageAndNeverTheBridge(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishReact(tok, reactSource)
	bob := h.browser("bob", "pw-b")

	full, _ := h.do(bob, "GET", "/raw/"+a.ID, "", nil)
	card, body := h.do(bob, "GET", "/raw/"+a.ID+"?preview", "", nil)
	if card.StatusCode != http.StatusOK {
		t.Fatalf("preview: %d %.200s", card.StatusCode, body)
	}
	csp, fullCSP := card.Header.Get("Content-Security-Policy"), full.Header.Get("Content-Security-Policy")
	if sandboxOf(csp) != sandboxOf(fullCSP) || strings.Contains(csp, "allow-same-origin") {
		t.Errorf("preview sandbox %q, the page's is %q", sandboxOf(csp), sandboxOf(fullCSP))
	}
	// Scripts are allowed by hash and by path, as in the page, and the bridge's hash is not among them.
	scriptSrc := regexp.MustCompile(`script-src ([^;]*)`).FindStringSubmatch(csp)[1]
	if strings.Contains(scriptSrc, scriptHash(bridgeJS)) || !strings.Contains(scriptSrc, scriptHash(previewJS)) || strings.Contains(scriptSrc, "'self'") {
		t.Errorf("preview script-src: %q", csp)
	}
	if strings.Contains(body, bridgeJS) {
		t.Error("a preview of a react page carries the bridge")
	}
}

func TestAPreviewIsRefusedToWhoCannotOpenThePageAndKeepsTheSameAnswer(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publish(h.token("alice"), "Private", previewPage)
	bob := h.browser("bob", "pw-b")

	plain, plainBody := h.do(bob, "GET", "/raw/"+a.ID, "", nil)
	card, cardBody := h.do(bob, "GET", "/raw/"+a.ID+"?preview", "", nil)
	if card.StatusCode == http.StatusOK || card.StatusCode != plain.StatusCode || cardBody != plainBody || strings.Contains(cardBody, "on the card") {
		t.Errorf("bob on a private page: preview %d %q, page %d %q", card.StatusCode, cardBody, plain.StatusCode, plainBody)
	}
}

func TestAPreviewOfAPageWithFilesKeepsBeingAPreviewAcrossTheGrantRedirect(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishWith(h.token("alice"), previewPage, map[string]string{"style.css": "p{color:red}"}, store.VisibilityOrganisation)
	bob := h.browser("bob", "pw-b")

	res, _ := h.do(bob, "GET", "/raw/"+a.ID+"?preview", "", nil)
	to := res.Header.Get("Location")
	if res.StatusCode != http.StatusFound || !regexp.MustCompile(`^/raw/`+a.ID+`/[^/]+/\?preview$`).MatchString(to) {
		t.Fatalf("redirect: %d %q", res.StatusCode, to)
	}
	granted, body := h.do(bob, "GET", to, "", nil)
	if granted.StatusCode != http.StatusOK || !strings.Contains(body, "on the card") || strings.Contains(body, bridgeJS) {
		t.Errorf("the granted preview: %d %.300s", granted.StatusCode, body)
	}
}
