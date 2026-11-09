package web

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/kind"
)

// The web tests that no page names a kind run over web/src/registered-kinds.json.
// A kind added without being listed there would escape them.
func TestTheWebTestsCoverEveryRegisteredKind(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../web/src/registered-kinds.json")
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(kind.Names(), listed) {
		t.Fatalf("web/src/registered-kinds.json lists %v, the server has %v", listed, kind.Names())
	}
}
