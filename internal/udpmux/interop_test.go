package udpmux

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/quic-go/quic-go"
	"golang.org/x/time/rate"
)

// The driven half of P02.S03's acceptance: interleaved QUIC and KRPC traffic at
// the same port, both arriving intact — with the REAL libraries, not a model of
// them. quic-go and anacrolix/dht are imported by this test file only, so the
// shipped binary is unaffected (go list -deps ./cmd/nib does not reach them).
//
// The topology is the real one and the choice matters. A carries a QUIC session
// with B and DHT traffic with C, on ONE local port. Pointing the DHT traffic at
// B's address instead would exercise the documented address collision and pass
// or fail for the wrong reason — see TestADHTNodeAtAQUICPeerAddressGoesToQUIC,
// which asserts that limitation deliberately rather than tripping over it.

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "udpmux-interop"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"udpmux-interop"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// node is one muxed socket running both protocols.
type node struct {
	mux *Mux
	dht *dht.Server
}

func newNode(t *testing.T) *node {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := New(pc)
	s, err := dht.NewServer(&dht.ServerConfig{
		Conn:          m.DHT(),
		NoSecurity:    true,
		StartingNodes: func() ([]dht.Addr, error) { return nil, nil },
		// Its own limiter. dht.DefaultSendLimiter is a PACKAGE-LEVEL GLOBAL
		// (globals.go:8, rate.NewLimiter(250, 25)) shared by every Server in the
		// process, and it deducts for replies too — so three nodes on loopback
		// exhaust it and the timed-out transaction looks exactly like a routing
		// bug. The limiter is politeness to the real network; on loopback it is
		// only noise in the measurement.
		SendLimiter: rate.NewLimiter(rate.Inf, 0),
	})
	if err != nil {
		m.Close()
		t.Fatal(err)
	}
	n := &node{mux: m, dht: s}
	t.Cleanup(func() { s.Close(); m.Close() })

	// Stimulus assertion for the whole file: the DHT really did take OUR socket.
	// If it had opened its own, every "both protocols at the same port" claim
	// below would be true of two different ports.
	if got, want := s.Addr().String(), m.LocalAddr().String(); got != want {
		t.Fatalf("the DHT is on %s, the muxed socket is %s — they are not the same port", got, want)
	}
	return n
}

