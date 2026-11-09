// Package designrender prepares inert source contexts. It emits no documents,
// author programs, resource grants or runtime admission decisions.
package designrender

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

// Snapshot must come from one immutable version or one atomic live read. The
// service owns authorization and atomic acquisition; this pure module cannot
// establish either from supplied strings.
type Snapshot struct {
	ArtifactID string
	Version    int
	Revision   string
	Input      designsource.ParseInput
}

// Request carries a service-derived edit decision, never an author assertion.
type Request struct {
	FrameID, Purpose string
	CanEdit          bool
}

// ResourceDigest identifies exact source bytes, not rewritten or rendered HTML.
type ResourceDigest struct{ File, SHA256 string }

// RenderContext is source preparation only. RuntimeSHA256 stays empty because
// no executable runtime has been admitted. It cannot be used as runtime proof.
type RenderContext struct {
	ArtifactID, Format, RuntimeID, RuntimeSHA256 string
	Version                                      int
	Revision                                     string
	FrameID, FrameFile, Purpose                  string
	CanEdit                                      bool
	Plan                                         designsource.BundlePlan
	Dependencies                                 []ResourceDigest
	DependencySHA256                             string
}

// BuildContext validates the complete snapshot before selecting a frame and its
// transitive source dependencies. There is no contextless or format-1 fallback.
func BuildContext(s Snapshot, r Request) (RenderContext, error) {
	if s.ArtifactID == "" || s.Version < 0 || (s.Version == 0 && s.Revision == "") || (s.Version > 0 && s.Revision != "") {
		return RenderContext{}, fmt.Errorf("context requires artifact and exactly one immutable version or live revision")
	}
	switch r.Purpose {
	case "canvas", "play", "preview", "raw", "overview", "offline", "print":
	case "history":
		if s.Version == 0 {
			return RenderContext{}, fmt.Errorf("history requires an immutable version")
		}
		r.CanEdit = false
	default:
		return RenderContext{}, fmt.Errorf("unsupported context purpose %q", r.Purpose)
	}
	plan, err := designsource.ParseBundle(s.Input)
	if err != nil {
		return RenderContext{}, err
	}
	file, err := frameFile(plan.Manifest, r.FrameID)
	if err != nil {
		return RenderContext{}, err
	}
	deps := sourceClosure(plan, file)
	// Struct fields and sorted dependency slices provide unambiguous boundaries.
	identity := struct {
		Format, Runtime, Manifest string
		Sources                   []ResourceDigest
	}{plan.Format, plan.RuntimeID, plan.Manifest, deps}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return RenderContext{}, fmt.Errorf("encode source dependency identity: %w", err)
	}
	return RenderContext{ArtifactID: s.ArtifactID, Format: plan.Format, RuntimeID: plan.RuntimeID, Version: s.Version, Revision: s.Revision, FrameID: r.FrameID, FrameFile: file, Purpose: r.Purpose, CanEdit: r.CanEdit, Plan: plan, Dependencies: deps, DependencySHA256: digest(string(encoded))}, nil
}

func frameFile(manifest, id string) (string, error) {
	// ParseBundle has already applied the authoritative manifest rules. This read
	// selects identity only; it does not validate a competing manifest grammar.
	var m struct {
		Pages []struct{ Rows [][]struct{ ID, File string } }
	}
	if err := json.Unmarshal([]byte(manifest), &m); err != nil {
		return "", fmt.Errorf("read validated manifest: %w", err)
	}
	for _, p := range m.Pages {
		for _, row := range p.Rows {
			for _, f := range row {
				if f.ID == id {
					return f.File, nil
				}
			}
		}
	}
	return "", fmt.Errorf("frame %q is not owned by this snapshot", id)
}

func sourceClosure(plan designsource.BundlePlan, file string) []ResourceDigest {
	seen := map[string]bool{}
	var visit func(string)
	visit = func(f string) {
		if seen[f] {
			return
		}
		seen[f] = true
		for _, child := range plan.Dependencies[f] {
			visit(child)
		}
	}
	visit(file)
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	deps := make([]ResourceDigest, 0, len(files))
	for _, f := range files {
		deps = append(deps, ResourceDigest{f, digest(plan.Frames[f].Source)})
	}
	return deps
}

func digest(source string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(source))) }
