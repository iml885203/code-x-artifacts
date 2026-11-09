package service

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// Who is looking at an artifact now (docs/viewers.md). A browser page sends a
// heartbeat every HeartbeatEvery; a person or guest who stops is gone after
// PersonStays. An agent is a viewer while it reads and for AgentStays after,
// the same idle time its caret has in a room.
const (
	PersonStays = 60 * time.Second
	AgentStays  = 15 * time.Second
)

// sweepEvery is how often a heartbeat also clears expired rows from the whole book: often enough that the
// rows of who left go within seconds of expiring, rarely enough that a heartbeat does not walk every row.
const sweepEvery = time.Second

// HeartbeatEvery is how often a page beats so that it is never forgotten between two beats: a third of
// how long a person stays.
func HeartbeatEvery(personStays time.Duration) time.Duration { return personStays / 3 }

// viewerRow is one person, one agent client, or one guest browser that has an artifact open.
type viewerRow struct {
	key, id, username, name string
	agent, guest            bool
	seenAt                  time.Time
}

// viewerBook is who has which artifact open. It lives in memory: it is only true for a minute, and the
// deployment is one replica (docs/realtime.md), so a restart costs the pages one beat.
type viewerBook struct {
	mu   sync.Mutex
	rows map[string]map[string]viewerRow // artifact id -> key -> row
	// guests is how many rows are guests', expired or not, so a full book is known without a walk.
	guests       int
	sweptBefore  time.Time
	sweptAtLeast bool
	// sweeps counts walks of the whole book.
	sweeps int
}

// sweep drops the rows last seen at or before forgetBefore, at most once per sweepEvery.
func (b *viewerBook) sweep(forgetBefore time.Time) {
	if b.sweptAtLeast && forgetBefore.Sub(b.sweptBefore) < sweepEvery {
		return
	}
	b.sweptBefore, b.sweptAtLeast = forgetBefore, true
	b.sweeps++
	for artifact, rows := range b.rows {
		for k, old := range rows {
			if !old.seenAt.After(forgetBefore) {
				b.remove(artifact, k)
			}
		}
	}
}

func (b *viewerBook) remove(artifact, key string) {
	old, ok := b.rows[artifact][key]
	if !ok {
		return
	}
	delete(b.rows[artifact], key)
	if old.guest {
		b.guests--
	}
	if len(b.rows[artifact]) == 0 {
		delete(b.rows, artifact)
	}
}

func (b *viewerBook) put(id string, r viewerRow) {
	if b.rows == nil {
		b.rows = map[string]map[string]viewerRow{}
	}
	if b.rows[id] == nil {
		b.rows[id] = map[string]viewerRow{}
	}
	if _, known := b.rows[id][r.key]; !known && r.guest {
		b.guests++
	}
	b.rows[id][r.key] = r
}

func (b *viewerBook) touch(id string, r viewerRow, forgetBefore time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweep(forgetBefore)
	b.put(id, r)
}

// admitGuest is touch for a guest: a guest already listed always beats again; a new one is refused while the
// artifact or the whole book holds as many guests as it may. A refusal never walks the book itself: rows that
// expired go with the next periodic sweep, so a flood of refused guests costs no more than the sweep's pace.
func (b *viewerBook) admitGuest(id string, r viewerRow, forgetBefore time.Time, perArtifact, total int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweep(forgetBefore)
	if _, known := b.rows[id][r.key]; !known && (b.guests >= total || b.liveGuests(id, forgetBefore) >= perArtifact) {
		return false
	}
	b.put(id, r)
	return true
}

func (b *viewerBook) liveGuests(id string, forgetBefore time.Time) int {
	n := 0
	for _, r := range b.rows[id] {
		if r.guest && r.seenAt.After(forgetBefore) {
			n++
		}
	}
	return n
}

func (b *viewerBook) forget(id string, keys ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, k := range keys {
		b.remove(id, k)
	}
}

func (b *viewerBook) of(id string) []viewerRow {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]viewerRow, 0, len(b.rows[id]))
	for _, r := range b.rows[id] {
		out = append(out, r)
	}
	return out
}

// SetViewerStays sets how long a person stays in the list after their last heartbeat (PersonStays when
// not set), and an agent after its last read (a quarter of it). A deployment never needs this; the smoke
// server uses it so a browser test does not wait a minute. Call before the service is used.
func (s *Service) SetViewerStays(person time.Duration) {
	s.personStays, s.agentStays = person, person/4
}

// ViewerStays is how long a person or guest stays listed after their last heartbeat.
func (s *Service) ViewerStays() time.Duration { return s.personStays }

// ViewerBeat is how often a page should send a heartbeat to this server.
func (s *Service) ViewerBeat() time.Duration { return HeartbeatEvery(s.personStays) }

// ViewerEntry is one person, one agent, or one guest that has the artifact open.
type ViewerEntry struct {
	// ID is who it is, whatever it is called: the account of a person, "<account>/<client>" of an agent,
	// "guest:<digest>" of a guest. A room's awareness carries the same id for the same person or agent.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Agent marks an agent; Guest a signed-out browser on a public link, named by a nickname that browser
	// keeps, which is not a verified person.
	Agent bool `json:"agent"`
	Guest bool `json:"guest"`
	// Self is the entry of the caller.
	Self bool `json:"self"`
}

