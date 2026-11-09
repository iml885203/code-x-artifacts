package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

// The live socket (docs/realtime.md): one WebSocket per person per open
// artifact, at /ws/{room} on the site's own origin. This file is a transport:
// it decides nothing about who may edit, which stays in service/access.go
// (asked through Service.Edit and Service.Artifact), and nothing about the
// document, which the realtime hub owns.

const (
	// ticketLife is how long a ticket is good for: long enough to open the
	// socket, too short to be worth stealing.
	ticketLife = 30 * time.Second
	// socketLifetime is how long one connection lives. A connection that has
	// reached it is closed and the client reconnects, which asks for a new
	// ticket and so decides its rights again.
	socketLifetime = time.Hour
	// socketHeartbeat is how often a quiet connection is pinged. The ingress
	// cuts a connection that carries nothing for its proxy-read-timeout.
	socketHeartbeat = 25 * time.Second
	// socketWrite bounds one write to a client: one that does not read is dropped.
	socketWrite = 10 * time.Second
	// socketSweep is how often idle rooms are unloaded, logs compacted, announced
	// agents that went idle removed and quiet rooms made into versions.
	socketSweep = 5 * time.Second
	// maxTicketsPerUser bounds the tickets one person can hold unused.
	maxTicketsPerUser = 20
)

// Clock is the server's source of time for what it limits and schedules:
// ticket expiry, connection lifetime and heartbeat, idle rooms. Tests replace it.
type Clock interface {
	Now() time.Time
	// After returns a channel that receives once d has passed.
	After(d time.Duration) <-chan time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time                         { return time.Now() }
func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Application close codes of the live socket, in the 4000 range the protocol
// leaves to applications. A browser cannot read why a handshake failed, so
// what a client must act on is said in a close frame, after the upgrade.
const (
	// CloseEpoch: the document was rebuilt; the reason is the new epoch. Drop
	// the local copy and reconnect with a new ticket.
	closeEpoch websocket.StatusCode = 4001
	// CloseLimit: a limit was hit; the reason names it and the value.
	closeLimit websocket.StatusCode = 4002
	// CloseReset: the room was reloaded from disk; reconnect.
	closeReset websocket.StatusCode = 4003
	// CloseSlow: the connection did not keep up.
	closeSlow websocket.StatusCode = 4009
	// CloseAccess: who may see or edit the artifact changed; ask for a new
	// ticket, which says what the client may do now.
	closeAccess websocket.StatusCode = 4403
	// CloseLifetime: the connection reached its maximum lifetime; reconnect.
	closeLifetime websocket.StatusCode = 4408
)

var (
	errAccessChanged = errors.New("who may see or edit this artifact changed; reconnect")
	errLifetime      = errors.New("the connection reached its maximum lifetime; reconnect")
	errBadMessage    = errors.New("the message could not be read")
)

// live is the realtime side of the server.
type live struct {
	hub *realtime.Hub
	// snap cuts a version from a room that has been quiet.
	snap    *roomkind.Snapshotter
	tickets *ticketBook
	clock   Clock
	// sweepEvery is how often Run tidies rooms and cuts versions: socketSweep,
	// or often enough to notice a shortened RoomQuiet.
	sweepEvery time.Duration
	// pings counts the heartbeats sent, for the tests of the heartbeat.
	pings atomic.Int64
}

// ticket is the one-use credential for opening a socket: it names a person and
// one artifact, and nothing else. The person's rights are decided again when
// the socket opens, not read from the ticket.
type ticket struct {
	user, room string
	expires    time.Time
}

type ticketBook struct {
	clock Clock
	mu    sync.Mutex
	byID  map[string]ticket
}

func newTicketBook(clock Clock) *ticketBook {
	return &ticketBook{clock: clock, byID: map[string]ticket{}}
}

func (b *ticketBook) issue(user, room string) string {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Sprintf("no randomness for a ticket: %v", err))
	}
	id := base64.RawURLEncoding.EncodeToString(raw[:])
	now := b.clock.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	mine, oldest := 0, ""
	for k, t := range b.byID {
		switch {
		case !now.Before(t.expires):
			delete(b.byID, k)
		case t.user == user:
			mine++
			if oldest == "" || t.expires.Before(b.byID[oldest].expires) {
				oldest = k
			}
		}
	}
	if mine >= maxTicketsPerUser {
		delete(b.byID, oldest)
	}
	b.byID[id] = ticket{user: user, room: room, expires: now.Add(ticketLife)}
	return id
}

