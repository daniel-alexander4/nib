package p2p

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// signAsInitiator does the local prepare + first signature the dialing side
// performs before opening the session.
func signAsInitiator(t *testing.T, certPEM, keyPEM, peerFP []byte) []byte {
	t.Helper()
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	place, err := NextPlacement(prepared)
	if err != nil {
		t.Fatal(err)
	}
	att := Attestation{Signer: "Alice", AcceptedPeer: hex.EncodeToString(peerFP), AcceptedPeerLabel: "Bob", Intent: "I agree to co-sign", When: time.Now()}
	signed, err := Contribute(prepared, certPEM, keyPEM, att, nil, place)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

type confirmer struct {
	accept bool
	intent string
	// err is what a real Confirmer returns when nobody answered. The server's bridge
	// returns ErrConsentTimedOut here; the fake takes it so the wire behaviour of a
	// timeout can be driven without waiting sessionConsentTimeout.
	err error
}

func (c confirmer) Confirm(SignerAttestation, []byte) (bool, string, []byte, time.Time, error) {
	return c.accept, c.intent, nil, time.Time{}, c.err
}

// TestConfirmCoSignedRequiresBothSignatures proves the initiator-side check accepts
// a genuine mutual co-signature but rejects a reply that carries only the peer's
// signature — e.g. a peer who ignores the sent document, signs a different one
// accepting the initiator, and returns that. Without the initiator's own valid
// signature on the result, it is not the document the initiator agreed to.
func TestConfirmCoSignedRequiresBothSignatures(t *testing.T) {
	aCert, aKey := newIdentity(t) // initiator (Alice)
	bCert, bKey := newIdentity(t) // peer (Bob)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	bAcceptsA := Attestation{Signer: "Bob", AcceptedPeer: hex.EncodeToString(aFP), AcceptedPeerLabel: "Alice", Intent: "I accept", When: time.Now()}

	// Genuine mutual co-sign: Alice signs accepting Bob, Bob co-signs accepting Alice.
	aSigned := signAsInitiator(t, aCert, aKey, bFP)
	mutual := contribute(t, aSigned, bCert, bKey, bAcceptsA)
	if err := confirmCoSigned(mutual, bFP, aFP, false); err != nil {
		t.Errorf("genuine mutual co-sign rejected: %v", err)
	}

	// Substitution: a document Bob signed accepting Alice that Alice never signed.
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	bOnly := contribute(t, prepared, bCert, bKey, bAcceptsA)
	if err := confirmCoSigned(bOnly, bFP, aFP, false); err == nil {
		t.Error("accepted a reply missing the initiator's own signature")
	}
}

// TestInitiateRejectsReplayedCoSignature proves the initiator rejects a peer that
// answers with a different, previously co-signed document. Both signature checks
// in confirmCoSigned pass on such a replay — Alice genuinely signed it, in an
// earlier session — so the byte-prefix binding to THIS session's document is the
// check that must catch it.
func TestInitiateRejectsReplayedCoSignature(t *testing.T) {
	eachTransport(t, func(t *testing.T, tr transport) {
		aCert, aKey := newIdentity(t) // initiator (Alice)
		bCert, bKey := newIdentity(t) // malicious peer (Bob)
		aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

		bAcceptsA := Attestation{Signer: "Bob", AcceptedPeer: hex.EncodeToString(aFP), AcceptedPeerLabel: "Alice", Intent: "I accept", When: time.Now()}

		// An earlier, genuine mutual co-signature between the same two identities.
		replay := contribute(t, signAsInitiator(t, aCert, aKey, bFP), bCert, bKey, bAcceptsA)
		if err := confirmCoSigned(replay, bFP, aFP, false); err != nil {
			t.Fatalf("fixture is not a valid mutual co-signature: %v", err)
		}

		// A new session: Alice signs afresh; the peer ignores her document and
		// answers with the old artifact.
		aSigned := signAsInitiator(t, aCert, aKey, bFP)
		if bytes.HasPrefix(replay, aSigned) {
			t.Fatal("degenerate fixture: the replay starts with this session's document")
		}

		ln, err := tr.listen("127.0.0.1:0", bCert, bKey, aFP)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		recvErr := make(chan error, 1)
		go func() {
			conn, err := ln.Accept()
			if err != nil {
				recvErr <- err
				return
			}
			defer conn.Close()
			if _, err := readFrame(conn.Stream); err != nil {
				recvErr <- err
				return
			}
			recvErr <- writeFrame(conn.Stream, replay)
		}()

		conn, err := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if _, err := Initiate(conn.Channel, aSigned, aFP, okVerifier{}, Roster{}); err == nil {
			t.Error("initiator accepted a replayed prior co-signature")
		}
		if err := <-recvErr; err != nil {
			t.Fatalf("malicious peer stub failed: %v", err)
		}

	})
}

func TestSessionRoundTrip(t *testing.T) {
	eachTransport(t, func(t *testing.T, tr transport) {
		aCert, aKey := newIdentity(t) // initiator
		bCert, bKey := newIdentity(t) // receiver
		aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

		aSigned := signAsInitiator(t, aCert, aKey, bFP)

		ln, err := tr.listen("127.0.0.1:0", bCert, bKey, aFP) // B accepts A
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		recvErr := make(chan error, 1)
		go func() {
			conn, err := ln.Accept()
			if err != nil {
				recvErr <- err
				return
			}
			defer conn.Close()
			_, e := Receive(conn.Channel, bCert, bKey, "Alice", confirmer{accept: true, intent: "I accept"}, okVerifier{}, nil, Roster{})
			recvErr <- e
		}()

		conn, err := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second) // A accepts B
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		final, err := Initiate(conn.Channel, aSigned, aFP, okVerifier{}, Roster{})
		if err != nil {
			t.Fatalf("initiate: %v", err)
		}
		if err := <-recvErr; err != nil {
			t.Fatalf("receive: %v", err)
		}

		// The result carries two valid, mutually cross-bound approval signatures.
		ats := ReadAttestations(final)
		if len(ats) != 2 {
			t.Fatalf("want 2 signers, got %d", len(ats))
		}
		for _, a := range ats {
			if !a.Valid {
				t.Errorf("signer %q signature invalid", a.Signer)
			}
			if !a.Matched {
				t.Errorf("signer %q not cross-bound to a real co-signer", a.Signer)
			}
		}

	})
}

