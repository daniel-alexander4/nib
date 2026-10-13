package p2p

import (
	"context"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go"

	"nib/internal/udpmux"
)

// TestAnUnvalidatedSourcePastTheBudgetMustProveItsAddress — ADR-160, the rule as a function. Inside the budget
// a source is given its handshake; past it the check demands a Retry; and the allowance comes back with time
// and not with anything a sender does.
func TestAnUnvalidatedSourcePastTheBudgetMustProveItsAddress(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	check := sourceCheck(unvalidatedBurst, unvalidatedPerSecond, func() time.Time { return now })
	a := &net.UDPAddr{IP: net.IPv4(203, 0, 113, 7), Port: 4000}
	for i := 0; i < unvalidatedBurst; i++ {
		if check(a) {
			t.Fatalf("Initial %d of %d inside the budget was sent a Retry — an honest racer's "+
				"candidates would each pay a round trip", i+1, unvalidatedBurst)
		}
	}
	// Past it: a Retry, whatever address the datagram claims — the address is the thing not known.
	for i := 0; i < 50; i++ {
		other := &net.UDPAddr{IP: net.IPv4(198, 51, 100, byte(i)), Port: 5000 + i}
		if !check(other) {
			t.Fatalf("Initial %d past the budget was given a handshake from a source nobody validated; "+
				"a sender writing a new source address per datagram draws one connection each", i+1)
		}
	}
	// One second gives back unvalidatedPerSecond, and no more.
	now = now.Add(time.Second)
	given := 0
	for i := 0; i < 3*unvalidatedPerSecond; i++ {
		if !check(a) {
			given++
		}
	}
	if given != unvalidatedPerSecond {
		t.Errorf("one second after the budget was spent %d handshake(s) were given, want %d", given, unvalidatedPerSecond)
	}
}

// TestEveryTransportChecksItsSources — ADR-160's routing (ADR-009: the guard checks the door). A transport
// built without the check gives every Initial a handshake again and nothing fails: the session works. Read
// from the source of every file in the package, so a fourth transport cannot be added without it.
func TestEveryTransportChecksItsSources(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	literal := regexp.MustCompile(`&quic\.Transport\{[^\n]*`)
	found := 0
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".go") || strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}
		src, rerr := os.ReadFile(f.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		for _, lit := range literal.FindAllString(string(src), -1) {
			found++
			if !strings.Contains(lit, "VerifySourceAddress: newSourceCheck()") {
				t.Errorf("%s builds a quic.Transport without the source check: %s", f.Name(), lit)
			}
		}
	}
	// STIMULUS: the dial-only transport, the owning listener's and the shared endpoint's.
	if found < 3 {
		t.Fatalf("found %d quic.Transport literal(s) in the package, want the three at least — the scan is "+
			"not reading the constructors", found)
	}
}

// retryingEndpoint is a shared endpoint whose transport demands a Retry of EVERY unvalidated Initial — the rule
// with its budget at nothing, which is what an endpoint under a flood looks like to an honest peer.
func retryingEndpoint(t *testing.T, retries *atomic.Int64) *SharedEndpoint {
	t.Helper()
	sock, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := udpmux.New(sock)
	e := &SharedEndpoint{mux: mux, tr: &quic.Transport{
		Conn: mux.QUIC(), ConnectionIDGenerator: newCIDGen(mux),
		VerifySourceAddress: func(net.Addr) bool { retries.Add(1); return true },
	}}
	t.Cleanup(func() { e.Close() })
	return e
}

