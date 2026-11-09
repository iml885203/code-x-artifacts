package designsourceedit

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

// ErrConflict distinguishes a stale unit from invalid source or malformed operations.
var ErrConflict = errors.New("design source unit changed since it was read")

// Op addresses authoritative source, never a projected repeat instance. Expected
// is mandatory. Schema includes defaults and changes atomically with Logic.
// A nil Value removes an authored override, preserving its declared default.
// Empty Logic removes its block; omitted Schema preserves props, while JSON null
// removes props under an additional ExpectedValuesSHA256 comparison. RestoreBlocks
// carries exact declaration placement/source for conditional own Undo.
// TextChild counts gaps between non-text children; Segment counts preceding
// binding holes, including empty literal slots, so deletion remains undoable.
type Op struct {
	Kind                 string          `json:"op"`
	File                 string          `json:"file"`
	Expected             string          `json:"expectedSHA256"`
	Key                  string          `json:"key,omitempty"`
	Value                json.RawMessage `json:"value,omitempty"`
	Logic                string          `json:"logic,omitempty"`
	Schema               json.RawMessage `json:"schema,omitempty"`
	PID                  string          `json:"pid,omitempty"`
	TextChild            int             `json:"textChild,omitempty"`
	Segment              int             `json:"segment,omitempty"`
	Text                 string          `json:"text,omitempty"`
	RawText              *string         `json:"rawText,omitempty"`
	ExpectedValuesSHA256 string          `json:"expectedValuesSHA256,omitempty"`
	RestoreBlocks        []BlockRestore  `json:"restoreBlocks,omitempty"`
}

// Rejected retains a detached complete submitted batch, including the failing
// operation index and current comparison digest. No partial result is returned.
type Rejected struct {
	Index   int
	Current string
	Draft   []Op
	Cause   error
}

func (e *Rejected) Error() string { return fmt.Sprintf("design operation %d: %v", e.Index, e.Cause) }
func (e *Rejected) Unwrap() error { return e.Cause }

// Result is a detached validated bundle and conditional own-undo operations.
// Applying its inverse can conflict; it never overrides a collaborator's unit.
type Result struct {
	Input   designsource.ParseInput
	Inverse []Op
}

func detached(ops []Op) []Op {
	cloned := slices.Clone(ops)
	for i := range cloned {
		cloned[i].Schema = slices.Clone(cloned[i].Schema)
		cloned[i].Value = slices.Clone(cloned[i].Value)
		cloned[i].RestoreBlocks = slices.Clone(cloned[i].RestoreBlocks)
		for j := range cloned[i].RestoreBlocks {
			cloned[i].RestoreBlocks[j].Anchor.Path = slices.Clone(cloned[i].RestoreBlocks[j].Anchor.Path)
		}
		if cloned[i].RawText != nil {
			raw := *cloned[i].RawText
			cloned[i].RawText = &raw
		}
	}
	return cloned
}

// Apply plans an all-or-nothing operation batch against one current snapshot.
// The caller must verify the explicit read basis and current permissions, then
// execute this comparison and persistence under the same room serialization.
// Parsing is inert; success does not admit executable format 2 dispatch.
func Apply(input designsource.ParseInput, ops []Op) (Result, error) {
	reject := func(i int, current string, err error) (Result, error) {
		return Result{}, &Rejected{i, current, detached(ops), err}
	}
	if len(ops) == 0 || len(ops) > 256 {
		return reject(0, "", fmt.Errorf("batch must have 1–256 operations"))
	}
	if _, err := designsource.ParseBundle(input); err != nil {
		return reject(0, "", err)
	}
	next := designsource.ParseInput{Format: input.Format, Manifest: input.Manifest, Files: map[string]string{}}
	for file, source := range input.Files {
		next.Files[file] = source
	}
	seen := map[string]bool{}
	editors := map[string]*frameEdit{}
	inverse := make([]Op, 0, len(ops))
	for i, op := range ops {
		key := op.target()
		if seen[key] {
			return reject(i, "", fmt.Errorf("duplicate operation target"))
		}
		seen[key] = true
		source, exists := next.Files[op.File]
		if !exists || !validExpected(op.Expected) {
			return reject(i, "", fmt.Errorf("missing source or expected digest"))
		}
		editor, err := editOf(editors, op.File, source)
		if err != nil {
			return reject(i, "", err)
		}
		undo, current, err := editor.apply(op)
		if err != nil {
			return reject(i, current, err)
		}
		inverse = append([]Op{undo}, inverse...)
	}
	if err := finishEdits(editors, next, inverse); err != nil {
		return reject(-1, "", err)
	}
	if _, err := designsource.ParseBundle(next); err != nil {
		return reject(len(ops)-1, "", err)
	}
	return Result{next, inverse}, nil
}

func finishEdits(editors map[string]*frameEdit, next designsource.ParseInput, inverse []Op) error {
	for file, editor := range editors {
		changed, err := editor.source()
		if err != nil {
			return err
		}
		next.Files[file] = changed
		for i := range inverse {
			if inverse[i].File == file && inverse[i].Kind == "set_logic" {
				if err := editor.anchorUndo(changed, &inverse[i]); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func conflict(expected, current string) error {
	if expected == "" || expected != current {
		return fmt.Errorf("%w: expected %s, current %s", ErrConflict, expected, current)
	}
	return nil
}

// Structured identity prevents delimiter aliases and duplicate writes to one unit.
func (op Op) target() string {
	identity := []any{op.Kind, op.File}
	switch op.Kind {
	case "set_prop":
		identity = append(identity, op.Key)
	case "set_literal":
		identity = append(identity, op.PID, op.TextChild, op.Segment)
	}
	key, _ := json.Marshal(identity)
	return string(key)
}

func validExpected(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
