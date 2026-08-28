package p2p

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// P07.S07b: inside a ceremony the recital is the RECORD's, and the party's own answer is
// discarded.
//
// # The clause, and why it is about discarding rather than about copying
//
// D20 makes the recital one sentence with one home, and C15 says every signature block carries it
// verbatim. The failure that wording guards against is not "the recital is missing" — it is N
// signatures agreeing on a commitment and disagreeing about what they agreed to, which is what a
// per-hop typed intent produces. Every party's consent screen has an intent box, and on the
// ceremony path what they type there must reach nothing.
//
// So the assertions below drive a `Confirmer` that returns a DIFFERENT sentence and one that
// returns nothing, and both must produce the record's. A test that drove a confirmer returning
// the right answer could not tell the discard from a coincidence.

// intentConfirmer returns whatever it is told to, standing in for a user typing into the consent
// screen's intent box.
type intentConfirmer struct{ intent string }

func (c intentConfirmer) Confirm(SignerAttestation, []byte) (bool, string, []byte, error) {
	return true, c.intent, nil, nil
}

const recital = "We agree to be bound by the lease of 14 Elm Row"

// ceremonyRoster is a two-party ceremony roster carrying a recital.
func ceremonyRoster(a, b l3Party) Roster {
	r := l3Roster(a, b)
	r.Commitment = strings.Repeat("cd", 32)
	r.CommitmentVersion = 4
	r.Intent = recital
	return r
}

// signedReason drives one receiving-side hop and returns the /Reason the local party signed.
func signedReason(t *testing.T, c Confirmer, roster Roster) string {
	t.Helper()
	a, b := l3Identity(t, "A"), l3Identity(t, "B")
	if len(roster.Entries) == 0 {
		roster = ceremonyRoster(a, b)
	} else {
		roster = withParties(roster, a, b)
	}
	aFP, _ := hex.DecodeString(a.fp)
	inbound := l3Chain(t, l3Prepared(t), []l3Party{a}, []l3Party{b}, "")
	signed, err := coSignExchange(b.cert, b.key, aFP, "A", inbound, c, nil, roster)
	if err != nil {
		t.Fatalf("the hop was refused: %v", err)
	}
	ats := ReadAttestations(signed)
	if len(ats) != 2 {
		t.Fatalf("setup: the exchange produced %d signature(s), want 2 — the local party did "+
			"not sign, so there is no /Reason to read", len(ats))
	}
	return ats[len(ats)-1].Reason
}

// withParties re-points a caller-supplied roster at the identities this hop actually uses.
func withParties(r Roster, a, b l3Party) Roster {
	base := l3Roster(a, b)
	base.Commitment, base.CommitmentVersion, base.Intent = r.Commitment, r.CommitmentVersion, r.Intent
	return base
}

func TestAPartysTypedIntentIsDiscardedInsideACeremony(t *testing.T) {
	const typed = "I am only witnessing this and agree to nothing"
	reason := signedReason(t, intentConfirmer{intent: typed}, Roster{})

	if strings.Contains(reason, typed) {
		t.Errorf("the signed /Reason carries what this party typed:\n  %s\n\nInside a ceremony "+
			"the recital has one home (D20). A hop that signs its own sentence produces N "+
			"signatures agreeing on a commitment and disagreeing about what they agreed to — "+
			"and this one would have signed a denial of the obligation everyone else accepted.",
			reason)
	}
	if !strings.Contains(reason, recital) {
		t.Errorf("the signed /Reason does not carry the ceremony's recital:\n  %s\n\nwant it to "+
			"contain %q verbatim (C15)", reason, recital)
	}
}

func TestDefaultIntentIsUnreachableInsideACeremony(t *testing.T) {
	// A Confirmer returning "" is the ordinary shape of the bug: the consent screen's intent box
	// left blank. Outside a ceremony `intent()` fills it with `defaultIntent`, which is right —
	// there is no proceeding to have a recital. Inside one it would be Nib inventing the sentence
	// the parties are bound by.
	reason := signedReason(t, intentConfirmer{intent: ""}, Roster{})

	if strings.Contains(reason, defaultIntent) {
		t.Errorf("the signed /Reason carries Nib's default sentence inside a ceremony:\n  %s\n\n"+
			"`defaultIntent` is %q — the recital is inside the commitment every other signature "+
			"carries, and this one made one up", reason, defaultIntent)
	}
	if !strings.Contains(reason, recital) {
		t.Errorf("the signed /Reason does not carry the recital:\n  %s", reason)
	}
}

// TestOutsideACeremonyTheTypedIntentIsStillTheIntent is the control, and it is the assertion the
// two above would be satisfied by deleting. An ordinary two-party co-sign has no record, no
// recital and no proceeding — what the user types IS the intent there, and always has been.
func TestOutsideACeremonyTheTypedIntentIsStillTheIntent(t *testing.T) {
	const typed = "I accept this quotation"
	a, b := l3Identity(t, "A"), l3Identity(t, "B")
	aFP, _ := hex.DecodeString(a.fp)
	inbound := l3Chain(t, l3Prepared(t), []l3Party{a}, []l3Party{b}, "")
	signed, err := coSignExchange(b.cert, b.key, aFP, "A", inbound,
		intentConfirmer{intent: typed}, nil, Roster{})
	if err != nil {
		t.Fatalf("the ordinary two-party co-sign was refused: %v", err)
	}
	ats := ReadAttestations(signed)
	reason := ats[len(ats)-1].Reason
	if !strings.Contains(reason, typed) {
		t.Errorf("a two-party co-sign no longer carries what the signer typed:\n  %s", reason)
	}
}

// TestACeremonySignatureWithNoRecitalIsRefusedRatherThanDefaulted is what makes the clause
// "`defaultIntent` is unreachable when a record is present" true by construction rather than by
// the roster always happening to carry one.
//
// The reachable cause is an invitation older than `Invitation.Intent`, or one assembled by hand.
// Refusing is the fail-closed direction: the alternative is a signature reading "I agree to sign
// this document." over a proceeding whose recital every other signature carries.
func TestACeremonySignatureWithNoRecitalIsRefusedRatherThanDefaulted(t *testing.T) {
	a, b := l3Identity(t, "A"), l3Identity(t, "B")
	roster := ceremonyRoster(a, b)
	roster.Intent = "" // an invitation that predates the field

	aFP, _ := hex.DecodeString(a.fp)
	inbound := l3Chain(t, l3Prepared(t), []l3Party{a}, []l3Party{b}, "")
	_, err := coSignExchange(b.cert, b.key, aFP, "A", inbound,
		intentConfirmer{intent: "whatever I typed"}, nil, roster)
	if err == nil {
		t.Fatal("a ceremony hop with no recital signed anyway — the /Reason would carry Nib's " +
			"own default sentence over a proceeding whose recital is inside the commitment")
	}
	if !errors.Is(err, ErrNoCeremonyIntent) {
		t.Errorf("the refusal is %v, which is not the named error a caller can match on", err)
	}
}
