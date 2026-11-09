package web

import (
	"net/http"
	"net/url"
	"testing"
)

func TestFullPageUsesTheViewerShellAndItsExistingAccessRules(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publish(h.token("alice"), "Private page", "<p>private content</p>")
	for name, client := range map[string]*http.Client{
		"author":    h.browser("alice", "pw-a"),
		"other":     h.browser("bob", "pw-b"),
		"anonymous": h.client(nil),
	} {
		ordinary, _ := h.do(client, "GET", "/a/"+a.ID, "", nil)
		for _, query := range []string{"?view=full", "?view=full&version=1"} {
			res, body := h.do(client, "GET", "/a/"+a.ID+query, "", nil)
			if res.StatusCode != ordinary.StatusCode || res.Header.Get("Content-Security-Policy") != ordinary.Header.Get("Content-Security-Policy") {
				t.Errorf("%s %s: status or CSP differs from the ordinary viewer", name, query)
			}
			if res.StatusCode == http.StatusSeeOther {
				if want := "/login?next=" + url.QueryEscape("/a/"+a.ID+query); res.Header.Get("Location") != want {
					t.Errorf("the sign-in redirect lost the full-page destination: %q", res.Header.Get("Location"))
				}
				continue
			}
			b := bootOf(t, body)
			if name == "author" {
				if b.Page != "viewer" || b.Artifact == nil || b.Artifact.ID != a.ID || b.Here != "/a/"+a.ID+query {
					t.Errorf("the full-page choice lost the authorized boot: %+v", b)
				}
			} else if b.Artifact != nil {
				t.Errorf("%s learned the private artifact through full-page mode", name)
			}
		}
	}
}
