package server

import (
	"net/netip"
	"testing"
)

// The port-mapping tier's answer is screened before it joins the record, and this is the grill's
// F1/#1 fix: a router legitimately returns a private, CGNAT or low external address (double-NAT,
// carrier-grade NAT — the tier's own caveat 8), and if such an address reached `preimage`'s
// all-or-nothing `addrscope.Target` check it would abort the WHOLE signed record, dropping the
// good DHT-reflexive candidates alongside it. So it is filtered here instead, drop-and-continue.
//
// The public control is not decoration: without it a screen that refused EVERYTHING would pass
// the "unroutable is dropped" rows and silently kill the tier for every user.
func TestAMappedEndpointIsScreenedBeforeItJoinsTheRecord(t *testing.T) {
	const port = 51234
	for _, tc := range []struct {
		name string
		ext  string
		want bool // true = kept (routable public), false = dropped
	}{
		{"ordinary public", "9.9.9.9", true},
		{"CGNAT external (double carrier NAT)", "100.64.0.1", false},
		{"RFC-1918 external (double NAT)", "192.168.9.9", false},
		{"loopback", "127.0.0.1", false},
		{"documentation space", "198.51.100.4", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ext := netip.MustParseAddr(tc.ext)
			ep, ok := screenedMappedEndpoint(ext, port)
			if ok != tc.want {
				t.Fatalf("screenedMappedEndpoint(%s) kept=%v, want %v — a gateway answer that "+
					"addrscope refuses must be dropped here, not fed to Sign where it aborts the "+
					"whole record", tc.ext, ok, tc.want)
			}
			if ok && ep.Addr.Addr() != ext {
				t.Errorf("kept endpoint addr %v, want %v", ep.Addr.Addr(), ext)
			}
		})
	}
}

// A sub-1024 mapped port is refused even for a public IP — a gateway that hands a co-signer an
// external port below the floor is not a port Nib aims traffic at (addrscope.MinPort), and the
// 80/443 exceptions are the only ones the floor admits.
func TestAMappedLowPortIsDropped(t *testing.T) {
	ext := netip.MustParseAddr("9.9.9.9")
	if _, ok := screenedMappedEndpoint(ext, 22); ok {
		t.Error("a mapped external port of 22 was kept — below MinPort, it should be dropped")
	}
	// Control: the same public IP at an ephemeral port is kept, so the drop above is the port.
	if _, ok := screenedMappedEndpoint(ext, 51234); !ok {
		t.Error("setup: the same public IP at a high port was dropped, so the low-port test proves nothing")
	}
}
