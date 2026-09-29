package rendezvous

import (
	"errors"
	"net"
	"net/netip"
	"sync"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
)

// # The outbound door: a query goes only where the cache rule would let a bootstrap start
//
// anacrolix decides whom a traversal contacts with `Server.TraversalNodeFilter`
// (server.go:1476), a METHOD — not a config field — which admits any address that is not
// port 0, not 0/8 and not on `IPBlocklist`. BEP-42 exempts private and loopback sources from
// the ID check, so a hostile PUBLIC node answering a `find_node` with `192.168.1.1:80` or
// `127.0.0.1:631` had us send KRPC there: a probe of the user's own LAN, aimed by a stranger
// (/pending 743).
//
// The library's only lever is `IPBlocklist`, and it is the wrong one: it is applied to
// inbound datagrams too (server.go:369) — every reply to a private-space querier dropped along
// with the query we should not send. So the door is here, on the one socket the library
// writes through (`screened` wraps it, see screen.go), and it refuses exactly one thing: a
// QUERY to an address `Server.scope` refuses. A reply goes back to whoever asked, whatever
// their address — answering is not something a stranger can aim.
//
// **The rule is `scope`, the cache's rule, and not a second one.** Production's is
// `addrscope.Seed` — what the invitation's seeds, the shipped seeds and the node cache
// already obey — so a traversal can now reach no address a bootstrap could not start from.
// Nib's LAN peers are found by its own discovery and dialled directly (ADR-011); nothing in
// production queries a DHT node in private space, and the hermetic tests, whose whole DHT is
// loopback, open with `seedOrLoopback` and keep traversing it. The port floor comes with the
// rule: a query to a public node on port 53 or 123 is the reflection shape `addrscope.MinPort`
// exists for, and a stranger's `nodes` list was a way around it.

// errRefusedSend is what the library's `writeToNode` sees for a refused query. It returns
// the error and hands the rate-limit token back (server.go:824-829), so the transaction
// fails at once instead of waiting out its resend budget.
var errRefusedSend = errors.New("rendezvous: refused to query an address outside the node-cache rule")

// maxContacts bounds what one Server remembers about whom it queried. A traversal is a few
// hundred queries; past this, new addresses are sent to but not remembered, which only
// means they can neither be saved as "answered" nor demoted as "silent" — the safe side of
// both.
const maxContacts = 4096

// contact is what this run knows about one address.
type contact uint8

const (
	// contactUnknown: never queried this run (a cached node no traversal reached, or one
	// past maxContacts).
	contactUnknown contact = iota
	// contactAnswered: we queried it and a response came back from it.
	contactAnswered
	// contactSilent: we queried it and nothing came back.
	contactSilent
)

// contacts is the node-quality signal anacrolix does not export (/pending 743 part 2).
//
// The library tracks `lastGotResponse` per node and exports it only as `WriteStatus` text;
// `Server.Nodes()` is every node not yet BAD, which includes any stranger that merely sent
// us a query (server.go:492). So Nib keeps its own: the addresses its queries went to
// (recorded by the outbound door) and which of those sent a response back (recorded by
// `screened.ReadFrom`). A response from an address we never queried records nothing — an
// unsolicited "reply" is how a stranger would otherwise write itself into the cache.
type contacts struct {
	mu       sync.Mutex
	queried  map[netip.AddrPort]bool // value: answered
	capacity int
}

func newContacts() *contacts {
	return &contacts{queried: make(map[netip.AddrPort]bool), capacity: maxContacts}
}

func (c *contacts) sent(ap netip.AddrPort) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.queried[ap]; ok || len(c.queried) >= c.capacity {
		return
	}
	c.queried[ap] = false
}

func (c *contacts) answered(ap netip.AddrPort) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.queried[ap]; ok {
		c.queried[ap] = true
	}
}

func (c *contacts) state(ap netip.AddrPort) contact {
	c.mu.Lock()
	defer c.mu.Unlock()
	answered, ok := c.queried[ap]
	switch {
	case !ok:
		return contactUnknown
	case answered:
		return contactAnswered
	default:
		return contactSilent
	}
}

// udpAddrPort is a socket address as the scope rule reads it, unmapped as nodeAddrPort does.
func udpAddrPort(a net.Addr) (netip.AddrPort, bool) {
	ua, ok := a.(*net.UDPAddr)
	if !ok || ua == nil {
		return netip.AddrPort{}, false
	}
	ap := ua.AddrPort()
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()), ap.IsValid()
}

// isReply reports whether an outbound datagram is a response or an error — the two kinds a
// node sends to whoever queried it. Anything else, including a datagram that does not decode
// (the library's own encoder wrote it, so that is not expected), is treated as a query and
// held to the rule: the door fails closed.
func isReply(b []byte) bool {
	var m struct {
		Y string `bencode:"y"`
	}
	if err := bencode.Unmarshal(b, &m); err != nil {
		return false
	}
	return m.Y == krpc.YResponse || m.Y == krpc.YError
}

// WriteTo is the outbound door. See the top of this file.
func (s *screened) WriteTo(p []byte, addr net.Addr) (int, error) {
	if !isReply(p) {
		ap, ok := udpAddrPort(addr)
		if !ok || !s.scope(ap) {
			s.refusedSends.Add(1)
			return 0, errRefusedSend
		}
		s.contacts.sent(ap)
	}
	return s.PacketConn.WriteTo(p, addr)
}
