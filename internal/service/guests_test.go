package service

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/iml885203/code-x-artifacts/internal/store/storetest"
)

func newPublicService(t *testing.T) *Service {
	t.Helper()
	disk, err := assets.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(storetest.Open(t), disk, true)
}

func guestID(n int) string { return fmt.Sprintf("browser-%016d", n) }

func publicPage(t *testing.T, svc *Service) string {
	t.Helper()
	p, err := svc.Create(t.Context(), svc.Viewer("alice"), PublishInput{Title: "Page", Content: "<p>x", Visibility: "public"})
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func guestsOf(list []ViewerEntry) []ViewerEntry {
	out := []ViewerEntry{}
	for _, e := range list {
		if e.Guest {
			out = append(out, e)
		}
	}
	return out
}

func TestAGuestsNicknameIsTwoReadableWordsDrawnFromItsIDAloneAndTheIDNeverLeavesTheBrowser(t *testing.T) {
	t.Parallel()
	raw := guestID(1)
	g, err := parseGuest(raw)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := parseGuest(raw)
	if g != again {
		t.Errorf("the same browser id gives %+v and %+v", g, again)
	}
	words := strings.Fields(g.nickname)
	if len(words) != 2 || !slices.Contains(guestAdjectives[:], words[0]) || !slices.Contains(guestAnimals[:], words[1]) {
		t.Errorf("nickname %q is not an adjective and an animal", g.nickname)
	}
	if !strings.HasPrefix(g.id, GuestIDPrefix) || strings.Contains(g.id, raw) {
		t.Errorf("listed id %q must be a digest under %q, never the browser's own id", g.id, GuestIDPrefix)
	}
	other, _ := parseGuest(guestID(2))
	if other.id == g.id {
		t.Errorf("two browsers share the listed id %q", g.id)
	}
}

func TestOnlyABoundedURLSafeGuestIDIsAccepted(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw string
		ok  bool
	}{
		{strings.Repeat("a", 16), true},
		{strings.Repeat("Z", 64), true},
		{"Ab0_-Ab0_-Ab0_-Ab0_-Ab", true},
		{"", false},
		{strings.Repeat("a", 15), false},
		{strings.Repeat("a", 65), false},
		{strings.Repeat("a", 15) + "!", false},
		{strings.Repeat("a", 15) + " ", false},
		{strings.Repeat("a", 15) + "é", false},
		{strings.Repeat("a", 15) + "/", false},
		{"alice:" + strings.Repeat("a", 16), false},
	}
	for _, c := range cases {
		_, err := parseGuest(c.raw)
		if (err == nil) != c.ok {
			t.Errorf("parseGuest(%q) = %v, want accepted %v", c.raw, err, c.ok)
		}
		if err != nil && CodeOf(err) != Invalid {
			t.Errorf("parseGuest(%q) refused with %v, want Invalid", c.raw, err)
		}
	}
}

func TestGuestsWhoseNicknamesCollideAreToldApartByAStableTagAndOthersKeepTheBareNickname(t *testing.T) {
	t.Parallel()
	seen := map[string]guest{}
	var a, b guest
	for n := 0; a.id == ""; n++ {
		g, err := parseGuest(guestID(n))
		if err != nil {
			t.Fatal(err)
		}
		if first, ok := seen[g.nickname]; ok {
			a, b = first, g
		}
		seen[g.nickname] = g
	}
	var c guest
	for n := 10_000; c.id == ""; n++ {
		if g, _ := parseGuest(guestID(n)); g.nickname != a.nickname {
			c = g
		}
	}
	entry := func(g guest) ViewerEntry { return ViewerEntry{ID: g.id, Name: g.nickname, Guest: true} }
	person := ViewerEntry{ID: "bob", Name: a.nickname}

	list := distinguishGuests([]ViewerEntry{entry(a), entry(b), entry(c), person})
	if list[0].Name == list[1].Name || !strings.HasPrefix(list[0].Name, a.nickname+" #") || !strings.HasPrefix(list[1].Name, a.nickname+" #") {
		t.Errorf("colliding guests are %q and %q", list[0].Name, list[1].Name)
	}
	if list[2].Name != c.nickname || list[3].Name != a.nickname {
		t.Errorf("a guest without a collision is %q, a person with the same name is %q; both keep their names", list[2].Name, list[3].Name)
	}
	reversed := distinguishGuests([]ViewerEntry{entry(b), entry(a)})
	if reversed[1].Name != list[0].Name || reversed[0].Name != list[1].Name {
		t.Errorf("a guest's tag depends on the order of the list: %+v vs %+v", reversed, list)
	}
}

