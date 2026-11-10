package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// Scope narrows a listing.
type Scope string

// The listing scopes. All is the zero value's meaning too.
const (
	ScopeAll    Scope = "all"
	ScopeMine   Scope = "mine"
	ScopeShared Scope = "shared"
)

// Order is how a listing is sorted.
type Order string

// The listing orders. Updated is the zero value's meaning too.
const (
	// OrderUpdated is every artifact, most recently updated first.
	OrderUpdated Order = "updated"
	// OrderOpened is only the artifacts the viewer opened, most recently opened first.
	OrderOpened Order = "opened"
)

// ListFilter narrows List; its zero value lists everything the viewer may see.
type ListFilter struct {
	Scope Scope
	// Query keeps artifacts whose title or author contains it, ignoring case.
	Query string
	Order Order
}

// List returns the artifacts the viewer may see, in the filter's order.
func (s *Service) List(ctx context.Context, v Viewer, f ListFilter) ([]store.Artifact, error) {
	if err := f.valid(); err != nil {
		return nil, err
	}
	all, err := s.store.Artifacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	list := all[:0]
	for _, a := range all {
		if v.CanView(a) && f.keeps(v, a) {
			list = append(list, a)
		}
	}
	ptrs := make([]*store.Artifact, len(list))
	for i := range list {
		ptrs[i] = &list[i]
	}
	if err := s.withViewerState(ctx, v, "", ptrs...); err != nil {
		return nil, err
	}
	if f.Order == OrderOpened {
		list = recentlyOpened(list)
	}
	for i := range list {
		list[i] = v.RedactArtifact(list[i])
	}
	return list, nil
}

func (f ListFilter) valid() error {
	switch f.Scope {
	case "", ScopeAll, ScopeMine, ScopeShared:
	default:
		return refuse(Invalid, "scope must be %q, %q or %q", ScopeMine, ScopeShared, ScopeAll)
	}
	switch f.Order {
	case "", OrderUpdated, OrderOpened:
	default:
		return refuse(Invalid, "order must be %q or %q", OrderUpdated, OrderOpened)
	}
	return nil
}

// keeps is whether an artifact the viewer may see is in the filter's scope and matches its query.
func (f ListFilter) keeps(v Viewer, a store.Artifact) bool {
	query := strings.ToLower(strings.TrimSpace(f.Query))
	switch {
	case f.Scope == ScopeMine && a.Author != v.Username:
		return false
	case f.Scope == ScopeShared && a.Author == v.Username:
		return false
	default:
		return query == "" || strings.Contains(strings.ToLower(a.Title), query) || strings.Contains(strings.ToLower(a.Author), query) || strings.Contains(strings.ToLower(a.AuthorDisplayName), query)
	}
}

// recentlyOpened keeps the artifacts the viewer opened, most recently opened first.
func recentlyOpened(list []store.Artifact) []store.Artifact {
	opened := slices.DeleteFunc(list, func(a store.Artifact) bool { return a.OpenedAt == nil })
	slices.SortStableFunc(opened, func(a, b store.Artifact) int { return b.OpenedAt.Compare(*a.OpenedAt) })
	return opened
}
