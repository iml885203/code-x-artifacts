package kind

import (
	"strings"
	"testing"
)

func TestTheDesignGuideDistinguishesPencilFromConnectedPenAndTeachesItsSourceContract(t *testing.T) {
	t.Parallel()
	guide := (Design{}).Guide()
	for _, want := range []string{"Pencil (P)", "Pen (Shift+P)", "512 anchors", "at least two distinct anchors", "Escape", "anchor + handle", "anchor - handle", "M/L/C/Z", "sourceBases", "baseVersion", "data-pid", "Readers cannot publish"} {
		if !strings.Contains(guide, want) {
			t.Errorf("drawing guide does not teach %q", want)
		}
	}
}