func TestAGuestIsListedOnceAcrossTabsForSignedInReadersAndLeavesAMinuteAfterItsLastBeat(t *testing.T) {
	t.Parallel()
	svc := newPublicService(t)
	clock := &fakeClock{at: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)}
	svc.now = clock.now
	id := publicPage(t, svc)
	beat := func() {
		t.Helper()
		if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(1)); err != nil {
			t.Fatal(err)
		}
	}
	seen := func() []ViewerEntry {
		t.Helper()
		list, err := svc.Viewers(t.Context(), svc.Viewer("alice"), id)
		if err != nil {
			t.Fatal(err)
		}
		return guestsOf(list)
	}
	for range 3 { // three tabs of one browser
		beat()
	}
	g, _ := parseGuest(guestID(1))
	if got := seen(); len(got) != 1 || got[0].ID != g.id || got[0].Name != g.nickname || got[0].Agent || got[0].Self {
		t.Fatalf("alice sees the guests %+v, want %q once", got, g.nickname)
	}
	for range 6 { // another tab keeps beating
		clock.at = clock.at.Add(HeartbeatEvery(PersonStays))
		beat()
	}
	clock.at = clock.at.Add(PersonStays - time.Second)
	if got := seen(); len(got) != 1 {
		t.Errorf("just under a minute after the last beat: %+v", got)
	}
	clock.at = clock.at.Add(time.Second)
	if got := seen(); len(got) != 0 {
		t.Errorf("a minute after the last beat: %+v", got)
	}
}

func TestAGuestsOwnAnswerHoldsOnlyItselfNeverThePeopleOrOtherGuests(t *testing.T) {
	t.Parallel()
	svc := newPublicService(t)
	id := publicPage(t, svc)
	if _, err := svc.Heartbeat(t.Context(), svc.Viewer("alice"), id, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(2)); err != nil {
		t.Fatal(err)
	}
	own, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(1))
	g, _ := parseGuest(guestID(1))
	if err != nil || len(own) != 1 || own[0].ID != g.id || !own[0].Guest || !own[0].Self {
		t.Errorf("the guest's own answer = %+v, %v; want only itself", own, err)
	}
	if _, err := svc.Viewers(t.Context(), svc.Anonymous(), id); CodeOf(err) != Forbidden {
		t.Errorf("a signed-out visitor reads the list: %v", err)
	}
}

func TestSigningInReplacesTheBrowsersGuestEntryWithTheAccountAtOnce(t *testing.T) {
	t.Parallel()
	svc := newPublicService(t)
	id := publicPage(t, svc)
	if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(2)); err != nil {
		t.Fatal(err)
	}
	list, err := svc.Heartbeat(t.Context(), svc.Viewer("bob"), id, guestID(1))
	if err != nil {
		t.Fatal(err)
	}
	other, _ := parseGuest(guestID(2))
	if got := guestsOf(list); len(got) != 1 || got[0].ID != other.id || list[0].ID != "bob" || !list[0].Self {
		t.Errorf("after signing in bob sees %+v; want himself and only the other browser's guest", list)
	}
	if _, err := svc.Heartbeat(t.Context(), svc.Viewer("bob"), id, "not a guest id"); CodeOf(err) != Invalid {
		t.Errorf("a malformed guest id from a signed-in page: %v", err)
	}
}

func TestAGuestIsOnlyAcceptedWhereTheAnonymousMayViewAndLeavesWhenThePublicLinkCloses(t *testing.T) {
	t.Parallel()
	svc := newPublicService(t)
	alice := svc.Viewer("alice")
	for _, visibility := range []store.Visibility{store.VisibilityPrivate, store.VisibilityOrganisation} {
		p, err := svc.Create(t.Context(), alice, PublishInput{Title: "Page", Content: "<p>x", Visibility: visibility})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), p.ID, guestID(1)); CodeOf(err) != NotFound {
			t.Errorf("a guest beat on a %s page: %v, want the answer for a missing page", visibility, err)
		}
	}
	id := publicPage(t, svc)
	if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Heartbeat(t.Context(), svc.Granted(Grant{ArtifactID: id, Visibility: "public"}), id, guestID(3)); CodeOf(err) != Forbidden {
		t.Errorf("a page's grant beat as a guest: %v", err)
	}
	if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, ""); CodeOf(err) != Forbidden {
		t.Errorf("a signed-out beat without a guest id (a page that was signed in until now): %v", err)
	}

	svc.publicLinks = false // the deployment turns public links off
	if list, _ := svc.Viewers(t.Context(), svc.Viewer("alice"), id); len(guestsOf(list)) != 0 {
		t.Errorf("guests stay listed after public links are turned off: %+v", list)
	}
	if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(1)); CodeOf(err) != NotFound {
		t.Errorf("a guest beat with public links off: %v", err)
	}
	svc.publicLinks = true
	if list, _ := svc.Viewers(t.Context(), svc.Viewer("alice"), id); len(guestsOf(list)) != 0 {
		t.Errorf("a guest dropped when the link closed is back without beating: %+v", list)
	}

	if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetVisibility(t.Context(), alice, id, "organisation"); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.Viewers(t.Context(), alice, id); len(guestsOf(list)) != 0 {
		t.Errorf("guests stay listed after the page stops being public: %+v", list)
	}
	if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(1)); CodeOf(err) != NotFound {
		t.Errorf("a guest beat after the page stops being public: %v", err)
	}
}

