package p2p

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/quic-go/quic-go"
)

// P02.S02's selection spike, and it is a spike rather than a unit test on purpose: the
// acceptance says the libraries must be proven against the socket-sharing constraint
// **by binding a socket and handing it in, not by reading the documentation**.
//
// Three things are being decided here, and the third is the one that disqualifies:
//
//  1. quic-go accepts an externally-supplied net.PacketConn (caveat 7, plan-review C3);
//  2. VerifyPeerCertificate is invoked exactly as crypto/tls does it, with
//     InsecureSkipVerify set and client certs required (caveat 1) — the entire pinned-peer
//     model rides on this, and if it is false D7 needs rework rather than adjustment;
//  3. **the two libraries work as a PAIR.** Each accepting an external conn is not enough:
//     separating them on one port needs a passthrough hook that one of them must expose.
//     A pair with no hook is disqualified even though each half passes (1).
//
// The cheap discriminator is refuted by arithmetic and is not attempted: every KRPC
// message is a bencode dictionary, so its first byte is 'd' = 0x64 — header-form bit
// clear, fixed bit set, which is exactly a QUIC short header. The collision is on the
// steady state of every established connection, not on an edge.

// TestSpikeQUICAcceptsAnExternalPacketConn — constraint (1), and caveat 1 with it.
func TestSpikeQUICAcceptsAnExternalPacketConn(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	// The socket is bound HERE and handed in — the whole point of the constraint. A
	// library that opens its own socket cannot share a port with the DHT and the
	// port-mapping probe, and a NAT mapping is a function of the internal IP:port.
	srvConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer srvConn.Close()
	cliConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer cliConn.Close()

	srvTr := &quic.Transport{Conn: srvConn}
	defer srvTr.Close()
	cliTr := &quic.Transport{Conn: cliConn}
	defer cliTr.Close()

	// SessionTLS unchanged — the phase's goal says it is reused as-is.
	srvTLS, err := SessionTLS(bCert, bKey, aFP, true)
	if err != nil {
		t.Fatal(err)
	}
	cliTLS, err := SessionTLS(aCert, aKey, bFP, false)
	if err != nil {
		t.Fatal(err)
	}

	// Caveat 1: does the QUIC stack invoke VerifyPeerCertificate the way crypto/tls does?
	// Recorded by wrapping the callback SessionTLS installed, so what is observed is the
	// real one rather than a stand-in.
	var mu sync.Mutex
	serverSawVerify, clientSawVerify := false, false

	srvTLS.NextProtos = []string{"nib-spike"}
	cliTLS.NextProtos = []string{"nib-spike"}

	srvVerify := srvTLS.VerifyPeerCertificate
	if srvVerify == nil {
		t.Fatal("setup: SessionTLS installed no VerifyPeerCertificate, so caveat 1 has nothing to observe")
	}
	srvTLS.VerifyPeerCertificate = func(raw [][]byte, chains [][]*x509.Certificate) error {
		mu.Lock()
		serverSawVerify = true
		mu.Unlock()
		return srvVerify(raw, chains)
	}
	cliVerify := cliTLS.VerifyPeerCertificate
	cliTLS.VerifyPeerCertificate = func(raw [][]byte, chains [][]*x509.Certificate) error {
		mu.Lock()
		clientSawVerify = true
		mu.Unlock()
		return cliVerify(raw, chains)
	}

	ln, err := srvTr.Listen(srvTLS, &quic.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	accepted := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c, err := ln.Accept(ctx)
		if err != nil {
			accepted <- err
			return
		}
		defer c.CloseWithError(0, "")
		st, err := c.AcceptStream(ctx)
		if err != nil {
			accepted <- err
			return
		}
		buf := make([]byte, 5)
		if _, err := st.Read(buf); err != nil {
			accepted <- err
			return
		}
		if string(buf) != "hello" {
			accepted <- errors.New("wrong payload: " + string(buf))
			return
		}
		accepted <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := cliTr.Dial(ctx, srvConn.LocalAddr(), cliTLS, &quic.Config{})
	if err != nil {
		t.Fatalf("QUIC dial over an externally-supplied PacketConn failed: %v", err)
	}
	defer conn.CloseWithError(0, "")
	st, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := <-accepted; err != nil {
		t.Fatalf("the QUIC session did not complete over the handed-in socket: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !clientSawVerify {
		t.Error("the client's VerifyPeerCertificate never fired under QUIC — the pinned-peer " +
			"model rides entirely on it, and caveat 1 is FALSE, which means D7 needs rework " +
			"rather than adjustment")
	}
	if !serverSawVerify {
		t.Error("the server's VerifyPeerCertificate never fired under QUIC — a listener would " +
			"accept any client, and RequireAnyClientCert is not being honoured")
	}
}

// TestSpikeTheLibrariesShareOneSocket — constraint (3), the disqualifying one, and the
// finding that decided this slice.
//
// **quic-go's own non-QUIC passthrough is built on the discriminator the Stage-2 grill
// refuted, so it cannot carry KRPC.** Read at the line
// (`internal/wire/header.go`): `IsPotentialQUICPacket(b) = b&0x40 > 0`. A bencode
// dictionary begins with `'d'` = 0x64 = 0110 0100 — the 0x40 fixed bit is SET — so quic-go
// classifies every KRPC message as a QUIC packet and `ReadNonQUICPacket` never sees one.
// Measured before it was read: a hand-sent KRPC ping vanished, with and without a DHT
// attached.
//
// The grill predicted the arithmetic and the plan recorded it. What the spike adds is one
// level further: **the library's hook does not rescue us from it**, because the hook uses
// the same test. `ReadNonQUICPacket` works for STUN (0x00/0x01 — fixed bit clear) and not
// for this.
//
// So the shape is settled the other way round: **the demultiplexer is ours and owns the
// socket**, and both libraries get shims — which is possible precisely because both accept
// an externally-supplied net.PacketConn. That is what makes this pair selectable; the
// passthrough hook turns out to be irrelevant rather than decisive.
//
// This test asserts the LIMIT rather than working around it, so nobody re-derives the hook
// as a solution. P02.S03 builds the real demultiplexer.
func TestSpikeQUICPassthroughCannotCarryBencode(t *testing.T) {
	sock, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sock.Close()
	tr := &quic.Transport{Conn: sock}
	defer tr.Close()

	delivered := make(chan int, 1)
	go func() {
		b := make([]byte, 512)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		n, _, err := tr.ReadNonQUICPacket(ctx, b)
		if err != nil {
			delivered <- -1
			return
		}
		delivered <- n
	}()
	time.Sleep(300 * time.Millisecond) // let the reader register

	peer, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	// A real KRPC ping — the colliding case, not a contrived byte.
	krpc := []byte("d1:ad2:id20:abcdefghij0123456789e1:q4:ping1:t2:aa1:y1:qe")
	if krpc[0]&0x40 == 0 {
		t.Fatal("setup: the probe's first byte does not have the QUIC fixed bit set, so it is " +
			"not the colliding case this test is about")
	}
	if _, err := peer.WriteTo(krpc, sock.LocalAddr()); err != nil {
		t.Fatal(err)
	}

	if n := <-delivered; n > 0 {
		t.Errorf("quic-go's passthrough delivered %d bytes of a bencode message. If that has "+
			"become true, its classifier no longer keys on the fixed bit alone — re-read "+
			"IsPotentialQUICPacket, because P02.S03 builds a demultiplexer on the assumption "+
			"that it does not.", n)
	}

	// The control, and it is what stops this being "the passthrough is simply broken":
	// a datagram with the fixed bit CLEAR — a STUN binding request's shape — does arrive.
	go func() {
		b := make([]byte, 512)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		n, _, err := tr.ReadNonQUICPacket(ctx, b)
		if err != nil {
			delivered <- -1
			return
		}
		delivered <- n
	}()
	time.Sleep(300 * time.Millisecond)
	stun := []byte{0x00, 0x01, 0x00, 0x00, 0x21, 0x12, 0xA4, 0x42}
	if stun[0]&0x40 != 0 {
		t.Fatal("setup: the control's fixed bit is set, so it is the same case as above")
	}
	if _, err := peer.WriteTo(stun, sock.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if n := <-delivered; n <= 0 {
		t.Error("the passthrough delivered nothing for a fixed-bit-CLEAR datagram either, so " +
			"the finding above is 'the hook does not work at all' rather than 'the hook cannot " +
			"classify bencode' — and those point at different solutions")
	}
}

// TestSpikeTheDHTTakesAnExternalPacketConn — the other half of the pair, proven by the
// compiler and by the server coming up on a conn it was handed.
//
// It is given a plain UDP socket here rather than a shim: the shim is P02.S03's, and what
// this slice owes is that the library will accept one at all. A library that opened its own
// socket could not share a port with QUIC and the mapping probe, and a NAT mapping is a
// function of the internal IP:port — that is the constraint, and it is satisfied.
func TestSpikeTheDHTTakesAnExternalPacketConn(t *testing.T) {
	sock, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sock.Close()

	s, err := dht.NewServer(&dht.ServerConfig{
		Conn:          sock,
		NoSecurity:    true,
		StartingNodes: func() ([]dht.Addr, error) { return nil, nil },
	})
	if err != nil {
		t.Fatalf("the DHT refused an externally-supplied PacketConn: %v — this half of the "+
			"pair is disqualified", err)
	}
	defer s.Close()

	if got := s.Addr().String(); got != sock.LocalAddr().String() {
		t.Errorf("the DHT is on %s and the socket we bound is %s — it did not take ours",
			got, sock.LocalAddr())
	}

	// It answers on that socket: a ping from a separate socket comes back. Without this the
	// test proves only that NewServer returned without error.
	peer, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if _, err := peer.WriteTo([]byte("d1:ad2:id20:abcdefghij0123456789e1:q4:ping1:t2:aa1:y1:qe"),
		sock.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	_ = peer.SetReadDeadline(time.Now().Add(10 * time.Second))
	buf := make([]byte, 2048)
	n, _, err := peer.ReadFrom(buf)
	if err != nil {
		t.Fatalf("the DHT did not answer on the socket it was handed: %v", err)
	}
	if n == 0 || buf[0] != 'd' {
		t.Errorf("the reply is not a bencode dictionary (%d bytes, first byte %q)", n, buf[0])
	}
}
