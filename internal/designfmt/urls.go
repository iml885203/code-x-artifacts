package designfmt

import (
	"fmt"
	"strings"
)

// checkAddress accepts what a frame may point at: a fragment of itself, a small
// picture or font written into the markup, and the pictures and fonts of its own
// artifact or the platform's. Everything else is an external address, which a
// frame cannot load (its policy has no place to load it from) and an author
// should not rely on.
func checkAddress(raw string) error {
	u := cleanAddress(raw)
	lower := strings.ToLower(u)
	switch {
	case u == "", strings.HasPrefix(u, "#"):
		return nil
	case strings.HasPrefix(lower, "data:image/"), strings.HasPrefix(lower, "data:font/"), strings.HasPrefix(lower, "data:application/font"), strings.HasPrefix(lower, "data:application/x-font"):
		return nil
	case strings.HasPrefix(u, "/files/"), strings.HasPrefix(u, "/static/fonts/"):
		return nil
	}
	return fmt.Errorf("external address %.60q is not allowed: a frame may use #fragments, data: pictures, and files uploaded to this artifact (/files/<id>/<name>)", raw)
}

// cleanAddress is an address as a browser reads it: trimmed, with the control
// characters it ignores removed.
func cleanAddress(raw string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
}

// isLink reports whether an address is one a person may follow from a frame:
// https or mailto. Only the href of an <a> may be one; a picture, a font or a
// stylesheet address outside the frame is still refused.
func isLink(raw string) bool {
	lower := strings.ToLower(cleanAddress(raw))
	return strings.HasPrefix(lower, "https:") || strings.HasPrefix(lower, "mailto:")
}
