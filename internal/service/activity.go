package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

type provenanceKey struct{}

// WithTokenProvenance attaches authenticated nonsecret identity. Client metadata is per request,
// never remembered by credential: two processes may legitimately share one token.
// What a token's request saves counts as an agent's publish in the usage.
func WithTokenProvenance(ctx context.Context, p store.TokenProvenance) context.Context {
	return context.WithValue(store.ThroughAgent(ctx), provenanceKey{}, p)
}

func (s *Service) mutationContext(ctx context.Context, operation string) context.Context {
	p, ok := ctx.Value(provenanceKey{}).(store.TokenProvenance)
	if !ok {
		return ctx
	}
	return store.ActivityContext(ctx, p, operation, s.now())
}

// ActivityQuery selects one bounded page of the caller's personal activity.
type ActivityQuery struct {
	TokenID string `json:"tokenID,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Cursor  string `json:"cursor,omitempty"`
}

// ActivityTarget names only an artifact the viewer can currently open.
type ActivityTarget struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// AgentActivityItem has no target metadata when current access is unavailable.
type AgentActivityItem struct {
	store.ActivityEntry
	Target *ActivityTarget `json:"target,omitempty"`
}

// AgentActivityPage combines bounded history with owned token summaries.
type AgentActivityPage struct {
	Items      []AgentActivityItem   `json:"items"`
	Tokens     []store.ActivityToken `json:"tokens"`
	NextCursor string                `json:"nextCursor,omitempty"`
}
type activityCursor struct {
	Owner, Token, ID string
	At               time.Time
}

// AgentActivity always rechecks current artifact access, including on paginated requests.
func (s *Service) AgentActivity(ctx context.Context, v Viewer, q ActivityQuery) (AgentActivityPage, error) {
	out := AgentActivityPage{Items: []AgentActivityItem{}, Tokens: []store.ActivityToken{}}
	if !v.CanReadAgentActivity() {
		return out, refuse(Forbidden, "sign in to read your agent activity")
	}
	f, err := q.storeFilter(v.Username)
	if err != nil {
		return out, err
	}
	now := s.now()
	tokens, err := s.store.ActivityTokens(ctx, v.Username, now)
	if err != nil {
		return out, err
	}
	out.Tokens = tokens
	if q.TokenID != "" {
		owned, err := s.store.HasActivityToken(ctx, v.Username, q.TokenID, now)
		if err != nil {
			return out, err
		}
		if !owned {
			return out, errNotFound
		}
	}
	rows, err := s.store.Activity(ctx, v.Username, f, now)
	if err != nil {
		return out, err
	}
	if len(rows) >= f.Limit {
		rows = rows[:f.Limit-1]
		last := rows[len(rows)-1]
		data, err := json.Marshal(activityCursor{Owner: v.Username, Token: q.TokenID, ID: last.ID, At: last.OccurredAt})
		if err != nil {
			return out, err
		}
		out.NextCursor = base64.RawURLEncoding.EncodeToString(data)
	}
	for _, e := range rows {
		item := AgentActivityItem{ActivityEntry: e}
		target, err := s.activityTarget(ctx, v, e.ArtifactID)
		if err != nil {
			return out, err
		}
		item.Target = target
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// RecordAgentConnection records successful protocol initialization; it is not an online session.
func (s *Service) RecordAgentConnection(ctx context.Context, p store.TokenProvenance, at time.Time) error {
	return s.store.RecordConnection(ctx, p, at)
}

// RecordAgentUse updates the throttled successful-use summary without adding a read event.
func (s *Service) RecordAgentUse(ctx context.Context, p store.TokenProvenance, at time.Time) error {
	return s.store.TouchTokenUse(ctx, p, at)
}

func (q ActivityQuery) storeFilter(owner string) (store.ActivityFilter, error) {
	if q.Limit == 0 {
		q.Limit = 50
	}
	f := store.ActivityFilter{TokenID: q.TokenID, Limit: q.Limit + 1}
	if q.Limit < 1 || q.Limit > 100 {
		return f, refuse(Invalid, "limit must be 1 to 100")
	}
	if len(q.TokenID) > 128 || len(q.Cursor) > 1024 {
		return f, refuse(Invalid, "invalid activity filter")
	}
	if q.Cursor == "" {
		return f, nil
	}
	var c activityCursor
	data, err := base64.RawURLEncoding.DecodeString(q.Cursor)
	if err != nil || json.Unmarshal(data, &c) != nil || c.Owner != owner || c.Token != q.TokenID || c.ID == "" || c.At.IsZero() {
		return f, refuse(Invalid, "invalid activity cursor")
	}
	f.BeforeID, f.BeforeTime = c.ID, c.At
	return f, nil
}

func (s *Service) activityTarget(ctx context.Context, v Viewer, id string) (*ActivityTarget, error) {
	if id == "" {
		return nil, nil
	}
	a, err := s.store.Artifact(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !v.CanView(a) {
		return nil, nil
	}
	return &ActivityTarget{ID: a.ID, Title: a.Title, URL: "/a/" + url.PathEscape(a.ID)}, nil
}
