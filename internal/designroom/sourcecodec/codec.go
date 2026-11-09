// Package sourcecodec encodes inert format 2 source templates. It is not
// registered as a production room codec and grants no executable admission.
package sourcecodec

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
)

const manifestKey = "v2|manifest"

type piece struct {
	Raw  string                 `json:"raw,omitempty"`
	Slot *designsourceedit.Slot `json:"slot,omitempty"`
}

// EnvelopeKey identifies immutable source scaffolding, never expanded DOM.
func EnvelopeKey(file string) string { return key("envelope", file) }

// SlotKey is a JSON tuple. Strings containing separators never alias another
// source ref; repeat/import instance paths are deliberately absent.
func SlotKey(file, kind, pid string, child, segment int, prop string) string {
	return key("slot", file, kind, pid, child, segment, prop)
}
func key(parts ...any) string { b, _ := json.Marshal(parts); return "v2|" + string(b) }
func slotKey(file string, s designsourceedit.Slot) string {
	return SlotKey(file, s.Kind, s.PID, s.Child, s.Segment, s.Key)
}

// State parses a complete bundle, assigns source PIDs once, then writes one
// source template with protected unit strings and independently editable runs.
func State(input designsource.ParseInput) ([]byte, error) {
	doc := crdt.New()
	defer doc.Destroy()
	if err := Seed(doc, input); err != nil {
		return nil, err
	}
	return crdt.EncodeStateAsUpdateV1(doc, nil), nil
}

// Seed initializes an empty isolated document before it is attached to a hub.
// Existing documents require a serialized conditional operation, never reseeding.
func Seed(doc *crdt.Doc, input designsource.ParseInput) error {
	plan, err := designsource.ParseBundle(input)
	if err != nil {
		return err
	}
	if len(doc.GetMap("design").Entries()) != 0 {
		return fmt.Errorf("source seed requires an empty document")
	}
	m := doc.GetMap("design")
	files := make([]string, 0, len(plan.Frames))
	for file := range plan.Frames {
		files = append(files, file)
	}
	sort.Strings(files)
	type entry struct {
		key, value string
		literal    bool
	}
	entries := []entry{{key: manifestKey, value: input.Manifest}}
	for _, file := range files {
		frame, err := designsource.ParseFrame(file, plan.Frames[file].Rewritten)
		if err != nil {
			return err
		}
		slots, err := designsourceedit.Layout(frame)
		if err != nil {
			return err
		}
		parts := make([]piece, 0, len(slots)*2+1)
		at := 0
		for _, s := range slots {
			parts = append(parts, piece{Raw: frame.Source[at:s.Start]})
			value := frame.Source[s.Start:s.End]
			at = s.End
			s.Start, s.End = 0, 0
			parts = append(parts, piece{Slot: &s})
			entries = append(entries, entry{slotKey(file, s), value, s.Kind == "literal"})
		}
		parts = append(parts, piece{Raw: frame.Source[at:]})
		raw, _ := json.Marshal(parts)
		entries = append(entries, entry{key: EnvelopeKey(file), value: string(raw)})
	}
	doc.Transact(func(txn *crdt.Transaction) {
		for _, e := range entries {
			if e.literal {
				text := crdt.NewTextPrelim()
				m.Set(txn, e.key, text)
				text.Insert(txn, 0, e.value, nil)
			} else {
				m.Set(txn, e.key, e.value)
			}
		}
	})
	return nil
}

func decodeEnvelope(raw string) ([]piece, error) {
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	var parts []piece
	if err := d.Decode(&parts); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("extra envelope data")
	}
	if len(parts) == 0 || len(parts) > 100000 {
		return nil, fmt.Errorf("invalid envelope size")
	}
	return parts, nil
}

