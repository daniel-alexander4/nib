package server

import (
	"net"
	"strconv"
	"testing"

	"nib/internal/discovery"
	"nib/internal/p2p"
	"nib/internal/pairing"
	"nib/internal/sign"
	"nib/internal/vault"
)

// TestAQUICArmedPeerIsDialledOverQUIC is ADR-010's seam, end to end and on real sockets.
//
// The announcement used to carry a port and no transport, and the dialing side picked its
// transport from its OWN request — so arming with `{"transport":"quic"}` and no bind, and
// initiating with no address and no transport, sent a TCP dial at a UDP port.
//
// **It stayed latent for two reasons worth naming, because both are shapes that hide other
// defects too.** The web client never sends a transport at all, so the GUI was TCP on both
// sides and the two halves agreed by never disagreeing; and `build/pairrepro.sh` passes
// `-F transport=` to BOTH sides out of band, so tier 4 — the harness whose entire purpose
// is two real binaries completing a ceremony — was configured past the bug it would
// otherwise have been the thing to find.
//
// The test walks the real path: a real QUIC listener, the announcement `startAnnouncing`
// would build from it, really ENCODED and really PARSED, resolved against a real pin, and
// dialled. The second half is the red proof, kept in the test rather than run once by hand:
// the same address with the transport the OLD code would have chosen does not connect.
func TestAQUICArmedPeerIsDialledOverQUIC(t *testing.T) {
	certA, keyA, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	certB, keyB, err := sign.GenerateIdentity("Bob")
	if err != nil {
		t.Fatal(err)
	}
	fpA, err := sign.Fingerprint(certA)
	if err != nil {
		t.Fatal(err)
	}
	fpB, err := sign.Fingerprint(certB)
	if err != nil {
		t.Fatal(err)
	}

	// Bob arms a QUIC session. This is the fact the announcement has to carry.
	ln, err := p2p.QUICListen("127.0.0.1:0", certB, keyB, fpA)
	if err != nil {
		t.Skipf("no QUIC listener available here: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	// STIMULUS: the listener really is QUIC, and it really says so. Both halves —
	// a listener that reported "tcp" would make the rest of this test assert nothing.
	if ln.Transport() != p2p.TransportQUIC {
		t.Fatalf("setup: a QUIC listener reports transport %q", ln.Transport())
	}
	if got := announcedTransport(ln.Transport()); got != discovery.TransportQUIC {
		t.Fatalf("setup: a QUIC listener announces as %v", got)
	}

	name, err := pairing.Name(fpB)
	if err != nil {
		t.Fatal(err)
	}
	port := portOf(ln)
	if port <= 0 {
		t.Fatalf("setup: the QUIC listener reports port %d", port)
	}

	// Through the wire, not around it: encoded and parsed, so the transport is being
	// read out of bytes rather than out of the struct that produced them.
	wire, err := discovery.Announcement{
		Name:      name,
		Port:      uint16(port),
		Transport: announcedTransport(ln.Transport()),
		Nonce:     [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
	}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := discovery.Parse(wire)
	if err != nil {
		t.Fatalf("the announcement does not survive its own parser: %v", err)
	}

	pins := []vault.PinnedPeer{{Fingerprint: fpB, Label: "Bob"}}
	c, ok := resolve(pins, discovery.Seen{
		Announcement: parsed,
		From:         &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: discovery.Port},
	})
	if !ok {
		t.Fatal("setup: the announcement did not resolve against Bob's pin")
	}
	if c.Transport != p2p.TransportQUIC {
		t.Fatalf("the candidate's transport is %q; a QUIC-armed peer resolved to a TCP dial "+
			"at a UDP port, which is a connection refused reported as an unreachable peer",
			c.Transport)
	}
	if want := net.JoinHostPort("127.0.0.1", strconv.Itoa(port)); c.Addr != want {
		t.Fatalf("candidate address is %q, want %q", c.Addr, want)
	}

	conn, err := dialAny([]candidate{c}, certA, keyA, fpB)
	if err != nil {
		t.Fatalf("the announced transport did not reach the peer: %v", err)
	}
	conn.Close()

	// THE RED PROOF, in the test. This is exactly what the code did before ADR-010:
	// the same discovered address, dialled with the transport the caller's own request
	// named. If this ever succeeds, the assertion above has stopped meaning anything —
	// either because the transports have become interchangeable or because dialAny has
	// stopped reading the candidate's.
	if bad, err := dialAny([]candidate{{Addr: c.Addr, Transport: p2p.TransportTCP}},
		certA, keyA, fpB); err == nil {
		bad.Close()
		t.Error("a TCP dial reached a QUIC-armed peer at the same port, so this test cannot " +
			"tell the announced transport from the requested one")
	}
}

// TestTheTransportNamesHaveOneDefinition.
//
// `internal/server` used to define its own "tcp"/"quic" copies beside internal/p2p's.
// Three vocabularies now meet: the request field, the listener's report, and the
// announcement's wire byte. The first two must be one value; the third is deliberately
// separate, because internal/discovery may not import internal/p2p (L1's structural
// guard) — so the join is a mapping and the mapping is asserted in both directions.
func TestTheTransportNamesHaveOneDefinition(t *testing.T) {
	if transportTCP != p2p.TransportTCP || transportQUIC != p2p.TransportQUIC {
		t.Errorf("server names (%q, %q) are not p2p's (%q, %q); the string that selects a "+
			"dialer and the string a listener reports have drifted",
			transportTCP, transportQUIC, p2p.TransportTCP, p2p.TransportQUIC)
	}
	for _, tc := range []struct {
		name string
		wire discovery.Transport
	}{
		{p2p.TransportTCP, discovery.TransportTCP},
		{p2p.TransportQUIC, discovery.TransportQUIC},
	} {
		if got := announcedTransport(tc.name); got != tc.wire {
			t.Errorf("%q announces as %v, want %v", tc.name, got, tc.wire)
		}
		if got := transportOf(tc.wire); got != tc.name {
			t.Errorf("%v resolves to %q, want %q", tc.wire, got, tc.name)
		}
	}
	// STIMULUS: the two transports must not map to the same thing in either
	// direction, or every assertion above is satisfied by a constant function.
	if announcedTransport(p2p.TransportTCP) == announcedTransport(p2p.TransportQUIC) {
		t.Fatal("both transports announce as the same wire byte")
	}
	if transportOf(discovery.TransportTCP) == transportOf(discovery.TransportQUIC) {
		t.Fatal("both wire bytes resolve to the same transport name")
	}
}
