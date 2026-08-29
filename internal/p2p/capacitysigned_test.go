package p2p

import (
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
)

// C19's SECOND half, found unbuilt by P07's phase-close ledger.
//
// The criterion: "A four-party ceremony convened with two distinct capacities produces a document
// whose blocks render each party's OWN capacity, AND whose signed `/Reason`s differ in capacity
// while carrying one identical recital."
//
// P07.S07a built the rendering half. P07.S07b listed the signed half as an acceptance bullet and
// did not build it — so the capacity reached the block, which a reader can SEE, and never the
// signed text, which is what a dispute RELIES ON. Nothing failed, because every test asked about
// the block.
//
// # Why the two halves are not the same claim
//
// `Contribute` takes an already-rasterised appearance from its caller and nothing compares it to
// the /Reason — `Attestation`'s own doc says so, and says the comment that once claimed otherwise
// "was a promise nothing kept". A capacity appearing only on the block is therefore a claim about
// a party's AUTHORITY that their key never signed. D20's capacity amendment puts capacity inside
// the commitment for exactly that reason: "it is what a third party relies on".
func TestTwoCapacitiesDifferInTheSignedReasonAndShareOneRecital(t *testing.T) {
	const (
		recital = "We agree to be bound by the lease of 14 Elm Row"
		cap0    = "as Director of Acme Ltd"
		cap2    = "as Guarantor"
	)
	r, parties := labelledRoster(t, 4, map[int]string{0: cap0, 2: cap2})
	r.Intent = recital

	reasons := make([]string, len(parties))
	for i, p := range parties {
		att := Attestation{When: time.Now()}
		StampCommitment(&att, r, p.fp)
		reasons[i] = att.reason()
	}

	// SETUP: the recital really is on every one of them, or "one identical recital" is a claim
	// about a field nobody set.
	for i, got := range reasons {
		if !strings.Contains(got, recital) {
			t.Fatalf("setup: party %d's /Reason does not carry the recital at all:\n  %s", i, got)
		}
	}

	// The capacities DIFFER, in the signed text.
	if !strings.Contains(reasons[0], cap0) {
		t.Errorf("party 0 signs %q and their /Reason does not say so:\n  %s\n\nThe block shows it "+
			"and the signature does not, so the only copy of that authority claim is a picture "+
			"nothing verifies against the signed text", cap0, reasons[0])
	}
	if !strings.Contains(reasons[2], cap2) {
		t.Errorf("party 2's /Reason does not carry %q:\n  %s", cap2, reasons[2])
	}
	if strings.Contains(reasons[0], cap2) || strings.Contains(reasons[2], cap0) {
		t.Errorf("the two capacities are not distinct in the signed text — one party's authority "+
			"claim appears over the other's signature:\n  %s\n  %s", reasons[0], reasons[2])
	}
	// A party with NO capacity signs no capacity clause. An empty one is the ordinary case, and a
	// dangling "Capacity: ." over somebody's signature is a claim they did not make.
	for _, i := range []int{1, 3} {
		if strings.Contains(reasons[i], "Capacity:") {
			t.Errorf("party %d has no capacity and their signed /Reason carries a capacity "+
				"clause:\n  %s", i, reasons[i])
		}
	}

	// And ONE identical recital across all four, which is C15 and is the half a per-party field
	// could have broken: the recital must not have become per-party on the way.
	for i, got := range reasons {
		if strings.Count(got, recital) != 1 {
			t.Errorf("party %d's /Reason carries the recital %d times, want exactly 1:\n  %s",
				i, strings.Count(got, recital), got)
		}
	}
}

// TestTheCapacityClauseDoesNotDisturbWhatAlreadyParses is the compatibility half: /Reason is a
// parsed format, and a new clause on the end must not move the tokens the readers find.
func TestTheCapacityClauseDoesNotDisturbWhatAlreadyParses(t *testing.T) {
	r, parties := labelledRoster(t, 3, map[int]string{1: "as Attorney under a power of attorney"})
	r.Intent = "We agree"
	att := Attestation{AcceptedPeer: strings.Repeat("ab", 32), AcceptedPeerLabel: "Peer", When: time.Now()}
	StampCommitment(&att, r, parties[1].fp)

	st := sign.Status{Signers: []sign.SignerInfo{
		{Name: "P", Fingerprint: parties[1].fp, Valid: true, Reason: att.reason()},
	}}
	got := Attestations(st, Proceeding{Commitment: r.Commitment})
	if len(got) != 1 {
		t.Fatalf("setup: %d attestations", len(got))
	}
	if got[0].AcceptedPeer != strings.Repeat("ab", 32) {
		t.Errorf("the SPKI token no longer parses with a capacity clause present: %q", got[0].Reason)
	}
	if !strings.EqualFold(got[0].RosterHash, r.Commitment) {
		t.Errorf("the roster token no longer parses with a capacity clause present: %q", got[0].Reason)
	}
	if got[0].TagVersion != attestationTagVersion {
		t.Errorf("the tag version no longer parses: %d", got[0].TagVersion)
	}
}
