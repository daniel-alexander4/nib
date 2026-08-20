package addrscope

import "net/netip"

// Routable reports whether an address may be treated as a real, dialable public host.
//
// **It unmaps first, and that is not cosmetic.** `netip.Prefix.Contains` is false across
// address families, so a v4-mapped v6 address such as `::ffff:240.0.0.1` clears every v4
// prefix below and every `Is4()`-guarded test. Measured. The probe that first used this
// rule happened to unmap at its call site, which made the predicate safe THERE and nowhere
// else — so the unmapping belongs inside, where it protects every caller.
func Routable(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsUnspecified() || a.IsLoopback() || a.IsMulticast() ||
		a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsInterfaceLocalMulticast() ||
		a.IsPrivate() || !a.IsGlobalUnicast() {
		return false
	}
	if SharedSpace(a) {
		return false // 100.64/10 is inside the carrier's NAT; nobody outside can dial it
	}
	for _, p := range reserved {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// MinPort is the lowest port Nib will aim traffic at, with two named exceptions.
//
// Real peers live on ephemeral ports or a user-chosen high one; nothing legitimate asks us
// to hammer a system port. Refusing below 1024 removes DNS (53), NTP (123), SNMP (161),
// chargen (19), echo (7) and the rest of the classic UDP reflection set from an attacker's
// target space.
//
// # Why 80 and 443 are exceptions, and why that is not a hole
//
// The reflection argument is entirely about UDP: there is no amplification without a
// connectionless protocol. But D8 races QUIC and **TCP** concurrently, and D14 exists
// precisely for the networks where "outbound TCP is permitted while UDP is blocked or
// throttled wholesale" — corporate, campus and guest networks. On exactly those, a peer's
// only reachable inbound port is plausibly 443. A blanket floor is correctly sized for the
// punch and over-blocks the TCP half of every other dialable tier.
//
// 80 and 443 are not UDP reflectors in any deployed service, so admitting them costs
// nothing the floor was protecting.
//
// # The residual, stated rather than left implicit
//
// `Seed` below refuses the same two ports, and its argument — that they are the ports
// likeliest to belong to an unrelated third party's web server, so aiming UDP at them is
// unsolicited traffic at somebody's HTTPS endpoint — is true of a QUIC candidate here too.
// The exception is kept anyway, and the difference is volume and attribution, not harm:
//
//   - A candidate is inside a record signed by an identity in the roster, so an address
//     offered here is attributable to a party the convener invited.
//   - It is dialled once, by at most the roster, during one ceremony: a handful of QUIC
//     Initials. A seed goes into a routing table, is re-pinged, and is handed to traversals.
//   - HTTP/3 makes UDP/443 a real deployment, so a peer on exactly D14's network genuinely
//     may be reachable there. No deployed DHT node listens on 443.
//
// So the exception is bounded and named. Do not read the sentence above it as a claim that
// UDP at 443 is harmless — it is a claim that it does not *amplify*, which is less.
const MinPort = 1024

var portExceptions = map[uint16]bool{80: true, 443: true}

// Target is Routable plus the port rule, for an address somebody else supplied.
func Target(ap netip.AddrPort) bool {
	if !Routable(ap.Addr()) {
		return false
	}
	return ap.Port() >= MinPort || portExceptions[ap.Port()]
}

// Seed is Routable plus the port floor, for a DHT bootstrap address.
//
// **Deliberately not Target**, and the difference is the transport. Target's 80 and 443
// exceptions exist for D14's TCP fallback — the networks where outbound TCP is permitted
// and UDP is not, and a peer's only reachable inbound port is plausibly 443. A DHT seed is
// spoken to over **UDP only**: it is fed to a KRPC traversal and pinged on the shared
// socket. So the exceptions buy a seed nothing, and cost it the thing the floor was for —
// 80 and 443 are the two ports likeliest to belong to an unrelated third party's web
// server, and an attacker-supplied seed list of them turns every recipient's bootstrap into
// unsolicited UDP at somebody's HTTPS endpoint.
//
// One table, three predicates, each stating which question it answers.
func Seed(ap netip.AddrPort) bool {
	return ap.Port() >= MinPort && Routable(ap.Addr())
}

// SharedSpace reports CGNAT space (100.64/10), which is a distinct fact from unroutable —
// the probe reports it to the user as a diagnosis, so it stays separately answerable.
func SharedSpace(a netip.Addr) bool {
	a = a.Unmap()
	return a.Is4() && sharedSpace.Contains(a)
}

// reserved are addresses that are not a usable public endpoint for this host, and
// the list is explicit because Go's own predicates do not cover them.
//
// `netip.Addr.IsGlobalUnicast` excludes only the unspecified address, loopback,
// multicast and link-local (plus 0.0.0.0 and 255.255.255.255 by name). Everything below
// passes it — measured, one by one — so relying on that predicate alone let through
// 0.0.0.0/8, the whole of 240/4, and the 6to4 relay anycast prefix, which is the one on
// this list that reaches a real machine somebody else owns.
//
// The bar for inclusion is "a responder claiming we live here is broken or aiming us",
// not "unusual". Each entry names why, so a future reader can argue with it.
var reserved = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network", RFC 1122
	netip.MustParsePrefix("192.0.2.0/24"),    // documentation, RFC 5737
	netip.MustParsePrefix("198.51.100.0/24"), // documentation
	netip.MustParsePrefix("203.0.113.0/24"),  // documentation
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking, RFC 2544
	netip.MustParsePrefix("192.88.99.0/24"),  // 6to4 relay ANYCAST, RFC 7526 — never a host
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, RFC 1112

	// ::/96 — IPv4-COMPATIBLE IPv6, RFC 4291's deprecated form.
	//
	// Unmap() handles ::ffff:0:0/96 and NOT this, which is one prefix over and undefended.
	// Measured: `::c0a8:101` is 192.168.1.1 and cleared every v4 prefix below; so did
	// `::7f00:1` (127.0.0.1), `::a00:1`, `::e000:1` and `::6440:1`. The doc above claims
	// the family-crossing class is closed, and it was only half closed — found by a review
	// of the very commit that closed the other half.
	netip.MustParsePrefix("::/96"),

	netip.MustParsePrefix("2001:20::/28"),  // ORCHIDv2, RFC 7343 — not a routable host
	netip.MustParsePrefix("2001:db8::/32"), // documentation, RFC 3849
	netip.MustParsePrefix("3fff::/20"),     // documentation, RFC 9637 — the newer half
	// Teredo and 6to4 are refused for BOTH objects, and the two objects want different
	// answers — so the choice is stated rather than inherited. As a self-address, "a
	// responder claiming we live here is broken or aiming us". As a PEER candidate the
	// reasoning inverts: a host genuinely behind Teredo or 6to4 has a real, globally
	// reachable address there, and refusing it removes a working tier-2 path. Both are
	// deprecated and declining, so the cost is small and the uniform rule is worth more
	// than the recovered tier.
	netip.MustParsePrefix("2001::/32"),      // Teredo, RFC 4380
	netip.MustParsePrefix("2002::/16"),      // 6to4, RFC 7526 (deprecated)
	netip.MustParsePrefix("64:ff9b::/96"),   // NAT64, RFC 6052 — a destination, not a source
	netip.MustParsePrefix("64:ff9b:1::/48"), // NAT64 local-use, RFC 8215
	netip.MustParsePrefix("100::/64"),       // discard-only, RFC 6666
	netip.MustParsePrefix("5f00::/16"),      // SRv6 SIDs, RFC 9602
}

var sharedSpace = netip.MustParsePrefix("100.64.0.0/10")