// TestTheGlareSurvivesARetryOfEveryInitial — ADR-160 decision 3, on the path it could have broken.
//
// The symmetric race has each party punch, listen and dial on ONE socket, so each endpoint is at once the
// client answering the other's Retry and the server demanding one. Both dials and both accepts must still
// complete, as the pinned peer, and a stream must still open by role afterwards.
func TestTheGlareSurvivesARetryOfEveryInitial(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)
	var retries atomic.Int64
	ea, eb := retryingEndpoint(t, &retries), retryingEndpoint(t, &retries)
	lna, err := QUICListenHandshakeOn(ea, aCert, aKey, bFP)
	if err != nil {
		t.Fatal(err)
	}
	defer lna.Close()
	lnb, err := QUICListenHandshakeOn(eb, bCert, bKey, aFP)
	if err != nil {
		t.Fatal(err)
	}
	defer lnb.Close()
	if err := ea.Punch(eb.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if err := eb.Punch(ea.LocalAddr()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	conns := make([]*HandshakedConn, 4)
	errs := make([]error, 4)
	wg.Add(4)
	go func() { defer wg.Done(); conns[0], errs[0] = lna.Accept(ctx) }()
	go func() { defer wg.Done(); conns[1], errs[1] = lnb.Accept(ctx) }()
	go func() {
		defer wg.Done()
		conns[2], errs[2] = QUICDialHandshakeOn(ctx, ea, eb.LocalAddr().String(), aCert, aKey, bFP, 15*time.Second)
	}()
	go func() {
		defer wg.Done()
		conns[3], errs[3] = QUICDialHandshakeOn(ctx, eb, ea.LocalAddr().String(), bCert, bKey, aFP, 15*time.Second)
	}()
	wg.Wait()
	for i, name := range []string{"A's accept", "B's accept", "A's dial", "B's dial"} {
		if errs[i] != nil {
			skipIfSandboxBlocksUDP(t, errs[i])
			t.Fatalf("%s failed with a Retry demanded of every Initial: %v", name, errs[i])
		}
	}
	// STIMULUS: the Retry really was demanded of both dials, so the four successes are about it.
	if n := retries.Load(); n < 2 {
		t.Fatalf("the source check ran %d time(s); both dials should have met it", n)
	}
	// The pin held through the Retry: each side reads the OTHER's fingerprint.
	for i, want := range [][]byte{bFP, aFP, bFP, aFP} {
		if string(conns[i].PeerFP) != string(want) {
			t.Errorf("connection %d is with %x, want the pinned peer %x", i, conns[i].PeerFP, want)
		}
	}
	// And the survivor still promotes by role: A's dial opens the stream, B's accept takes it.
	type res struct {
		c   *Conn
		err error
	}
	got := make(chan res, 1)
	go func() { c, err := conns[1].Promote(ctx, false); got <- res{c, err} }()
	init, err := conns[2].Promote(ctx, true)
	if err != nil {
		t.Fatalf("promoting the dialled connection as initiator: %v", err)
	}
	defer init.Close()
	if _, err := init.Channel.Stream.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	r := <-got
	if r.err != nil {
		t.Fatalf("promoting the accepted connection as receiver: %v", r.err)
	}
	r.c.Close()
}

// TestUnfinishedHandshakesPastTheBudgetHoldNothing — ADR-160's consequence, over real quic-go: a sender that
// never hears a reply, which is what a spoofed source is, gets handshakes for the budget and Retries for the
// rest, and an honest peer arriving in the middle of it still connects.
func TestUnfinishedHandshakesPastTheBudgetHoldNothing(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	xCert, xKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	var asked, retried atomic.Int64
	rule := newSourceCheck()
	sock, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := udpmux.New(sock)
	srv := &SharedEndpoint{mux: mux, tr: &quic.Transport{
		Conn: mux.QUIC(), ConnectionIDGenerator: newCIDGen(mux),
		VerifySourceAddress: func(a net.Addr) bool {
			asked.Add(1)
			r := rule(a)
			if r {
				retried.Add(1)
			}
			return r
		},
	}}
	defer srv.Close()
	ln, err := QUICListenHandshakeOn(srv, bCert, bKey, aFP)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// The flood: one socket that sends and never listens, 200 dials by an identity nobody pinned.
	fsock, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fmux := udpmux.New(fsock)
	ftr := &quic.Transport{Conn: deafConn{fmux.QUIC()}, ConnectionIDGenerator: newCIDGen(fmux)}
	defer func() { ftr.Close(); fmux.Close() }()
	cfg, err := SessionTLS(xCert, xKey, bFP, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg.NextProtos = sessionALPN
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	remote := srv.LocalAddr().(*net.UDPAddr)
	const flood = 200
	start := time.Now()
	for i := 0; i < flood; i++ {
		go func() { _, _ = ftr.Dial(ctx, remote, cfg, quicConfig()) }()
	}
	deadline := time.Now().Add(3 * time.Second)
	for asked.Load() < flood && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	// STIMULUS: the Initials arrived and the check judged them.
	if asked.Load() < flood {
		skipIfSandboxBlocksUDP(t, context.DeadlineExceeded)
		t.Fatalf("only %d of %d unfinished handshakes reached the source check", asked.Load(), flood)
	}
	// Given a handshake: the budget, plus what came back while the flood ran. Everything else was a Retry.
	allowed := int64(unvalidatedBurst) + int64(time.Since(start).Seconds()*unvalidatedPerSecond) + 1
	if given := asked.Load() - retried.Load(); given > allowed {
		t.Errorf("%d unvalidated Initials were given a handshake in %v, want at most %d — each holds a "+
			"connection for the handshake timeout", given, time.Since(start).Round(time.Millisecond), allowed)
	}

	// An honest pinned peer, in the middle of it: asked for a Retry, never refused.
	cli, err := NewSharedEndpoint("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	acc := make(chan error, 1)
	go func() { _, err := ln.Accept(ctx); acc <- err }()
	hc, err := QUICDialHandshakeOn(ctx, cli, srv.LocalAddr().String(), aCert, aKey, bFP, 10*time.Second)
	if err != nil {
		t.Fatalf("an honest pinned peer could not connect while unfinished handshakes were arriving: %v", err)
	}
	if string(hc.PeerFP) != string(bFP) {
		t.Errorf("the honest dial reached %x, want the pinned peer", hc.PeerFP)
	}
	if err := <-acc; err != nil {
		t.Errorf("the listener did not accept the honest peer: %v", err)
	}
}

// deafConn sends and never hears: every datagram that arrives is dropped. A dial over it starts a handshake on
// the far side and can never finish one, or answer a Retry.
type deafConn struct{ net.PacketConn }

// quic-go asks its conn for bigger buffers and logs when it cannot; forwarded so the test is quiet.
func (d deafConn) SetReadBuffer(n int) error {
	return d.PacketConn.(interface{ SetReadBuffer(int) error }).SetReadBuffer(n)
}

func (d deafConn) SetWriteBuffer(n int) error {
	return d.PacketConn.(interface{ SetWriteBuffer(int) error }).SetWriteBuffer(n)
}

func (d deafConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		if _, _, err := d.PacketConn.ReadFrom(p); err != nil {
			return 0, nil, err
		}
	}
}
