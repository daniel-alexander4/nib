//go:build linux

package portmap

import (
	"strings"
	"testing"
)

// A real /proc/net/route sample. The default route (Destination 00000000) has gateway
// 0102A8C0, which is little-endian for C0.A8.02.01 = 192.168.2.1. A second row is an on-link
// route with gateway 00000000 (no next hop) that must be skipped, not read as 0.0.0.0.
const procNetRoute = `Iface	Destination	Gateway	Flags	RefCnt	Use	Metric	Mask	MTU	Window	IRTT
eth0	00000000	0102A8C0	0003	0	0	100	00000000	0	0	0
eth0	0002A8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
`

func TestParseProcNetRouteFindsTheDefaultGatewayLittleEndian(t *testing.T) {
	gw, err := parseProcNetRoute(strings.NewReader(procNetRoute))
	if err != nil {
		t.Fatal(err)
	}
	if gw.String() != "192.168.2.1" {
		t.Errorf("gateway %v, want 192.168.2.1 — the hex is little-endian (0102A8C0 → C0.A8.02.01), and reading it big-endian gives 1.2.168.192", gw)
	}
}

// A table with a default route but NO gateway (a point-to-point link) must miss, not return
// 0.0.0.0 — a mapping request to 0.0.0.0 is the void, and it would look like a gateway that
// never answered rather than the "no gateway here" it is.
func TestADefaultRouteWithNoGatewayIsAMiss(t *testing.T) {
	const noGW = `Iface	Destination	Gateway	Flags	RefCnt	Use	Metric	Mask	MTU	Window	IRTT
tun0	00000000	00000000	0001	0	0	0	00000000	0	0	0
`
	if _, err := parseProcNetRoute(strings.NewReader(noGW)); err != ErrNoGateway {
		t.Errorf("a gateway-less default route did not miss: %v", err)
	}
}

// No default route at all is a miss.
func TestNoDefaultRouteIsAMiss(t *testing.T) {
	const onLinkOnly = `Iface	Destination	Gateway	Flags	RefCnt	Use	Metric	Mask	MTU	Window	IRTT
eth0	0002A8C0	00000000	0001	0	0	0	00FFFFFF	0	0	0
`
	if _, err := parseProcNetRoute(strings.NewReader(onLinkOnly)); err != ErrNoGateway {
		t.Errorf("a table with no default route did not miss: %v", err)
	}
}