func TestAnonymousTrafficCannotGrowTheGuestsPastTheirBoundsAndListedGuestsKeepTheirPlace(t *testing.T) {
	t.Parallel()
	svc := newPublicService(t)
	clock := &fakeClock{at: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)}
	svc.now = clock.now
	svc.maxGuests, svc.maxGuestsPerArtifact = 5, 3
	first, second := publicPage(t, svc), publicPage(t, svc)
	beat := func(id string, n int) error {
		_, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(n))
		return err
	}
	for n := range 3 {
		if err := beat(first, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := beat(first, 3); CodeOf(err) != Busy {
		t.Errorf("a fourth guest on a page that holds three: %v", err)
	}
	if err := beat(first, 0); err != nil {
		t.Errorf("a listed guest is refused its next beat: %v", err)
	}
	for n := 10; n < 12; n++ {
		if err := beat(second, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := beat(second, 12); CodeOf(err) != Busy {
		t.Errorf("a sixth guest when the server holds five: %v", err)
	}
	if list, _ := svc.Viewers(t.Context(), svc.Viewer("alice"), first); len(guestsOf(list)) != 3 {
		t.Errorf("the first page lists %+v", list)
	}
	clock.at = clock.at.Add(PersonStays)
	if err := beat(second, 12); err != nil {
		t.Errorf("guests who left make room for new ones: %v", err)
	}
	if svc.viewers.guests > svc.maxGuests {
		t.Errorf("the book holds %d guest rows, more than %d", svc.viewers.guests, svc.maxGuests)
	}
}

func TestAGuestsOwnAnswerIsItsBareNicknameEvenWhenAnotherGuestDrewTheSame(t *testing.T) {
	t.Parallel()
	svc := newPublicService(t)
	id := publicPage(t, svc)
	seen := map[string]int{}
	first, second := -1, -1
	for n := 0; second == -1; n++ {
		g, _ := parseGuest(guestID(n))
		if at, ok := seen[g.nickname]; ok {
			first, second = at, n
		}
		seen[g.nickname] = n
	}
	alone, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(first))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(second)); err != nil {
		t.Fatal(err)
	}
	together, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(first))
	if err != nil || len(together) != 1 || together[0] != alone[0] || strings.Contains(together[0].Name, "#") {
		t.Errorf("the guest's answer is %+v next to a namesake and %+v alone; it must not tell that another guest is here", together, alone)
	}
	list, _ := svc.Viewers(t.Context(), svc.Viewer("alice"), id)
	if got := guestsOf(list); len(got) != 2 || got[0].Name == got[1].Name {
		t.Errorf("a signed-in reader still tells the namesakes apart: %+v", got)
	}
}

func TestRefusedGuestsDoNotWalkTheBookMoreThanOncePerSecond(t *testing.T) {
	t.Parallel()
	svc := newPublicService(t)
	clock := &fakeClock{at: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)}
	svc.now = clock.now
	svc.maxGuests, svc.maxGuestsPerArtifact = 3, 3
	id := publicPage(t, svc)
	for n := range 3 {
		if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(n)); err != nil {
			t.Fatal(err)
		}
	}
	before := svc.viewers.sweeps
	for n := 100; n < 200; n++ {
		if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(n)); CodeOf(err) != Busy {
			t.Fatalf("guest %d on a full book: %v", n, err)
		}
	}
	if walks := svc.viewers.sweeps - before; walks > 1 {
		t.Errorf("a hundred refused guests in one second walked the book %d times", walks)
	}
	clock.at = clock.at.Add(PersonStays)
	if _, err := svc.Heartbeat(t.Context(), svc.Anonymous(), id, guestID(200)); err != nil {
		t.Errorf("once the guests left, a new one is refused: %v", err)
	}
}
