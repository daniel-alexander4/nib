package p2p

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"strings"
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
//
// `fail` makes `Store` report a persist failure, which is P08.S02's disk-full path: the production
// implementation writes the mirror durably inside `Store`, so an injected error here is the only
// in-process way to drive D24's "signed but not saved" without a real full disk.
type mapReDeliverer struct {
	m    map[string][]byte
	fail error
	// readErr makes `Cached` report UNKNOWN rather than a miss — /pending 320's case. The
	// production implementation returns it when the mirror cannot be read or is damaged, and
	// without a way to drive it here the p2p-side branch is exercised by nothing.
	readErr error
}

func (r *mapReDeliverer) Cached(inbound []byte) ([]byte, error) {
	if r.readErr != nil {
		return nil, r.readErr
	}
	sum := sha256.Sum256(inbound)
	return r.m[string(sum[:])], nil
}
func (r *mapReDeliverer) Store(inbound, final []byte) error {
	sum := sha256.Sum256(inbound)
	// **Cached even when the durable half fails, exactly as production does.** The signature was
	// made and the peer is going to receive it, so a reconnect must re-deliver rather than re-sign
	// — the second block D24 forbids. What failed is only this machine's own copy.
	r.m[string(sum[:])] = final
	return r.fail
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

// TestAFailedPersistStillDeliversAndStillSaysSo — D24 as amended 2026-08-29 (Dan, option A).
//
// # Why the delivery proceeds, which is the part that changed
//
// D24 originally said the delivery is not attempted. That cannot be achieved by withholding the
// frame: `rd.Cached` is consulted BEFORE the consent gate, so an initiator that gets EOF re-races,
// reconnects, hits the cache and is served the document anyway — one reconnect later, with the local
// write still un-retried. Dropping the cache instead would make that reconnect RE-SIGN, producing
// the second block from one identity D24 forbids two bullets above its own disk-full clause.
//
// So the peer receives the signature they are owed — it is real and it was consented to — and the
// signer is told their machine kept no copy. This drives both halves of that at once, because
// either alone is satisfiable by the wrong design: "the document came back" is true of a build that
// never noticed the failure, and "an error was reported" is true of one that withheld the frame.
func TestAFailedPersistStillDeliversAndStillSaysSo(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)
	inbound := signAsInitiator(t, aCert, aKey, bFP)

	calls := 0
	conf := countingConfirmer{intent: "I accept", n: &calls}

	// Stimulus: the identical exchange with a WORKING persist returns no error, so the error below
	// is the injected failure and not something about this fixture.
	okRD := &mapReDeliverer{m: map[string][]byte{}}
	if _, err := coSignExchange(bCert, bKey, aFP, "Alice", inbound, conf, okRD, Roster{}); err != nil {
		t.Fatalf("setup: the same exchange with a working persist failed: %v", err)
	}

	calls = 0
	rd := &mapReDeliverer{m: map[string][]byte{}, fail: errors.New("no space left on device")}
	final, err := coSignExchange(bCert, bKey, aFP, "Alice", inbound, conf, rd, Roster{})

	// **The document exists and is complete.** Withholding it protects nobody: the peer consented,
	// the signature is real, and they will get it on a reconnect regardless.
	if final == nil {
		t.Fatal("a failed persist produced no document — the signature was still made and the " +
			"peer is still owed it; withholding it only delays delivery by one reconnect")
	}
	if n := len(ReadAttestations(final)); n != 2 {
		t.Fatalf("the delivered document carries %d signers, want 2", n)
	}

	// **And the failure is carried out, distinguishably.** Not a refusal — nothing was refused.
	if err == nil {
		t.Fatal("a failed persist was silent — the signer has used their key and their machine " +
			"kept nothing, which is the one thing D24 exists to tell them")
	}
	if !PersistFailed(err) {
		t.Errorf("the failure is not recognisable as a persist failure: %v — the caller has to be "+
			"able to tell it from a refusal, because one means 'save a copy somewhere' and the "+
			"other means 'this did not happen'", err)
	}
	if IsContributionRefusal(err) {
		t.Error("a failed persist reports as a contribution refusal — nothing was refused, and a " +
			"caller would render it as the peer having rejected the signature")
	}
	if !strings.Contains(err.Error(), "do not close Nib") {
		t.Errorf("the error does not carry D24's second clause: %q — 'signed but not saved' names "+
			"the state and 'do not close Nib' is the half that prevents the loss", err)
	}

	// The cache holds it, so a reconnect re-delivers rather than re-signing.
	if got, _ := rd.Cached(inbound); got == nil {
		t.Error("a failed persist left nothing cached — a reconnect would then re-sign, which is " +
			"the second block from one identity D24 forbids")
	}
	if calls != 1 {
		t.Errorf("Confirm was called %d times, want 1", calls)
	}
}