// redeem spends a ticket. It is spent whatever the answer: a ticket that was
// presented for the wrong artifact, or too late, is not kept for another try.
func (b *ticketBook) redeem(id, room string) (user string, ok bool) {
	b.mu.Lock()
	t, found := b.byID[id]
	delete(b.byID, id)
	b.mu.Unlock()
	if !found || !b.clock.Now().Before(t.expires) || t.room != room {
		return "", false
	}
	return t.user, true
}

func newLive(cfg Config, svc *service.Service) *live {
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	codecs := roomkind.DefaultCodecs()
	snap := roomkind.NewSnapshotter(roomkind.Config{Store: cfg.Store, Versions: loggedRoomVersions{svc: svc, log: cfg.Logger}, Files: svc, Codecs: codecs, Now: clock.Now, Quiet: cfg.RoomQuiet, Logger: cfg.Logger})
	hub, err := realtime.New(realtime.Config{
		ReceiveGuard: svc.ArtifactGuard().Shared,
		Log:          roomkind.SeededLog(cfg.Store.RealtimeLog(), cfg.Store, svc, codecs, cfg.Realtime.DocBytes),
		Validate:     codecs.Check,
		Limits:       cfg.Realtime,
		Now:          clock.Now,
		OnUpdate:     snap.Updated,
		Logger:       cfg.Logger,
	})
	if err != nil {
		panic(fmt.Sprintf("realtime hub: %v", err)) // only a missing log, which the store always gives
	}
	snap.Use(hub)
	svc.SetLiveRooms(snap)
	sweep := socketSweep
	if cfg.RoomQuiet > 0 {
		sweep = min(sweep, max(cfg.RoomQuiet/4, 50*time.Millisecond))
	}
	return &live{hub: hub, snap: snap, tickets: newTicketBook(clock), clock: clock, sweepEvery: sweep}
}

// Run keeps the realtime hub tidy (idle rooms unloaded, logs compacted) and cuts
// versions from rooms that went quiet, until ctx ends, and then disconnects
// everyone. main starts it next to the HTTP server.
func (s *Server) Run(ctx context.Context) {
	snapshots := make(chan struct{})
	go func() {
		defer close(snapshots)
		s.live.snap.Run(ctx, s.live.sweepEvery)
	}()
	s.live.hub.Run(ctx, s.live.sweepEvery)
	<-snapshots
}

// liveTicket answers POST /api/artifacts/{id}/live: a signed-in person who can
// see the artifact gets a one-use ticket for its socket. Whoever cannot see it
// gets what a missing artifact gets, and an anonymous caller is refused like
// anywhere else in the API.
func (s *Server) liveTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.svc.Artifact(r.Context(), currentViewer(r), id); err != nil {
		s.fail(w, err)
		return
	}
	epoch, err := s.live.hub.Epoch(r.Context(), id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":           s.live.tickets.issue(currentUser(r), id),
		"epoch":            epoch,
		"expiresInSeconds": int(ticketLife / time.Second),
		"path":             "/ws/" + id,
	})
}

// socketMode is what a person may do on an artifact's socket.
type socketMode int

const (
	socketNone socketMode = iota
	socketRead
	socketWritable
)

