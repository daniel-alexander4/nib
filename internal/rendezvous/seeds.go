package rendezvous

import "net"

// # Where a genuinely cold machine starts
//
// D6's amendment of 2026-08-19: "populated on first contact" never said first contact
// with *what*, and the answer was nothing. A fresh install has an empty cache, no
// hostname bootstrap (D6 forbids it), and therefore no way to reach the DHT at all —
// which makes the self-address probe and the record publish both unreachable on the one
// machine that matters most.
//
// These are IP literals and not hostnames, and that is the whole point: it keeps both
// reasons hostnames were refused. No DNS lookup tells a third party who is starting a
// ceremony and when, and a network that blocks or rewrites DNS does not take the
// rendezvous with it.
//
// # They are consulted only when the cache is empty, and they WILL rot
//
// Measured 2026-08-19, three pings each from one host:
//
//	212.129.33.59:6881   dht.transmissionbt.com   ANSWERS
//	87.98.162.88:6881    dht.transmissionbt.com   ANSWERS
//	67.215.246.10:6881   router.bittorrent.com    silent
//	82.221.103.244:6881  router.utorrent.com      silent
//	185.157.221.247:6881 dht.libtorrent.org       silent
//
// Three of the five shipped were already dead the day the list was written, and
// router.bittorrent.com and router.utorrent.com are the two every guide still names. A
// sixth, dht.aelitis.com, was dead too and is not shipped. That is the honest state of
// this mechanism, and it is worse than it looks: within an hour of writing this, heavy
// use of the two live routers got them to keep answering `ping` while returning NO
// NODES to `find_node`, so three consecutive cold starts bootstrapped to an empty
// table. Patience recovered it, but a two-address list is a single point of failure and
// this is what hitting one looks like.
//
// Hence two things. The silent addresses are kept, because a dead seed costs one
// datagram and a timeout while a resurrected one costs nothing to have listed. And
// `Stats().Bootstrapped` exists so a diagnostic can say "every seed we ship is dead
// now" — a real thing a user will hit, and a terrible thing to have to guess at.
//
// The durable mechanism is the cache, not this list. One successful contact replaces it
// forever, and D6's second half — seed nodes carried in the invitation, filed against
// P04.S03 — is what keeps the common case from ever consulting it.
func seedNodes() []*net.UDPAddr {
	return []*net.UDPAddr{
		{IP: net.IPv4(212, 129, 33, 59), Port: 6881},
		{IP: net.IPv4(87, 98, 162, 88), Port: 6881},
		{IP: net.IPv4(67, 215, 246, 10), Port: 6881},
		{IP: net.IPv4(82, 221, 103, 244), Port: 6881},
		{IP: net.IPv4(185, 157, 221, 247), Port: 6881},
	}
}
