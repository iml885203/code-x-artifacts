package designrender

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

// InstanceSegment is an import site (nil Key) or a typed repeat slot. It names
// source identity, never a DOM index or an execution authority.
type InstanceSegment struct {
	Site designsource.SourceRef
	Key  *designsource.InstanceKey
}

// InstanceData supplies already available plain data for one source instance.
// Every visited root/import requires an explicit entry, even for empty data.
// Values cannot replace the reserved props/state roots.
type InstanceData struct {
	File          string
	Path          []InstanceSegment
	Values, State map[string]any
}

// ProjectedAttribute is data for attribute assignment, never an HTML fragment.
type ProjectedAttribute struct{ Name, Namespace, Value string }

// ProjectedNode contains ordinary elements/text only. Event names and navigation
// are passive source metadata; projection neither dispatches nor grants them.
type ProjectedNode struct {
	Kind, Tag, Namespace, Text string
	Source                     designsource.SourceRef
	Span                       designsource.SourceSpan
	Path                       []InstanceSegment
	Attributes                 []ProjectedAttribute
	Events                     map[string]string
	Navigation                 string
	Children                   []*ProjectedNode
}

const (
	// MaxProjectionNodes bounds both work and ordinary output for one root.
	MaxProjectionNodes = 10000
	// MaxProjectionBytes bounds aggregate projected text and attribute payload.
	MaxProjectionBytes = 1 << 20
)

type projector struct {
	plan          designsource.BundlePlan
	data          map[string]InstanceData
	used          map[string]bool
	work, payload int
}

// Project lowers an explicit source context using supplied JSON data only. It
// reparses exact context sources, never calls author logic, emits no document,
// and returns no partial tree on failure. Source IR and data remain unchanged.
func Project(c RenderContext, data []InstanceData) ([]*ProjectedNode, error) {
	verified, err := verifyProjectionContext(c)
	if err != nil {
		return nil, err
	}
	p := projector{plan: verified.Plan, used: map[string]bool{}}
	p.data, err = indexData(data)
	if err != nil {
		return nil, p.problem(err, designsource.SourceSpan{File: c.FrameFile})
	}
	scope, err := p.scope(c.FrameFile, nil, nil)
	if err != nil {
		return nil, p.problem(err, designsource.SourceSpan{File: c.FrameFile})
	}
	nodes, err := p.nodes(p.plan.Frames[c.FrameFile].Nodes, scope, nil, designsource.SourceRef{File: c.FrameFile})
	if err != nil {
		return nil, err
	}
	for _, d := range data {
		if !p.used[instanceID(d.File, d.Path)] {
			return nil, p.problem(fmt.Errorf("unused snapshot data for %s", d.File), designsource.SourceSpan{File: d.File})
		}
	}
	return nodes, nil
}

func verifyProjectionContext(c RenderContext) (RenderContext, error) {
	files := map[string]string{}
	for file, frame := range c.Plan.Frames {
		files[file] = frame.Source
	}
	verified, err := BuildContext(Snapshot{ArtifactID: c.ArtifactID, Version: c.Version, Revision: c.Revision, Input: designsource.ParseInput{Format: c.Format, Manifest: c.Plan.Manifest, Files: files}}, Request{FrameID: c.FrameID, Purpose: c.Purpose, CanEdit: c.CanEdit})
	if err != nil {
		return RenderContext{}, err
	}
	if verified.RuntimeID != c.RuntimeID || verified.FrameFile != c.FrameFile || verified.DependencySHA256 != c.DependencySHA256 || c.RuntimeSHA256 != "" {
		return RenderContext{}, fmt.Errorf("projection context identity does not match its source snapshot")
	}
	return verified, nil
}

func indexData(data []InstanceData) (map[string]InstanceData, error) {
	if len(data) > MaxProjectionNodes {
		return nil, fmt.Errorf("too many projection instance snapshots")
	}
	out := map[string]InstanceData{}
	total := 0
	for _, d := range data {
		if err := checkInstanceData(d); err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(d)
		if err != nil {
			return nil, err
		}
		total += len(encoded)
		if total > 256<<10 {
			return nil, fmt.Errorf("projection snapshot data exceeds 256 KiB")
		}
		var detached InstanceData
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.UseNumber()
		if err := decoder.Decode(&detached); err != nil {
			return nil, err
		}
		key := instanceID(detached.File, detached.Path)
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate instance data for %s", d.File)
		}
		out[key] = detached
	}
	return out, nil
}

func checkInstanceData(d InstanceData) error {
	if len(d.Path) > 128 {
		return fmt.Errorf("instance path exceeds 128 segments")
	}
	for _, reserved := range []string{"props", "state"} {
		if _, ok := d.Values[reserved]; ok {
			return fmt.Errorf("render values cannot replace %s", reserved)
		}
	}
	if err := designsource.CheckData(d.Values); err != nil {
		return err
	}
	if err := designsource.CheckData(d.State); err != nil {
		return err
	}
	path := make([]any, 0, len(d.Path))
	for _, s := range d.Path {
		var key any
		if s.Key != nil {
			key = map[string]any{"kind": s.Key.Kind, "value": s.Key.Value}
		}
		path = append(path, map[string]any{"file": s.Site.File, "pid": s.Site.PID, "key": key})
	}
	return designsource.CheckData(map[string]any{"file": d.File, "path": path})
}

func instanceID(file string, path []InstanceSegment) string {
	// Empty and nil paths both designate the root.
	if len(path) == 0 {
		path = nil
	}
	encoded, _ := json.Marshal(struct {
		File string
		Path []InstanceSegment
	}{file, path})
	return string(encoded)
}

func (p *projector) scope(file string, path []InstanceSegment, overrides map[string]any) (map[string]any, error) {
	id := instanceID(file, path)
	d, ok := p.data[id]
	if !ok {
		return nil, fmt.Errorf("missing explicit snapshot data for %s", file)
	}
	p.used[id] = true
	props := p.plan.Frames[file].Props
	if props == nil {
		props = &designsource.Props{Schema: map[string]designsource.Property{}}
	}
	resolved, err := props.Resolve(overrides)
	if err != nil {
		return nil, err
	}
	scope := make(map[string]any, len(d.Values)+2)
	for k, v := range d.Values {
		scope[k] = v
	}
	scope["props"] = resolved
	scope["state"] = d.State
	return scope, nil
}

func (p *projector) problem(err error, span designsource.SourceSpan) error {
	var problems designsource.Problems
	if errors.As(err, &problems) {
		return err
	}
	return designsource.Problems{{Code: "projection", Message: err.Error(), Span: span, Severity: "error"}}
}

func (p *projector) step() error {
	p.work++
	if p.work > MaxProjectionNodes {
		return fmt.Errorf("projection work exceeds %d nodes/iterations", MaxProjectionNodes)
	}
	return nil
}

func (p *projector) text(value string) error {
	p.payload += len(value)
	if p.payload > MaxProjectionBytes {
		return fmt.Errorf("projection text/attributes exceed %d bytes", MaxProjectionBytes)
	}
	return nil
}
