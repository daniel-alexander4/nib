package p2p

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// pacedVerifier answers after `after`, with `ok` and `err`.
type pacedVerifier struct {
	after time.Duration
	ok    bool
	err   error
}

func (v pacedVerifier) ConfirmVerification(string) (bool, error) {
	time.Sleep(v.after)
	return v.ok, v.err
}

// runVerdictPair runs one real session over tr: `dial` on the dialing side with verifier dv, `recv`
// on the listening side with rv. It returns each side's error.
func runVerdictPair(t *testing.T, tr transport,
	dial func(ch Channel, myFP []byte, v Verifier) error,
	recv func(ch Channel, cert, key []byte, v Verifier) error,
	dv, rv Verifier) (dialErr, recvErr error) {
	t.Helper()
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)
	ln, err := tr.listen("127.0.0.1:0", bCert, bKey, aFP)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	rc := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			rc <- fmt.Errorf("accept: %w", err)
			return
		}
		defer conn.Close() // the production shape: the caller closes as soon as the core returns
		rc <- recv(conn.Channel, bCert, bKey, rv)
	}()
	conn, err := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !conn.Channel.SpeaksVerdict() {
		t.Fatalf("the %s pair negotiated %q, which does not carry the verdict — the test would be "+
			"measuring the pre-alpn4 protocol", tr.name, conn.Channel.Proto)
	}
	dialErr = dial(conn.Channel, aFP, dv)
	conn.Close()
	return dialErr, <-rc
}

// TestARejectionOfTheWordsReachesTheSideThatConfirmed — /pending 802.
//
// The side whose user DID NOT confirm used to return without a byte, and the side whose user did
// read the closed connection as a broken pipe or EOF; `IsTransportLoss` called that a lost channel
// and the server re-raced or re-accepted. Over a real loopback connection on each transport, in
// both directions and on both document flows, the confirming side must now report
// `ErrPeerDidNotConfirm` — a decided outcome, not a transport loss.
//
// The `slow` case is the one QUIC needed: the confirming user answers AFTER the decliner has given
// up lingering and closed. A verdict left unread in a QUIC stream is destroyed by that close, so it
// passes only because the confirming side reads the verdict concurrently with its own gate.
func TestARejectionOfTheWordsReachesTheSideThatConfirmed(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	initiate := func(ch Channel, myFP []byte, v Verifier) error {
		_, e := Initiate(ch, pdf, myFP, v, Roster{})
		return e
	}
	receive := func(ch Channel, cert, key []byte, v Verifier) error {
		_, e := Receive(ch, cert, key, "peer", nil, v, nil, Roster{})
		return e
	}
	send := func(ch Channel, myFP []byte, v Verifier) error {
		return SendDocument(ch, pdf, myFP, v, PeerGatesHuman)
	}

	no := pacedVerifier{ok: false}
	timedOut := pacedVerifier{err: ErrVerificationTimedOut}
	yesSoon := pacedVerifier{after: 200 * time.Millisecond, ok: true}
	yesLate := pacedVerifier{after: verdictLinger + time.Second, ok: true}

	for _, tc := range []struct {
		name          string
		dial          func(Channel, []byte, Verifier) error
		recv          func(Channel, []byte, []byte, Verifier) error
		dv, rv        Verifier
		dialerRejects bool
		wantOwn       error
		slow          bool
	}{
		{"co-sign, receiver declines", initiate, receive, yesSoon, no, false, ErrVerificationDeclined, false},
		{"co-sign, dialer declines", initiate, receive, no, yesSoon, true, ErrVerificationDeclined, false},
		{"co-sign, receiver times out", initiate, receive, yesSoon, timedOut, false, ErrVerificationTimedOut, false},
		{"transfer, receiver declines", send, receiveDocFP(), yesSoon, no, false, ErrVerificationDeclined, false},
		{"transfer, dialer declines", send, receiveDocFP(), no, yesSoon, true, ErrVerificationDeclined, false},
		{"co-sign, dialer declines, receiver answers after the linger", initiate, receive, no, yesLate, true, ErrVerificationDeclined, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.slow && testing.Short() {
				t.Skip("waits out verdictLinger")
			}
			eachTransport(t, func(t *testing.T, tr transport) {
				dErr, rErr := runVerdictPair(t, tr, tc.dial, tc.recv, tc.dv, tc.rv)
				rejecter, confirmer := rErr, dErr
				if tc.dialerRejects {
					rejecter, confirmer = dErr, rErr
				}
				if !errors.Is(rejecter, tc.wantOwn) {
					t.Errorf("the side that did not confirm returned %v, want %v", rejecter, tc.wantOwn)
				}
				if !errors.Is(confirmer, ErrPeerDidNotConfirm) {
					t.Errorf("the side that confirmed returned %v (transport loss: %t), want "+
						"ErrPeerDidNotConfirm — a rejection of the words read as a dropped channel is "+
						"retried, which is the one thing the re-race whitelist exists never to do",
						confirmer, IsTransportLoss(confirmer))
				}
				if IsTransportLoss(confirmer) {
					t.Errorf("the confirming side's error %v classifies as a transport loss, so the "+
						"server would re-race it", confirmer)
				}
			})
		})
	}
}

// receiveDocFP is ReceiveDocument in the shape the table takes: it is handed the cert and derives
// the fingerprint, as `serveOneSession` does.
func receiveDocFP() func(Channel, []byte, []byte, Verifier) error {
	return func(ch Channel, cert, _ []byte, v Verifier) error {
		fp, err := ownFingerprint(cert)
		if err != nil {
			return err
		}
		_, e := ReceiveDocument(ch, nil, fp, v)
		return e
	}
}

// TestTheVerdictGoesOnlyToAPeerThatNegotiatedIt — the floor, failing closed (ADR-076).
func TestTheVerdictGoesOnlyToAPeerThatNegotiatedIt(t *testing.T) {
	for _, tc := range []struct {
		proto string
		want  bool
	}{{alpn4, true}, {alpn3, false}, {alpn2, false}, {alpn, false}, {"", false}, {"h2", false}} {
		if got := (Channel{Proto: tc.proto}).SpeaksVerdict(); got != tc.want {
			t.Errorf("SpeaksVerdict(%q) = %t, want %t — an older peer reads the dialer's verdict "+
				"as its document", tc.proto, got, tc.want)
		}
	}
	saved := sessionALPN
	t.Cleanup(func() { sessionALPN = saved })
	sessionALPN = []string{alpn3, alpn2, alpn}
	if (Channel{Proto: alpn4}).SpeaksVerdict() {
		t.Error("with alpn4 no longer offered, a peer still reports that it speaks the verdict")
	}
}
