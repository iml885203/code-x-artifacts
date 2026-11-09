package kind

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"testing"
)

// What the panel writes (web/src/design/properties/owned.json) is what the
// inspector in each frame is willing to show, no more and no less.
func TestTheInspectorShowsExactlyThePropertiesThePanelWrites(t *testing.T) {
	raw, err := os.ReadFile("../../web/src/design/properties/owned.json")
	if err != nil {
		t.Fatal(err)
	}
	var list struct{ Owned, Shown []string }
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)(?:const|var)\s+OWNED\s*=\s*\[(.*?)\];`).FindStringSubmatch(designInspector)
	if block == nil {
		t.Fatal("the inspector has no OWNED list")
	}
	var got []string
	for _, m := range regexp.MustCompile(`"([a-z-]+)"`).FindAllStringSubmatch(block[1], -1) {
		got = append(got, m[1])
	}
	want := append(slices.Clone(list.Owned), list.Shown...)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("the inspector's list differs from owned.json:\ninspector %v\nowned.json %v", got, want)
	}
}