func TestSessionReceiverDeclines(t *testing.T) {
	eachTransport(t, func(t *testing.T, tr transport) {
		aCert, aKey := newIdentity(t)
		bCert, bKey := newIdentity(t)
		aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

		aSigned := signAsInitiator(t, aCert, aKey, bFP)

		ln, err := tr.listen("127.0.0.1:0", bCert, bKey, aFP)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		recvErr := make(chan error, 1)
		go func() {
			conn, err := ln.Accept()
			if err != nil {
				recvErr <- err
				return
			}
			defer conn.Close()
			_, e := Receive(conn.Channel, bCert, bKey, "Alice", confirmer{accept: false}, okVerifier{}, nil, Roster{})
			recvErr <- e
		}()

		conn, err := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		// **Which error, not merely an error.** This used to assert only `err != nil`, and
		// that is why the defect it was written to catch survived it: the receiver wrote
		// nothing and closed, so `Initiate` returned `receive co-signed document: EOF` and
		// the assertion passed. The user was shown a 502 that reads as a network fault and
		// invites a retry — for a refusal.
		_, ierr := Initiate(conn.Channel, aSigned, aFP, okVerifier{}, Roster{})
		if !errors.Is(ierr, ErrCoSignDeclined) {
			t.Errorf("initiator got %v; want ErrCoSignDeclined. A decline that reaches the "+
				"peer as EOF is reported as a transport failure and invites the retry a "+
				"refusal must not invite.", ierr)
		}
		if err := <-recvErr; !errors.Is(err, ErrCoSignDeclined) {
			t.Errorf("receiver returned %v; want ErrCoSignDeclined", err)
		}

	})
}

