package auth

import (
	"context"
	"errors"
	"testing"
)

func TestNormalizeUsernameAcceptsEveryFormPeopleType(t *testing.T) {
	for _, in := range []string{"ada.lovelace", " Ada.Lovelace ", `RAVI\ada.lovelace`, "ada.lovelace@example.com"} {
		if got := NormalizeUsername(in); got != "ada.lovelace" {
			t.Errorf("NormalizeUsername(%q) = %q", in, got)
		}
	}
}

func TestEmptyPasswordNeverReachesTheDirectory(t *testing.T) {
	l := &LDAP{URL: "ldaps://unreachable.invalid:636", Domain: "example.com"}
	if _, err := l.Authenticate(context.Background(), "ada.lovelace", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("got %v", err)
	}
}

func TestThePasswordGoesInClearOnlyInsideTLS(t *testing.T) {
	for url, want := range map[string]bool{
		"ldaps://127.0.0.1:636":  true,
		"LDAPS://dc.example.com": true,
		"ldap://127.0.0.1:389":   false,
		"ldapi:///":              false,
		"":                       false,
	} {
		if got := simpleBindIsSafe(url); got != want {
			t.Errorf("simpleBindIsSafe(%q) = %v, want %v", url, got, want)
		}
	}
}
