package web

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
)

// The server wires the live side together: edits in a room are noticed, a quiet
// room becomes a version with its authors, and a restore goes through the room.
func TestAQuietRoomBecomesAVersionOverTheAPIAndARestoreGoesThroughTheRoom(t *testing.T) {
	t.Parallel()
	var logs syncBuffer
	h := newLiveHarness(t, func(c *Config) { c.Logger = slog.New(slog.NewTextHandler(&logs, nil)) })
	tok := h.token("alice")
	res, out := api(h.harness, "POST", "/api/artifacts", `{"title":"Doc","kind":"doc","content":"# Plan\n\nfirst\n"}`, tok)
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		t.Fatalf("publish: %d %s", res.StatusCode, out)
	}
	id := decode[artifactView](t, out).ID

	edited := docfmt.Doc{Tabs: []docfmt.Tab{{ID: "t_x", Title: "Document", Blocks: []docfmt.Block{
		{ID: "b_x", Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: "typed in the room"}}},
	}}}}
	err := h.app.live.hub.Edit(t.Context(), id, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return docroom.Open(doc).Write(edited), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(roomkind.Quiet)
	h.app.live.snap.Sweep(t.Context())
	if out := logs.String(); !strings.Contains(out, `level=INFO msg="artifact saved from its room" id=`+id+` authors=[alice]`) {
		t.Errorf("background snapshot lost its audit: %s", out)
	}

	var list []struct {
		Version int      `json:"version"`
		Authors []string `json:"authors"`
	}
	_, raw := api(h.harness, "GET", "/api/artifacts/"+id+"/versions", "", tok)
	list = decode[[]struct {
		Version int      `json:"version"`
		Authors []string `json:"authors"`
	}](t, raw)
	if len(list) != 2 || list[0].Version != 2 || len(list[0].Authors) != 1 || list[0].Authors[0] != "alice" {
		t.Fatalf("a quiet room is a new version by who edited it: %s", raw)
	}

	res, out = api(h.harness, "POST", "/api/artifacts/"+id+"/versions/1/restore", "", tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d %s", res.StatusCode, out)
	}
	var inRoom docfmt.Doc
	err = h.app.live.hub.Read(t.Context(), id, func(doc *crdt.Doc) (err error) {
		inRoom, err = docroom.Open(doc).Read()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := inRoom.Tabs[0].Blocks; len(got) != 2 || got[1].Type != docfmt.Paragraph || got[1].Content[0].Text != "first" {
		t.Errorf("after the restore the room holds %+v, want version 1", got)
	}
}

// The history says which versions an agent made, whether it wrote into an open
// room (the version is cut when the room goes quiet) or saved directly.
func TestTheVersionsAnAgentMadeNameItAndThePeoplesDoNot(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	id := h.editableDoc(tok)
	openRoom(t, h, id)

	if res, body := api(h.harness, "POST", "/api/artifacts/"+id+"/edit", `{"ops":[{"op":"replace","id":"p2","markdown":"by an agent"}]}`, tok); res.StatusCode != http.StatusOK {
		t.Fatalf("edit: %d %s", res.StatusCode, body)
	}
	h.clock.Advance(roomkind.Quiet)
	h.app.live.snap.Sweep(t.Context())
	// A restore is saved directly, by the same agent.
	if res, body := api(h.harness, "POST", "/api/artifacts/"+id+"/versions/1/restore", "", tok); res.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d %s", res.StatusCode, body)
	}

	type agent struct {
		By     string `json:"by"`
		Client string `json:"client"`
	}
	_, raw := api(h.harness, "GET", "/api/artifacts/"+id+"/versions", "", tok)
	rows := decode[[]struct {
		Version int     `json:"version"`
		Agents  []agent `json:"agents"`
	}](t, raw)
	want := map[int][]agent{1: {}, 2: {{"alice", "agent"}}, 3: {{"alice", "agent"}}}
	if len(rows) != 3 {
		t.Fatalf("versions: %s", raw)
	}
	for _, r := range rows {
		if r.Agents == nil || len(r.Agents) != len(want[r.Version]) || (len(r.Agents) == 1 && r.Agents[0] != want[r.Version][0]) {
			t.Errorf("version %d agents = %+v, want %+v (%s)", r.Version, r.Agents, want[r.Version], raw)
		}
	}
}
