package designrender

import (
	"reflect"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

func snapshot() Snapshot {
	return Snapshot{ArtifactID: "artifact", Version: 4, Input: designsource.ParseInput{Format: "2", Manifest: `{"version":2,"runtime":"pictor-design/2.0.0","pages":[{"id":"page","name":"Page","rows":[[{"id":"a","name":"A","file":"frames/a.html","w":100,"h":100},{"id":"b","name":"B","file":"frames/b.html","w":100,"h":100},{"id":"c","name":"C","file":"frames/c.html","w":100,"h":100}]]}]}`, Files: map[string]string{"frames/a.html": `<p-import data-pid="site" src="frames/b.html"></p-import>`, "frames/b.html": `<p data-pid="b">你好</p>`, "frames/c.html": `<p data-pid="c">Other</p>`}}}
}

func TestEveryContextPurposeRetainsOneStoredSnapshotAndSourceClosure(t *testing.T) {
	for _, purpose := range []string{"canvas", "play", "preview", "history", "raw", "overview", "offline", "print"} {
		t.Run(purpose, func(t *testing.T) {
			s := snapshot()
			c, err := BuildContext(s, Request{FrameID: "a", Purpose: purpose, CanEdit: true})
			if err != nil {
				t.Fatal(err)
			}
			checkStoredContext(t, c)
			if purpose == "history" && c.CanEdit {
				t.Fatal("history is editable")
			}
			before := c.Plan.Frames["frames/b.html"].Source
			s.Input.Files["frames/b.html"] = `<p data-pid="b">Changed</p>`
			if c.Plan.Frames["frames/b.html"].Source != before {
				t.Fatal("snapshot aliases caller source map")
			}
			changed, err := BuildContext(s, Request{FrameID: "a", Purpose: purpose})
			if err != nil {
				t.Fatal(err)
			}
			if c.DependencySHA256 == changed.DependencySHA256 {
				t.Fatal("child source absent from digest")
			}
			s = snapshot()
			s.Input.Files["frames/c.html"] = `<p data-pid="c">Unrelated change</p>`
			unrelated, err := BuildContext(s, Request{FrameID: "a", Purpose: purpose})
			if err != nil {
				t.Fatal(err)
			}
			if unrelated.DependencySHA256 != c.DependencySHA256 {
				t.Fatal("unrelated source entered closure")
			}
		})
	}
}

func TestContextRefusesMissingSnapshotIdentityAndNeverFallsBackToRunnableOutput(t *testing.T) {
	for _, change := range []func(*Snapshot, *Request){
		func(s *Snapshot, _ *Request) { s.ArtifactID = "" },
		func(s *Snapshot, _ *Request) { s.Version = 0 },
		func(s *Snapshot, _ *Request) { s.Version = -1 },
		func(s *Snapshot, _ *Request) { s.Revision = "mixed" },
		func(_ *Snapshot, r *Request) { r.Purpose = "" },
		func(_ *Snapshot, r *Request) { r.FrameID = "missing" },
		func(s *Snapshot, _ *Request) { s.Input.Format = "1" },
		func(s *Snapshot, _ *Request) {
			s.Input.Manifest = strings.ReplaceAll(s.Input.Manifest, designsource.RuntimeID, "unknown")
		},
		func(s *Snapshot, _ *Request) { delete(s.Input.Files, "frames/b.html") },
		func(s *Snapshot, _ *Request) { s.Version = 0; s.Revision = "live" },
	} {
		s := snapshot()
		r := Request{FrameID: "a", Purpose: "history"}
		change(&s, &r)
		if _, err := BuildContext(s, r); err == nil {
			t.Fatal("accepted missing or inconsistent context")
		}
	}
	s := snapshot()
	s.Version = 0
	s.Revision = "server-generation"
	c, err := BuildContext(s, Request{FrameID: "a", Purpose: "canvas", CanEdit: false})
	if err != nil || c.Revision != s.Revision || c.CanEdit {
		t.Fatalf("live read-only identity: %#v %v", c, err)
	}
}

func checkStoredContext(t *testing.T, c RenderContext) {
	t.Helper()
	facts := []any{c.ArtifactID, c.Version, c.Format, c.RuntimeID, c.RuntimeSHA256, c.FrameFile}
	if !reflect.DeepEqual(facts, []any{"artifact", 4, "2", designsource.RuntimeID, "", "frames/a.html"}) {
		t.Fatalf("mixed or invented context: %#v", c)
	}
	var files []string
	for _, d := range c.Dependencies {
		files = append(files, d.File)
	}
	if !reflect.DeepEqual(files, []string{"frames/a.html", "frames/b.html"}) {
		t.Fatalf("source closure: %v", files)
	}
}
