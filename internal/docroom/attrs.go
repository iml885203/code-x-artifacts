package docroom

import (
	"math"
	"strconv"

	"github.com/reearth/ygo/crdt"
)

// The quirks of ygo's attribute API, handled here once. An attribute is typed
// on the wire: the editor writes a heading's level as a number and a task's
// checked as a boolean, and what comes back from the room is an int64 when the
// update was made by Go, an int64 or a float64 when a JavaScript client made it
// (it depends on the number), and nothing at all for a null the editor left
// out. GetAttribute would turn the number into text; these read the value
// itself and treat a value of the wrong type as absent.

func stringAttr(e *crdt.YXmlElement, key string) (string, bool) {
	v, ok := e.GetAttributeValue(key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func intAttr(e *crdt.YXmlElement, key string) int {
	v, _ := e.GetAttributeValue(key)
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		if n == math.Trunc(n) && math.Abs(n) < 1<<31 {
			return int(n)
		}
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
		}
	}
	return 0
}

func boolAttr(e *crdt.YXmlElement, key string) bool {
	v, _ := e.GetAttributeValue(key)
	b, _ := v.(bool)
	return b
}
