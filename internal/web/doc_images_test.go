package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestRestoringAnOlderVersionWhoseImageWasDeletedShowsThePlaceholderNotABrokenPicture(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	res, out := h.do(nil, "POST", "/api/artifacts", input(t, artifactInput{Title: "Doc", Kind: "doc", Content: "# Doc\n"}), h.bearer("alice"))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("%d %s", res.StatusCode, out)
	}
	a := decode[artifactView](t, out)
	up := h.mustUpload(tok, a.ID, "chart.png", pngBytes)
	h.mustUpload(tok, a.ID, "kept.png", pngBytes)
	withImages := "# Doc\n\n![Revenue](" + up.URL + " \"small\")\n\n![Kept](/files/" + a.ID + "/kept.png)\n"
	if res, out := api(h, "PUT", "/api/artifacts/"+a.ID, input(t, artifactInput{Title: "Doc", Content: withImages}), tok); res.StatusCode != http.StatusOK {
		t.Fatalf("%d %s", res.StatusCode, out)
	}
	if res, out := api(h, "PUT", "/api/artifacts/"+a.ID, input(t, artifactInput{Title: "Doc", Content: "# Doc\n\nno pictures\n"}), tok); res.StatusCode != http.StatusOK {
		t.Fatalf("%d %s", res.StatusCode, out)
	}
	if res, _ := api(h, "DELETE", "/api/artifacts/"+a.ID+"/assets/chart.png", "", tok); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete the asset: %d", res.StatusCode)
	}
	if res, out := api(h, "POST", "/api/artifacts/"+a.ID+"/versions/2/restore", "", tok); res.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d %s", res.StatusCode, out)
	}

	alice := h.browser("alice", "pw-a")
	for _, path := range []string{"/raw/" + a.ID, "/raw/" + a.ID + "?version=2"} {
		_, page := h.do(alice, "GET", path, "", nil)
		if !strings.Contains(page, `<p class="missing"`) || !strings.Contains(page, "Picture deleted: Revenue") {
			t.Errorf("%s: the deleted picture is not a placeholder:\n%s", path, page)
		}
		if strings.Contains(page, "chart.png") {
			t.Errorf("%s: the deleted file is still named in the page", path)
		}
		if !strings.Contains(page, `alt="Kept"`) || !strings.Contains(page, "/files/"+a.ID+"/") {
			t.Errorf("%s: a picture whose file is there was changed:\n%s", path, page)
		}
	}
}
