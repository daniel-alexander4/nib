package discovery

import (
	"net"
	"strings"
)

// chooseInterfaces picks the interfaces a link-local announcement should be sent on
// and joined on.
//
// It is a pure function over the interface list on purpose. The alternative — calling
// net.Interfaces() inside the socket constructor — means the selection can only be
// tested on a machine that happens to have the interesting cases on it, and the
// interesting cases are a docker bridge with nothing attached, a VPN tun, and an
// interface whose IPv4 lease has not arrived yet. A table test states them all.
//
// # What is skipped, and why each one
//
//   - **Down interfaces.** Nothing to say to.
//
//   - **Loopback.** Joining it buys nothing: IP_MULTICAST_LOOP already delivers a
//     copy of every announcement to other processes on this machine, which is what
//     makes same-host discovery work. Joining lo as well would duplicate every
//     datagram. (Measured: lo carries multicast memberships but does not even set
//     FlagMulticast, so a naive flag filter skips it by accident rather than on
//     purpose. Doing it on purpose is the difference.)
//
//   - **Point-to-point.** A tun/ppp/WireGuard link has no L2 broadcast domain, so a
//     link-local join is either a no-op or it leaks "who is signing on my LAN" into
//     a VPN, which is the wrong trust boundary for a protocol whose whole premise is
//     the people in the room. Note the flag does not catch every VPN — an OpenVPN
//     *tap* adapter presents as ethernet — so this narrows the exposure rather than
//     closing it, and that limit is worth knowing rather than assuming.
//
//   - **Interfaces with no address.** Nothing to send from.
//
//   - **Interfaces with no IPv4 address, for the IPv4 group only.** Not because
//     Linux needs one — measured, it does not — but because WINDOWS does: its
//     IPv4 join resolves the interface to an *address* (setIPv4MreqToInterface walks
//     ifi.Addrs() and fails without an IPv4 one) where Linux uses the *index*. An
//     interface with IPv6 up and DHCP still pending is joinable on Linux and refused
//     on Windows, which is a divergence no Linux test would ever show. Requiring the
//     address on both platforms makes the chosen set identical on both, which is
//     worth more than the one interface it costs.
//
// # What is deliberately NOT used
//
// FlagRunning is the carrier signal that correctly skips an idle docker bridge on
// Linux — and on Windows it is set from the same condition as FlagUp
// (interface_windows.go sets both inside one `if`), so it carries no information
// there. Selecting on it would silently mean two different things per platform. It is
// therefore reported by describe() as a diagnostic and never used as a filter.
//
// FlagMulticast is not required either: the kernel does not enforce it (measured — a
// join on an interface without it succeeds), and on Windows it is derived from a
// media-type whitelist with a TODO in the source, so a working adapter can lack it.
// flagsAllow is the FLAG half of the decision, split out so it can be tested at all.
//
// The first version of this file made the flag decision inline, and its table test built
// synthetic net.Interface values to exercise it. That does not work and passed by luck:
// Addrs() queries the KERNEL by index, so a fixture with Index: 3 reports the addresses
// of the host's interface 3. On the development machine those happened to be non-empty,
// so the docker0 row survived the address filter; on a CI container with only lo and
// eth0 it would be dropped and the test would fail claiming "FlagRunning became a
// filter", which is the wrong diagnosis entirely.
//
// A decision that queries the kernel cannot be table-tested. So the decision and the
// query are separate, and this half is pure.
func flagsAllow(f net.Flags) bool {
	switch {
	case f&net.FlagUp == 0:
		return false
	case f&net.FlagLoopback != 0:
		return false
	case f&net.FlagPointToPoint != 0:
		return false
	}
	return true
}

func chooseInterfaces(all []net.Interface, wantV4 bool) []net.Interface {
	var out []net.Interface
	for _, ifi := range all {
		if !flagsAllow(ifi.Flags) {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}
		if wantV4 && !hasV4(addrs) {
			continue
		}
		out = append(out, ifi)
	}
	return out
}

// HasIPv4 is exported for the diagnostic, which prints it per interface — on Windows an
// IPv4 group join resolves the interface to an ADDRESS, so this is the difference between
// an interface that will join there and one that will not.
func HasIPv4(addrs []net.Addr) bool { return hasV4(addrs) }

func hasV4(addrs []net.Addr) bool {
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil && ip.To4() != nil {
			return true
		}
	}
	return false
}

// describe renders the selection for a diagnostic. Discovery fails silently by
// nature — a firewall, a wrong interface, a VPN swallowing the group — so the one
// thing that must never be missing is a plain statement of what was tried.
func describe(all []net.Interface, chosen []net.Interface) string {
	picked := map[string]bool{}
	for _, ifi := range chosen {
		picked[ifi.Name] = true
	}
	var b strings.Builder
	for _, ifi := range all {
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString(ifi.Name)
		if picked[ifi.Name] {
			b.WriteString(" JOINED")
		} else {
			b.WriteString(" skipped")
		}
		b.WriteString(" (")
		b.WriteString(ifi.Flags.String())
		b.WriteString(")")
	}
	if b.Len() == 0 {
		return "no interfaces at all"
	}
	return b.String()
}
