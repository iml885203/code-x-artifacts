package service

import (
	"context"
	"net/http"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

type agentKey struct{}

// WithAgent marks a request as made by an agent (a personal token, over MCP or
// HTTP) rather than by a person at a browser, and says which client it is: the
// name the client gave when it started the MCP session, or empty when the
// request does not know it. A doc an agent writes through a live room shows the
// agent in the room's roster while it works (docs/doc-edit.md).
func WithAgent(ctx context.Context, client string) context.Context {
	return context.WithValue(ctx, agentKey{}, client)
}

// agentLabel is how the agent of this request is shown, or false for a request
// that is not an agent's: "<account> 的 <client>", and "agent" for a client
// that is not known. The key tells one agent of an account from another.
func agentLabel(ctx context.Context, v Viewer) (key, label string, ok bool) {
	client, ok := ctx.Value(agentKey{}).(string)
	if !ok || v.Anonymous() {
		return "", "", false
	}
	if client == "" {
		client = "agent"
	}
	return v.Username + "/" + client, v.Username + " 的 " + client, true
}

// versionAgent is the agent a version saved by this request is credited to, for
// the history to name it, or false for a request that is not an agent's.
func versionAgent(ctx context.Context, v Viewer) (store.VersionAgent, bool) {
	client, ok := ctx.Value(agentKey{}).(string)
	if !ok || v.Anonymous() {
		return store.VersionAgent{}, false
	}
	if client == "" {
		client = "agent"
	}
	return store.VersionAgent{By: v.Username, Client: client}, true
}

// agents is the list a revision carries for this request: empty for a person.
func agents(ctx context.Context, v Viewer) []store.VersionAgent {
	if ag, ok := versionAgent(ctx, v); ok {
		return []store.VersionAgent{ag}
	}
	return nil
}

// showAgent puts the agent of the request in the room of the artifact at a
// block. It never fails the edit it accompanies: a roster that cannot be
// updated is logged.
func (s *Service) showAgent(ctx context.Context, v Viewer, id, block string) {
	key, label, ok := agentLabel(ctx, v)
	if !ok || s.rooms == nil {
		return
	}
	if err := s.rooms.Present(ctx, id, key, label, block); err != nil {
		report(ctx, Warning, "show an agent in a room", "artifact", id, "label", label, "err", err)
	}
	// The version cut from the room names it among its makers.
	if ag, ok := versionAgent(ctx, v); ok {
		if err := s.store.MarkPendingAgent(ctx, id, ag.By, ag.Client); err != nil {
			report(ctx, Warning, "remember an agent that wrote into a room", "artifact", id, "label", label, "err", err)
		}
	}
}

// changedBlock is the id of the first top-level block of after that is not as it
// was in before, for the caret of an agent that wrote the whole doc.
func changedBlock(before, after string) string {
	b, errB := docfmt.Parse(before)
	a, errA := docfmt.Parse(after)
	if errA != nil {
		return ""
	}
	if errB != nil {
		b = docfmt.Doc{}
	}
	return docroom.FirstChanged(b, a)
}

// AgentRequest marks an http request as an agent's whose client is not known.
func AgentRequest(r *http.Request) *http.Request {
	return r.WithContext(WithAgent(r.Context(), ""))
}
