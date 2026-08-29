package ceremony

import (
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// C04's MESSAGE clause, found unbuilt by P07's phase-close ledger — and the criterion predicted
// exactly this.
//
// C04 asks for three things: the attempt is refused, "the attempt is refused with a distinct
// message naming a new ceremony as the answer", AND "that message states that the signatures
// already collected cannot be carried into it". Then it says why the third is written separately:
//
//	"The carried-signatures clause is asserted separately because it is the half a builder omits,
//	 being the bad news."
//
// It was omitted. The refusal read "this document is already part of a ceremony" and stopped —
// true, and it tells a convener what is wrong while saying nothing about what it costs them.
//
// # Why the cost is the point rather than politeness
//
// Adding a party changes `rosterHash`, so every invitation already issued fails `MatchesRecord`
// on roster length. A new ceremony is the only way forward and it starts from a document with no
// signatures on it. A convener who remembers the second landlord after three people have signed
// needs that before they start over, not after.
func TestTheAlreadyConvenedRefusalNamesTheAnswerAndItsCost(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	first, err := Convene(base, conveneReq(t, cfp, afp), cert, key, now)
	if err != nil {
		t.Fatalf("setup: the first convene failed: %v", err)
	}
	// SETUP: the second convene is over the CONVENED document, which is the case C04 is about —
	// a convener who has already issued invitations and wants to add somebody.
	_, err = Convene(first.Document, conveneReq(t, cfp, afp), cert, key, now)
	if err == nil {
		t.Fatal("convening twice over one document succeeded, so C04's refusal does not happen at all")
	}
	if !errors.Is(err, ErrAlreadyConvened) {
		t.Fatalf("the refusal is %v, not the already-convened error", err)
	}

	msg := err.Error()
	// Clause 2: a NEW ceremony is named as the answer. "This is refused" leaves a convener with
	// no next action, and the next action is not a corrected field — it is a different act.
	if !strings.Contains(strings.ToLower(msg), "new ceremony") {
		t.Errorf("the refusal does not name a new ceremony as the answer, so a convener is told "+
			"what is wrong and not what to do:\n  %s", msg)
	}
	// Clause 3, asserted separately because it is the half a builder omits: the signatures
	// already collected cannot be carried.
	low := strings.ToLower(msg)
	if !strings.Contains(low, "cannot be carried") && !strings.Contains(low, "sign again") {
		t.Errorf("the refusal does not say that the signatures already collected are lost. C04 "+
			"asserts this separately BECAUSE it is the half a builder omits, being the bad news "+
			"— and a convener who starts a new ceremony without knowing it discovers it after "+
			"asking three people to sign twice:\n  %s", msg)
	}
}
