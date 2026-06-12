package p2p

import (
	"bytes"
	"crypto/tls"
	"encoding/hex"
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
}

func (c confirmer) Confirm(SignerAttestation, []byte) (bool, string, []byte, error) {
	return c.accept, c.intent, nil, nil
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
	if err := confirmCoSigned(mutual, bFP, aFP); err != nil {
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
	if err := confirmCoSigned(bOnly, bFP, aFP); err == nil {
		t.Error("accepted a reply missing the initiator's own signature")
	}
}

func TestSessionRoundTrip(t *testing.T) {
	aCert, aKey := newIdentity(t) // initiator
	bCert, bKey := newIdentity(t) // receiver
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	aSigned := signAsInitiator(t, aCert, aKey, bFP)

	ln, err := Listen("127.0.0.1:0", bCert, bKey, aFP) // B accepts A
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
		_, e := Receive(conn.(*tls.Conn), bCert, bKey, "Alice", confirmer{accept: true, intent: "I accept"})
		recvErr <- e
	}()

	conn, err := Dial(ln.Addr().String(), aCert, aKey, bFP, 5*time.Second) // A accepts B
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	final, err := Initiate(conn, aSigned, aFP)
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
}

func TestSessionReceiverDeclines(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	aSigned := signAsInitiator(t, aCert, aKey, bFP)

	ln, err := Listen("127.0.0.1:0", bCert, bKey, aFP)
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
		_, e := Receive(conn.(*tls.Conn), bCert, bKey, "Alice", confirmer{accept: false})
		recvErr <- e
	}()

	conn, err := Dial(ln.Addr().String(), aCert, aKey, bFP, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := Initiate(conn, aSigned, aFP); err == nil {
		t.Error("initiator got a result though the receiver declined")
	}
	if err := <-recvErr; err == nil {
		t.Error("receiver returned nil though it declined")
	}
}

func TestReadFrameRejectsOversizedDeclaredLength(t *testing.T) {
	// 0xFFFFFFFF declared length, no body — must be rejected before allocating.
	hdr := []byte{0xff, 0xff, 0xff, 0xff}
	if _, err := readFrame(bytes.NewReader(hdr)); err == nil {
		t.Error("readFrame accepted an over-cap declared length")
	}
}
