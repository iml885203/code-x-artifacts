package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"time"
)

// agentPresence is an agent the server shows in a room's roster: it is not a
// connection, so it is there only because the server says so, and it goes when
// the server stops saying so.
type agentPresence struct {
	id      uint64
	clock   uint64
	expires time.Time
}

// Announce shows an agent in the room's roster as label, at the block it just
// changed, for AgentIdle from now; announcing again with the same key moves it
// to the new block and extends its stay. After AgentIdle without an
// announcement Sweep removes it. The state carries the block id and the time
// of the announcement, so a client can draw the caret and let it fade.
//
// Nothing a connection sends can be this: the identity of the roster entry is
// the server's, awareness entries of connections are rebuilt from the
// connection's own identity, and a connection cannot publish under an id the
// server holds.
func (h *Hub) Announce(ctx context.Context, roomID, key, label, block string) error {
	return h.run(ctx, roomID, func(r *room) error { return r.announceLocked(key, label, map[string]any{"block": block}) })
}

// MaxTouched is how many elements one announcement can name; the rest are left out.
const MaxTouched = 50

// AnnounceTouched is Announce for a room whose content is elements in frames
// (a design): the state carries the pids of the elements the agent changed in
// each frame, `touched` as {frame: [pid…]}, so a client can outline them under
// the agent's name until they fade. An announcement with nothing touched is
// still the agent being in the room.
func (h *Hub) AnnounceTouched(ctx context.Context, roomID, key, label string, touched map[string][]string) error {
	capped := map[string][]string{}
	left := MaxTouched
	frames := make([]string, 0, len(touched))
	for frame := range touched {
		frames = append(frames, frame)
	}
	sort.Strings(frames)
	for _, frame := range frames {
		pids := touched[frame]
		if len(pids) > left {
			pids = pids[:left]
		}
		if len(pids) > 0 {
			capped[frame] = pids
			left -= len(pids)
		}
	}
	return h.run(ctx, roomID, func(r *room) error { return r.announceLocked(key, label, map[string]any{"touched": capped}) })
}

func (r *room) announceLocked(key, label string, fields map[string]any) error {
	if r.agents == nil {
		r.agents = map[string]*agentPresence{}
	}
	now := r.hub.cfg.Now()
	a := r.agents[key]
	if a == nil {
		id := uint64(r.hub.cfg.NewClientID())
		for _, taken := r.owner[id]; taken; _, taken = r.owner[id] {
			id++
		}
		a = &agentPresence{id: id}
		r.agents[key] = a
		r.owner[id] = presenceOwner
		r.presences++
	}
	state, err := agentState(key, label, fields, now)
	if err != nil {
		return err
	}
	a.clock++
	a.expires = now.Add(r.hub.cfg.AgentIdle)
	update := encodeAwareness([]awarenessEntry{{clientID: a.id, clock: a.clock, state: state}})
	if err := r.aw.ApplyUpdate(update, presenceOwner); err != nil {
		return fmt.Errorf("show %q in the room: %w", label, err)
	}
	r.broadcastAwarenessLocked(update)
	r.activeAt = now
	return nil
}

func agentState(key, label string, extra map[string]any, now time.Time) ([]byte, error) {
	state, err := withAuthenticatedUser([]byte("{}"), key, Identity{Label: label}, true)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(state, &fields); err != nil {
		return nil, err
	}
	maps.Copy(fields, extra)
	fields["at"] = now.UnixMilli()
	return json.Marshal(fields)
}

// expireAgentsLocked removes the announced agents that have been idle for AgentIdle.
func (r *room) expireAgentsLocked(now time.Time) {
	for key, a := range r.agents {
		if now.Before(a.expires) {
			continue
		}
		r.leaveLocked(map[uint64]struct{}{a.id: {}}, presenceOwner)
		delete(r.agents, key)
		r.presences--
	}
}
