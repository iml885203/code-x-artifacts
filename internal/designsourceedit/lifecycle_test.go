package designsourceedit_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
)

const (
	simple  = `<body data-pid="root"><p data-pid="p">Text &#38; bytes</p></body>`
	classic = `class Frame extends PictorLogic { renderVals() { return {}; } }`
)

func unit(t *testing.T, text string) string {
	t.Helper()
	p, err := designsource.ParseFrame("frames/source.html", text)
	if err != nil {
		t.Fatal(err)
	}
	u, err := designsourceedit.Inspect(p)
	if err != nil {
		t.Fatal(err)
	}
	return u.LogicDeclarationSHA256
}

func wireUndo(t *testing.T, ops []designsourceedit.Op) []designsourceedit.Op {
	t.Helper()
	b, err := json.Marshal(ops)
	if err != nil {
		t.Fatal(err)
	}
	var out []designsourceedit.Op
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAbsentLogicAndPropsCanBeCreatedAndDeletedWithConditionalExactUndo(t *testing.T) {
	t.Parallel()
	for _, schema := range []string{"", `{}`, `{"count":{"type":"number","default":1}}`} {
		for _, logic := range []string{"", classic} {
			if schema == "" && logic == "" {
				continue
			}
			op := designsourceedit.Op{Kind: "set_logic", File: "frames/source.html", Expected: designsourceedit.UnitDigest("", nil), Logic: logic, Schema: json.RawMessage(schema)}
			got, err := designsourceedit.Apply(input(simple), []designsourceedit.Op{op})
			if err != nil {
				t.Fatal(err)
			}
			back, err := designsourceedit.Apply(got.Input, wireUndo(t, got.Inverse))
			if err != nil || back.Input.Files[op.File] != simple {
				t.Fatalf("creation undo: %s %v", back.Input.Files[op.File], err)
			}
			again, err := designsourceedit.Apply(back.Input, wireUndo(t, back.Inverse))
			if err != nil || again.Input.Files[op.File] != got.Input.Files[op.File] {
				t.Fatalf("redo: %v", err)
			}
		}
	}
}

func TestBlockDeletionUndoPreservesExactDeclarationsAndUnrelatedConcurrentSource(t *testing.T) {
	t.Parallel()
	for _, frame := range []string{
		`<html><head><script type='text/x-pictor'>` + classic + "\r\n" + `</script></head>` + simple + `</html>`,
		strings.Replace(simple, `<p`, `<script type = 'application/x-pictor-props+json'>{ "schema" : {}, "values" : {} }</script><!--kept--><script type='text/x-pictor'>`+classic+`</script><p`, 1),
		strings.Replace(simple, `Text &#38; bytes`, `A<script type='text/x-pictor'>`+classic+`</script>B`, 1),
	} {
		op := designsourceedit.Op{Kind: "set_logic", File: "frames/source.html", Expected: unit(t, frame), Schema: json.RawMessage(`null`), ExpectedValuesSHA256: designsourceedit.ValueDigest(json.RawMessage(`{}`))}
		got, err := designsourceedit.Apply(input(frame), []designsourceedit.Op{op})
		if err != nil {
			t.Fatal(err)
		}
		back, err := designsourceedit.Apply(got.Input, wireUndo(t, got.Inverse))
		if err != nil || back.Input.Files[op.File] != frame {
			t.Fatalf("deletion undo: %s %v", back.Input.Files[op.File], err)
		}
		// A source edit outside the restoration gap survives conditional own Undo.
		if strings.Contains(frame, `Text &#38; bytes`) {
			concurrent := got.Input
			concurrent.Files[op.File] = strings.Replace(concurrent.Files[op.File], `Text &#38; bytes`, `Someone else`, 1)
			back, err = designsourceedit.Apply(concurrent, wireUndo(t, got.Inverse))
			if err != nil || back.Input.Files[op.File] != strings.Replace(frame, `Text &#38; bytes`, `Someone else`, 1) {
				t.Fatalf("concurrent source: %v", err)
			}
		}
	}
}

func TestBlockLifecycleRejectsStaleUnitsAndValuesAndRetainsDetachedDrafts(t *testing.T) {
	t.Parallel()
	create := designsourceedit.Op{Kind: "set_logic", File: "frames/source.html", Expected: unit(t, simple), Logic: classic}
	first, err := designsourceedit.Apply(input(simple), []designsourceedit.Op{create})
	if err != nil {
		t.Fatal(err)
	}
	stale := create
	stale.Logic += "\n// competitor"
	result, err := designsourceedit.Apply(first.Input, []designsourceedit.Op{stale})
	var rejected *designsourceedit.Rejected
	if !errors.Is(err, designsourceedit.ErrConflict) || !errors.As(err, &rejected) || result.Input.Files != nil || len(rejected.Draft) != 1 || rejected.Current != unit(t, first.Input.Files[create.File]) {
		t.Fatalf("stale draft: %v", err)
	}
	schema := json.RawMessage(`{"title":{"type":"string","maxLength":80},"count":{"type":"number"}}`)
	remove := designsourceedit.Op{Kind: "set_logic", File: create.File, Expected: unit(t, source), Schema: json.RawMessage(`null`), ExpectedValuesSHA256: designsourceedit.ValueDigest(json.RawMessage(`{"title":"Before","count":1}`))}
	props, err := designsourceedit.Apply(input(source), []designsourceedit.Op{prop("count", `1`, `2`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := designsourceedit.Apply(props.Input, []designsourceedit.Op{remove}); !errors.Is(err, designsourceedit.ErrConflict) {
		t.Fatalf("deleted concurrent values: %v", err)
	}
	remove.Schema = schema
	remove.Logic = classic
	merged, err := designsourceedit.Apply(props.Input, []designsourceedit.Op{remove})
	if err != nil || !strings.Contains(merged.Input.Files[create.File], `"count": 2`) {
		t.Fatalf("kept props overwritten: %v", err)
	}
}

func TestMalformedBlockCreationRejectsTheWholeBatchWithoutExecutingLogic(t *testing.T) {
	t.Parallel()
	literal := designsourceedit.Op{Kind: "set_literal", File: "frames/source.html", PID: "p", Expected: designsourceedit.Digest(`Text &#38; bytes`), Text: "changed"}
	for _, op := range []designsourceedit.Op{
		{Kind: "set_logic", File: literal.File, Expected: unit(t, simple), Logic: `not a class`},
		{Kind: "set_logic", File: literal.File, Expected: unit(t, simple), Logic: classic, Schema: json.RawMessage(`{"x":{"type":"unknown"}}`)},
		{Kind: "set_logic", File: literal.File, Expected: unit(t, simple), Logic: classic + `</script><p data-pid="injected">injected</p>`},
	} {
		result, err := designsourceedit.Apply(input(simple), []designsourceedit.Op{literal, op})
		var rejected *designsourceedit.Rejected
		if err == nil || result.Input.Files != nil || !errors.As(err, &rejected) || len(rejected.Draft) != 2 {
			t.Fatalf("partial creation: %v", err)
		}
	}
}

func TestDeclarationRestorationIsOrderedIdempotentAndConflictsOnChangedPlacement(t *testing.T) {
	t.Parallel()
	props := `<script type='application/x-pictor-props+json'>{ "schema": {}, "values": {} }</script>`
	logic := `<script type='text/x-pictor'>` + classic + `</script>`
	for _, blocks := range []string{props + logic, logic + props} {
		frame := strings.Replace(simple, `<p`, blocks+`<p`, 1)
		remove := designsourceedit.Op{Kind: "set_logic", File: "frames/source.html", Expected: unit(t, frame), Schema: json.RawMessage(`null`), ExpectedValuesSHA256: designsourceedit.ValueDigest(json.RawMessage(`{}`))}
		got, err := designsourceedit.Apply(input(frame), []designsourceedit.Op{remove})
		if err != nil {
			t.Fatal(err)
		}
		inverse := wireUndo(t, got.Inverse)
		back, err := designsourceedit.Apply(got.Input, inverse)
		if err != nil || back.Input.Files[remove.File] != frame {
			t.Fatalf("ordered restoration: %s %v", back.Input.Files[remove.File], err)
		}
		retry, err := designsourceedit.Apply(back.Input, inverse)
		if err != nil || retry.Input.Files[remove.File] != frame {
			t.Fatalf("identical restoration retry: %v", err)
		}
		changed := input(strings.Replace(simple, `data-pid="p"`, `data-pid="other"`, 1))
		// The inverse's nearest element boundary may be root rather than p; change
		// both identities so neither can accidentally authorize another placement.
		changed.Files[remove.File] = strings.Replace(changed.Files[remove.File], `data-pid="root"`, `data-pid="otherroot"`, 1)
		result, err := designsourceedit.Apply(changed, inverse)
		var rejected *designsourceedit.Rejected
		if !errors.Is(err, designsourceedit.ErrConflict) || !errors.As(err, &rejected) || result.Input.Files != nil {
			t.Fatalf("placement overwritten: %v", err)
		}
		if len(inverse[0].RestoreBlocks[0].Anchor.Path) > 0 {
			inverse[0].RestoreBlocks[0].Anchor.Path[0] = 99
			if rejected.Draft[0].RestoreBlocks[0].Anchor.Path[0] == 99 {
				t.Fatal("draft aliases inverse placement")
			}
		}
	}
}

func TestDeclarationDeletionRejectsOverlappingLiteralAndPropTargetsAtomically(t *testing.T) {
	t.Parallel()
	frame := strings.Replace(simple, `Text &#38; bytes`, `A<script type='text/x-pictor'>`+classic+`</script>B`, 1)
	remove := designsourceedit.Op{Kind: "set_logic", File: "frames/source.html", Expected: unit(t, frame)}
	literal := designsourceedit.Op{Kind: "set_literal", File: remove.File, PID: "p", Expected: designsourceedit.Digest("A"), Text: "edited"}
	for _, ops := range [][]designsourceedit.Op{{remove, literal}, {literal, remove}} {
		result, err := designsourceedit.Apply(input(frame), ops)
		if err == nil || result.Input.Files != nil {
			t.Fatalf("overlapping source targets accepted: %v", err)
		}
	}
	remove.Expected = unit(t, source)
	remove.Schema = json.RawMessage(`null`)
	remove.ExpectedValuesSHA256 = designsourceedit.ValueDigest(json.RawMessage(`{"title":"Before","count":1}`))
	value := prop("count", `1`, `2`)
	for _, ops := range [][]designsourceedit.Op{{remove, value}, {value, remove}} {
		result, err := designsourceedit.Apply(input(source), ops)
		if err == nil || result.Input.Files != nil {
			t.Fatalf("deletion and value edit accepted: %v", err)
		}
	}
}

func TestDeclarationPresenceTransitionsKeepIndependentPropsAndExposeDeletionDigest(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{"title":{"type":"string","maxLength":80},"count":{"type":"number"}}`)
	remove := designsourceedit.Op{Kind: "set_logic", File: "frames/source.html", Expected: unit(t, source)}
	got, err := designsourceedit.Apply(input(source), []designsourceedit.Op{remove})
	if err != nil {
		t.Fatal(err)
	}
	concurrent, err := designsourceedit.Apply(got.Input, []designsourceedit.Op{prop("count", `1`, `2`)})
	if err != nil {
		t.Fatal(err)
	}
	back, err := designsourceedit.Apply(concurrent.Input, wireUndo(t, got.Inverse))
	if err != nil || back.Input.Files[remove.File] != strings.Replace(source, `"count": 1`, `"count": 2`, 1) {
		t.Fatalf("logic deletion Undo lost props: %v", err)
	}
	plan, err := designsource.ParseFrame(remove.File, source)
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := designsourceedit.Inspect(plan)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.PropsValuesSHA256 != designsourceedit.ValueDigest(json.RawMessage(`{"title":"Before","count":1}`)) {
		t.Fatal("missing authored values deletion digest")
	}
	for _, frame := range []string{
		`<script type='text/x-pictor'>` + classic + `</script>` + simple,
		`<script type='application/x-pictor-props+json'>{ "schema": ` + string(schema) + ` }</script>` + simple,
	} {
		op := designsourceedit.Op{Kind: "set_logic", File: remove.File, Expected: unit(t, frame), Logic: classic, Schema: schema}
		result, err := designsourceedit.Apply(input(frame), []designsourceedit.Op{op})
		if err != nil {
			t.Fatal(err)
		}
		undone, err := designsourceedit.Apply(result.Input, wireUndo(t, result.Inverse))
		if err != nil || undone.Input.Files[remove.File] != frame {
			t.Fatalf("presence transition Undo: %s %v", undone.Input.Files[remove.File], err)
		}
	}
}

func TestRestoredDeclarationsRejectMovedBlocksAndKeepDetachedDrafts(t *testing.T) {
	t.Parallel()
	logic := `<script type='text/x-pictor'>` + classic + `</script>`
	props := `<script type='application/x-pictor-props+json'>{"schema":{},"values":{}}</script>`
	for _, blocks := range []string{logic, logic + props, props + logic} {
		t.Run(blocks, func(t *testing.T) {
			frame := strings.Replace(simple, `<p`, blocks+`<p`, 1)
			remove := designsourceedit.Op{Kind: "set_logic", File: "frames/source.html", Expected: unit(t, frame), Schema: json.RawMessage(`null`), ExpectedValuesSHA256: designsourceedit.ValueDigest(json.RawMessage(`{}`))}
			deleted, err := designsourceedit.Apply(input(frame), []designsourceedit.Op{remove})
			if err != nil {
				t.Fatal(err)
			}
			inverse := wireUndo(t, deleted.Inverse)
			restored, err := designsourceedit.Apply(deleted.Input, inverse)
			if err != nil {
				t.Fatal(err)
			}
			retry, err := designsourceedit.Apply(restored.Input, inverse)
			if err != nil || retry.Input.Files[remove.File] != frame {
				t.Fatalf("same placement retry: %v", err)
			}
			for _, moved := range []string{logic, blocks} {
				peer := strings.Replace(frame, moved, "", 1)
				peer = strings.Replace(peer, `</body>`, moved+`</body>`, 1)
				current := input(peer)
				before := wireUndo(t, inverse)
				result, err := designsourceedit.Apply(current, inverse)
				var rejected *designsourceedit.Rejected
				if !errors.Is(err, designsourceedit.ErrConflict) || !errors.As(err, &rejected) || result.Input.Files != nil {
					t.Fatalf("moved declaration accepted: %v", err)
				}
				if current.Files[remove.File] != peer || !reflect.DeepEqual(inverse, before) || !reflect.DeepEqual(rejected.Draft, before) {
					t.Fatal("conflict changed peer source or lost the complete draft")
				}
				inverse[0].RestoreBlocks[0].Anchor.Path[0] = 99
				inverse[0].Schema[0] = 'x'
				inverse[0].RestoreBlocks[0].Source = "changed"
				if !reflect.DeepEqual(rejected.Draft, before) {
					t.Fatal("draft aliases transported inverse")
				}
				inverse = before
			}
		})
	}
}
