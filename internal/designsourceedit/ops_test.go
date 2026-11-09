package designsourceedit_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
)

const (
	source   = `<!DOCTYPE html><html><head><style>/* keep */.x{color:var(--ink)}</style></head><body data-pid="root"><script type="application/x-pictor-props+json">{ "schema": {"title":{"type":"string","maxLength":80},"count":{"type":"number"}}, "values": { "title": "Before", "count": 1 } }</script><script type="text/x-pictor">class Frame extends PictorLogic { renderVals() { return {items:[]}; } }</script><p-for data-pid="repeat" list="{{items}}" as="item" key="{{item.id}}"><p data-pid="line"><strong data-pid="bold">Hello &amp; {{item.name}}!</strong></p></p-for><svg data-pid="svg"><path data-pid="path" d="M0 0L1 1"></path></svg></body></html>`
	manifest = `{"version":2,"runtime":"pictor-design/2.0.0","tokens":{"--ink":"#222"},"pages":[{"id":"main","name":"Main","rows":[[{"id":"source","name":"Source","file":"frames/source.html","w":400,"h":300}]]}]}`
)

func input(text string) designsource.ParseInput {
	return designsource.ParseInput{Format: "2", Manifest: manifest, Files: map[string]string{"frames/source.html": text}}
}

func prop(key, old, value string) designsourceedit.Op {
	return designsourceedit.Op{Kind: "set_prop", File: "frames/source.html", Key: key, Expected: designsourceedit.ValueDigest(json.RawMessage(old)), Value: json.RawMessage(value)}
}