// Read projects source, never generated instances. Every entry has one owner;
// mixed/unknown protocol keys and non-text literal values are refused.
func Read(doc *crdt.Doc) (designsource.ParseInput, error) {
	input := designsource.ParseInput{Format: "2", Files: map[string]string{}}
	m := doc.GetMap("design")
	entries := m.Entries()
	input.Manifest, _ = entries[manifestKey].(string)
	allowed := map[string]bool{manifestKey: true}
	for k, v := range entries {
		if !strings.HasPrefix(k, `v2|["envelope",`) {
			continue
		}
		var tuple []string
		if err := json.Unmarshal([]byte(strings.TrimPrefix(k, "v2|")), &tuple); err != nil || len(tuple) != 2 {
			return input, fmt.Errorf("invalid source envelope key")
		}
		file := tuple[1]
		raw, ok := v.(string)
		if !ok {
			return input, fmt.Errorf("source envelope is not a string")
		}
		allowed[k] = true
		source, err := readFrame(m, file, raw, allowed)
		if err != nil {
			return input, err
		}
		input.Files[file] = source
	}
	for k := range entries {
		if !allowed[k] {
			return input, fmt.Errorf("unknown source entry %q", k)
		}
	}
	if _, err := designsource.ParseBundle(input); err != nil {
		return input, err
	}
	return input, nil
}

// CheckTransition is the raw-client policy, to run on isolated before/after
// candidates before any log or broadcast. It is not a replacement for wiring
// that hook into Hub; service CAS writes use a separate authorized transaction.
func CheckTransition(before, after *crdt.Doc) error {
	if _, err := Read(before); err != nil {
		return err
	}
	if _, err := Read(after); err != nil {
		return err
	}
	a, b := before.GetMap("design"), after.GetMap("design")
	ae, be := a.Entries(), b.Entries()
	if len(ae) != len(be) {
		return fmt.Errorf("raw source update changes protected entry set")
	}
	for k, av := range ae {
		bv, exists := be[k]
		if !exists {
			return fmt.Errorf("raw source update removes protected entry")
		}
		actual, _ := a.Get(k)
		if _, ok := actual.(*crdt.YText); ok {
			continue
		}
		if av != bv {
			return fmt.Errorf("raw source update changes protected source")
		}
	}
	return nil
}

func readFrame(m *crdt.YMap, file, raw string, allowed map[string]bool) (string, error) {
	parts, err := decodeEnvelope(raw)
	if err != nil {
		return "", err
	}
	var source strings.Builder
	for _, part := range parts {
		if part.Slot == nil {
			source.WriteString(part.Raw)
			continue
		}
		if part.Raw != "" {
			return "", fmt.Errorf("ambiguous source piece")
		}
		s := *part.Slot
		sk := slotKey(file, s)
		if allowed[sk] {
			return "", fmt.Errorf("duplicate source slot")
		}
		allowed[sk] = true
		value, err := readSlot(m, sk, s.Kind)
		if err != nil {
			return "", err
		}
		source.WriteString(value)
		if source.Len() > 512<<10 {
			return "", fmt.Errorf("frame exceeds source limit")
		}
	}
	return source.String(), nil
}

func readSlot(m *crdt.YMap, sk, kind string) (string, error) {
	value, ok := m.Get(sk)
	if !ok {
		return "", fmt.Errorf("missing source slot")
	}
	if kind == "literal" {
		text, ok := value.(*crdt.YText)
		if !ok {
			return "", fmt.Errorf("literal is not Y.Text")
		}
		for _, delta := range text.ToDelta() {
			if _, ok := delta.Insert.(string); !ok || len(delta.Attributes) > 0 {
				return "", fmt.Errorf("literal text contains embeds or attributes")
			}
		}
		raw := text.ToString()
		if err := designsourceedit.CheckLiteralSource(raw); err != nil {
			return "", err
		}
		return raw, nil
	}
	if kind != "logic" && kind != "schema" && kind != "prop" {
		return "", fmt.Errorf("unknown source slot kind")
	}
	raw, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("protected source is not a string")
	}
	return raw, nil
}
