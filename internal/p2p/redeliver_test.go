package p2p

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

// countingConfirmer accepts and counts how many times consent was asked — a re-delivery must ask
// zero further times.
type countingConfirmer struct {
	intent string
	n      *int
}

func (c countingConfirmer) Confirm(SignerAttestation, []byte) (bool, string, []byte, error) {
	*c.n++
	return true, c.intent, nil, nil
}

// mapReDeliverer is a test cache keyed on sha256(inbound), the same key the server's ceremonyID uses.
type mapReDeliverer struct{ m map[string][]byte }

func (r *mapReDeliverer) Cached(inbound []byte) []byte {
	sum := sha256.Sum256(inbound)
	return r.m[string(sum[:])]
}
func (r *mapReDeliverer) Store(inbound, final []byte) {
	sum := sha256.Sum256(inbound)
	r.m[string(sum[:])] = final
}

// TestReDeliveryIsIdempotent — P05.S10 T05, the crux. Co-signing the SAME inbound twice through a
// ReDeliverer must produce the SAME bytes and ask consent only ONCE: the first is a real co-sign
// (Confirm + Contribute + cache), the second a re-delivery of the cached result. Contribute is
// non-deterministic (random ECDSA nonce + a wall-clock timestamp), so a design that re-SIGNED would
// hand back different bytes — a second signature block, which D25 forbids and D24 exists to prevent.
func TestReDeliveryIsIdempotent(t *testing.T) {
	aCert, aKey := newIdentity(t) // Alice, the initiator whose signed document arrives
	bCert, bKey := newIdentity(t) // Bob, the receiver co-signing
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)
	inbound := signAsInitiator(t, aCert, aKey, bFP) // Alice's signed doc accepting Bob

	calls := 0
	conf := countingConfirmer{intent: "I accept", n: &calls}
	rd := &mapReDeliverer{m: map[string][]byte{}}

	final1, err := coSignExchange(bCert, bKey, aFP, "Alice", inbound, conf, rd, Roster{})
	if err != nil {
		t.Fatalf("first co-sign: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Confirm called %d times on the first co-sign, want 1", calls)
	}
	if n := len(ReadAttestations(final1)); n != 2 {
		t.Fatalf("first result has %d signers, want 2", n)
	}

	// The reconnect: the SAME inbound again.
	final2, err := coSignExchange(bCert, bKey, aFP, "Alice", inbound, conf, rd, Roster{})
	if err != nil {
		t.Fatalf("re-delivery: %v", err)
	}
	if calls != 1 {
		t.Errorf("Confirm was asked again on a re-delivery (now %d) — the user was re-prompted to "+
			"consent to a document they already signed", calls)
	}
	if !bytes.Equal(final1, final2) {
		t.Errorf("re-delivery produced DIFFERENT bytes than the first co-sign — it re-SIGNED instead " +
			"of re-delivering the cached signature, stacking a second block (D25/D24)")
	}
	if n := len(ReadAttestations(final2)); n != 2 {
		t.Errorf("re-delivered result has %d signers, want 2", n)
	}
}

// TestReDelivererMissRunsAFreshExchange — a DIFFERENT inbound at the same receiver is a cache miss:
// it consents and signs fresh (the per-distinct-inbound invariant, grill P2), never re-delivering
// the wrong document's signature.
func TestReDelivererMissRunsAFreshExchange(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)
	in1 := signAsInitiator(t, aCert, aKey, bFP)
	in2 := signAsInitiator(t, aCert, aKey, bFP) // a second, distinct signed document (fresh nonce/time)

	if bytes.Equal(in1, in2) {
		t.Skip("the two inbounds are byte-identical; cannot distinguish a miss")
	}
	calls := 0
	conf := countingConfirmer{intent: "I accept", n: &calls}
	rd := &mapReDeliverer{m: map[string][]byte{}}

	if _, err := coSignExchange(bCert, bKey, aFP, "Alice", in1, conf, rd, Roster{}); err != nil {
		t.Fatal(err)
	}
	if _, err := coSignExchange(bCert, bKey, aFP, "Alice", in2, conf, rd, Roster{}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("Confirm called %d times for two DISTINCT documents, want 2 — a distinct inbound "+
			"must be a cache miss that consents and signs fresh, not a re-delivery", calls)
	}
}