func TestIndependentPropChangesMergeAndStaleOrInvalidBatchesKeepTheEntireDraft(t *testing.T) {
	t.Parallel()
	original := input(source)
	title := prop("title", `"Before"`, `"After"`)
	first, err := designsourceedit.Apply(original, []designsourceedit.Op{title})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(source, `"title": "Before"`, `"title": "After"`, 1)
	if first.Input.Files[title.File] != want || original.Files[title.File] != source {
		t.Fatal("source envelope changed or input mutated")
	}
	second, err := designsourceedit.Apply(first.Input, []designsourceedit.Op{prop("count", `1`, `2`)})
	if err != nil || !strings.Contains(second.Input.Files[title.File], `"title": "After", "count": 2`) {
		t.Fatalf("independent: %+v %v", second, err)
	}
	for _, ops := range [][]designsourceedit.Op{
		{prop("count", `1`, `2`), title},
		{prop("count", `1`, `2`), prop("title", `"After"`, `42`)},
		{prop("missing", ``, `true`)},
		{prop("count", `1`, `2`), prop("count", `2`, `3`)},
	} {
		result, err := designsourceedit.Apply(first.Input, ops)
		if err == nil || result.Input.Files != nil {
			t.Fatalf("partially accepted: %+v %v", result, err)
		}
		var rejected *designsourceedit.Rejected
		if !errors.As(err, &rejected) || len(rejected.Draft) != len(ops) {
			t.Fatalf("draft lost: %v", err)
		}
	}
	undone, err := designsourceedit.Apply(second.Input, first.Inverse)
	if err != nil || !strings.Contains(undone.Input.Files[title.File], `"title": "Before", "count": 2`) {
		t.Fatalf("own undo: %v", err)
	}
	third, err := designsourceedit.Apply(first.Input, []designsourceedit.Op{prop("title", `"After"`, `"Someone else"`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := designsourceedit.Apply(third.Input, first.Inverse); !errors.Is(err, designsourceedit.ErrConflict) {
		t.Fatal("undo overwrote collaborator")
	}
}

func TestLiteralEditsAddressOneTemplateRunAndPreserveBindingsMarksAndUnownedBytes(t *testing.T) {
	t.Parallel()
	op := designsourceedit.Op{Kind: "set_literal", File: "frames/source.html", PID: "bold", TextChild: 0, Segment: 0, Expected: designsourceedit.Digest("Hello &amp; "), Text: "Hi <朋友> "}
	got, err := designsourceedit.Apply(input(source), []designsourceedit.Op{op})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(source, "Hello &amp; ", "Hi &lt;朋友&gt; ", 1)
	if got.Input.Files[op.File] != want {
		t.Fatalf("unexpected source: %s", got.Input.Files[op.File])
	}
	back, err := designsourceedit.Apply(got.Input, got.Inverse)
	if err != nil || back.Input.Files[op.File] != source {
		t.Fatalf("inverse lost exact source: %v", err)
	}
	for _, bad := range []designsourceedit.Op{
		{Kind: "set_literal", File: op.File, PID: "bold", TextChild: 0, Segment: 1, Expected: designsourceedit.Digest("{{item.name}}"), Text: "erase binding"},
		{Kind: "set_literal", File: op.File, PID: "missing", Expected: designsourceedit.Digest(""), Text: "x"},
		{Kind: "set_literal", File: op.File, PID: "bold", TextChild: -1, Expected: op.Expected},
	} {
		if _, err := designsourceedit.Apply(input(source), []designsourceedit.Op{bad}); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}

func TestLogicAndDeclarationsAreOneConditionalUnitAndNeverOverwriteConcurrentProps(t *testing.T) {
	t.Parallel()
	plan, err := designsource.ParseFrame("frames/source.html", source)
	if err != nil {
		t.Fatal(err)
	}
	schema := json.RawMessage(`{"title":{"type":"string","maxLength":80},"count":{"type":"number"}}`)
	op := designsourceedit.Op{Kind: "set_logic", File: plan.File, Expected: designsourceedit.UnitDigest(plan.Logic.Source, schema), Logic: plan.Logic.Source + "\n// changed", Schema: schema}
	one, err := designsourceedit.Apply(input(source), []designsourceedit.Op{op})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := designsourceedit.Apply(one.Input, []designsourceedit.Op{op}); err != nil {
		t.Fatalf("identical retry: %v", err)
	}
	op.Logic += "\n// another writer"
	if _, err := designsourceedit.Apply(one.Input, []designsourceedit.Op{prop("count", `1`, `2`), op}); err == nil {
		t.Fatal("stale logic accepted")
	}
	props, err := designsourceedit.Apply(input(source), []designsourceedit.Op{prop("count", `1`, `2`)})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := designsourceedit.Apply(props.Input, []designsourceedit.Op{op})
	if err != nil || !strings.Contains(merged.Input.Files[plan.File], `"count": 2`) {
		t.Fatalf("props overwritten: %v", err)
	}
}

func TestConditionalUnitsRejectBindingInjectionBadDeclarationsAndAliasedTargets(t *testing.T) {
	t.Parallel()
	for _, ops := range [][]designsourceedit.Op{
		{{Kind: "unknown", File: "frames/source.html", Expected: "digest"}},
		{{Kind: "set_prop", File: "frames/missing.html", Expected: "digest"}},
		{{Kind: "set_prop", File: "frames/source.html", Key: "title", Value: json.RawMessage(`"x"`)}},
		{prop("title", `"Before"`, `"x"`), {Kind: "set_prop", File: "frames/source.html", Key: "title", PID: "alias", Expected: designsourceedit.ValueDigest(json.RawMessage(`"x"`)), Value: json.RawMessage(`"y"`)}},
		{{Kind: "set_literal", File: "frames/source.html", PID: "bold", TextChild: 0, Segment: 0, Expected: designsourceedit.Digest("Hello &amp; "), Text: "{{item.name}}", RawText: stringPointer("{{item.name}}")}},
		{prop("count", `1`, `{} trailing`)},
	} {
		if _, err := designsourceedit.Apply(input(source), ops); err == nil {
			t.Fatalf("accepted %+v", ops)
		}
	}
	schema := json.RawMessage(`{"title":{"type":"string","maxLength":80},"count":{"type":"number"}}`)
	plan, err := designsource.ParseFrame("frames/source.html", source)
	if err != nil {
		t.Fatal(err)
	}
	changed := designsourceedit.Op{Kind: "set_logic", File: plan.File, Expected: designsourceedit.UnitDigest(plan.Logic.Source, schema), Logic: plan.Logic.Source, Schema: json.RawMessage(`{"title":{"type":"number"},"count":{"type":"number"}}`)}
	if _, err := designsourceedit.Apply(input(source), []designsourceedit.Op{changed}); err == nil {
		t.Fatal("incompatible schema accepted against authored values")
	}
	changed.Schema = json.RawMessage(`{"title":{"type":"string","maxLength":90},"count":{"type":"number"}}`)
	got, err := designsourceedit.Apply(input(source), []designsourceedit.Op{changed})
	if err != nil {
		t.Fatal(err)
	}
	back, err := designsourceedit.Apply(got.Input, got.Inverse)
	if err != nil || back.Input.Files[plan.File] != source {
		t.Fatalf("declaration undo: %v", err)
	}
	// A changed declaration participates even when the competing writer changes only logic.
	changed.Schema = schema
	changed.Logic += "\n// competitor"
	if _, err := designsourceedit.Apply(got.Input, []designsourceedit.Op{changed}); err == nil {
		t.Fatal("stale declaration ignored")
	}
}

func TestAuthoredPropAbsenceAndNullHaveDifferentDigestsAndUndoKeepsSource(t *testing.T) {
	t.Parallel()
	if designsourceedit.ValueDigest(nil) == designsourceedit.ValueDigest(json.RawMessage(`null`)) {
		t.Fatal("absence aliased to null")
	}
	absent := strings.Replace(source, `"title": "Before", `, "", 1)
	add := prop("title", ``, `"Added"`)
	got, err := designsourceedit.Apply(input(absent), []designsourceedit.Op{add})
	if err != nil {
		t.Fatal(err)
	}
	back, err := designsourceedit.Apply(got.Input, got.Inverse)
	if err != nil || back.Input.Files[add.File] != absent {
		t.Fatalf("add/remove changed source: %s %v", back.Input.Files[add.File], err)
	}
}

func TestDeletingWholeLiteralRunsKeepsTheirOwnUndoTargetsAcrossTransport(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		before             string
		textChild, segment int
		raw                string
	}{
		{source, 0, 0, "Hello &amp; "},
		{strings.Replace(source, "Hello &amp; {{item.name}}!", "Just &#38; text", 1), 0, 0, "Just &#38; text"},
		{strings.Replace(source, "Hello &amp; {{item.name}}!", "{{item.name}}!", 1), 0, 1, "!"},
	} {
		op := designsourceedit.Op{Kind: "set_literal", File: "frames/source.html", PID: "bold", TextChild: c.textChild, Segment: c.segment, Expected: designsourceedit.Digest(c.raw), Text: ""}
		deleted, err := designsourceedit.Apply(input(c.before), []designsourceedit.Op{op})
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.Marshal(deleted.Inverse)
		if err != nil {
			t.Fatal(err)
		}
		var inverse []designsourceedit.Op
		if err := json.Unmarshal(wire, &inverse); err != nil {
			t.Fatal(err)
		}
		back, err := designsourceedit.Apply(deleted.Input, inverse)
		if err != nil || back.Input.Files[op.File] != c.before {
			t.Fatalf("empty run undo lost source: %v", err)
		}
	}
}

func TestSchemaAndAuthoredValueTransitionsValidateTheFinalBatchInEitherOrder(t *testing.T) {
	t.Parallel()
	plan, err := designsource.ParseFrame("frames/source.html", source)
	if err != nil {
		t.Fatal(err)
	}
	schema := json.RawMessage(`{"title":{"type":"string","maxLength":80},"count":{"type":"number"}}`)
	logic := designsourceedit.Op{Kind: "set_logic", File: plan.File, Expected: designsourceedit.UnitDigest(plan.Logic.Source, schema), Logic: plan.Logic.Source, Schema: json.RawMessage(`{"title":{"type":"number"},"count":{"type":"number"}}`)}
	value := prop("title", `"Before"`, `42`)
	for _, ops := range [][]designsourceedit.Op{{logic, value}, {value, logic}} {
		got, err := designsourceedit.Apply(input(source), ops)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := designsource.ParseFrame(plan.File, got.Input.Files[plan.File])
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Props.Schema["title"].Type != "number" || parsed.Props.Values["title"] != json.Number("42") {
			t.Fatal("batch lost schema/value")
		}
		back, err := designsourceedit.Apply(got.Input, got.Inverse)
		if err != nil || back.Input.Files[plan.File] != source {
			t.Fatalf("batch own Undo failed: %v", err)
		}
	}
}

func stringPointer(value string) *string { return &value }

func TestTypingTemplateDelimitersKeepsThemLiteralAndRoundTripsOriginalEntityBytes(t *testing.T) {
	t.Parallel()
	encoded := strings.Replace(source, "Hello &amp; ", "&#123;&#123;not.a.binding&#125;&#125; &amp; ", 1)
	raw := "&#123;&#123;not.a.binding&#125;&#125; &amp; "
	op := designsourceedit.Op{Kind: "set_literal", File: "frames/source.html", PID: "bold", TextChild: 0, Segment: 0, Expected: designsourceedit.Digest(raw), Text: "{{literal}}"}
	got, err := designsourceedit.Apply(input(encoded), []designsourceedit.Op{op})
	if err != nil {
		t.Fatal(err)
	}
	if got.Input.Files[op.File] != strings.Replace(encoded, raw, "&#123;&#123;literal&#125;&#125;", 1) {
		t.Fatal("typed delimiters became binding syntax")
	}
	back, err := designsourceedit.Apply(got.Input, got.Inverse)
	if err != nil || back.Input.Files[op.File] != encoded {
		t.Fatalf("entity undo: %v", err)
	}
}

func TestOpaqueCommentsSeparateSourceLiteralTargetsAndWholeRunUndo(t *testing.T) {
	t.Parallel()
	before := strings.Replace(source, "Hello &amp; {{item.name}}!", "First<!--keep bytes-->Second", 1)
	op := designsourceedit.Op{Kind: "set_literal", File: "frames/source.html", PID: "bold", TextChild: 1, Segment: 0, Expected: designsourceedit.Digest("Second"), Text: ""}
	deleted, err := designsourceedit.Apply(input(before), []designsourceedit.Op{op})
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Input.Files[op.File] != strings.Replace(before, "Second", "", 1) {
		t.Fatal("edited another source run or removed comment")
	}
	back, err := designsourceedit.Apply(deleted.Input, deleted.Inverse)
	if err != nil || back.Input.Files[op.File] != before {
		t.Fatalf("comment boundary undo: %v", err)
	}
}

func TestDeclaredPropsWithoutAnAuthoredValuesObjectAcceptConditionalOverrides(t *testing.T) {
	t.Parallel()
	before := strings.Replace(source, `, "values": { "title": "Before", "count": 1 }`, "", 1)
	op := prop("title", ``, `"Authored"`)
	got, err := designsourceedit.Apply(input(before), []designsourceedit.Op{op})
	if err != nil {
		t.Fatal(err)
	}
	back, err := designsourceedit.Apply(got.Input, got.Inverse)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := designsource.ParseFrame(op.File, back.Input.Files[op.File])
	if err != nil {
		t.Fatal(err)
	}
	if _, present := plan.Props.Values["title"]; present {
		t.Fatal("own Undo retained authored override")
	}
}

func TestLogicWithoutPropsKeepsAbsentDeclarationsAndOwnUndo(t *testing.T) {
	t.Parallel()
	code := `class Frame extends PictorLogic { renderVals() { return {}; } }`
	frame := `<body data-pid="root"><script type="text/x-pictor">` + code + `</script><p data-pid="p">Text</p></body>`
	op := designsourceedit.Op{Kind: "set_logic", File: "frames/source.html", Expected: designsourceedit.UnitDigest(code, nil), Logic: code + "\n// own change"}
	got, err := designsourceedit.Apply(input(frame), []designsourceedit.Op{op})
	if err != nil {
		t.Fatal(err)
	}
	back, err := designsourceedit.Apply(got.Input, got.Inverse)
	if err != nil || back.Input.Files[op.File] != frame {
		t.Fatalf("logic Undo created props or changed other source: %v", err)
	}
}
