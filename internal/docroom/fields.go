package docroom

import (
	"slices"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/reearth/ygo/crdt"
)

type fieldAttribute struct{ node, key, value string }

func attributeOf(run docfmt.Inline) (fieldAttribute, bool) {
	switch {
	case run.Date != "":
		return fieldAttribute{nodeDate, attrDate, run.Date}, true
	case run.Dropdown != nil:
		return fieldAttribute{nodeDropdown, attrConfig, run.Dropdown.JSON()}, true
	default:
		return fieldAttribute{}, false
	}
}

// Field edits retain the surrounding shared text so concurrent typing survives.
func (p *planner) fields(parent *crdt.YXmlElement, old, next []docfmt.Inline) bool {
	if len(old) != len(next) {
		return false
	}
	masked := slices.Clone(next)
	var values []fieldAttribute
	for i, run := range old {
		was, field := attributeOf(run)
		if !field {
			continue
		}
		value, present := attributeOf(next[i])
		if !present || value.node != was.node {
			return false
		}
		values = append(values, value)
		masked[i].Date, masked[i].Dropdown = run.Date, run.Dropdown
	}
	if len(values) == 0 || keyOf(old) != keyOf(masked) {
		return false
	}
	elements := fieldElements(parent)
	if len(elements) != len(values) {
		return false
	}
	for i, element := range elements {
		if element.NodeName != values[i].node {
			return false
		}
	}
	for i, element := range elements {
		value := values[i]
		was, _ := stringAttr(element, value.key)
		p.syncString(element, value.key, was, value.value)
	}
	return true
}

func fieldElements(parent *crdt.YXmlElement) []*crdt.YXmlElement {
	var elements []*crdt.YXmlElement
	for _, child := range parent.Children() {
		if element, ok := child.(*crdt.YXmlElement); ok && (element.NodeName == nodeDate || element.NodeName == nodeDropdown) {
			elements = append(elements, element)
		}
	}
	return elements
}
