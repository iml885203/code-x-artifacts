package sourcecodec_test

import (
	"os"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designroom/sourcecodec"
	"github.com/iml885203/code-x-artifacts/internal/designsource"
	"github.com/reearth/ygo/crdt"
)

func fixture(t *testing.T) designsource.ParseInput {
	t.Helper()
	manifest, err := os.ReadFile("../../designsource/testdata/design.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../designsource/testdata/source.html")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(source), "{{item.name}}", "Hello &amp; {{item.name}}!", 1)
	text = strings.Replace(text, `"values":{}`, `"values":{"title":"Cards"}`, 1)
	text = strings.Replace(text, "</body>", `<p-import data-pid="child-site" src="frames/child.html"></p-import><svg data-pid="svg"><path data-pid="path" d="M0 0L1 1"></path></svg></body>`, 1)
	manifestText := strings.Replace(string(manifest), `"h":300}]]`, `"h":300},{"id":"child","name":"Child","file":"frames/child.html","w":200,"h":100}]]`, 1)
	return designsource.ParseInput{Format: "2", Manifest: manifestText, Files: map[string]string{"frames/source.html": text, "frames/child.html": `<body data-pid="child"><p data-pid="child-text">Shared child</p></body>`}}
}

func TestSourceCodecRetainsOneTemplateAndExactBytesWhileLiteralTextCoedits(t *testing.T) {
	t.Parallel()
	input := fixture(t)
	state, err := sourcecodec.State(input)
	if err != nil {
		t.Fatal(err)
	}
	doc := crdt.New()
	defer doc.Destroy()
	if err := crdt.ApplyUpdateV1(doc, state, nil); err != nil {
		t.Fatal(err)
	}
	got, err := sourcecodec.Read(doc)
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest != input.Manifest || got.Files["frames/source.html"] != input.Files["frames/source.html"] {
		t.Fatalf("byte loss\nmanifest %q\nwant %q\nsource %q\nwant %q", got.Manifest, input.Manifest, got.Files["frames/source.html"], input.Files["frames/source.html"])
	}
	key := sourcecodec.SlotKey("frames/source.html", "literal", "label", 0, 0, "")
	value, _ := doc.GetMap("design").Get(key)
	text, ok := value.(*crdt.YText)
	if !ok {
		t.Fatalf("literal isn't Y.Text: %T", value)
	}
	before := crdt.New()
	defer before.Destroy()
	if err := crdt.ApplyUpdateV1(before, state, nil); err != nil {
		t.Fatal(err)
	}
	doc.Transact(func(txn *crdt.Transaction) { text.Insert(txn, 0, "Hi ", nil) })
	if err := sourcecodec.CheckTransition(before, doc); err != nil {
		t.Fatal(err)
	}
	got, err = sourcecodec.Read(doc)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files["frames/source.html"] != strings.Replace(input.Files["frames/source.html"], "Hello &amp; ", "Hi Hello &amp; ", 1) {
		t.Fatal("edit changed other source")
	}
}

func TestRawSourceUpdatesCannotChangeProtectedUnitsOrConvertLiteralRunsToBindings(t *testing.T) {
	t.Parallel()
	input := fixture(t)
	state, err := sourcecodec.State(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"logic", "schema", "prop", "envelope", "unknown", "binding"} {
		t.Run(kind, func(t *testing.T) {
			before, after := crdt.New(), crdt.New()
			defer before.Destroy()
			defer after.Destroy()
			for _, doc := range []*crdt.Doc{before, after} {
				if err := crdt.ApplyUpdateV1(doc, state, nil); err != nil {
					t.Fatal(err)
				}
			}
			m := after.GetMap("design")
			value, _ := m.Get(sourcecodec.SlotKey("frames/source.html", "literal", "label", 0, 0, ""))
			after.Transact(func(txn *crdt.Transaction) {
				switch kind {
				case "binding":
					value.(*crdt.YText).Insert(txn, 0, "{{item.name}}", nil)
				case "envelope":
					m.Set(txn, sourcecodec.EnvelopeKey("frames/source.html"), "[]")
				default:
					value := "tampered"
					prop := ""
					switch kind {
					case "logic":
						value = "class Frame extends PictorLogic { renderVals() { return {items: []}; } choose(event) {} }"
					case "schema":
						value = `{"title":{"type":"string","default":"Cards","maxLength":90}}`
					case "prop":
						value = `"Changed"`
						prop = "title"
					}
					m.Set(txn, sourcecodec.SlotKey("frames/source.html", kind, "", 0, 0, prop), value)
				}
			})
			if kind == "logic" || kind == "schema" || kind == "prop" {
				if _, err := sourcecodec.Read(after); err != nil {
					t.Fatalf("fixture must be valid source before protection check: %v", err)
				}
			}
			if err := sourcecodec.CheckTransition(before, after); err == nil {
				t.Fatal("raw protected update accepted")
			}
		})
	}
}

func TestDeclarationsInTheHeadStayProtectedWhileInertBodyBlocksKeepDistinctLiteralGaps(t *testing.T) {
	t.Parallel()
	for _, placement := range []string{"body", "head"} {
		t.Run(placement, func(t *testing.T) {
			input := fixture(t)
			if placement == "head" {
				text := input.Files["frames/source.html"]
				start := strings.Index(text, `<script type="application/x-pictor-props+json">`)
				end := strings.Index(text[start:], "</script>") + start + len("</script>")
				props := text[start:end]
				text = text[:start] + text[end:]
				start = strings.Index(text, `<script type="text/x-pictor">`)
				end = strings.Index(text[start:], "</script>") + start + len("</script>")
				logic := text[start:end]
				text = text[:start] + text[end:]
				input.Files["frames/source.html"] = strings.Replace(text, "</head>", props+"\r\n"+logic+"</head>", 1)
			}
			state, err := sourcecodec.State(input)
			if err != nil {
				t.Fatal(err)
			}
			doc := crdt.New()
			defer doc.Destroy()
			if err := crdt.ApplyUpdateV1(doc, state, nil); err != nil {
				t.Fatal(err)
			}
			got, err := sourcecodec.Read(doc)
			if err != nil {
				t.Fatal(err)
			}
			if got.Files["frames/source.html"] != input.Files["frames/source.html"] {
				t.Fatal("source bytes changed")
			}
			for _, kind := range []string{"logic", "schema", "prop"} {
				prop := ""
				if kind == "prop" {
					prop = "title"
				}
				if _, ok := doc.GetMap("design").Get(sourcecodec.SlotKey("frames/source.html", kind, "", 0, 0, prop)); !ok {
					t.Fatalf("%s declaration is not a protected source slot", kind)
				}
			}
		})
	}
}

func TestOpaqueCommentsNeverAliasDistinctLiteralSourceSlots(t *testing.T) {
	t.Parallel()
	input := fixture(t)
	input.Files["frames/source.html"] = strings.Replace(input.Files["frames/source.html"], "Hello &amp; {{item.name}}!", "First<!--keep bytes-->Second", 1)
	state, err := sourcecodec.State(input)
	if err != nil {
		t.Fatal(err)
	}
	doc := crdt.New()
	defer doc.Destroy()
	if err := crdt.ApplyUpdateV1(doc, state, nil); err != nil {
		t.Fatal(err)
	}
	got, err := sourcecodec.Read(doc)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files["frames/source.html"] != input.Files["frames/source.html"] {
		t.Fatal("comment source changed")
	}
}
