package designfmt

import (
	"fmt"
	"strings"
)

// The single-value rules of design.json, for what checks one value at a time
// (a room whose structure is kept as one entry per value, docs/design-room.md)
// rather than a whole manifest: each returns what is wrong, or "".

// IDProblem says what is wrong with a page or frame id.
func IDProblem(id string) string {
	if !idPattern.MatchString(id) {
		return "an id is 1 to 40 characters of lower case letters, digits, dash and underscore, starting with a letter or digit"
	}
	return ""
}

// NameProblem says what is wrong with a page or frame name.
func NameProblem(name string) string {
	switch {
	case strings.TrimSpace(name) == "":
		return "a name is required"
	case len([]rune(name)) > MaxNameLength:
		return fmt.Sprintf("a name is at most %d characters", MaxNameLength)
	}
	return ""
}

// SideProblem says what is wrong with a frame's width or height.
func SideProblem(field string, n int, ok bool) string {
	if !ok || n < 1 || n > MaxSide {
		return fmt.Sprintf("%s must be a whole number of pixels from 1 to %d", field, MaxSide)
	}
	return ""
}
