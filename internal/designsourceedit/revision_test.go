package designsourceedit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
)

func TestReadBasesAreExplicitAndBoundToTheirExactSourceAndContext(t *testing.T) {
	t.Parallel()
	issuer, err := designsourceedit.NewRevisionIssuer()
	if err != nil {
		t.Fatal(err)
	}
	basis := designsourceedit.Basis{ArtifactID: "artifact", File: "frames/card.html", Format: "1", SavedVersion: 3, LiveEpoch: "epoch", Principal: "alice"}
	first, err := issuer.Issue(basis, "<p>first</p>")
	if err != nil {
		t.Fatal(err)
	}
	second, err := issuer.Issue(basis, "<p>second</p>")
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct{ token, source string }{{first, "<p>first</p>"}, {second, "<p>second</p>"}} {
		if err := issuer.Verify(pair.token, basis, pair.source); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name, token, source string
		basis               designsourceedit.Basis
	}{
		{"missing", "", "<p>first</p>", basis},
		{"other bytes", first, "<p>second</p>", basis},
		{"truncated", first[:len(first)-1], "<p>first</p>", basis},
		{"forged", strings.Repeat("A", len(first)), "<p>first</p>", basis},
	}
	for _, field := range []string{"artifact", "file", "format", "version", "epoch", "principal"} {
		changed := changedBasis(basis, field)
		cases = append(cases, struct {
			name, token, source string
			basis               designsourceedit.Basis
		}{field, first, "<p>first</p>", changed})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := issuer.Verify(c.token, c.basis, c.source); !errors.Is(err, designsourceedit.ErrSourceBase) {
				t.Fatalf("got %v", err)
			}
		})
	}
	restarted, err := designsourceedit.NewRevisionIssuer()
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Verify(first, basis, "<p>first</p>"); !errors.Is(err, designsourceedit.ErrSourceBase) {
		t.Fatalf("restart: %v", err)
	}
}

func TestAReadBaseCannotBeIssuedForAnIncompleteOrAmbiguousContext(t *testing.T) {
	t.Parallel()
	issuer, err := designsourceedit.NewRevisionIssuer()
	if err != nil {
		t.Fatal(err)
	}
	for _, basis := range []designsourceedit.Basis{
		{},
		{ArtifactID: "a", File: "../escape", Format: "1", SavedVersion: 1, LiveEpoch: "e", Principal: "a"},
		{ArtifactID: "a", File: "frames/a.html", Format: "3", SavedVersion: 1, LiveEpoch: "e", Principal: "a"},
		{ArtifactID: "a", File: "frames/a.html", Format: "2", SavedVersion: 0, LiveEpoch: "e", Principal: "a"},
		{ArtifactID: "a", File: "frames/a.html", Format: "2", SavedVersion: 1, Principal: "a"},
	} {
		if _, err := issuer.Issue(basis, ""); !errors.Is(err, designsourceedit.ErrSourceBase) {
			t.Fatalf("%+v: %v", basis, err)
		}
	}
}

func changedBasis(basis designsourceedit.Basis, field string) designsourceedit.Basis {
	changed := basis
	switch field {
	case "artifact":
		changed.ArtifactID = "other"
	case "file":
		changed.File = "frames/other.html"
	case "format":
		if basis.Format == "1" {
			changed.Format = "2"
		} else {
			changed.Format = "1"
		}
	case "version":
		changed.SavedVersion++
	case "epoch":
		changed.LiveEpoch = "restored"
	case "principal":
		changed.Principal = "bob"
	}
	return changed
}
