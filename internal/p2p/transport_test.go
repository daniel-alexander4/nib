package p2p

import (
	"bytes"
	"context"
	"crypto/tls"
	"testing"
	"time"

	"errors"
	"net"
	"nib/internal/sign"
	"strings"
	"sync"
	"syscall"
)

func newIdentity(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	c, k, err := sign.GenerateIdentity("Test")
	if err != nil {
		t.Fatal(err)
	}
	return c, k
}

func fingerprint(t *testing.T, certPEM []byte) []byte {
	t.Helper()
	f, err := sign.Fingerprint(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// rawChain mints a transport leaf for an identity and returns the [leaf, identity]
// DER chain a peer would present.
func rawChain(t *testing.T, certPEM, keyPEM []byte) [][]byte {
	t.Helper()
	idCert, idKey, err := sign.ParseIdentity(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := mintTransportCert(idCert, idKey)
	if err != nil {
		t.Fatal(err)
	}
	return leaf.Certificate
}

func TestVerifyPinnedPeer(t *testing.T) {
	aCert, aKey := newIdentity(t)
	aFP := fingerprint(t, aCert)
	chainA := rawChain(t, aCert, aKey)

	// The pinned peer is accepted and its fingerprint returned.
	got, err := verifyPinnedPeer(chainA, aFP, time.Now())
	if err != nil {
		t.Fatalf("pinned peer rejected: %v", err)
	}
	if !bytes.Equal(got, aFP) {
		t.Errorf("returned fingerprint = %x, want %x", got, aFP)
	}

	// A peer whose identity isn't the pinned one is rejected.
	bCert, _ := newIdentity(t)
	if _, err := verifyPinnedPeer(chainA, fingerprint(t, bCert), time.Now()); err == nil {
		t.Error("accepted a peer that isn't the pinned identity")
	}

	// An incomplete chain (no identity cert) is rejected.
	if _, err := verifyPinnedPeer(chainA[:1], aFP, time.Now()); err == nil {
		t.Error("accepted an incomplete chain")
	}

	// A cert outside its validity window is rejected (freshness / replay bound).
	if _, err := verifyPinnedPeer(chainA, aFP, time.Now().Add(transportTTL+time.Hour)); err == nil {
		t.Error("accepted an expired transport cert")
	}

	// A leaf signed by a DIFFERENT key than the presented (pinned) identity is
	// rejected: present identity A (pin matches) but a leaf B signed by identity B.
	bCert2, bKey2 := newIdentity(t)
	chainB := rawChain(t, bCert2, bKey2)
	idCertA, _, err := sign.ParseIdentity(aCert, aKey)
	if err != nil {
		t.Fatal(err)
	}
	mixed := [][]byte{chainB[0], idCertA.Raw} // leaf signed by B, identity claimed as A
	if _, err := verifyPinnedPeer(mixed, aFP, time.Now()); err == nil {
		t.Error("accepted a leaf not signed by the pinned identity")
	}
}

// TestSessionHandshakeAcceptsPinnedPeer drives a real crypto/tls handshake end to
// end: it proves VerifyPeerCertificate is actually invoked, the mutual pinning
// passes, and each side records the other's verified fingerprint.
func TestSessionHandshakeAcceptsPinnedPeer(t *testing.T) {
	aCert, aKey := newIdentity(t) // listener
	bCert, bKey := newIdentity(t) // dialer
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	srvCfg, err := SessionTLS(aCert, aKey, bFP, true) // A accepts B
	if err != nil {
		t.Fatal(err)
	}
	cliCfg, err := SessionTLS(bCert, bKey, aFP, false) // B accepts A
	if err != nil {
		t.Fatal(err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srvErr := make(chan error, 1)
	srvPeer := make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			srvErr <- err
			return
		}
		defer conn.Close()
		tc := conn.(*tls.Conn)
		if err := tc.Handshake(); err != nil {
			srvErr <- err
			return
		}
		srvErr <- nil
		srvPeer <- peerFP(t, tc)
	}()

	conn, err := tls.Dial("tcp", ln.Addr().String(), cliCfg)
	if err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}
	defer conn.Close()
	if err := <-srvErr; err != nil {
		t.Fatalf("server handshake failed: %v", err)
	}

	// Each side sees the other's pinned identity in the completed handshake.
	if got := peerFP(t, conn); !bytes.Equal(got, aFP) {
		t.Errorf("client sees peer %x, want %x", got, aFP)
	}
	if got := <-srvPeer; !bytes.Equal(got, bFP) {
		t.Errorf("server sees peer %x, want %x", got, bFP)
	}
}

// peerFP reads the verified peer's SPKI fingerprint from a completed handshake —
// PeerCertificates is [leaf, identity], and verification has already passed.
func peerFP(t *testing.T, c *tls.Conn) []byte {
	t.Helper()
	certs := c.ConnectionState().PeerCertificates
	if len(certs) < 2 {
		t.Fatalf("peer presented %d certificates, want >= 2", len(certs))
	}
	return sign.FingerprintCert(certs[1])
}

// TestSessionHandshakeRejectsUnpinnedPeer proves the handshake actually drops a
// peer the listener hasn't pinned — the load-bearing guarantee of the armed listener.
func TestSessionHandshakeRejectsUnpinnedPeer(t *testing.T) {
	aCert, aKey := newIdentity(t) // listener
	bCert, bKey := newIdentity(t) // dialer (a stranger to A)
	cCert, _ := newIdentity(t)    // the only peer A will accept
	aFP, cFP := fingerprint(t, aCert), fingerprint(t, cCert)

	srvCfg, err := SessionTLS(aCert, aKey, cFP, true) // A accepts only C
	if err != nil {
		t.Fatal(err)
	}
	cliCfg, err := SessionTLS(bCert, bKey, aFP, false) // B dials A
	if err != nil {
		t.Fatal(err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srvErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			srvErr <- err
			return
		}
		defer conn.Close()
		// TLS 1.3 rejects an unpinned *dialer* on the server, when it processes the
		// client certificate — the listener is where the guarantee lives. (The
		// client's Dial may return before it sees the rejection, per TLS 1.3 flight
		// ordering, so we assert on the server side.)
		srvErr <- conn.(*tls.Conn).Handshake()
	}()

	if conn, err := tls.Dial("tcp", ln.Addr().String(), cliCfg); err == nil {
		conn.Close()
	}
	if err := <-srvErr; err == nil {
		t.Fatal("listener accepted an unpinned peer")
	}
}

// TestAStalledPeerDoesNotBlockTheAcceptPath.
//
// The handshake used to run INLINE in Accept: `ln.Accept()` then `TLSChannel(tc)` in one
// call. The server's arm loop is a single goroutine calling this, so one TCP connection that
// opened and sent nothing held it for the whole handshakeTimeout — 30 seconds. The listener
// binds `0.0.0.0:0` with its port broadcast to the multicast group every 500 ms, so ten of
// them consumed the entire five-minute arm window and the genuine peer was never accepted.
//
// **The measurement is the test.** `TestAStrayConnectionDoesNotConsumeTheSession` already
// proved the session SURVIVES a stray connection; it never measured that the loop was
// blocked, which is why the fix for the one-shot-consumption defect left this in place.
func TestAStalledPeerDoesNotBlockTheAcceptPath(t *testing.T) {
	certA, keyA, _ := sign.GenerateIdentity("Alice")
	certB, keyB, _ := sign.GenerateIdentity("Bob")
	fpA, err := sign.Fingerprint(certA)
	if err != nil {
		t.Fatal(err)
	}
	fpB, err := sign.Fingerprint(certB)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := Listen("127.0.0.1:0", certB, keyB, fpA)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Stalled connections: opened, never a byte sent. Each would have cost the accept path
	// handshakeTimeout in turn.
	const stalled = 5
	for i := 0; i < stalled; i++ {
		c, derr := net.Dial("tcp", ln.Addr().String())
		if derr != nil {
			t.Fatalf("setup: could not open stalled connection %d: %v", i, derr)
		}
		defer c.Close()
	}
	// STIMULUS: they really are connected and really are silent. Without this the timing
	// below is measuring an accept path with nothing in front of it.
	time.Sleep(200 * time.Millisecond)

	accepted := make(chan error, 1)
	go func() {
		conn, aerr := ln.Accept()
		if conn != nil {
			conn.Close()
		}
		accepted <- aerr
	}()

	start := time.Now()
	go func() {
		conn, derr := Dial(context.Background(), ln.Addr().String(), certA, keyA, fpB, 10*time.Second)
		if conn != nil {
			conn.Close()
		}
		_ = derr
	}()

	select {
	case aerr := <-accepted:
		if aerr != nil {
			t.Fatalf("the genuine peer was not accepted: %v", aerr)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the genuine peer was never accepted")
	}
	elapsed := time.Since(start)
	// Serially, five stalled connections cost 5 x handshakeTimeout before the real one is
	// even looked at. The bound is deliberately loose — the assertion is "it did not queue
	// behind them", not a latency budget.
	if elapsed > handshakeTimeout {
		t.Errorf("the genuine peer waited %v behind %d stalled connections; the handshake "+
			"timeout alone is %v, so it was queued behind them", elapsed, stalled, handshakeTimeout)
	}
	t.Logf("accepted in %v with %d stalled connections in front", elapsed, stalled)
}

// TestTheListenerTerminatesThroughExactlyOneDoor.
//
// The rewrite that moved the handshake off the accept path introduced two termination signals
// — `close(l.ready)` in the accept loop and `close(l.done)` in `Close` — and a handshake
// goroutine selecting on a send into `ready` up to 30 seconds later. A send on a closed
// channel is a *ready* select case that panics, so after `Close` roughly half of every
// in-flight successful handshake panicked, and after a non-close accept error every one did.
//
// **None of these six questions had an answer in the suite.** `TestAStalledPeerDoesNotBlock…`
// uses connections that never speak, so not one of them ever reaches the statement that panics.
func TestTheListenerTerminatesThroughExactlyOneDoor(t *testing.T) {
	// **Both transports, since P05.S02.** This guard was written for `tlsListener` and
	// called `Listen` directly. That slice gave `quicListener` the same termination
	// protocol — one `done`, a `ready` that is never closed, a `cerr` that always wraps
	// net.ErrClosed — and copying a protocol without its guard is how the second copy
	// drifts. Every question below is about the CONTRACT, not about TCP, so it runs
	// against whichever listener the table names.
	eachTransport(t, func(t *testing.T, tr transport) {
		certA, keyA, _ := sign.GenerateIdentity("Alice")
		certB, keyB, _ := sign.GenerateIdentity("Bob")
		fpA, err := sign.Fingerprint(certA)
		if err != nil {
			t.Fatal(err)
		}
		fpB, err := sign.Fingerprint(certB)
		if err != nil {
			t.Fatal(err)
		}

		t.Run("Accept after Close reports net.ErrClosed", func(t *testing.T) {
			ln, lerr := tr.listen("127.0.0.1:0", certB, keyB, fpA)
			if lerr != nil {
				t.Fatal(lerr)
			}
			if cerr := ln.Close(); cerr != nil {
				t.Fatalf("Close: %v", cerr)
			}
			_, aerr := ln.Accept()
			if !errors.Is(aerr, net.ErrClosed) {
				t.Errorf("Accept after Close = %v; runSession exits only on net.ErrClosed, so "+
					"anything else spins that loop at 100%% of a core with no syscall", aerr)
			}
		})

		t.Run("a second Close is harmless", func(t *testing.T) {
			ln, lerr := tr.listen("127.0.0.1:0", certB, keyB, fpA)
			if lerr != nil {
				t.Fatal(lerr)
			}
			if cerr := ln.Close(); cerr != nil {
				t.Fatalf("first Close: %v", cerr)
			}
			if cerr := ln.Close(); cerr != nil {
				t.Errorf("second Close: %v", cerr)
			}
		})

		t.Run("Close before Accept was ever called", func(t *testing.T) {
			ln, lerr := tr.listen("127.0.0.1:0", certB, keyB, fpA)
			if lerr != nil {
				t.Fatal(lerr)
			}
			// Close without ever calling Accept: `start()` has not run, so the loop goroutine
			// does not exist. A later Accept must still return rather than blocking forever on
			// a listener that is already gone.
			if cerr := ln.Close(); cerr != nil {
				t.Fatal(cerr)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = ln.Accept()
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("Accept blocked forever on a listener closed before it was ever called")
			}
		})

		t.Run("Close while a handshake is in flight does not panic", func(t *testing.T) {
			ln, lerr := tr.listen("127.0.0.1:0", certB, keyB, fpA)
			if lerr != nil {
				t.Fatal(lerr)
			}
			// Nobody calls Accept, so a completed handshake has nowhere to go — which is the
			// exact state that used to panic on a closed `ready`.
			//
			// **Delivering a handshake is not the same act on both transports.** On TCP a
			// completed handshake is itself acceptable. On QUIC a stream is invisible to
			// the peer until data crosses it, so the listener's stream-accept is still
			// waiting and nothing reaches `Accept` — the dialer has to SPEAK. The property
			// below is transport-neutral; only the stimulus differs, and writing one frame
			// is what makes it real on both.
			accepted := make(chan struct{})
			go func() {
				defer close(accepted)
				c, aerr := ln.Accept()
				if c != nil {
					c.Close()
				}
				_ = aerr
			}()
			dialed := make(chan struct{})
			go func() {
				defer close(dialed)
				c, derr := tr.dial(context.Background(), ln.Addr().String(), certA, keyA, fpB, 5*time.Second)
				if c != nil {
					_ = writeFrame(c.Stream, []byte("hello"))
					// **Held open until the peer has it**, and that is not tidiness.
					// `gracefulClose`'s own doc: closing a QUIC connection "sends
					// CONNECTION_CLOSE immediately and abandons anything still
					// unacknowledged". Writing and closing in the next statement races
					// the frame against the close — lose it and the listener's
					// stream-accept never fires, nothing reaches Accept, and this test
					// hangs rather than fails. Which is what it did.
					<-accepted
					c.Close()
				}
				_ = derr
			}()
			<-accepted // STIMULUS: a handshake really completed and was delivered.
			<-dialed
			if cerr := ln.Close(); cerr != nil {
				t.Errorf("Close after a delivered handshake: %v", cerr)
			}
			// A second dial into the closed listener, then give any in-flight goroutine time to
			// reach its send. safe.Recover would swallow a panic, so the observable is that the
			// listener stays usable and Close stays clean.
			go func() {
				c, _ := tr.dial(context.Background(), ln.Addr().String(), certA, keyA, fpB, time.Second)
				if c != nil {
					c.Close()
				}
			}()
			time.Sleep(300 * time.Millisecond)
			if _, aerr := ln.Accept(); !errors.Is(aerr, net.ErrClosed) {
				t.Errorf("Accept on the closed listener = %v, want net.ErrClosed", aerr)
			}
		})

		t.Run("the handshake pool is bounded", func(t *testing.T) {
			if maxConcurrentHandshakes <= 0 {
				t.Fatal("the pool is unbounded — one goroutine and one fd per inbound connection " +
					"for the whole handshakeTimeout, driven by any host on the segment")
			}
			if maxConcurrentHandshakes > 64 {
				t.Errorf("maxConcurrentHandshakes = %d is not a bound a 256-descriptor GUI "+
					"process survives", maxConcurrentHandshakes)
			}
		})
	})
}

// failingListener returns err from every Accept, standing in for EMFILE.
//
// A fake, because the real trigger is descriptor exhaustion and a test cannot arrange that
// without wrecking the machine running it — but the terminal-accept-error path is the one
// that produced the permanent hot spin, so it needs a driver.
type failingListener struct {
	err    error
	addr   net.Addr
	closed chan struct{}
	once   sync.Once
}

func (f *failingListener) Accept() (net.Conn, error) { return nil, f.err }
func (f *failingListener) Addr() net.Addr            { return f.addr }
func (f *failingListener) Close() error              { f.once.Do(func() { close(f.closed) }); return nil }

// TestATerminalAcceptErrorStillReportsNetErrClosed.
//
// `runSession`'s accept loop exits only on `errors.Is(err, net.ErrClosed)` — everything else
// is "this peer failed, stay armed". So when `loop` hit an accept error it could not continue
// past (EMFILE from descriptor exhaustion), `Accept` returned the bare `EMFILE` with no
// syscall and that loop spun at 100 % of a core. `Close` could not rescue it either, because
// it only sets `cerr` when `cerr` is nil — so the disarm timer fired, `Close` ran, and
// `closeErr` went on returning EMFILE forever. `runSession` never returned, its defers never
// ran, and the LAN announcer broadcast for the life of the process.
//
// **The other listener already maps this**: `quicListener.Accept` turns `quic.ErrServerClosed`
// into `net.ErrClosed` with a comment naming the same hazard. The TCP one did not.
func TestATerminalAcceptErrorStillReportsNetErrClosed(t *testing.T) {
	emfile := &net.OpError{Op: "accept", Net: "tcp", Err: syscall.EMFILE}
	fake := &failingListener{
		err:    emfile,
		addr:   &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1},
		closed: make(chan struct{}),
	}
	l := &tlsListener{ln: fake, done: make(chan struct{})}

	got := make(chan error, 1)
	go func() {
		_, aerr := l.Accept()
		got <- aerr
	}()

	var err error
	select {
	case err = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("Accept never returned after a terminal accept error")
	}

	// STIMULUS: the loop really did terminate through Close, or the assertion below is
	// about a listener that simply had nothing to accept.
	select {
	case <-fake.closed:
	default:
		t.Error("the accept loop did not close the underlying listener — the terminal error " +
			"leaves it open and the loop goroutine unaccounted for")
	}

	if !errors.Is(err, net.ErrClosed) {
		t.Errorf("Accept after a terminal accept error = %v; runSession exits only on "+
			"net.ErrClosed, so this spins that loop at 100%% of a core with no syscall", err)
	}
	// And the cause survives, or a diagnostic cannot say why it stopped.
	if !errors.Is(err, syscall.EMFILE) && !strings.Contains(err.Error(), "too many open files") {
		t.Errorf("the cause was discarded: %v", err)
	}

	// A subsequent Accept must behave the same rather than blocking.
	second := make(chan error, 1)
	go func() { _, e := l.Accept(); second <- e }()
	select {
	case e := <-second:
		if !errors.Is(e, net.ErrClosed) {
			t.Errorf("the second Accept = %v, want net.ErrClosed", e)
		}
	case <-time.After(5 * time.Second):
		t.Error("a second Accept blocked forever")
	}
}
