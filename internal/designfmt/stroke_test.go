package designfmt

import (
	"strings"
	"testing"
)

func TestAStrokeIsVectorMarkupAndInvalidPathsNameTheirNode(t *testing.T) {
	for name, path := range map[string]string{"line": "M0 0L10 20", "curve": "M1 2Q3 4 5 6T7 8", "arc": "M0,0 A5 6 45 0 1 10 20z", "dot": "M2 2L2 2", "relative": "m.5-.5c1e-2 2 3 4 5 6s1 2 3 4h2v-3"} {
		t.Run(name, func(t *testing.T) {
			if err := validateHero(`<svg viewBox="0 0 24 24"><path data-pid="stroke-a" d="` + path + `" stroke="#336699" stroke-width="3" fill="none"/></svg>`); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, path := range []string{"", "L1 2", "M1", "M0 0L", "M0,,0", "M0 0L,1 2", "M0 0,", "M0 0X3 4", "MNaN 0", "M1e999 0", "M1000001 0", "M0 0 A2 2 0 2 1 3 4", "M0 0;alert(1)"} {
		err := validateHero(`<svg><path data-pid="stroke-a" d="` + path + `"/></svg>`)
		if err == nil || !strings.Contains(err.Error(), "stroke-a") {
			t.Errorf("path %q: got %v, want refusal naming stroke-a", path, err)
		}
	}
	for _, width := range []string{"-1", "NaN", "1e999", "1000001", "url(#x)"} {
		if err := validateHero(`<svg><path data-pid="stroke-a" d="M0 0L2 2" stroke-width="` + width + `"/></svg>`); err == nil {
			t.Errorf("width %q accepted", width)
		}
	}
}

func TestVectorPathLimitsAndNativeStrokeWidthsAreKept(t *testing.T) {
	for _, path := range []string{"M1000000 -1000000", "M0 0a1.5 1.5 0 013 0", "M0 0 5 6zM1 1h2v3s1 2 3 4t5 6"} {
		if err := CheckPath(path); err != nil {
			t.Errorf("valid SVG %q: %v", path, err)
		}
	}
	for _, tc := range []struct{ path, want string }{
		{"M0 0" + strings.Repeat("L1 2", 20000), "bytes"},
		{"M0 0" + strings.Repeat("a0 0 0 000 0", 5000), "coordinates"},
		{"M0 0A-1 2 0 0 1 3 4", "negative"},
		{"M0 0Z1 2", "commands"},
	} {
		if err := CheckPath(tc.path); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("got %v, want %s", err, tc.want)
		}
	}
	for _, width := range []string{"0", "0.5", "2px", "50%", "var(--stroke-width)"} {
		if err := validateHero(`<svg><path d="M0 0L1 1" stroke="var(--accent)" stroke-width="` + width + `"/></svg>`); err != nil {
			t.Errorf("width %s: %v", width, err)
		}
	}
}