func TestQUICAndKRPCInterleavedOnOnePort(t *testing.T) {
	a, b, c := newNode(t), newNode(t), newNode(t)

	cert := selfSigned(t)
	srvTLS := &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"nib-mux"}}
	cliTLS := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"nib-mux"}}

	srvTr := &quic.Transport{Conn: b.mux.QUIC()}
	defer srvTr.Close()
	cliTr := &quic.Transport{Conn: a.mux.QUIC()}
	defer cliTr.Close()

	ln, err := srvTr.Listen(srvTLS, &quic.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// 1 MiB, so the session is hundreds of SHORT-header datagrams — the packets
	// the cheap discriminator cannot tell from a bencode dictionary.
	payload := make([]byte, 1<<20)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The server echoes a digest of everything it received, so the assertion is
	// on the bytes that crossed, not on the connection having opened.
	srvErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept(ctx)
		if err != nil {
			srvErr <- err
			return
		}
		st, err := conn.AcceptStream(ctx)
		if err != nil {
			srvErr <- err
			return
		}
		got, err := io.ReadAll(st)
		if err != nil {
			srvErr <- err
			return
		}
		sum := sha256.Sum256(got)
		if _, err := st.Write(sum[:]); err != nil {
			srvErr <- err
			return
		}
		srvErr <- st.Close()
	}()

	conn, err := cliTr.Dial(ctx, b.mux.LocalAddr(), cliTLS, &quic.Config{})
	if err != nil {
		t.Fatalf("QUIC dial over the muxed socket failed: %v", err)
	}
	defer conn.CloseWithError(0, "")

	st, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// KRPC in BOTH directions while the QUIC transfer is in flight. This is the
	// interleaving: the same local port on A is carrying a QUIC session with B
	// and DHT queries with C at the same time.
	var wg sync.WaitGroup
	results := make(chan pingRun, 2)
	stop := make(chan struct{})
	wg.Add(2)
	go func() { defer wg.Done(); results <- pingLoop(a.dht, c.mux.LocalAddr(), stop) }()
	go func() { defer wg.Done(); results <- pingLoop(c.dht, a.mux.LocalAddr(), stop) }()

	if _, err := st.Write(payload); err != nil {
		t.Fatalf("QUIC write failed while KRPC was flowing on the same port: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	var got [sha256.Size]byte
	if _, err := io.ReadFull(st, got[:]); err != nil {
		t.Fatalf("reading the server's digest failed: %v", err)
	}
	close(stop)
	wg.Wait()

	if got != want {
		t.Fatalf("the QUIC payload did not survive: digest %x, want %x", got, want)
	}
	if err := <-srvErr; err != nil {
		t.Fatalf("server side: %v", err)
	}
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("a KRPC ping on the shared port failed: %v (A: %+v)", r.err, a.mux.Stats())
		}
		if r.ok == 0 {
			t.Fatalf("no KRPC ping completed (%d local send faults) — the DHT half of the "+
				"interleaving never happened, so this test could not have failed", r.localFaults)
		}
		if r.localFaults > 0 {
			t.Logf("%d local send faults tolerated (see pingLoop)", r.localFaults)
		}
	}

	// The stimulus assertion for "interleaved": A's one socket really did carry
	// both. Without this the test would pass if the DHT had gone quiet.
	as := a.mux.Stats()
	if as.RoutedByPeer == 0 {
		t.Fatal("no datagram reached QUIC by the peer rule — the session never used short headers")
	}
	if as.RoutedLongHeader == 0 {
		t.Fatal("no long header was routed — the handshake did not go through the mux")
	}
	if as.RoutedToDHT == 0 {
		t.Fatal("no datagram reached the DHT — the two protocols were not interleaved")
	}
	if as.DroppedQUIC != 0 || as.DroppedDHT != 0 {
		t.Errorf("datagrams were dropped for want of queue depth: quic=%d dht=%d",
			as.DroppedQUIC, as.DroppedDHT)
	}
	t.Logf("A: longheader=%d bypeer=%d dht=%d peers=%d",
		as.RoutedLongHeader, as.RoutedByPeer, as.RoutedToDHT, as.Peers)
}

