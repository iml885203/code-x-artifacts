package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/reearth/ygo/awareness"
	"github.com/reearth/ygo/crdt"
	"github.com/reearth/ygo/encoding"

	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/mcptools"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
)

// rosterOf joins the room as a watcher and reads the agents it is told of: name to block.
func rosterOf(t *testing.T, h *liveHarness, id string) map[string]string {
	t.Helper()
	peer, err := h.app.live.hub.Join(t.Context(), id, realtime.Identity{Username: "bob"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	aw := awareness.New(1)
	for {
		select {
		case msg := <-peer.Out():
			dec := encoding.NewDecoder(msg)
			if kind, _ := dec.ReadVarUint(); kind == 1 {
				payload, _ := dec.ReadVarBytes()
				if err := aw.ApplyUpdate(payload, "remote"); err != nil {
					t.Fatal(err)
				}
			}
		default:
			out := map[string]string{}
			for _, st := range aw.GetStates() {
				user, _ := st.State["user"].(map[string]any)
				if user["agent"] == true {
					block, _ := st.State["block"].(string)
					out[user["name"].(string)] = block
				}
			}
			return out
		}
	}
}

func openRoom(t *testing.T, h *liveHarness, id string) {
	t.Helper()
	err := h.app.live.hub.Read(t.Context(), id, func(doc *crdt.Doc) error {
		_, err := docroom.Open(doc).Read()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTheClientNamedOnAnMCPRequestIsTheNameOfTheAgentInTheRoom(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	id := h.editableDoc(tok)
	openRoom(t, h, id)

	for name, want := range map[string]string{"claude-code": "alice 的 Claude Code", "codex-mcp-client": "alice 的 Codex", "my-own-script": "alice 的 my-own-script", "CodeXArtifacts Acceptance A": "alice 的 CodeXArtifacts Acceptance A", "CodeXArtifacts Acceptance B": "alice 的 CodeXArtifacts Acceptance B"} {
		client := mcp.NewClient(&mcp.Implementation{Name: name, Version: "1"}, nil)
		session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
			Endpoint:   h.srv.URL + "/api/mcp",
			HTTPClient: &http.Client{Transport: bearerTransport{token: tok, next: h.srv.Client().Transport}},
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "doc_edit", Arguments: map[string]any{"id": id, "ops": []any{map[string]any{"op": "replace", "id": "p1", "markdown": "by " + name}}}})
		_ = session.Close()
		if err != nil || res.IsError {
			t.Fatalf("doc_edit as %s: %v %+v", name, err, res)
		}
		if got := rosterOf(t, h, id); got[want] != "p1" {
			t.Errorf("a client called %q: the roster has %v, want %q", name, got, want)
		}
	}
	if err := h.app.live.snap.Flush(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	a, err := h.st.Artifact(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	v, err := h.st.Version(t.Context(), id, a.Version)
	if err != nil {
		t.Fatal(err)
	}
	clients := map[string]bool{}
	for _, ag := range v.Agents {
		if ag.By != "alice" {
			t.Fatalf("client changed authenticated author: %+v", ag)
		}
		clients[ag.Client] = true
	}
	for _, name := range []string{"Claude Code", "Codex", "my-own-script", "CodeXArtifacts Acceptance A", "CodeXArtifacts Acceptance B"} {
		if !clients[name] {
			t.Errorf("saved version lost client %q: %+v", name, v.Agents)
		}
	}
}

func TestAnHTTPTokenEditIsAnAgentToo(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	id := h.editableDoc(tok)
	openRoom(t, h, id)
	if res, body := api(h.harness, "POST", "/api/artifacts/"+id+"/edit", `{"ops":[{"op":"replace","id":"p2","markdown":"over HTTP"}]}`, tok); res.StatusCode != http.StatusOK {
		t.Fatalf("edit: %d %s", res.StatusCode, body)
	}
	if got := rosterOf(t, h, id); got["alice 的 agent"] != "p2" {
		t.Errorf("the roster has %v", got)
	}
}

func TestClientNamesAreRecognisedByTheStartOfWhatTheClientCallsItself(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"claude-code": "Claude Code", "Claude-Code-VSCode": "Claude Code", "claude-ai": "Claude", "Codex": "Codex", "cursor-vscode": "Cursor", "": "", "something else": "something else", " CodeXArtifacts Acceptance A ": "CodeXArtifacts Acceptance A", "CodeXArtifacts\n Acceptance B": "CodeXArtifacts Acceptance B", "\t\n\x00": "", "\x00 \x00": "", strings.Repeat("界", 61): strings.Repeat("界", 60), " cOdEx-mcp-client ": "Codex"} {
		if got := mcptools.ClientName(in); got != want {
			t.Errorf("ClientName(%q) = %q, want %q", in, got, want)
		}
	}
}
