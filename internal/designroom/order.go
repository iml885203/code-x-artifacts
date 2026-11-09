package designroom

import "strings"

const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

const base = len(digits)

func digitAt(s string, i int) int {
	if i >= len(s) {
		return 0
	}
	return strings.IndexByte(digits, s[i])
}

// between is an order key that sorts strictly after a and before b, where an
// empty a is the start and an empty b the end. Keys are strings of base 62
// digits read as fractions, so there is always one between two others and
// inserting never renumbers a sibling: two people moving different elements
// never touch each other's keys.
func between(a, b string) string {
	if b != "" && a >= b {
		b = "" // no room between equal keys (two people inserted at one place): go after a
	}
	var out []byte
	constrained := b != ""
	for i := 0; ; i++ {
		lo, hi := digitAt(a, i), base
		if constrained {
			hi = digitAt(b, i)
		}
		if lo+1 < hi {
			return string(append(out, digits[(lo+hi)/2]))
		}
		out = append(out, digits[lo])
		if constrained && lo < hi {
			constrained = false
		}
	}
}

// startKeys are n increasing keys for the first children of a node.
func startKeys(n int) []string {
	keys := make([]string, n)
	prev := ""
	for i := range keys {
		prev = between(prev, "")
		keys[i] = prev
	}
	return keys
}
