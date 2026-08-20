package server

import (
	"net"
	"strconv"
	"time"

	"nib/internal/discovery"
	"nib/internal/pairing"
	"nib/internal/vault"
)

// browseWindow is how long a browse listens before giving up on the local link.
//
// D16 fixes it at 2 s, and the reason is in that decision's own table: the ladder's
// tiers have latencies an order of magnitude apart, and the LAN answers in
// milliseconds. Two seconds is long enough for a peer that is there and short enough
// that a peer that is not costs almost nothing before the next tier is tried.
const browseWindow = 2 * time.Second

// candidate is a peer found on the link and matched to a pin the user already holds.
//
// Fingerprint is the PINNED one, copied from the vault — never anything the
// announcement supplied. That is not an implementation detail, it is L1: the network
// said where to look, and the vault says who this is. The dial that follows pins this
// fingerprint at the TLS handshake, so an announcer who lies about a name reaches a
// handshake that rejects it.
type candidate struct {
	Fingerprint []byte
	Label       string
	Name        string
	Addr        string // host:port, ready to dial
}

// hostOf renders a source address as a dialable host, keeping the zone that a
// link-local IPv6 address is meaningless without.
func hostOf(a *net.UDPAddr) string {
	if a.Zone != "" {
		return a.IP.String() + "%" + a.Zone
	}
	return a.IP.String()
}

// browser is the part of a discovery socket this file needs. An interface so the
// resolution can be driven with really-encoded announcements and no socket at all —
// the socket is separately driven at tier 5, and one test that could fail for either
// reason would tell you less than two that each fail for one.
type browser interface {
	Read(deadline time.Time) (discovery.Seen, error)
}

// resolve matches one announcement against the pinned peers.
//
// **This lives here and not in internal/discovery, and the reason is a guard.**
// TestNothingHereCanReachAnIdentity forbids that package importing vault, sign or
// p2p, so it cannot see a pin — which is L1 made structural rather than remembered.
// The layer that holds pins is the layer that decides what to dial, and putting the
// join here keeps the guard meaningful instead of routing around it.
//
// It uses pairing.Matches rather than comparing against a precomputed name table
// because Matches is the exported comparison and it normalises case and whitespace;
// a table keyed on exact bytes would quietly miss a peer whose name arrived
// differently cased. The cost is one Name() per pin per announcement, over a handful
// of pins.
func resolve(pins []vault.PinnedPeer, s discovery.Seen) (candidate, bool) {
	if s.From == nil || s.Port == 0 {
		return candidate{}, false
	}
	for _, p := range pins {
		if !pairing.Matches(p.Fingerprint, s.Name) {
			continue
		}
		return candidate{
			// Copied, not aliased: the caller's slice outlives this loop and a
			// shared backing array is how one peer's identity becomes another's.
			Fingerprint: append([]byte(nil), p.Fingerprint...),
			Label:       p.Label,
			Name:        s.Name,
			// The announced PORT with the observed ADDRESS. The announcement does
			// not carry an address because a claimed one can be anything, whereas
			// the source of the datagram is where a reply actually goes.
			//
			// **With the zone.** A link-local IPv6 source arrives as fe80::… and is
			// NOT DIALABLE without the interface it was heard on — measured:
			// `dial tcp [fe80::1]:9: connect: invalid argument`. The kernel cannot
			// pick a link on its own, and on a link-local discovery protocol this is
			// the common case, not an edge. net.UDPAddr carries Zone separately from
			// IP, so building the host from IP alone silently drops it and produces
			// a candidate that looks fine and cannot be reached. The first version
			// of this line did exactly that.
			Addr: net.JoinHostPort(hostOf(s.From), strconv.Itoa(int(s.Port))),
		}, true
	}
	return candidate{}, false
}

// browsePeers listens for the window and returns the pinned peers it found, most
// recently seen first, one entry per peer.
//
// Announcements repeat, so the same peer is seen several times in two seconds; the
// caller wants a candidate list, not a packet log.
// It returns EVERY distinct address seen, not one per peer.
//
// Deduping by fingerprint alone was a defect with a security consequence: two different
// HOSTS can claim one name — the name is broadcast in the clear every 500 ms and is
// displayed beside a signature, so it is not a secret — and the loser was thrown away
// rather than returned. An attacker announcing faster than the real peer therefore
// captured the browse outright and the genuine address was unreachable to any caller.
// Keyed on fingerprint AND address, so the caller gets both and can try both.
//
// # And CAPPED, because that fix created the next attack
//
// The address is the observed source host plus the port **from the datagram payload**
// (`resolve` above). So one on-link host needs no address spoofing at all: it announces the
// six-word name — which the armed side broadcasts in the clear every 500 ms, and which this
// very comment records is not a secret — with a different port each time, and every
// datagram becomes another distinct candidate. `dialAny` then walked all of them serially
// at 30 s each inside an HTTP handler, so a few hundred datagrams wedged
// `/api/session/initiate` for hours.
//
// The cap has to be more than one, or it reintroduces the capture attack above; the whole
// point is that the genuine peer must still be reachable when something else is also
// announcing. Eight is well past any real host's address count (a dual-stack machine with
// several interfaces is two or three) and small enough that the walk is bounded.
//
// The cap alone is not the fix. `dialAny` also takes a total budget — see `lanDialBudget`
// — because a cap bounds how many candidates there are and says nothing about how long each
// one is allowed to take.
// maxLANCandidates bounds what one browse may hand to the dialer. See browsePeers.
const maxLANCandidates = 8

func browsePeers(b browser, pins []vault.PinnedPeer, window time.Duration) []candidate {
	deadline := time.Now().Add(window)
	seen := map[string]candidate{}
	var order []string
	for {
		s, err := b.Read(deadline)
		if err != nil {
			// Every error here is ordinary: a timeout ends the window, and our own
			// announcements, foreign traffic and malformed datagrams are all
			// expected on a shared link. The socket counts them; this loop's job is
			// only to stop when the window closes.
			if time.Now().After(deadline) {
				break
			}
			continue
		}
		c, ok := resolve(pins, s)
		if !ok {
			continue
		}
		// Fingerprint AND address. A dual-stack peer arrives twice from two of its
		// own addresses and both are worth keeping; two different hosts claiming one
		// name is the attack above, and keeping both is what lets the caller get past
		// it. Order is first-seen, which is at least the faster path first.
		key := string(c.Fingerprint) + "\x00" + c.Addr
		if _, dup := seen[key]; dup {
			continue
		}
		order = append(order, key)
		seen[key] = c
		if len(order) >= maxLANCandidates {
			// Stop reading rather than keep accumulating: the window's remaining time
			// buys nothing once the list is full, and draining it is the flood's goal.
			break
		}
	}
	out := make([]candidate, 0, len(order))
	for _, k := range order {
		out = append(out, seen[k])
	}
	return out
}