// socketRights asks the one place that decides: Service.Edit says editor,
// and a refusal of Forbidden (they can see it, they may not edit it) says
// reader; not found says nothing, in the words of a missing artifact.
func (s *Server) socketRights(ctx context.Context, username, id string) (socketMode, error) {
	_, err := s.svc.Edit(ctx, s.svc.Viewer(username), id)
	switch service.CodeOf(err) {
	case 0:
		if err != nil {
			return socketNone, err
		}
		return socketWritable, nil
	case service.Forbidden:
		return socketRead, nil
	case service.NotFound:
		return socketNone, nil
	case service.Invalid, service.Conflict, service.Busy:
		return socketNone, err
	default:
		return socketNone, err
	}
}

func identityFor(user, label string, mode socketMode) realtime.Identity {
	return realtime.Identity{Username: user, Label: label, ReadOnly: mode != socketWritable}
}

func modeOf(who realtime.Identity) socketMode {
	if who.ReadOnly {
		return socketRead
	}
	return socketWritable
}

// liveSocket answers GET /ws/{room}. The upgrade is refused unless it comes
// from this site's own pages (Origin equal to Host; a page in a sandbox sends
// Origin: null), carries a ticket that is good for this artifact, and the ticket's
// person may still see the artifact. What the person may do is then decided
// right here, as the socket opens.
func (s *Server) liveSocket(w http.ResponseWriter, r *http.Request) {
	room := r.PathValue("room")
	if !sameOrigin(r) {
		jsonError(w, http.StatusForbidden, "cross-origin socket refused")
		return
	}
	user, ok := s.live.tickets.redeem(r.URL.Query().Get("ticket"), room)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "the ticket is missing, used, expired or for another artifact; ask for a new one")
		return
	}
	// Admission covers rights and Join, never the connection's lifetime.
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()
	c, peer := s.openLivePeer(ctx, w, r, user, room)
	if peer == nil {
		return
	}

	readerDone := make(chan struct{})
	go s.socketReads(ctx, readerDone, c, peer)
	s.socketWrites(ctx, readerDone, c, peer)
}

// openLivePeer prevents a join from straddling an authority-change watcher.
func (s *Server) openLivePeer(ctx context.Context, w http.ResponseWriter, r *http.Request, user, room string) (*websocket.Conn, *realtime.Peer) {
	defer s.svc.ArtifactGuard().Shared(room)()
	mode, err := s.socketRights(ctx, user, room)
	if err != nil {
		s.serverError(w, err)
		return nil, nil
	}
	if mode == socketNone {
		jsonError(w, http.StatusNotFound, "not found")
		return nil, nil
	}
	label := user
	if me, err := s.svc.Me(ctx, s.svc.Viewer(user)); err == nil && me.DisplayName != "" {
		label = me.DisplayName
	}
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return nil, nil // Accept has answered the client
	}
	c.SetReadLimit(int64(s.live.hub.Limits().UpdateBytes) + 4096)
	peer, err := s.live.hub.Join(ctx, room, identityFor(user, label, mode), r.URL.Query().Get("epoch"))
	if err != nil {
		closeSocket(c, err)
		return nil, nil
	}
	return c, peer
}

// socketReads hands what the client sends to the room until the connection ends.
func (s *Server) socketReads(ctx context.Context, done chan<- struct{}, c *websocket.Conn, peer *realtime.Peer) {
	// The writer is told through a channel, not by cancelling ctx: the library
	// drops a connection whose write or read is cancelled, without the close
	// frame that tells the client why.
	defer close(done)
	for {
		kind, data, err := c.Read(ctx)
		if err != nil {
			peer.Close()
			return
		}
		if kind != websocket.MessageBinary {
			peer.CloseBecause(errBadMessage)
			return
		}
		if err := peer.Receive(ctx, data); err != nil {
			if errors.Is(err, realtime.ErrPeerClosed) {
				return
			}
			s.log.Debug("live message refused", "err", err)
			peer.CloseBecause(err)
			return
		}
	}
}

