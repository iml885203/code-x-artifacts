package designroom

import "strings"

// Keep a covered authored declaration literal so an explicit shared override is
// appended after its covering shorthand without changing the authored cascade.
func coveredByLaterShorthand(prop string, later []string) bool {
	for _, declaration := range later {
		shorthand, _, ok := propertyOf(declaration)
		if !ok {
			continue
		}
		if strings.HasPrefix(prop, "background-") && shorthand == "background" {
			return true
		}
		if isBorderSideProperty(prop) {
			pieces := strings.Split(prop, "-")
			if shorthand == "border" || shorthand == "border-"+pieces[1] || shorthand == "border-"+pieces[2] {
				return true
			}
		}
	}
	return false
}

// Promote a source edit to a covered longhand into a shared override, keeping
// the authored declaration and every unrelated source change in the skeleton.
func promoteCoveredStyleEdits(base, next node) node {
	before, after := splitDeclarations(base.rest), splitDeclarations(next.rest)
	for i, declaration := range after {
		prop, at, ok := propertyOf(declaration)
		if !ok || !Owned(prop) || !coveredByLaterShorthand(prop, after[i+1:]) || strings.Contains(declaration, slotOpen) {
			continue
		}
		var originals []string
		for _, old := range before {
			name, _, ok := propertyOf(old)
			if ok && name == prop {
				originals = append(originals, old)
			}
		}
		if len(originals) != 1 || originals[0] == declaration || strings.Contains(originals[0], slotOpen) {
			continue
		}
		next.style[prop] = declaration[at:]
		after[i] = originals[0]
	}
	next.rest = strings.Join(after, ";")
	return next
}

func coveredSlot(skeleton, prop string) bool {
	parts := splitDeclarations(skeleton)
	for i, part := range parts {
		if strings.Contains(part, slotOpen+prop+slotClose) && coveredByLaterShorthand(prop, parts[i+1:]) {
			return true
		}
	}
	return false
}

// Overrides are separate shared entries, so Undo and concurrent edits never
// rewrite the authored slot or a common skeleton.
func (n node) joinStyle(hadStyle bool) (string, bool) {
	parts := splitDeclarations(n.rest)
	owned := n.visible()
	if len(n.styleOverrides) > 0 {
		owned = make(map[string]string, len(n.style)+len(n.styleOverrides))
		for prop, value := range n.visible() {
			owned[prop] = value
		}
		for prop, value := range n.styleOverrides {
			owned[prop] = value
		}
	}
	for i, part := range parts {
		open, end := strings.Index(part, slotOpen), strings.Index(part, slotClose)
		if open < 0 || end < open {
			continue
		}
		prop := part[open+len(slotOpen) : end]
		if _, overridden := n.styleOverrides[prop]; overridden {
			parts[i] = part[:open] + n.style[prop]
		}
	}
	return joinStyle(strings.Join(parts, ";"), owned, hadStyle)
}

// Only the final override of a covered authored longhand gets a shared slot;
// ordinary duplicate/fallback declarations keep their existing literal form.
func overrideAfterShorthand(prop string, parts []string, at int) bool {
	found := false
	for i, part := range parts {
		name, _, ok := propertyOf(part)
		if !ok || name != prop || i == at {
			continue
		}
		if i > at || !coveredByLaterShorthand(prop, parts[i+1:at]) {
			return false
		}
		found = true
	}
	return found
}

// Parsing a legacy frame leaves covered longhands literal. An unrelated source
// change must retain their existing room slots rather than append stale values.
func retainUntouchedCoveredSlots(base, next, current node) (node, node) {
	before, after := splitDeclarations(base.rest), splitDeclarations(next.rest)
	for _, part := range splitDeclarations(current.rest) {
		prop, _, ok := propertyOf(part)
		if ok && retainCoveredSlot(base, next, current, prop) {
			restoreCoveredSlot(before, after, prop, part)
		}
	}
	base.rest, next.rest = strings.Join(before, ";"), strings.Join(after, ";")
	return base, next
}

func retainCoveredSlot(base, next, current node, prop string) bool {
	if !coveredSlot(current.rest, prop) {
		return false
	}
	if _, owned := next.style[prop]; owned {
		return false
	}
	_, owned := base.style[prop]
	_, overridden := current.styleOverrides[prop]
	return !owned || overridden
}

func restoreCoveredSlot(before, after []string, prop, slot string) {
	for i, old := range before {
		name, _, ok := propertyOf(old)
		if !ok || name != prop || strings.Contains(old, slotOpen) {
			continue
		}
		for j, changed := range after {
			if changed == old {
				before[i], after[j] = slot, slot
				break
			}
		}
	}
}

func (f *frameMerge) setStyleValue(id, prop, value string) {
	current := f.ci[id]
	key := "s|" + prop
	old, exists := current.style[prop]
	if override, overridden := current.styleOverrides[prop]; overridden || coveredSlot(current.rest, prop) {
		key = "override|" + prop
		old, exists = override, overridden
	}
	if !exists || old != value {
		f.set(id, key, value)
	}
}

func (f *frameMerge) dropStyleValue(id, prop string) {
	current := f.ci[id]
	if _, exists := current.styleOverrides[prop]; exists {
		f.drop([]string{nodeKey(f.file, id, "override|"+prop)})
	} else if _, exists := current.style[prop]; exists {
		f.drop([]string{styleKey(f.file, id, prop)})
	}
}
