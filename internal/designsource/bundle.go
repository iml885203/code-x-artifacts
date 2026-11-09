package designsource

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ParseBundle validates a complete format/runtime pair and all frame dependencies.
// Manifest and source bytes remain authored bytes. The static manifest validator
// is reused by changing only a private validation copy's version/runtime fields.
func ParseBundle(input ParseInput) (BundlePlan, error) {
	p := BundlePlan{Format: input.Format, RuntimeID: RuntimeID, Manifest: input.Manifest, Frames: map[string]FramePlan{}, Sources: map[SourceRef]*Node{}, Dependencies: map[string][]string{}, Rewritten: map[string]string{}}
	add := func(code, msg, file string) {
		size := len(input.Manifest)
		if file != "design.json" {
			size = len(input.Files[file])
		}
		p.Diagnostics = append(p.Diagnostics, Diagnostic{code, msg, SourceSpan{file, 0, size}, "error"})
	}
	if input.Format != Format {
		add("format", "source plan requires artifact format 2", "design.json")
	}
	if len(input.Manifest) > designfmt.MaxManifestBytes {
		add("limit", "manifest exceeds 256 KiB", "design.json")
		return p, Problems(p.Diagnostics)
	}
	var root map[string]json.RawMessage
	if e := decodeJSON(input.Manifest, &root); e != nil {
		add("manifest", e.Error(), "design.json")
		return p, Problems(p.Diagnostics)
	}
	if root == nil {
		add("manifest", "manifest must be an object", "design.json")
		return p, Problems(p.Diagnostics)
	}
	if string(root["version"]) != "2" {
		add("version", "format 2 requires manifest version 2", "design.json")
	}
	var runtime string
	if e := json.Unmarshal(root["runtime"], &runtime); e != nil || runtime != RuntimeID {
		add("runtime", "format 2 requires runtime "+RuntimeID, "design.json")
	}
	delete(root, "runtime")
	root["version"] = json.RawMessage("1")
	validation, e := json.Marshal(root)
	if e != nil {
		return p, e
	}
	manifest, issues := designfmt.Parse(string(validation))
	for _, issue := range issues {
		add("manifest", issue, "design.json")
	}
	owned := p.parseFrames(manifest, input, add)
	p.checkReferences(input, owned, add)
	p.checkGraph(add)

	sortDiagnostics(p.Diagnostics)
	if len(p.Diagnostics) > 0 {
		return p, Problems(p.Diagnostics)
	}
	return p, nil
}

// RequireExecutable deliberately fails until a runtime compiler supplies and
// verifies the immutable worker program. Syntax checking alone is not admission.
func (p FramePlan) RequireExecutable() error {
	if p.Logic != nil {
		return fmt.Errorf("native logic compilation and runtime proof are required")
	}
	for _, n := range p.Nodes {
		if e := requiresLogic(n); e != nil {
			return e
		}
	}
	return nil
}

func requiresLogic(n *Node) error {
	if len(n.Events) > 0 {
		return fmt.Errorf("event method declarations require native logic compilation")
	}
	for _, c := range n.Children {
		if e := requiresLogic(c); e != nil {
			return e
		}
	}
	return nil
}

func (p *BundlePlan) parseFrames(manifest designfmt.Manifest, input ParseInput, add func(string, string, string)) map[string]bool {
	owned := map[string]bool{}
	for _, placed := range manifest.Frames() {
		file := placed.Frame.File
		if file == "" || owned[file] {
			continue
		}
		owned[file] = true
		source, ok := input.Files[file]
		if !ok {
			add("missing-frame", "manifest frame is missing", file)
			continue
		}
		frame, _ := ParseFrame(file, source)
		p.Frames[file] = frame
		p.Diagnostics = append(p.Diagnostics, frame.Diagnostics...)
		if frame.Rewritten != source {
			p.Rewritten[file] = frame.Rewritten
		}
		var walk func([]*Node)
		walk = func(nodes []*Node) {
			for _, n := range nodes {
				if n.PID != "" {
					p.Sources[SourceRef{file, n.PID}] = n
				}
				if n.Import != "" {
					p.Dependencies[file] = append(p.Dependencies[file], n.Import)
				}
				walk(n.Children)
			}
		}
		walk(frame.Nodes)
	}

	return owned
}

func (p *BundlePlan) checkReferences(input ParseInput, owned map[string]bool, add func(string, string, string)) {
	for _, file := range sortedKeys(p.Frames) {
		var walk func([]*Node)
		walk = func(nodes []*Node) {
			for _, n := range nodes {
				if n.Navigation != "" && !owned[n.Navigation] {
					add("navigation", "prototype link target is not a manifest-owned frame: "+n.Navigation, file)
				}
				walk(n.Children)
			}
		}
		walk(p.Frames[file].Nodes)
	}
	for _, file := range sortedKeys(input.Files) {
		if !owned[file] {
			add("unowned-file", "file is not owned by a manifest frame", file)
		}
	}
	for _, file := range sortedKeys(p.Dependencies) {
		for _, dep := range p.Dependencies[file] {
			if !owned[dep] {
				add("import", "import target is not a manifest-owned frame: "+dep, file)
			}
		}
	}
}

func (p *BundlePlan) checkGraph(add func(string, string, string)) {
	state := map[string]int{}
	depth := map[string]int{}
	var visit func(string) int
	visit = func(file string) int {
		if state[file] == 1 {
			add("import-cycle", "import cycle includes "+file, file)
			return 0
		}
		if state[file] == 2 {
			return depth[file]
		}
		state[file] = 1
		longest := 0
		unique := map[string]bool{}
		for _, child := range p.Dependencies[file] {
			if !unique[child] {
				unique[child] = true
				longest = max(longest, visit(child)+1)
			}
		}
		state[file] = 2
		depth[file] = longest
		if longest > 8 {
			add("import-depth", "import dependency depth exceeds 8", file)
		}
		return longest
	}
	for _, file := range sortedKeys(p.Frames) {
		visit(file)
	}
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.SliceStable(diagnostics, func(i, j int) bool {
		a, b := diagnostics[i], diagnostics[j]
		if a.Span.File != b.Span.File {
			return a.Span.File < b.Span.File
		}
		if a.Span.StartByte != b.Span.StartByte {
			return a.Span.StartByte < b.Span.StartByte
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
}
