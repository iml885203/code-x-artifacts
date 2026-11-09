package service

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"regexp"
	"slices"
	"strings"
)

// A guest is a signed-out browser reading a public link (docs/viewers.md). The browser makes up a random id and keeps
// it; the server lists only a digest of it and a nickname drawn from that digest, so the id never leaves the browser
// and nobody who reads the list can beat as that guest. A guest is a presence label and nothing more: it is never a
// Viewer, so it grants no access of any kind.

// GuestIDPrefix starts the listed id of every guest. A colon is in no account name and in no agent id
// ("<account>/<client>"), so a guest can never be listed as, or merged with, a person or an agent.
const GuestIDPrefix = "guest:"

// How many guests the in-memory viewer book holds, per artifact and in all. Anonymous traffic is the only
// traffic that adds rows without an account behind them, so these bound what it can cost: a guest past them
// is refused, and the guests already listed keep their places.
const (
	MaxGuestsPerArtifact = 100
	MaxGuests            = 5000
)

var guestIDForm = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

var guestAdjectives = [...]string{
	"Amber", "Brave", "Bright", "Calm", "Clever", "Cosmic", "Curious", "Daring",
	"Gentle", "Golden", "Happy", "Hidden", "Jolly", "Keen", "Lively", "Lucky",
	"Mellow", "Misty", "Nimble", "Quiet", "Rapid", "Silver", "Sleepy", "Snowy",
	"Sunny", "Swift", "Tidy", "Velvet", "Witty", "Wild", "Wise", "Zesty",
}

var guestAnimals = [...]string{
	"Badger", "Beaver", "Bison", "Crane", "Dolphin", "Falcon", "Ferret", "Finch",
	"Fox", "Gecko", "Heron", "Ibis", "Koala", "Lemur", "Lynx", "Marten",
	"Moose", "Narwhal", "Otter", "Owl", "Panda", "Puffin", "Quokka", "Raven",
	"Robin", "Seal", "Stoat", "Swan", "Tapir", "Walrus", "Wombat", "Yak",
}

// guest is how a browser's guest id is listed.
type guest struct {
	id, nickname string
}

func (g guest) key() string { return "g:" + g.id }

// parseGuest checks a browser's guest id and derives how it is listed: a digest for the id, two words for the
// nickname. The digest's first bytes are the id (and the tag that tells colliding nicknames apart), later ones
// pick the words, so the tag is not a function of the nickname.
func parseGuest(raw string) (guest, error) {
	if !guestIDForm.MatchString(raw) {
		return guest{}, refuse(Invalid, "guest must be the browser's random guest id: 16 to 64 letters, digits, '-' or '_'")
	}
	sum := sha256.Sum256([]byte("guest-id/v1\x00" + raw))
	adjective := guestAdjectives[int(binary.BigEndian.Uint16(sum[16:]))%len(guestAdjectives)]
	animal := guestAnimals[int(binary.BigEndian.Uint16(sum[18:]))%len(guestAnimals)]
	return guest{id: GuestIDPrefix + hex.EncodeToString(sum[:12]), nickname: adjective + " " + animal}, nil
}

// distinguishGuests tells apart guests of one list who drew the same nickname: each gets the shortest tag from
// its own id that no other guest with that nickname shares, so a guest's name depends on who else has its
// nickname, never on the order of the list. People and agents are left as they are.
func distinguishGuests(list []ViewerEntry) []ViewerEntry {
	byName := map[string][]int{}
	for i, e := range list {
		if e.Guest {
			byName[e.Name] = append(byName[e.Name], i)
		}
	}
	out := slices.Clone(list)
	for name, at := range byName {
		if len(at) < 2 {
			continue
		}
		digests := make([]string, len(at))
		for j, i := range at {
			digests[j] = strings.TrimPrefix(list[i].ID, GuestIDPrefix)
		}
		n := tagLength(digests)
		for j, i := range at {
			out[i].Name = name + " #" + digests[j][:min(n, len(digests[j]))]
		}
	}
	return out
}

// tagLength is the shortest prefix length, from four characters, at which the digests all differ.
func tagLength(digests []string) int {
	longest := 0
	for _, d := range digests {
		longest = max(longest, len(d))
	}
	for n := 4; n < longest; n++ {
		seen := map[string]bool{}
		unique := true
		for _, d := range digests {
			p := d[:min(n, len(d))]
			if seen[p] {
				unique = false
				break
			}
			seen[p] = true
		}
		if unique {
			return n
		}
	}
	return longest
}
