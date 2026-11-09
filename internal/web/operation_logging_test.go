package web

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplacingAnAssetKeepsItsSuccessAndLogsAnUnremovableOldFile(t *testing.T) {
	t.Parallel()
	var logs syncBuffer
	h := newHarness(t, func(c *Config) { c.Logger = slog.New(slog.NewTextHandler(&logs, nil)) })
	tok := h.token("alice")
	a := h.publish(tok, "Report", "<p>x")
	h.mustUpload(tok, a.ID, "chart.png", pngBytes)
	records, err := h.st.Assets(t.Context(), a.ID)
	if err != nil || len(records) != 1 {
		t.Fatalf("assets: %v %v", records, err)
	}
	old := records[0].Blob
	path := filepath.Join(h.assetsDir, a.ID, old)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// A nonempty directory cannot be removed as a file, regardless of the user's permissions.
	if err := os.Mkdir(path, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "held"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, body := h.upload(tok, a.ID, "chart.png", pngBytes)
	if res.StatusCode != 200 || strings.Contains(body, "unreferenced") {
		t.Fatalf("replacement lost its successful response: %d %s", res.StatusCode, body)
	}
	if status, body := h.fileStatus(h.browser("alice", "pw-a"), "/files/"+a.ID+"/chart.png"); status != 200 || body != pngBytes {
		t.Fatalf("replacement: %d %q", status, body)
	}
	out := logs.String()
	for _, want := range []string{`level=INFO msg="artifact created" id=` + a.ID + ` user=alice`, `level=ERROR msg="could not delete an unreferenced asset file" id=` + a.ID + ` key=` + old + ` err=`, `level=INFO msg="asset stored" id=` + a.ID + ` name=chart.png type=image/png bytes=`, `user=alice replaced=true`} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
}

func TestAnMCPSessionCommitLogsItsNestedPublishOnceAtTheRequestEdge(t *testing.T) {
	t.Parallel()
	var logs syncBuffer
	h := newHarness(t, func(c *Config) { c.Logger = slog.New(slog.NewTextHandler(&logs, nil)) })
	agent := h.agent(h.token("alice"))
	content := "<p>report"
	begun := agent.ok("begin_publish", map[string]any{"title": "Report", "files": fileList(declared("content", content))})
	h.curl(begun, map[string]string{"content": content})
	done := agent.ok("commit_publish", map[string]any{"session": begun["session"]})
	id := done["id"].(string)
	if h.storedContent(id) != content {
		t.Fatal("commit did not persist the content")
	}
	out := logs.String()
	for _, message := range []string{"publish session begun", "artifact created", "publish session committed"} {
		if count := strings.Count(out, `level=INFO msg="`+message+`"`); count != 1 {
			t.Errorf("%q logged %d times:\n%s", message, count, out)
		}
	}
	if !strings.Contains(out, "session="+begun["session"].(string)+" user=alice id="+id) {
		t.Errorf("commit lost its audit fields:\n%s", out)
	}
}
