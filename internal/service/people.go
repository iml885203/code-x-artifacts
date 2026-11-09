package service

import (
	"context"
	"fmt"
	"strings"
)

// MaxPeopleFound is how many people one search returns.
const MaxPeopleFound = 8

// Person is someone a doc can mention: the sign-in account, the name the
// directory gave, and whether that person can open the doc as it is now.
type Person struct {
	User    string `json:"user"`
	Name    string `json:"name"`
	CanOpen bool   `json:"canOpen"`
}

// People searches the cached organisation directory, for the editor of a doc to
// mention. Only someone who may edit the doc may search (the same refusal as
// editing; one who cannot see it gets the answer for a missing doc), and a
// mention never changes who may open the doc: CanOpen only tells the editor
// whether the person they picked can.
func (s *Service) People(ctx context.Context, v Viewer, id, query string) ([]Person, error) {
	a, err := s.Edit(ctx, v, id)
	if err != nil {
		return nil, err
	}
	found, err := s.store.SearchUsers(ctx, strings.TrimSpace(query), MaxPeopleFound)
	if err != nil {
		return nil, fmt.Errorf("search people for artifact %s: %w", id, err)
	}
	out := make([]Person, len(found))
	for i, u := range found {
		out[i] = Person{User: u.Username, Name: u.DisplayName, CanOpen: s.Viewer(u.Username).CanView(a)}
	}
	return out, nil
}
