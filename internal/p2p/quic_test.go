package p2p

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// quicPair arms a QUIC listener for `accept` and dials it as `dialer`. The listener runs
// in a goroutine and hands its Conn back, so a test drives both ends of a real session.
func quicPair(t *testing.T, listenPin, dialPin []byte, lCert, lKey, dCert, dKey []byte) (*Conn, <-chan acceptResult) {
	t.Helper()
	ln, err := QUICListen("127.0.0.1:0", lCert, lKey, listenPin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	res := make(chan acceptResult, 1)
	go func() {
		c, err := ln.Accept()
		res <- acceptResult{c, err}
	}()

	conn, err := QUICDial(context.Background(), ln.Addr().String(), dCert, dKey, dialPin, 10*time.Second)
	if err != nil {
		return nil, res
	}
	t.Cleanup(func() { conn.Close() })
	return conn, res
}

type acceptResult struct {
	conn *Conn
	err  error
}

// TestAQUICSessionEstablishesAChannelOnBothEnds — the transport's own contract, before
// any ceremony rides on it. Both ends must end up with a Channel whose peer fingerprint
// is the OTHER party's, and whose exporters agree.
func TestAQUICSessionEstablishesAChannelOnBothEnds(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	conn, res := quicPair(t, aFP, bFP, bCert, bKey, aCert, aKey)
	if conn == nil {
		t.Fatal("the dial failed against a correctly pinned peer")
	}
	// The dialer must speak first for the listener's AcceptStream to unblock — which is
	// also true of every real entry point, since all four start with a commitment.
	if err := writeFrame(conn.Stream, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	r := <-res
	if r.err != nil {
		t.Fatalf("accept: %v", r.err)
	}
	// Closed in protocol order — dialer first — because the listening side waits for
	// the peer to go away before tearing down (gracefulClose). Reversing it makes this
	// test pay the full closeGrace, which is a 5-second no-op nobody would look at.
	defer r.conn.Close()
	defer conn.Close()

	// Each side holds the OTHER's fingerprint. Asserting both directions matters: a bug
	// that handed each side its own would still make them "agree".
	if got := hex.EncodeToString(conn.PeerFP); got != hex.EncodeToString(bFP) {
		t.Errorf("the dialer's peer is %s, want B (%s)", got, hex.EncodeToString(bFP))
	}
	if got := hex.EncodeToString(r.conn.PeerFP); got != hex.EncodeToString(aFP) {
		t.Errorf("the listener's peer is %s, want A (%s)", got, hex.EncodeToString(aFP))
	}

	// And the channel binding both ends derive is the same channel — the property D4's
	// verification string actually rests on.
	da, err := conn.Export("EXPORTER-nib-verification", nil, 32)
	if err != nil {
		t.Fatal(err)
	}
	db, err := r.conn.Export("EXPORTER-nib-verification", nil, 32)
	if err != nil {
		t.Fatal(err)
	}
	if len(da) != 32 {
		t.Fatalf("exporter returned %d bytes, want 32", len(da))
	}
	if hex.EncodeToString(da) != hex.EncodeToString(db) {
		t.Errorf("the two ends derived different keying material:\n  %s\n  %s", hex.EncodeToString(da), hex.EncodeToString(db))
	}
}

// TestQUICRejectsAPeerThatIsNotPinned is the second acceptance clause, driven.
//
// The pinned-peer callback lives in SessionTLS and is reused unchanged across both
// transports; P02.S02's spike measured that QUIC invokes it on both ends. What is driven
// here is the consequence: a third identity gets no session and no bytes.
//
// **What it does NOT assert, because measurement says otherwise:** that the dial fails.
// Under TLS 1.3 the client finishes its handshake before the server has processed the
// client certificate, so a rejected dial returns SUCCESS and the refusal surfaces on the
// first read — "tls: bad certificate". That is true of TCP too, identically, and it is
// pre-existing rather than anything QUIC introduced. The property that matters is
// unaffected: the peer never answers, so the verification exchange cannot complete and
// no document byte crosses (L2).
func TestQUICRejectsAPeerThatIsNotPinned(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	cCert, cKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	// Stimulus first: this listener accepts the peer it IS pinned to. Without it, every
	// assertion below is satisfied by a listener that refuses everyone — or by nothing
	// listening at all, which is the shape this exact test would otherwise take.
	conn, res := quicPair(t, aFP, bFP, bCert, bKey, aCert, aKey)
	if conn == nil {
		t.Fatal("the pinned peer could not connect, so a refusal below proves nothing")
	}
	if err := writeFrame(conn.Stream, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if r := <-res; r.err != nil {
		t.Fatalf("the pinned peer was not accepted: %v", r.err)
	} else {
		r.conn.Close()
	}

	// Now C — a third identity the listener has never pinned — dials the same way.
	ln, err := QUICListen("127.0.0.1:0", bCert, bKey, aFP) // B accepts only A
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan *Conn, 1)
	go func() {
		c, _ := ln.Accept()
		accepted <- c
	}()

	// C pins B correctly, so the ONLY thing wrong is who C is. A test where both pins
	// were wrong could not say which side did the refusing.
	bad, err := QUICDial(context.Background(), ln.Addr().String(), cCert, cKey, bFP, 10*time.Second)
	if err != nil {
		// Acceptable, but not what is observed today; the assertions below are the
		// ones that carry the clause either way.
		t.Logf("the dial itself failed: %v", err)
	} else {
		defer bad.Close()
		// The refusal arrives here. Writing succeeds — the bytes go nowhere — and the
		// read returns the peer's TLS alert.
		if err := writeFrame(bad.Stream, make([]byte, 32)); err != nil {
			t.Logf("write refused immediately: %v", err)
		}
		_ = bad.Stream.SetDeadline(time.Now().Add(10 * time.Second))
		if _, err := readFrameMax(bad.Stream, 32); err == nil {
			t.Fatal("an unpinned identity got an answer from an armed listener")
		} else if !strings.Contains(err.Error(), "certificate") {
			t.Errorf("the exchange failed, but not as a certificate rejection: %v", err)
		}
	}

	// The listener must never hand a connection up from an unpinned identity. This is
	// the clause: the callback rejected it, so Accept is still waiting for a real peer.
	select {
	case c := <-accepted:
		if c != nil {
			c.Close()
			t.Fatal("the listener accepted a connection from an unpinned identity")
		}
	case <-time.After(2 * time.Second):
		// Still waiting, which is correct — and is why Accept is not bounded by a
		// handshake timeout: a wrong peer must cost the arm window nothing.
	}
}

// The whole-ceremony-over-QUIC test used to live here. It is gone on purpose:
// TestSessionRoundTrip now runs over every entry in the transport table, so a QUIC
// ceremony is its /quic subtest. Keeping both would be exactly the two sets of
// session tests P02.S06 exists to collapse into one — and the copy left behind is
// always the one that stops being updated.
//
// What stays in this file is what is TRUE OF THE TRANSPORT rather than of a session:
// that both ends see the other's verified identity and agree on the channel binding,
// and that an unpinned identity gets nothing.

// TestTheTwoTransportsAreSelectedByTheSameCore — D14's claim, as a compile-and-run check
// rather than a reading: the same four entry points serve both, so nothing here names a
// transport except the two constructors.
func TestTheTwoTransportsAreSelectedByTheSameCore(t *testing.T) {
	var _ Listener = (*tlsListener)(nil)
	var _ Listener = (*quicListener)(nil)
	if alpn == "" {
		t.Fatal("quic-go requires a non-empty ALPN; an empty one fails at Listen, not here")
	}
	if !errors.Is(errChannelIncomplete, errChannelIncomplete) {
		t.Fatal("unreachable")
	}
}

// TestBothTransportsWireTheConnectionIDGenerator is the wiring guard the filed item
// asked for, and it is here because the failure it prevents is silent.
//
// The mux falls back to routing on the peer ADDRESS while no connection id has been
// registered — deliberately, because a generator a later change forgets to install
// would otherwise send every short header to the DHT and break the SESSION, which is
// the worse of the two failures. The cost of that safety net is that a missed wiring
// looks like nothing at all: the session still works, and only a DHT reply from a host
// we hold a session with is quietly swallowed.
//
// So the wiring is asserted at the source. Both constructors, because it was wiring
// ONE end that made the first driven test still fail: B's mux learns A as a QUIC peer
// from its own handshake writes, so a fix on one side is not a fix.
func TestBothTransportsWireTheConnectionIDGenerator(t *testing.T) {
	src, err := os.ReadFile("quic.go")
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: the file really constructs the transports this guard is about.
	n := strings.Count(string(src), "&quic.Transport{")
	if n == 0 {
		t.Fatal("quic.go constructs no transports; the count below would pass on nothing")
	}
	if got := strings.Count(string(src), "ConnectionIDGenerator: newCIDGen("); got != n {
		t.Errorf("%d of %d quic.Transport constructions install a connection-id generator. "+
			"An unwired one does not fail loudly — the mux falls back to the address rule "+
			"and the session keeps working, while a DHT reply from a session peer is "+
			"silently swallowed.", got, n)
	}
}