// socketWrites sends what the room queues for the client, pings it so the
// ingress keeps the connection, and ends it at its lifetime. When the peer ends,
// whoever ended it, the socket is closed with the code that says why.
func (s *Server) socketWrites(ctx context.Context, readerDone <-chan struct{}, c *websocket.Conn, peer *realtime.Peer) {
	// However the loop ends, the peer is closed (a no-op when it already was,
	// which keeps the reason it was closed with) and the socket says why.
	defer func() {
		peer.Close()
		closeSocket(c, peer.Err())
	}()
	lifetime := s.live.clock.After(socketLifetime)
	beat := s.live.clock.After(socketHeartbeat)
	for {
		select {
		case msg, open := <-peer.Out():
			if !open {
				return
			}
			wctx, cancel := context.WithTimeout(ctx, socketWrite)
			err := c.Write(wctx, websocket.MessageBinary, msg)
			cancel()
			if err != nil {
				return
			}
		case <-beat:
			s.live.pings.Add(1)
			pctx, cancel := context.WithTimeout(ctx, socketWrite)
			err := c.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
			beat = s.live.clock.After(socketHeartbeat)
		case <-lifetime:
			lifetime = nil // once; the peer's Out closes next
			peer.CloseBecause(errLifetime)
		case <-readerDone:
			return
		}
	}
}

// closeSocket ends the WebSocket with the close code that tells the client
// what to do: the room's reason becomes a code and a short text.
func closeSocket(c *websocket.Conn, why error) {
	code, reason := closeFor(why)
	if len(reason) > 120 { // a close frame holds 123 bytes
		reason = reason[:120]
	}
	_ = c.Close(code, reason) // the client may already be gone
}

func closeFor(why error) (websocket.StatusCode, string) {
	var epoch *realtime.EpochError
	var limit *realtime.LimitError
	switch {
	case why == nil:
		return websocket.StatusNormalClosure, ""
	case errors.As(why, &epoch):
		return closeEpoch, epoch.Current
	case errors.As(why, &limit):
		return closeLimit, limit.Error()
	case errors.Is(why, realtime.ErrRoomReset):
		return closeReset, realtime.ErrRoomReset.Error()
	case errors.Is(why, realtime.ErrSlowPeer):
		return closeSlow, realtime.ErrSlowPeer.Error()
	case errors.Is(why, realtime.ErrShutdown):
		return websocket.StatusGoingAway, "the server is restarting"
	case errors.Is(why, errAccessChanged):
		return closeAccess, errAccessChanged.Error()
	case errors.Is(why, errLifetime):
		return closeLifetime, errLifetime.Error()
	case errors.Is(why, realtime.ErrNoRoom):
		return websocket.StatusPolicyViolation, "not found"
	default:
		return websocket.StatusPolicyViolation, "the message was refused"
	}
}

// liveAccessChanged is the service's word that who may see or edit an artifact
// changed (or that it is gone). It asks again, for every distinct connection
// to the artifact's room, what that person may do now, and closes the
// connections whose answer is no longer the one they were opened with: the
// removed editor, the person the new audience excludes, everyone when the
// artifact is deleted. It runs before the change returns, so the next write of a
// removed editor has nowhere to go.
func (s *Server) liveAccessChanged(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stale := map[realtime.Identity]bool{}
	for _, who := range s.live.hub.Connected(id) {
		if _, checked := stale[who]; checked {
			continue
		}
		mode, err := s.socketRights(ctx, who.Username, id)
		if err != nil {
			s.log.Error("could not check a live connection after an access change; closing it", "id", id, "user", who.Username, "err", err)
		}
		stale[who] = err != nil || mode != modeOf(who)
	}
	if len(stale) == 0 {
		return
	}
	s.live.hub.Disconnect(id, func(who realtime.Identity) bool { return stale[who] }, errAccessChanged)
}

// routesLive registers the realtime socket.
func (s *Server) routesLive(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/artifacts/{id}/live", s.browserAPI(s.liveTicket))
	mux.HandleFunc("GET /ws/{room}", s.liveSocket)
}
