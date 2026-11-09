package main

import "testing"

func TestAssetsLiveNextToTheDatabaseUnlessTheDeployerSaysOtherwise(t *testing.T) {
	cases := []struct{ db, configured, want string }{
		{"/data/pictor.db", "", "/data/assets"},
		{"pictor.db", "", "assets"},
		{"/data/pictor.db", "/files/pictor", "/files/pictor"},
	}
	for _, c := range cases {
		if got := assetsDir(c.db, c.configured); got != c.want {
			t.Errorf("assetsDir(%q, %q) = %q, want %q", c.db, c.configured, got, c.want)
		}
	}
}
