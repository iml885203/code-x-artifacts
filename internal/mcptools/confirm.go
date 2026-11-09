package mcptools

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

const (
	// confirmTTL is how long a plan stays confirmable: long enough for a person
	// to read it, short enough that an old plan cannot be applied by accident.
	confirmTTL = 10 * time.Minute
	// maxPlansPerUser bounds memory: a caller that keeps asking for plans
	// loses its oldest ones.
	maxPlansPerUser = 20
)

// confirmations issues and redeems the tokens a destructive tool hands out
// with its plan. A token is bound to the caller, the tool and a hash of the
// arguments, so an agent that showed the person one plan cannot apply a
// different one; it works once, and only for confirmTTL. Tokens live in memory
// on purpose: a restart cancels every plan, which is the safe direction.
type confirmations struct {
	mu      sync.Mutex
	now     func() time.Time
	pending map[string]confirmation
}

type confirmation struct {
	user, tool string
	args       string
	expires    time.Time
}

func newConfirmations(now func() time.Time) *confirmations {
	return &confirmations{now: now, pending: map[string]confirmation{}}
}

// argsHash identifies one set of arguments. Callers pass a struct that holds
// exactly the arguments that decide what the tool does, never the token.
func argsHash(args any) (string, error) {
	b, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("hash tool arguments: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// issue returns a token that confirms exactly this call by this user.
func (c *confirmations) issue(user, tool string, args any) (string, error) {
	h, err := argsHash(args)
	if err != nil {
		return "", err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("make confirm token: %w", err)
	}
	token := "cf_" + hex.EncodeToString(raw)

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	own := 0
	var oldest string
	for t, p := range c.pending {
		switch {
		case !now.Before(p.expires):
			delete(c.pending, t)
		case p.user == user:
			own++
			if oldest == "" || p.expires.Before(c.pending[oldest].expires) {
				oldest = t
			}
		}
	}
	if own >= maxPlansPerUser {
		delete(c.pending, oldest)
	}
	c.pending[token] = confirmation{user: user, tool: tool, args: h, expires: now.Add(confirmTTL)}
	return token, nil
}

// redeem reports whether token confirms this call, and spends it if so. A
// token presented with other arguments, by another user or for another tool is
// refused and stays valid for the call it was issued for.
func (c *confirmations) redeem(user, tool string, args any, token string) (bool, error) {
	h, err := argsHash(args)
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.pending[token]
	if !ok || !c.now().Before(p.expires) {
		delete(c.pending, token)
		return false, nil
	}
	if p.user != user || p.tool != tool || p.args != h {
		return false, nil
	}
	delete(c.pending, token)
	return true, nil
}