// TestADHTNodeAtAQUICPeerAddressGoesToQUIC asserts the package's one documented
// limitation, on purpose, so it is a measured property rather than a surprise
// found later: the router keys on peer address, so a KRPC message arriving from
// an address that is an ACTIVE QUIC peer is handed to QUIC and lost.
//
// It is recorded rather than fixed because the failure is bounded and one-sided.
// The rule only ever over-claims for QUIC, so a session is never misrouted to the
// DHT; the cost is that two peers' DHT nodes cannot query each other while they
// have a session open, which the DHT already tolerates as an unresponsive node.
// The exact fix — routing short headers on the destination connection ID, via a
// quic.Transport.ConnectionIDGenerator that registers what it issues — is filed,
// not built: it would make a mis-wired generator break the SESSION rather than a
// DHT query, which is the worse failure of the two.
func TestADHTNodeAtAQUICPeerAddressGoesToQUIC(t *testing.T) {
	m, peer := newTestMux(t)

	// Stimulus: before the address is a QUIC peer, KRPC reaches the DHT.
	if _, err := peer.WriteTo(krpcPing, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if !arrives(t, m.DHT(), krpcPing) {
		t.Fatal("KRPC does not reach the DHT even before the collision; the test proves nothing")
	}

	if _, err := m.QUIC().WriteTo([]byte("x"), peer.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if _, err := peer.WriteTo(krpcPing, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if !arrives(t, m.QUIC(), krpcPing) {
		t.Fatal("the documented collision no longer happens — if the router was made exact, " +
			"delete this test and the limitation in the package doc")
	}
}

// TestQUICTransportCloseReturnsOverTheShim pins the library fact the shim's
// deadline exists for. Transport.Close on a user-supplied conn does NOT close it
// (transport.go:460): it unblocks its own read loop by setting a read deadline,
// waits for the loop to return, then resets the deadline. A shim whose deadline
// were a no-op — or whose deadline error were not Temporary() — would make this
// call hang forever, and it would hang in a defer at the end of a passing test,
// which is the worst place to learn it.
func TestQUICTransportCloseReturnsOverTheShim(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := New(pc)
	defer m.Close()

	tr := &quic.Transport{Conn: m.QUIC()}
	ln, err := tr.Listen(&tls.Config{
		Certificates: []tls.Certificate{selfSigned(t)},
		NextProtos:   []string{"nib-mux"},
	}, &quic.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: the transport is really listening on the shim, so its read loop
	// is really blocked in our ReadFrom. Without this the close below could
	// return promptly for want of anything to unblock.
	if got, want := ln.Addr().String(), m.LocalAddr().String(); got != want {
		t.Fatalf("the listener is on %s, the muxed socket is %s", got, want)
	}

	done := make(chan error, 1)
	go func() { done <- tr.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Transport.Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("quic.Transport.Close hung on the shim — its read loop was never unblocked")
	}

	// And it left the shared socket alone, which is the other half of the point.
	if _, err := m.DHT().WriteTo(krpcPing, m.LocalAddr()); err != nil {
		t.Fatalf("the shared socket died with the transport: %v", err)
	}
}

// pingRun is what one direction of KRPC traffic did while the QUIC transfer ran.
type pingRun struct {
	ok          int
	localFaults int
	err         error
}

// pingLoop queries one DHT server from another until stopped.
//
// It distinguishes a LOCAL send failure from a routing outcome, and the distinction
// is what keeps the test both honest and stable. A misrouted KRPC message is
// delivered to the wrong consumer and the query TIMES OUT — that is the failure this
// test exists to catch, and it is fatal here. A write that never left the host is a
// different animal: on this machine ufw periodically rebuilds its iptables chains
// (audit records of "iptables -N ufw-caps-test..."), and a locally-generated packet
// crossing that rebuild is dropped with sendto: EPERM. Grading the mux on that would
// make the suite fail for the firewall's reasons.
//
// Tolerated, not ignored: the count is reported, and a run where NOTHING succeeded
// fails regardless — otherwise a totally blocked socket would read as a pass.
func pingLoop(from *dht.Server, to net.Addr, stop <-chan struct{}) pingRun {
	var r pingRun
	for {
		select {
		case <-stop:
			return r
		default:
		}
		res := from.Ping(to.(*net.UDPAddr))
		switch {
		case res.Err == nil:
			r.ok++
		case isLocalSendFault(res.Err):
			r.localFaults++
		default:
			r.err = res.Err
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// isLocalSendFault reports whether an error is the local socket refusing to send,
// as opposed to anything that happened to a datagram after it left.
//
// It matches on the library's own wrapper text rather than on the error value,
// because it has to: anacrolix/dht formats the underlying *net.OpError with %s and
// not %w (server.go:831), so the typed error is flattened to a string before it ever
// reaches a caller and errors.As cannot see it. The prefix is exact for the purpose
// — dht emits it only when WriteTo on the conn failed, which by construction is a
// datagram that never left this host. Every routing failure this test exists to
// catch surfaces as a transaction timeout instead, and does not match.
func isLocalSendFault(err error) bool {
	if err == nil {
		return false
	}
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "write" {
		return true
	}
	return strings.Contains(err.Error(), "error writing ")
}

// cidGen mirrors what internal/p2p installs on its transports: it registers every
// connection id this endpoint issues with the mux, so short headers route on the
// DESTINATION connection id rather than on the sender's address.
type cidGen struct {
	m *Mux
	n int
}

func (g *cidGen) ConnectionIDLen() int { return g.n }

func (g *cidGen) GenerateConnectionID() (quic.ConnectionID, error) {
	b := make([]byte, g.n)
	if _, err := rand.Read(b); err != nil {
		return quic.ConnectionID{}, err
	}
	g.m.RegisterConnectionID(b)
	return quic.ConnectionIDFromBytes(b), nil
}

// TestKRPCFromAQUICPeerReachesTheDHT is the collision this package documented and left,
// now closed — and it is here because this is where the rule lives.
//
// The address rule over-claims for QUIC: a datagram from an address we have sent QUIC
// to goes to the QUIC view, so a KRPC reply from a host we ALSO hold a session with is
// swallowed. P02.S03 recorded that as a bounded, one-sided cost. P04.S01's first
// driven test hit it in the shape that matters — a rendezvous ping to a peer we are
// mid-session with — and measured it: with no session, the ping succeeds; with one, it
// times out and the mux reports RoutedByPeer.
//
// The fix routes on the connection id, which is exact. This drives BOTH halves, and
// the pair is the point: the same ping, to the same host, with and without a QUIC
// session in flight.
func TestKRPCFromAQUICPeerReachesTheDHT(t *testing.T) {
	a, b := newNode(t), newNode(t)

	// The generator on BOTH ends, which is what the product does (QUICDial and
	// QUICListen each install one) and is not optional.
	//
	// Wiring only A was the first attempt and it still failed — because B's mux
	// learns A as a "QUIC peer" from B's own handshake WRITES, so B swallowed A's
	// ping by the same address rule, at the other end of the same exchange. The
	// collision is symmetric; a fix on one side is not a fix.
	tr := &quic.Transport{Conn: a.mux.QUIC(), ConnectionIDGenerator: &cidGen{m: a.mux, n: 8}}
	defer tr.Close()
	srvTr := &quic.Transport{Conn: b.mux.QUIC(), ConnectionIDGenerator: &cidGen{m: b.mux, n: 8}}
	defer srvTr.Close()

	ln, err := srvTr.Listen(&tls.Config{
		Certificates: []tls.Certificate{selfSigned(t)}, NextProtos: []string{"nib-cid"},
	}, &quic.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go func() { ln.Accept(ctx) }()

	// Stimulus: with no QUIC session at all, A can reach B over KRPC. Without this,
	// "it worked with a session" is not distinguishable from a link that always works
	// — and more importantly, a link that never does.
	if res := a.dht.Ping(b.mux.LocalAddr().(*net.UDPAddr)); res.Err != nil {
		t.Fatalf("with NO QUIC session, the ping already fails (%v) — nothing below is "+
			"about the collision", res.Err)
	}

	// Now a real QUIC session A->B, which is what taught the old rule that B is a
	// "QUIC peer" and made it swallow B's KRPC replies.
	conn, err := tr.Dial(ctx, b.mux.LocalAddr(), &tls.Config{
		InsecureSkipVerify: true, NextProtos: []string{"nib-cid"},
	}, &quic.Config{})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseWithError(0, "")

	// Traffic AFTER the handshake, because that is when short headers start — the
	// handshake itself is all long headers, and Dial returns the moment it completes.
	// The first version asserted here and read RoutedByCID:0 with RoutedLongHeader:2,
	// which is the rule not yet having anything to decide rather than the rule failing.
	st, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write(make([]byte, 4096)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && a.mux.Stats().RoutedByCID == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	before := a.mux.Stats()
	if before.RoutedByCID == 0 {
		t.Fatalf("no datagram routed by connection id after a completed handshake and a "+
			"stream write (%+v) — the generator is not wired, so the rule under test "+
			"never ran", before)
	}

	if res := a.dht.Ping(b.mux.LocalAddr().(*net.UDPAddr)); res.Err != nil {
		t.Fatalf("the KRPC ping to a host we hold a QUIC session with failed: %v "+
			"(mux %+v) — this is the collision the connection-id rule exists to remove",
			res.Err, a.mux.Stats())
	}

	// And the address rule must NOT have been what saved it: once ids are registered,
	// a short header that is not one of ours goes to the DHT, full stop.
	if st := a.mux.Stats(); st.RoutedByPeer != before.RoutedByPeer {
		t.Errorf("RoutedByPeer advanced %d -> %d while connection ids were registered — the "+
			"address rule is still deciding, and it is the rule with the collision",
			before.RoutedByPeer, st.RoutedByPeer)
	}
	t.Logf("CIDROUTE byCID=%d byPeer=%d toDHT=%d",
		a.mux.Stats().RoutedByCID, a.mux.Stats().RoutedByPeer, a.mux.Stats().RoutedToDHT)
}
