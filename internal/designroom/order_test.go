package designroom

import "testing"

func TestBetweenGivesAKeyStrictlyBetweenWhateverTheNeighbours(t *testing.T) {
	cases := [][2]string{{"", ""}, {"", "1"}, {"V", ""}, {"V", "W"}, {"V", "V1"}, {"zz", ""}, {"0", "1"}, {"a", "a"}, {"b", "a"}}
	for _, c := range cases {
		got := between(c[0], c[1])
		if got <= c[0] {
			t.Errorf("between(%q, %q) = %q, not after the first", c[0], c[1], got)
		}
		if c[1] != "" && c[0] < c[1] && got >= c[1] {
			t.Errorf("between(%q, %q) = %q, not before the second", c[0], c[1], got)
		}
	}
	// Inserting again and again at one place never runs out of keys.
	lo, hi := "V", "W"
	for range 200 {
		mid := between(lo, hi)
		if mid <= lo || mid >= hi {
			t.Fatalf("%q is not between %q and %q", mid, lo, hi)
		}
		hi = mid
	}
}

func TestStartKeysIncrease(t *testing.T) {
	keys := startKeys(300)
	for i := 1; i < len(keys); i++ {
		if keys[i] <= keys[i-1] {
			t.Fatalf("key %d (%q) is not after %q", i, keys[i], keys[i-1])
		}
	}
}

func TestAStyleAttributeSplitsIntoASkeletonAndComesBackExactly(t *testing.T) {
	for _, style := range []string{
		"position:absolute;left:48px;top:300px;width:220px",
		"left: 1px !important; color:red;",
		"display:-webkit-box;display:flex;left:5px",
		"background:url('data:image/svg+xml;utf8,<svg/>') no-repeat;top:0",
		"left:1px;left:2px",
		"",
		";;",
		"--x:{a:b;c};width:3px",
	} {
		skeleton, owned := splitStyle(style)
		back, ok := joinStyle(skeleton, owned, true)
		if !ok || back != style {
			t.Errorf("%q came back as %q (ok %v)", style, back, ok)
		}
	}
	skeleton, owned := splitStyle("left:1px;left:2px;top:3px")
	if _, has := owned["left"]; has || owned["top"] != "3px" {
		t.Errorf("a property written twice must not be owned: %v (%q)", owned, skeleton)
	}
}
