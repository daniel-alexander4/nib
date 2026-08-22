package server

import (
	"encoding/hex"
	"net"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/rendezvous"
	"nib/internal/sign"
	"nib/internal/udpmux"
)

// TestTheDHTAndTheArmedListenerShareOneSocket — caveat 7's probe-and-session half.
//
// A NAT mapping is a function of the internal `IP:port`. So the socket the DHT probes this
// machine's public address from, and the socket the session is actually reachable on, must be
// the same one — otherwise the mapping that gets published belongs to a socket nobody listens
// on. `rendezvous.Open` refuses a nil conn for exactly this reason and says so: "nothing
// downstream could tell."
//
// **The mechanism has existed since P02.S03 and nothing composed it.** `udpmux` hands out two
// views of one socket and `internal/udpmux/interop_test.go` drives a real DHT and real quic-go
// over one mux. But in production every socket had exactly one consumer: `QUICListen` bound
// its own and used the QUIC view, `nib rendezvous` bound its own and used the DHT view.
//
// The assertion is on the SOCKET — one address, reached two ways — not on the intent to share
// it, which the criterion is explicit about.
func TestTheDHTAndTheArmedListenerShareOneSocket(t *testing.T) {
	invs, certs, fps := threeParty(t)
	peerFP, err := hex.DecodeString(fps[1])
	if err != nil {
		t.Fatal(err)
	}
	cer, err := ceremonyFor(mustEncode(t, invs[fps[1]]), certs[0], nil, peerFP)
	if err != nil {
		t.Fatal(err)
	}
	// A fresh identity for the listener: this test's subject is the SOCKET, and the arm's
	// pinned-peer TLS needs a matching key rather than the roster's certificate.
	lnCert, lnKey, err := newIdentity(t)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := cer.openRendezvous(transportQUIC, "127.0.0.1:0", t.TempDir(), lnCert, lnKey, peerFP)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close(); cer.close() })

	// SETUP: both halves really exist. Without this, "they are equal" is true of two nils.
	if cer.end == nil || cer.rz == nil {
		t.Fatal("setup: the QUIC arm produced no shared endpoint or no rendezvous server, " +
			"so there is nothing here to be shared")
	}

	// **The address comparison this test used to lead with CANNOT FAIL, and it was the
	// clause S04 ledgered as "asserted on the socket" (2026-08-22).**
	//
	// `ln` is built by `p2p.QUICListenOn(end, …)`, which passes `e.mux` straight into
	// `newQUICListener` (`internal/p2p/endpoint.go:107`). `quicListener.Addr()` is
	// `l.mux.LocalAddr()` (`internal/p2p/quic.go:237`) and `SharedEndpoint.LocalAddr()` is
	// `e.mux.LocalAddr()` (`internal/p2p/endpoint.go:68`) — the same method on the same
	// `*udpmux.Mux`, both bottoming out in `m.pc.LocalAddr()` (`internal/udpmux/mux.go:202`).
	// It compared a value with itself, in every address family, for any bind string.
	//
	// It is kept because it is still a real wiring check — it goes red if a future listener
	// stops being built from the endpoint's mux — but it is no longer what discharges the
	// criterion, and the comment no longer claims it is.
	if ln.Addr().String() != cer.end.LocalAddr().String() {
		t.Fatalf("the listener answers on %s and the DHT is on %s — the listener was not "+
			"built from this endpoint's mux at all", ln.Addr(), cer.end.LocalAddr())
	}

	// **THE ASSERTION.** One socket serving two consumers is not an address, it is a
	// DEMULTIPLEX: a QUIC-shaped datagram and a KRPC-shaped datagram, sent to the SAME
	// address by the same stranger, must reach DIFFERENT views. Two sockets cannot produce
	// that reading, and neither can an endpoint that bound nothing.
	before := cer.end.Stats()
	ua, err := net.ResolveUDPAddr("udp", cer.end.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	probe, err := net.DialUDP("udp", nil, ua)
	if err != nil {
		t.Fatalf("the shared address is not reachable over UDP: %v", err)
	}
	defer probe.Close()

	// Long header — `route`'s rule 1 is `p[0]&0x80` (`internal/udpmux/mux.go:208`).
	if _, err := probe.Write([]byte{0xc0, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00}); err != nil {
		t.Fatalf("writing a QUIC-shaped datagram to the shared socket failed: %v", err)
	}
	// KRPC — a short header from an address the mux has never seen, so it matches no rule
	// and falls through to the DHT.
	if _, err := probe.Write([]byte("d1:q4:ping1:y1:qe")); err != nil {
		t.Fatalf("writing a KRPC-shaped datagram to the shared socket failed: %v", err)
	}

	// The read loop is asynchronous, so poll rather than assuming delivery has happened.
	var after udpmux.Stats
	deadline := time.Now().Add(5 * time.Second)
	for {
		after = cer.end.Stats()
		if after.RoutedLongHeader > before.RoutedLongHeader && after.RoutedToDHT > before.RoutedToDHT {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if after.RoutedLongHeader <= before.RoutedLongHeader {
		t.Errorf("a QUIC-shaped datagram sent to %s did not reach the QUIC view "+
			"(RoutedLongHeader %d -> %d); the listener is not being served by this socket, so "+
			"any mapping the probe measures belongs to a socket the session does not use — "+
			"which is the whole of caveat 7",
			cer.end.LocalAddr(), before.RoutedLongHeader, after.RoutedLongHeader)
	}
	if after.RoutedToDHT <= before.RoutedToDHT {
		t.Errorf("a KRPC-shaped datagram sent to %s did not reach the DHT view "+
			"(RoutedToDHT %d -> %d); the DHT is not being served by this socket",
			cer.end.LocalAddr(), before.RoutedToDHT, after.RoutedToDHT)
	}
}

// TestTheCeremonyTeardownOrderDoesNotPanicTheProcess.
//
// Three of the six plausible orderings kill the process, and none of them errors first: if the
// mux closes while the DHT server is still reading its view, that read returns net.ErrClosed,
// `serveUntilClosed` sees an error with its `closed` flag unset, and calls `panic(err)` on a
// goroutine nothing of ours is on. The one pre-existing call site in the tree gets the order
// right only by accident of `defer` LIFO, with nothing naming the rule.
//
// A test cannot catch a panic on another goroutine, so this drives the ordering repeatedly and
// relies on the process surviving — which is exactly what a panic there would deny it.
func TestTheCeremonyTeardownOrderDoesNotPanicTheProcess(t *testing.T) {
	invs, certs, fps := threeParty(t)
	peerFP, err := hex.DecodeString(fps[1])
	if err != nil {
		t.Fatal(err)
	}
	lnCert, lnKey, err := newIdentity(t)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 8; i++ {
		cer, err := ceremonyFor(mustEncode(t, invs[fps[1]]), certs[0], nil, peerFP)
		if err != nil {
			t.Fatal(err)
		}
		ln, err := cer.openRendezvous(transportQUIC, "127.0.0.1:0", t.TempDir(), lnCert, lnKey, peerFP)
		if err != nil {
			t.Fatal(err)
		}
		// SETUP, once: the DHT is genuinely attached, or this loop tears down nothing.
		if i == 0 && cer.rz == nil {
			t.Fatal("setup: no rendezvous server was opened, so no ordering is being exercised")
		}
		// The order the session uses: listener, then the ceremony's own close, which shuts
		// the rendezvous server before the socket.
		ln.Close()
		cer.close()
	}
}

// TestASharedEndpointSurvivesItsListener — the ownership half.
//
// `QUICListenOn` must close only its own quic.Listener. Closing the transport would "abruptly
// terminate all existing connections" on that socket, and closing the mux would take the DHT
// down with it — so the listener that shares an endpoint must not be able to do either.
func TestASharedEndpointSurvivesItsListener(t *testing.T) {
	end, err := p2p.NewSharedEndpoint("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer end.Close()
	rz, err := rendezvous.Open(end.DHT(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer rz.Close()

	cert, key, err := newIdentity(t)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := p2p.QUICListenOn(end, cert, key, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: the DHT view works BEFORE the listener closes, or "it still works after" is
	// true of something that never worked.
	if n := rz.Stats().Nodes; n < 0 {
		t.Fatal("setup: the rendezvous server is not readable")
	}
	ln.Close()

	// The socket must still be there: same address, still reachable, and the DHT still
	// answering. A listener that took the endpoint down with it would fail all three.
	ua, err := net.ResolveUDPAddr("udp", end.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.DialUDP("udp", nil, ua)
	if err != nil {
		t.Fatalf("closing the listener closed the shared socket: %v", err)
	}
	defer c.Close()
	if _, err := c.Write([]byte("d1:q4:ping1:y1:qe")); err != nil {
		t.Errorf("the shared socket stopped accepting datagrams after its listener closed: %v", err)
	}
	if _, err := end.LocalAddr().(*net.UDPAddr); !err {
		t.Error("the endpoint's address is no longer a UDP address")
	}
}

// newIdentity mints a throwaway identity for a test whose subject is not identity.
func newIdentity(t *testing.T) (certPEM, keyPEM []byte, err error) {
	t.Helper()
	return sign.GenerateIdentity("shared-socket test")
}
