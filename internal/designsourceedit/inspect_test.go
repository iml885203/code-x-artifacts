package designsourceedit_test

import (
	"encoding/json"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
)

func TestInspectionReturnsAuthoredUnitDigestsAndExactLiteralSourceCoordinates(t *testing.T) {
	t.Parallel()
	plan, err := designsource.ParseFrame("frames/source.html", source)
	if err != nil {
		t.Fatal(err)
	}
	units, err := designsourceedit.Inspect(plan)
	if err != nil {
		t.Fatal(err)
	}
	schema := json.RawMessage(`{"title":{"type":"string","maxLength":80},"count":{"type":"number"}}`)
	if units.LogicDeclarationSHA256 != designsourceedit.UnitDigest(plan.Logic.Source, schema) || !units.Props["title"].Present || units.Props["title"].ExpectedSHA256 != designsourceedit.ValueDigest(json.RawMessage(`"Before"`)) {
		t.Fatal("authored unit hashes differ")
	}
	found := false
	for _, literal := range units.Literals {
		if literal.PID != "bold" || literal.Segment != 0 {
			continue
		}
		found = true
		if literal.File != plan.File || literal.TextChild != 0 || source[literal.Span.StartByte:literal.Span.EndByte] != "Hello &amp; " || literal.ExpectedSHA256 != designsourceedit.Digest("Hello &amp; ") {
			t.Fatal("literal lost source byte coordinates")
		}
	}
	if !found {
		t.Fatal("missing literal metadata")
	}
}

func TestInspectionDistinguishesAbsentOverridesFromDefaultsAndHasNoWriteToken(t *testing.T) {
	t.Parallel()
	frame := `<body data-pid="root"><script type="application/x-pictor-props+json">{"schema":{"title":{"type":"string","default":"Default","maxLength":80}},"values":{}}</script><p data-pid="text">Hi</p></body>`
	plan, err := designsource.ParseFrame("frames/a.html", frame)
	if err != nil {
		t.Fatal(err)
	}
	units, err := designsourceedit.Inspect(plan)
	if err != nil {
		t.Fatal(err)
	}
	if units.Props["title"].Present || units.Props["title"].ExpectedSHA256 != designsourceedit.ValueDigest(nil) {
		t.Fatal("default mistaken for authored value")
	}
	raw, err := json.Marshal(units)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["sourceRevision"]; exists {
		t.Fatal("inspection issued write authority")
	}
	empty, err := designsource.ParseFrame("frames/b.html", `<p data-pid="p">Static</p>`)
	if err != nil {
		t.Fatal(err)
	}
	emptyUnits, err := designsourceedit.Inspect(empty)
	if err != nil {
		t.Fatal(err)
	}
	if emptyUnits.LogicDeclarationSHA256 != designsourceedit.UnitDigest("", nil) || len(emptyUnits.Props) != 0 {
		t.Fatal("absent declaration digest is implicit")
	}
}
