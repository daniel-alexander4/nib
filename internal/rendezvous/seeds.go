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
// # One of the three was not dead, it was on the wrong port (2026-08-20)
//
// `dht.libtorrent.org` listens on **25401**, not 6881. Re-measured through P04.S06's live
// harness, one seed at a time, each from a cold table:
//
//	185.157.221.247:25401  ->  26 nodes learned
//	185.157.221.247:6881   ->   2 nodes learned
//	212.129.33.59 + 87.98.162.88 (:6881)  ->  25 nodes learned
//	67.215.246.10 + 82.221.103.244 (:6881)  ->  0 nodes learned
//
// So the list was carrying a transcription error and reading it as rot. "Silent" is a
// verdict about an address, and an address is a host AND a port — the 2026-08-19 pass
// varied the host and held the port fixed at the one every guide prints, which cannot
// distinguish a dead host from a live one listening elsewhere. The port is corrected below;
// the two routers really are silent and are kept for the reason the next paragraph gives.
//
// This also revises the gloomiest sentence here: the shipped list is **not** uniformly
// failing. One run in the same session bootstrapped to 25 nodes off the transmissionbt
// pair alone, with the invitation seeds never reached. What the day actually shows is a
// list with two dead entries, one mistyped entry and two that work intermittently — which
// is a thin margin, not an outage, and is still the argument for D6's second half.
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
// P04.S06 — is what keeps the common case from ever consulting it.
// # The IPv6 seed (P05.S05, 2026-08-22)
//
// A v6-only host could not bootstrap at all: every literal above is IPv4, so `Bootstrap` had
// nothing to send a datagram to. That made D8's tier 2 unreachable from the one kind of host
// it exists for.
//
// **Only the operators already listed above were considered**, so nothing new is contacted —
// Dan's call, and it keeps D6's reason for literals intact: a v6 seed is still an address, not
// a name, so no lookup tells a third party who is starting a ceremony.
//
// **Measured 2026-08-22, three pings each plus a find_node, from one dual-stack host.** The
// measurement is the point, because a DNS lookup would have shipped two addresses and one of
// them is dead:
//
//	[2001:41d0:203:4cca:5::]:6881    dht.transmissionbt.com  SILENT, 3 of 3   -> NOT SHIPPED
//	[2a02:752:0:18::128]:25401       dht.libtorrent.org      ANSWERS, 3 of 3, and find_node
//	                                                         returned 114 bytes of nodes6
//
// router.bittorrent.com and router.utorrent.com publish no AAAA at all, so there is nothing to
// add for them — which costs nothing, since both have been measured silent over IPv4 since the
// list was written.
//
// **This is the same trap the port transcription above was.** `dht.transmissionbt.com` has an
// AAAA record and answers on IPv4, and both facts are irrelevant to whether anything is
// listening on that v6 address — "silent" is a verdict about an address, and an address is a
// host AND a port AND a family. It is recorded here rather than omitted silently so the next
// person does not re-derive it from the same lookup and reach the opposite conclusion.
//
// One live v6 seed is thin, and it is not the whole of the fix: `DefaultWant` (see `dht.go`)
// is what lets a v6 node be learned through an IPv4 contact, and one of the three v4 seeds was
// measured returning eight v6 nodes when asked. Neither alone reaches tier 2.
func seedNodes() []*net.UDPAddr {
	return []*net.UDPAddr{
		{IP: net.IPv4(212, 129, 33, 59), Port: 6881},
		{IP: net.IPv4(87, 98, 162, 88), Port: 6881},
		{IP: net.IPv4(67, 215, 246, 10), Port: 6881},
		{IP: net.IPv4(82, 221, 103, 244), Port: 6881},
		// 25401, not 6881 — see the re-measurement above. Shipping this on the port
		// every guide prints made a live node look like a fourth dead one.
		{IP: net.IPv4(185, 157, 221, 247), Port: 25401},
		// dht.libtorrent.org over IPv6, same operator and same port as the line above.
		{IP: net.ParseIP("2a02:752:0:18::128"), Port: 25401},
	}
}
