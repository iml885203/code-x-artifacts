package designroom

import (
	"fmt"
	"regexp"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

var (
	nodeID  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
	tagName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)
)

// checkNodes holds every element of a frame, however it came into the room, to
// the rules a published frame is held to: its tag and each of its attributes (so
// an <img> points only at what a frame may point at). Its style is held to the
// grammar of values, entry by entry (checkStyleEntries). The editor
// writes new elements itself (an inserted text box, a duplicate, a group), and a
// modified client could write anything, so the room does not trust what it holds
// to be a published frame's.
func checkNodes(file string, m frameModel) error {
	tags := map[string]string{} // a tag is judged once however many elements have it
	for _, n := range m.nodes {
		if n.kind != kindElement || n.id == m.root {
			continue
		}
		if !nodeID.MatchString(n.id) {
			return fmt.Errorf("%s: %q is not an id an element can have", file, n.id)
		}
		if !tagName.MatchString(n.tag) {
			return fmt.Errorf("%s: %s: %q is not a tag", file, n.id, n.tag)
		}
		if n.ns == "" {
			verdict, known := tags[n.tag]
			if !known {
				if problems := designfmt.CheckFragment("<" + n.tag + ">"); len(problems) > 0 {
					verdict = problems[0]
				}
				tags[n.tag] = verdict
			}
			if verdict != "" {
				return fmt.Errorf("%s: %s: %s", file, n.id, verdict)
			}
		}
		for _, a := range n.attrs {
			if a.Key == markPID || a.Key == markStyle {
				continue
			}
			key := a.Key
			if a.Ns != "" {
				key = a.Ns + ":" + a.Key
			}
			if problems := designfmt.CheckAttribute(n.tag, key, a.Val); len(problems) > 0 {
				return fmt.Errorf("%s: %s: %s", file, n.id, problems[0])
			}
		}
	}
	return nil
}