// TestARefusalTellsThePeerWHICHRefusalItWas drives both gates and both refusals.
//
// Two defects, one shape. The consent bridges collapsed "the user said no" and "nobody was
// at the machine" into one `(false, nil)` **before any sentinel existed**, so a peer who
// walked away was reported to the far side as having declined — a false statement about a
// person's decision, sent over the wire. And the co-signature half wrote no receipt at all,
// so its refusal arrived as EOF.
//
// The table is the point: four cases across two flows, and each asserts the sentinel the
// far side must see. A single-case test would pass while the other three collapsed.
func TestARefusalTellsThePeerWHICHRefusalItWas(t *testing.T) {
	cases := []struct {
		name    string
		coSign  bool
		confirm confirmer
		accept  accepterFake
		want    error
	}{
		{"co-sign declined", true, confirmer{accept: false}, accepterFake{}, ErrCoSignDeclined},
		{"co-sign unanswered", true, confirmer{err: ErrConsentTimedOut}, accepterFake{}, ErrConsentTimedOut},
		{"transfer declined", false, confirmer{}, accepterFake{accept: false}, ErrDeclined},
		{"transfer unanswered", false, confirmer{}, accepterFake{err: ErrConsentTimedOut}, ErrConsentTimedOut},
		// P08.S05a: accepted by the human, refused by the disk. The third outcome, and it must
		// reach the sender as ITS OWN sentinel — reporting it as `ErrDeclined` would be the same
		// false statement about a person that `ackTimedOut` was added to stop one gate earlier.
		{"transfer accepted but not stored", false, confirmer{}, accepterFake{err: ErrNotStored}, ErrNotStored},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eachTransport(t, func(t *testing.T, tr transport) {
				aCert, aKey := newIdentity(t)
				bCert, bKey := newIdentity(t)
				aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

				ln, err := tr.listen("127.0.0.1:0", bCert, bKey, aFP)
				if err != nil {
					t.Fatal(err)
				}
				defer ln.Close()

				recvErr := make(chan error, 1)
				go func() {
					conn, aerr := ln.Accept()
					if aerr != nil {
						recvErr <- aerr
						return
					}
					defer conn.Close()
					if tc.coSign {
						_, e := Receive(conn.Channel, bCert, bKey, "Alice", tc.confirm, okVerifier{}, nil, Roster{})
						recvErr <- e
						return
					}
					_, e := ReceiveDocument(conn.Channel, tc.accept, bFP, okVerifier{})
					recvErr <- e
				}()

				conn, err := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()

				var got error
				if tc.coSign {
					_, got = Initiate(conn.Channel, signAsInitiator(t, aCert, aKey, bFP), aFP, okVerifier{}, Roster{})
				} else {
					got = SendDocument(conn.Channel, []byte("%PDF-1.4\nhello"), aFP, okVerifier{}, PeerGatesHuman)
				}
				if !errors.Is(got, tc.want) {
					t.Errorf("the SENDING side saw %v; want %v. This is what the user is told "+
						"happened on the other machine, so a wrong sentinel here is a false "+
						"statement about a person shown in the UI.", got, tc.want)
				}
				// The receiving side must reach the same conclusion, or the two ends of one
				// refusal disagree about what it was.
				if rerr := <-recvErr; !errors.Is(rerr, tc.want) {
					t.Errorf("the RECEIVING side saw %v; want %v", rerr, tc.want)
				}
			})
		})
	}
}

// accepterFake is the transfer gate's Accepter, with the same err seat as confirmer.
type accepterFake struct {
	accept bool
	err    error
}

func (a accepterFake) Accept([]byte, []byte) (bool, error) { return a.accept, a.err }

func TestReadFrameRejectsOversizedDeclaredLength(t *testing.T) {
	// 0xFFFFFFFF declared length, no body — must be rejected before allocating.
	hdr := []byte{0xff, 0xff, 0xff, 0xff}
	if _, err := readFrame(bytes.NewReader(hdr)); err == nil {
		t.Error("readFrame accepted an over-cap declared length")
	}
}
