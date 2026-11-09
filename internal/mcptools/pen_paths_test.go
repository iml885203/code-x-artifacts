package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store/storetest"
)

func TestConnectedPenPathsKeepTheirIdentityThroughHumanRoomEditsMCPAndSavedReload(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	svc, snap, hub, srv := newPenEndpoint(t)
	agent := connect(t, srv, "alice")
	const file = "frames/hero.html"
	const manifest = `{"version":1,"pages":[{"id":"home","name":"Home","rows":[[{"id":"hero","name":"Hero","file":"frames/hero.html","w":800,"h":600}]]}]}`
	created := call(t, agent, "publish", map[string]any{"title": "Pen paths", "kind": "design", "format": "1", "visibility": "organisation", "content": manifest, "files": map[string]any{file: `<!doctype html><html><head><meta charset="utf-8"></head><body data-pid="body"><p data-pid="copy">Keep this text</p></body></html>`}})
	id := created["id"].(string)
	// These are the ordinary SVG entries the Pen writes, including a straight segment and dragged cubic handles.
	const geometry = "M1.5 1.5L51.5 1.5C51.5 1.5 71.5 21.5 91.5 41.5C111.5 61.5 1.5 1.5 1.5 1.5Z"
	err := hub.Edit(ctx, id, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		m := doc.GetMap(designroom.Map)
		return func(txn *crdt.Transaction) {
			nodes := map[string]map[string]string{
				"pen":      {"kind": "el", "ns": "svg", "tag": "svg", "parent": "body", "order": "z", "attrs": `[["","data-pen","stroke"],["","data-name","Path"],["","viewBox","0 0 113 63"],["","preserveAspectRatio","none"]]`, "s|position": "absolute", "s|left": "8.5px", "s|top": "18.5px", "s|width": "113px", "s|height": "63px"},
				"pen-path": {"kind": "el", "ns": "svg", "tag": "path", "parent": "pen", "order": "V", "attrs": `[["","d","` + geometry + `"],["","stroke","#336699"],["","stroke-width","3"],["","fill","none"],["","stroke-linecap","round"],["","stroke-linejoin","round"],["","pointer-events","none"]]`},
			}
			for pid, fields := range nodes {
				for field, value := range fields {
					m.Set(txn, file+"|n|"+pid+"|"+field, value)
				}
			}
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	read := call(t, agent, "read", map[string]any{"id": id, "file": file})
	source := read["file"].(map[string]any)["text"].(string)
	if !strings.Contains(source, `d="`+geometry+`"`) || !strings.Contains(source, `data-pid="pen-path"`) {
		t.Fatalf("MCP lost the human path or identity: %s", source)
	}
	proof := read["sourceBases"]
	if proof == nil {
		t.Fatal("current format 1 source did not issue a write proof")
	}
	// A person's independent move after the agent's read must survive its geometry-only update.
	err = hub.Edit(ctx, id, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return designroom.Open(doc).SetStyle(file, "pen", "left", "100px")
	})
	if err != nil {
		t.Fatal(err)
	}
	nextGeometry := "M1.5 1.5L51.5 1.5C51.5 1.5 71.5 21.5 91.5 41.5"
	next := strings.Replace(source, geometry, nextGeometry, 1)
	out := call(t, agent, "publish", map[string]any{"id": id, "title": "Pen paths", "content": manifest, "baseVersion": read["version"], "sourceBases": proof, "files": map[string]any{file: next}})
	if out["live"] != true {
		t.Fatalf("MCP bypassed the room: %v", out)
	}
	want := strings.Replace(next, "left:8.5px", "left:100px", 1)
	if want == next {
		t.Fatal("fixture had no move to preserve")
	}
	assertPenRoomSource(t, hub, id, file, want)
	version := assertSavedPenReload(t, svc, snap, id, file, want)
	reader := connect(t, srv, "bob")
	refused, err := reader.CallTool(ctx, &mcp.CallToolParams{Name: "publish", Arguments: map[string]any{"id": id, "title": "Pen paths", "content": manifest, "baseVersion": version, "files": map[string]any{file: next}}})
	if err != nil || !refused.IsError {
		t.Fatalf("reader wrote a Pen path: %v %+v", err, refused)
	}
	_, err = svc.Update(ctx, svc.Viewer("bob"), id, service.PublishInput{Title: "Pen paths", Content: manifest, BaseVersion: version, Files: map[string]*string{file: &next}})
	var denied *service.Error
	if !errors.As(err, &denied) || denied.Code != service.Forbidden {
		t.Fatalf("service did not refuse the same reader: %v", err)
	}
	assertPenRoomSource(t, hub, id, file, want)
}

func assertPenRoomSource(t *testing.T, hub *realtime.Hub, id, file, want string) {
	t.Helper()
	if err := hub.Read(context.Background(), id, func(doc *crdt.Doc) error {
		if err := designroom.Check(doc); err != nil {
			return err
		}
		held, err := designroom.Open(doc).Read()
		if err != nil {
			return err
		}
		if held.Frames[file] != want {
			t.Fatalf("path geometry, identities or independent content changed:\nwant %s\ngot %s", want, held.Frames[file])
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func newPenEndpoint(t *testing.T) (*service.Service, *roomkind.Snapshotter, *realtime.Hub, *httptest.Server) {
	t.Helper()
	st := storetest.Open(t)
	disk, err := assets.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(st, disk, false)
	codecs := roomkind.DefaultCodecs()
	snap := roomkind.NewSnapshotter(roomkind.Config{Store: st, Versions: svc, Files: svc, Codecs: codecs})
	hub, err := realtime.New(realtime.Config{Log: roomkind.SeededLog(st.RealtimeLog(), st, svc, codecs, 0), Validate: codecs.Check, OnUpdate: snap.Updated})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hub.Close(context.Background()) })
	snap.Use(hub)
	svc.SetLiveRooms(snap)
	endpoint := New(Config{Service: svc, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	srv := httptest.NewServer(asUser(svc, endpoint.Handler()))
	t.Cleanup(srv.Close)
	return svc, snap, hub, srv
}

func assertSavedPenReload(t *testing.T, svc *service.Service, snap *roomkind.Snapshotter, id, file, want string) int {
	t.Helper()
	if err := snap.Flush(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	saved, err := svc.Artifact(t.Context(), svc.Viewer("alice"), id)
	if err != nil {
		t.Fatal(err)
	}
	files, err := svc.FileTexts(t.Context(), id, saved.Version)
	if err != nil {
		t.Fatal(err)
	}
	if files[file] != want {
		t.Fatalf("saved path differs:\n%s", files[file])
	}
	reloaded := crdt.New()
	write, err := designroom.Open(reloaded).Write(designroom.Design{Manifest: saved.Content, Frames: files})
	if err != nil {
		t.Fatal(err)
	}
	reloaded.Transact(write)
	if err := designroom.Check(reloaded); err != nil {
		t.Fatal(err)
	}
	held, err := designroom.Open(reloaded).Read()
	if err != nil || held.Frames[file] != want {
		t.Fatalf("reload changed path identity or geometry: %v %s", err, held.Frames[file])
	}
	return saved.Version
}

func TestConnectedPenSerializedCoordinatesUseTheNativePathValidator(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../web/src/design/pen/path-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name     string
			Expected *string
			Rejected string
		}
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("no shared client serialization fixtures")
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if c.Expected != nil {
				if err := designfmt.CheckPath(*c.Expected); err != nil {
					t.Fatalf("client serialization is not valid native geometry: %v", err)
				}
			} else {
				if c.Rejected == "" {
					t.Fatal("missing rejected serialization")
				}
				if err := designfmt.CheckPath(c.Rejected); err == nil {
					t.Fatal("native validator accepted the overflow the client must refuse")
				}
			}
		})
	}
}
