package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/iml885203/code-x-artifacts/internal/auth"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// MaxEditors is how many accounts an author can name as editors.
const MaxEditors = 50

// maxAccountChars bounds one account name; AD's sAMAccountName is at most 20,
// so this only stops nonsense.
const maxAccountChars = 64

// ShareInput is a change to who may edit an artifact. Every part is optional,
// at least one must be given.
type ShareInput struct {
	// AddEditors and RemoveEditors name accounts; the same rule as sign-in
	// applies, so "RAVI\Bob" and "bob" are the same person.
	AddEditors, RemoveEditors []string
	// OrganisationEdit switches "everyone who can sign in may edit" on or off;
	// nil leaves it as it is. Switching it on needs an audience wider than the
	// author.
	OrganisationEdit *bool
}

// Share changes who besides the author may edit the viewer's own artifact. The
// change is checked as a whole and applied in one transaction, so a refused
// request changes nothing. It takes effect on the next request of everyone
// concerned, because the access rules read it from the artifact every time.
func (s *Service) Share(ctx context.Context, v Viewer, id string, in ShareInput) (store.Artifact, error) {
	defer s.admission.Exclusive(id)()
	a, err := s.Own(ctx, v, id)
	if err != nil {
		return store.Artifact{}, err
	}
	add, remove, err := checkShare(a, in)
	if err != nil {
		return store.Artifact{}, err
	}
	editors := slices.Clone(a.Editors)
	for _, u := range add {
		if !slices.Contains(editors, u) {
			editors = append(editors, u)
		}
	}
	editors = slices.DeleteFunc(editors, func(u string) bool { return slices.Contains(remove, u) })
	if len(editors) > MaxEditors {
		return store.Artifact{}, refuse(Invalid, "an artifact can have at most %d named editors; remove some first, or switch on organisation editing", MaxEditors)
	}
	if err := s.store.SetEditors(ctx, a.ID, add, remove, in.OrganisationEdit); err != nil {
		return store.Artifact{}, fmt.Errorf("set editors of artifact %s: %w", a.ID, err)
	}
	report(ctx, Information, "artifact editors changed", "id", a.ID, "user", v.Username, "add", add, "remove", remove, "organisationEdit", in.OrganisationEdit)
	s.recordAccess(ctx, a, v.Username)
	return s.reload(ctx, v, a.ID)
}

// accounts cleans the account names of one list: each is normalised, empty or
// malformed ones are refused naming the argument, repeats collapse.
func accounts(arg string, names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	for _, n := range names {
		u := auth.NormalizeUsername(n)
		switch {
		case u == "":
			return nil, refuse(Invalid, "%s has an empty account name; send account names such as \"bob\"", arg)
		case utf8.RuneCountInString(u) > maxAccountChars || strings.IndexFunc(u, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0:
			return nil, refuse(Invalid, "%q in %s is not an account name: send the sign-in name, such as \"bob\", without spaces", n, arg)
		}
		if !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out, nil
}

// checkShare validates a whole change before anything is applied and returns
// the cleaned accounts to add and to remove.
func checkShare(a store.Artifact, in ShareInput) (add, remove []string, err error) {
	if len(in.AddEditors) == 0 && len(in.RemoveEditors) == 0 && in.OrganisationEdit == nil {
		return nil, nil, refuse(Invalid, "nothing to change: send add (accounts to make editors), remove (accounts to stop being editors) or organisation_edit (true or false)")
	}
	if add, err = accounts("add", in.AddEditors); err != nil {
		return nil, nil, err
	}
	if remove, err = accounts("remove", in.RemoveEditors); err != nil {
		return nil, nil, err
	}
	if i := slices.IndexFunc(add, func(u string) bool { return slices.Contains(remove, u) }); i >= 0 {
		return nil, nil, refuse(Invalid, "%s is in both add and remove; send one", add[i])
	}
	if slices.Contains(add, a.Author) {
		return nil, nil, refuse(Invalid, "%s is the author and can always edit; leave them out of add", a.Author)
	}
	if in.OrganisationEdit != nil && *in.OrganisationEdit && a.Visibility != store.VisibilityOrganisation && a.Visibility != store.VisibilityPublic {
		return nil, nil, refuse(Invalid, "everyone in the organisation can only be editors of an artifact the organisation can see: set its audience to \"organisation\" first, or name the people who may edit")
	}
	return add, remove, nil
}
