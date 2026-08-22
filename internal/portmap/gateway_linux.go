//go:build linux

package portmap

import (
	"encoding/hex"
	"io"
	"net/netip"
	"os"
	"strings"
)

// defaultGateway reads /proc/net/route, the kernel's IPv4 routing table in a stable text
// format. The default route is the row whose Destination is 00000000; its Gateway column is
// the next-hop IPv4, in the machine's native byte order as hex (little-endian on every
// platform Nib targets).
//
// IPv4 only, deliberately and stated: NAT-PMP and PCP are IPv4-NAT traversal, and a host with
// a global IPv6 address does not need a mapping — S05's tier 2 dials it directly. So the v6
// routing table (/proc/net/ipv6_route) is not consulted, and that is correct rather than a gap.
func defaultGateway() (netip.Addr, error) {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return netip.Addr{}, ErrNoGateway
	}
	defer f.Close()
	return parseProcNetRoute(f)
}

// parseProcNetRoute is the pure core, split out so it tests against fixture bytes rather than
// the host's live routing table — the host's real default route is not a fixture a test can
// pin, and a test that read it would assert the machine's network, not the parser.
func parseProcNetRoute(r io.Reader) (netip.Addr, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return netip.Addr{}, ErrNoGateway
	}
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // header, or a trailing blank
		}
		f := strings.Fields(line)
		// Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT
		if len(f) < 3 {
			continue
		}
		if f[1] != "00000000" {
			continue // not the default route
		}
		gwHex := f[2]
		if gwHex == "00000000" {
			continue // a default route with no gateway (e.g. a point-to-point link)
		}
		raw, err := hex.DecodeString(gwHex)
		if err != nil || len(raw) != 4 {
			continue
		}
		// The kernel writes the address in native (little-endian) byte order, so the four
		// hex bytes are the IP reversed: 0102A8C0 → bytes [01 02 A8 C0] → 192.168.2.1.
		return netip.AddrFrom4([4]byte{raw[3], raw[2], raw[1], raw[0]}), nil
	}
	return netip.Addr{}, ErrNoGateway
}