func viewerKey(v Viewer) string { return "p:" + v.Username }

// Heartbeat records that the caller has the artifact open and answers with who the caller may see has it
// open: the whole list for someone signed in, only themselves for a guest. It asks the same question as
// reading the artifact, so who cannot see it gets the same answer. A signed-out caller is listed as the
// guest its browser id names, where the anonymous may be (Viewer.CanBeAGuest); a signed-in page that sends
// the guest id its browser used before signing in takes that guest's place.
func (s *Service) Heartbeat(ctx context.Context, v Viewer, id, guestID string) ([]ViewerEntry, error) {
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return nil, err
	}
	if v.Anonymous() {
		return s.guestBeat(a, v, guestID)
	}
	if guestID != "" {
		g, err := parseGuest(guestID)
		if err != nil {
			return nil, err
		}
		s.viewers.forget(a.ID, g.key())
	}
	name := v.Username
	if me, err := s.Me(ctx, v); err == nil && me.DisplayName != "" {
		name = me.DisplayName
	}
	s.touchViewer(a.ID, viewerRow{key: viewerKey(v), id: v.Username, username: v.Username, name: name})
	return s.Viewers(ctx, v, id)
}

// guestBeat lists a signed-out browser as its guest and answers with that guest alone: a guest never reads
// who else is here.
func (s *Service) guestBeat(a store.Artifact, v Viewer, guestID string) ([]ViewerEntry, error) {
	if !v.CanBeAGuest(a) {
		return nil, refuse(Forbidden, "only a signed-out browser on a public link is listed as a guest")
	}
	if guestID == "" {
		// A page that was signed in until its session ended sends no guest id: it is told it is signed out.
		return nil, refuse(Forbidden, "sign in, or send guest, the browser's random guest id, to be shown as a guest")
	}
	g, err := parseGuest(guestID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	row := viewerRow{key: g.key(), id: g.id, name: g.nickname, guest: true, seenAt: now}
	if !s.viewers.admitGuest(a.ID, row, now.Add(-s.personStays), s.maxGuestsPerArtifact, s.maxGuests) {
		return nil, refuse(Busy, "this page already lists as many guests as it can; reading it still works, and the next heartbeat tries again")
	}
	// The guest's own row, as it is: telling namesakes apart would tell this guest that another one is here.
	return []ViewerEntry{{ID: g.id, Name: g.nickname, Guest: true, Self: true}}, nil
}

func (s *Service) touchViewer(id string, row viewerRow) {
	now := s.now()
	row.seenAt = now
	s.viewers.touch(id, row, now.Add(-s.personStays))
}

// Viewers is who has the artifact open now, for someone who may see it: people
// and guests by their last heartbeat, agents while they read. Whoever lost access
// is no longer in it. The caller who is an agent is shown as a viewer from this
// call on, and is not in its own answer. Signed-out visitors may read the
// artifact but not who else is looking.
func (s *Service) Viewers(ctx context.Context, v Viewer, id string) ([]ViewerEntry, error) {
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return nil, err
	}
	if !v.CanSeeViewers(a) {
		return nil, refuse(Forbidden, "sign in to see who is viewing; signed-out visitors cannot read the names")
	}
	agentID, agentName, isAgent := agentLabel(ctx, v)
	out := s.current(a, viewerKey(v), agentID)
	if isAgent {
		s.touchViewer(id, viewerRow{key: "a:" + agentID, id: agentID, username: v.Username, name: agentName, agent: true})
	}
	return out, nil
}

// current is the rows that still count: not expired, still allowed on the
// artifact, and not the caller's own agent. selfKey is the caller's row. A row
// whose person or guest may no longer be here leaves the book at once.
func (s *Service) current(a store.Artifact, selfKey, ownAgent string) []ViewerEntry {
	now := s.now()
	out := []ViewerEntry{}
	var gone []string
	for _, r := range s.viewers.of(a.ID) {
		stays := s.personStays
		if r.agent {
			stays = s.agentStays
		}
		if now.Sub(r.seenAt) >= stays || (r.agent && ownAgent != "" && r.key == "a:"+ownAgent) {
			continue
		}
		if !s.rowMayStay(r, a) {
			gone = append(gone, r.key)
			continue
		}
		out = append(out, ViewerEntry{ID: r.id, Name: r.name, Agent: r.agent, Guest: r.guest, Self: !r.agent && r.key == selfKey})
	}
	if len(gone) > 0 {
		s.viewers.forget(a.ID, gone...)
	}
	out = distinguishGuests(out)
	slices.SortFunc(out, func(x, y ViewerEntry) int {
		if x.Self != y.Self {
			if x.Self {
				return -1
			}
			return 1
		}
		if x.Guest != y.Guest {
			if y.Guest {
				return -1
			}
			return 1
		}
		return strings.Compare(x.Name, y.Name)
	})
	return out
}

// rowMayStay asks the access rules, for the one the row stands for, whether they may still be listed.
func (s *Service) rowMayStay(r viewerRow, a store.Artifact) bool {
	if r.guest {
		return s.Anonymous().CanBeAGuest(a)
	}
	return s.Viewer(r.username).CanView(a)
}
