package designfmt

import (
	"strings"
	"testing"
)

func TestALinkToHTTPSOrMailtoIsAcceptedAndOpensInANewTabThatKnowsNothingOfThePage(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"https":                {`<a href="https://x.test/a?b=1">x</a>`, `href="https://x.test/a?b=1" rel="noopener noreferrer" target="_blank"`},
		"mailto":               {`<a href="mailto:a@x.test">x</a>`, `rel="noopener noreferrer" target="_blank"`},
		"upper case scheme":    {`<a href="HTTPS://x.test/">x</a>`, `rel="noopener noreferrer" target="_blank"`},
		"own target is forced": {`<a href="https://x.test/" target="_self" rel="opener">x</a>`, `rel="noopener noreferrer" target="_blank"`},
		"a link in text":       {`<p>go <b><a href="https://x.test/">there</a></b></p>`, `rel="noopener noreferrer" target="_blank"`},
	} {
		if err := validateHero(tc.in); err != nil {
			t.Errorf("%s: refused: %v", name, err)
			continue
		}
		out, err := Normalize(frameDoc(tc.in))
		if err != nil || !strings.Contains(out, tc.want) {
			t.Errorf("%s: normalised to %q (%v), want %q", name, out, err, tc.want)
		}
		if again, _ := Normalize(out); again != out {
			t.Errorf("%s: normalising twice differs", name)
		}
	}
	// A link inside the frame is left as it is.
	out, _ := Normalize(frameDoc(`<a href="#top">x</a>`))
	if strings.Contains(out, "target") || strings.Contains(out, "rel=") {
		t.Errorf("a fragment link was changed: %s", out)
	}
}

func TestAFrameOverTheSizeLimitIsToldToUploadAndWhichPictureIsTheBiggest(t *testing.T) {
	big := `<img alt="" src="data:image/png;base64,` + strings.Repeat("A", 300<<10) + `">`
	small := `<img alt="" src="data:image/png;base64,AAAA">`
	err := validateHero(small + big + big + `<p>text</p>`)
	if err == nil {
		t.Fatal("an oversized frame validated")
	}
	for _, want := range []string{"512 KB", "upload_asset", "reference the returned URL", "largest data: address is 301 KB, a data:image/png"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
	err = validateHero(strings.Repeat("a", MaxFrameBytes))
	if err == nil || !strings.Contains(err.Error(), "upload_asset") || strings.Contains(err.Error(), "largest data") {
		t.Errorf("a frame of text is told to upload but not about a picture it has none of: %v", err)
	}
}
