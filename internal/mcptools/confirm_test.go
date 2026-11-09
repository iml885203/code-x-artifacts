package mcptools

import (
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestAConfirmTokenWorksOnceForTheSameCallerToolAndArguments(t *testing.T) {
	c := newConfirmations((&clock{t: time.Unix(1000, 0)}).now)
	token, err := c.issue("alice", "delete", deleteArgs{ID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if wrongTool, _ := c.issue("alice", "db_delete", deleteArgs{ID: "a1"}); wrongTool == "" {
		t.Fatal("no token issued")
	} else if ok, _ := c.redeem("alice", "delete", deleteArgs{ID: "a1"}, wrongTool); ok {
		t.Error("a token issued for one tool confirmed another")
	}
	for _, tc := range []struct {
		name       string
		user, tool string
		args       deleteArgs
		want       bool
	}{
		{"other arguments", "alice", "delete", deleteArgs{ID: "a2"}, false},
		{"other caller", "bob", "delete", deleteArgs{ID: "a1"}, false},
		{"the call it was issued for", "alice", "delete", deleteArgs{ID: "a1"}, true},
		{"the same call a second time", "alice", "delete", deleteArgs{ID: "a1"}, false},
	} {
		got, err := c.redeem(tc.user, tc.tool, tc.args, token)
		if err != nil || got != tc.want {
			t.Errorf("%s: redeem = %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
}

func TestAConfirmTokenExpires(t *testing.T) {
	clk := &clock{t: time.Unix(1000, 0)}
	c := newConfirmations(clk.now)
	token, _ := c.issue("alice", "delete", deleteArgs{ID: "a1"})
	clk.t = clk.t.Add(confirmTTL - time.Second)
	if ok, _ := c.redeem("alice", "delete", deleteArgs{ID: "a1"}, token); !ok {
		t.Fatal("a token just inside its lifetime was refused")
	}
	token, _ = c.issue("alice", "delete", deleteArgs{ID: "a1"})
	clk.t = clk.t.Add(confirmTTL)
	if ok, _ := c.redeem("alice", "delete", deleteArgs{ID: "a1"}, token); ok {
		t.Fatal("an expired token was accepted")
	}
}

func TestAnUnknownTokenConfirmsNothing(t *testing.T) {
	c := newConfirmations((&clock{t: time.Unix(1000, 0)}).now)
	if ok, _ := c.redeem("alice", "delete", deleteArgs{ID: "a1"}, "cf_guess"); ok {
		t.Fatal("a token nobody issued was accepted")
	}
}

func TestACallerThatKeepsAskingForPlansLosesItsOldestOnes(t *testing.T) {
	clk := &clock{t: time.Unix(1000, 0)}
	c := newConfirmations(clk.now)
	first, _ := c.issue("alice", "delete", deleteArgs{ID: "a0"})
	for i := range maxPlansPerUser {
		clk.t = clk.t.Add(time.Second)
		if _, err := c.issue("alice", "delete", deleteArgs{ID: string(rune('b' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	if ok, _ := c.redeem("alice", "delete", deleteArgs{ID: "a0"}, first); ok {
		t.Error("the oldest plan survived past the per-caller limit")
	}
	if got := len(c.pending); got > maxPlansPerUser {
		t.Errorf("%d plans pending, limit %d", got, maxPlansPerUser)
	}
}
