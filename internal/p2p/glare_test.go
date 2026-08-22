package p2p

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

// handshakedPair brings up a real pinned QUIC connection over two shared endpoints and returns
// BOTH ends as HandshakedConns — the session stream opened on neither — so a test may Promote each
// to whichever ceremony role it likes. The dial side is the real QUICDialHandshakeOn; the accept
// side is taken raw off the shared transport's own listener, because the standing QUICListenOn
// accepts the stream as the RECEIVER and so cannot hand back a pre-stream connection to promote as
// the initiator. That listener-yields-handshaked plumbing is S09's coordinator; here the raw
// se.tr.Listen stands in for it and is faithful — it is the exact call QUICListenOn makes.
func handshakedPair(t *testing.T, acceptCert, acceptKey, acceptPinsDialer, dialCert, dialKey, dialPinsAccepter []byte) (dial, accept *HandshakedConn) {
	t.Helper()

	se, err := NewSharedEndpoint("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { se.Close() })
	sCfg, err := SessionTLS(acceptCert, acceptKey, acceptPinsDialer, true)
	if err != nil {
		t.Fatal(err)
	}
	sCfg.NextProtos = []string{alpn}
	sln, err := se.tr.Listen(sCfg, quicConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sln.Close() })

	accCh := make(chan *quic.Conn, 1)
	go func() {
		qc, err := sln.Accept(context.Background())
		if err != nil {
			accCh <- nil
			return
		}
		accCh <- qc
	}()

	ce, err := NewSharedEndpoint("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ce.Close() })
	dial, err = QUICDialHandshakeOn(context.Background(), ce, se.LocalAddr().String(), dialCert, dialKey, dialPinsAccepter, 10*time.Second)
	if err != nil {
		t.Fatalf("dial handshake: %v", err)
	}

	qc := <-accCh
	if qc == nil {
		t.Fatal("the accepter never completed a handshake against a correctly pinned dialer")
	}
	afp, err := verifiedPeerFingerprint(qc.ConnectionState().TLS)
	if err != nil {
		t.Fatalf("accepter fingerprint: %v", err)
	}
	return dial, &HandshakedConn{qc: qc, PeerFP: afp}
}

// TestQUICStreamOpensByRoleNotByDialer — P05.S09a, the glare deadlock fix. Under symmetric racing
// the party that DIALLED may be the ceremony RECEIVER and the party that ACCEPTED the INITIATOR.
// The QUIC session stream must be opened by the initiator regardless of who dialled: the initiator
// writes first, and AcceptStream on the other end unblocks only on that first frame. If stream
// direction followed the DIAL direction instead — dialer opens, listener accepts, as the pre-S09a
// code welded it — this arrangement deadlocks: the dial-won receiver opens a stream it then only
// reads, while the accept-won initiator sits on AcceptStream waiting for a frame its own role will
// never send first. Promote keys off the ROLE, so this completes; a Promote that keyed off the
// dialer hangs, which the redproof in test/redproofs/ drives explicitly.
func TestQUICStreamOpensByRoleNotByDialer(t *testing.T) {
	dCert, dKey := newIdentity(t) // the party that DIALS — but is the ceremony receiver
	aCert, aKey := newIdentity(t) // the party that ACCEPTS — but is the ceremony initiator
	dFP, aFP := fingerprint(t, dCert), fingerprint(t, aCert)

	dialHC, acceptHC := handshakedPair(t, aCert, aKey, dFP, dCert, dKey, aFP)

	type res struct {
		s   string
		err error
	}
	dc := make(chan res, 1) // dial side = receiver
	ac := make(chan res, 1) // accept side = initiator
	ctx := context.Background()

	go func() {
		conn, err := dialHC.Promote(ctx, false) // dialer promoted as the RECEIVER
		if err != nil {
			dc <- res{err: err}
			return
		}
		defer conn.Close()
		s, e := verificationExchange(conn.Channel, false, dFP)
		dc <- res{s, e}
	}()
	go func() {
		conn, err := acceptHC.Promote(ctx, true) // accepter promoted as the INITIATOR
		if err != nil {
			ac <- res{err: err}
			return
		}
		defer conn.Close()
		s, e := verificationExchange(conn.Channel, true, aFP)
		ac <- res{s, e}
	}()

	// A deadlock manifests as neither goroutine ever reporting; bound it so the suite fails fast
	// and by name instead of hanging (the tier-2 hazard: a hang is worse than a fail).
	deadline := time.After(20 * time.Second)
	var dr, ar res
	for got := 0; got < 2; {
		select {
		case dr = <-dc:
			got++
		case ar = <-ac:
			got++
		case <-deadline:
			t.Fatal("deadlocked: the role-opposite-dialer session never completed — stream direction is following the dialer, not the role")
		}
	}
	if dr.err != nil {
		t.Fatalf("receiver (dial side): %v", dr.err)
	}
	if ar.err != nil {
		t.Fatalf("initiator (accept side): %v", ar.err)
	}
	if dr.s != ar.s {
		t.Fatalf("the two sides derived different words: %q vs %q", dr.s, ar.s)
	}
	if n := len(strings.Fields(dr.s)); n != 4 {
		t.Errorf("the verification string is %d words, want 4: %q", n, dr.s)
	}
}
