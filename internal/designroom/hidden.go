package designroom

import (
	"fmt"
	"strings"
)

// Hiding an element is a state of its own (the `hidden` entry), not a value of
// the display property: the panel's layout controls write display (flex), and
// hiding then showing must give that back. While an element is hidden the file
// says display:none; showing it again takes the declaration away and the display
// the panel wrote, if any, is in the file again. "0" is for an element whose own
// markup says display:none and that the editor shows: it then reads display:revert
// unless the editor has written a display of its own.

// visible is the element's owned style as the file should say it, with the
// hidden state applied.
func (n node) visible() map[string]string {
	switch n.hidden {
	case "1":
		out := make(map[string]string, len(n.style)+1)
		for k, v := range n.style {
			out[k] = v
		}
		out["display"] = "none"
		return out
	case "0":
		if strings.TrimSpace(n.style["display"]) == "" && declaresNone(n.rest) {
			out := make(map[string]string, len(n.style)+1)
			for k, v := range n.style {
				out[k] = v
			}
			out["display"] = "revert"
			return out
		}
	}
	return n.style
}

// declaresNone reports whether a style skeleton declares display:none.
func declaresNone(skeleton string) bool {
	none := false
	for _, p := range splitDeclarations(skeleton) {
		if name, at, ok := propertyOf(p); ok && name == "display" {
			v := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(p[at:]), "!important")))
			none = v == "none"
		}
	}
	return none
}

// checkStyleEntries holds every style and visibility entry of the room to the
// grammar of values, whoever wrote it.
func checkStyleEntries(e map[string]any) error {
	for k, v := range e {
		_, rest, ok := strings.Cut(k, "|n|")
		if !ok {
			continue
		}
		id, field, ok := strings.Cut(rest, "|")
		if !ok {
			continue
		}
		if prop, isStyle := strings.CutPrefix(field, "s|"); isStyle {
			if err := ValidateStyleValue(prop, str(v)); err != nil {
				return fmtStyle(k, id, err)
			}
		} else if prop, isOverride := strings.CutPrefix(field, "override|"); isOverride {
			if err := ValidateStyleValue(prop, str(v)); err != nil {
				return fmtStyle(k, id, err)
			}
		} else if field == "hidden" {
			if err := ValidateVisibility(str(v)); err != nil {
				return fmtStyle(k, id, err)
			}
		}
	}
	return nil
}

func fmtStyle(key, id string, err error) error {
	file, _, _ := strings.Cut(key, "|")
	return fmt.Errorf("%s: the style of %s: %w", file, id, err)
}
