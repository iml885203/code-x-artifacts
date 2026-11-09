package realtime

import (
	"context"
	"errors"
	"time"
)

// Peer is one connection to a room. A transport moves bytes between it and
// the wire: what a client sends goes to Receive, what Out yields goes to the
// client. Both carry y-websocket's framing, so a stock Yjs provider works
// at the other end.
type Peer struct {
	room     *room
	who      Identity
	src      *source
	out      chan []byte
	done     chan struct{}
	err      error
	closed   bool
	ids      map[uint64]struct{}
	dropped  int
	joinedAt time.Time
}

// Identity is who the connection was admitted as.
func (p *Peer) Identity() Identity { return p.who }

// JoinedAt is when the connection was admitted, for a transport that limits its lifetime.
func (p *Peer) JoinedAt() time.Time { return p.joinedAt }

// Out yields the messages for the client, in order. It is closed when the
// connection ends, after the messages that were already queued.
func (p *Peer) Out() <-chan []byte { return p.out }

// Done is closed when the connection has ended, whoever ended it.
func (p *Peer) Done() <-chan struct{} { return p.done }

// Err says why the connection ended: the reason it was closed with, or nil
// when the client left. It is nil while the connection is open.
func (p *Peer) Err() error {
	select {
	case <-p.done:
		return p.err
	default:
		return nil
	}
}

// Dropped counts the writes of a read-only connection that were discarded.
func (p *Peer) Dropped() int {
	p.room.mu.Lock()
	defer p.room.mu.Unlock()
	return p.dropped
}

// Receive handles one message from the client. It returns when the message
// has been dealt with: a write that returned nil is applied to the room, is on
// disk and has been queued for the other peers. An error means the message was
// not applied; a *LimitError names the limit, and the connection stays open.
func (p *Peer) Receive(ctx context.Context, msg []byte) error {
	if admit := p.room.hub.cfg.ReceiveGuard; admit != nil {
		defer admit(p.room.id)()
	}
	err := p.room.run(ctx, func() error {
		if p.closed {
			return ErrPeerClosed
		}
		return p.room.handleLocked(p, msg)
	})
	if errors.Is(err, errRoomGone) {
		return ErrPeerClosed
	}
	return err
}

// Close ends the connection because the client left.
func (p *Peer) Close() { p.CloseBecause(nil) }

// CloseBecause ends the connection for a reason of the transport's own (a
// connection that reached its lifetime, a message it could not accept);
// Peer.Err reports it.
func (p *Peer) CloseBecause(reason error) {
	r := p.room
	r.mu.Lock()
	defer r.mu.Unlock()
	p.closeLocked(reason)
}

func (p *Peer) enqueueLocked(msg []byte) {
	if p.closed {
		return
	}
	select {
	case p.out <- msg:
	default:
		p.closeLocked(ErrSlowPeer)
	}
}

func (p *Peer) closeLocked(reason error) {
	if p.closed {
		return
	}
	p.closed = true
	p.err = reason
	r := p.room
	delete(r.peers, p)
	r.activeAt = r.hub.cfg.Now()
	close(p.out)
	close(p.done)
	r.leaveLocked(p.ids, p)
}
