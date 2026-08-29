package ceremony

import (
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// D33's fourth number, driven past its bound — found missing at P07's close.
//
// # Why this was absent, and why its absence was invisible
//
// C12: "Each of D33's four numbers is enforced on the externally-supplied path, driven by
// supplying a value past the bound. **A test that supplies a value inside the bound cannot see an
// unenforced parameter.**"
//
// Three of the four were driven past: the candidate cap (`gate_test.go` feeds 18 against a cap of
// 8 and asserts `DroppedOverCap`), the punch ceiling (`punch_test.go` spends the cap plus 500),
// and the ceremony-deadline maximum (`record_test.go` signs `MaxCeremonyLife + time.Hour`). The
// roster maximum was enforced at `canonicalRoster` and driven by nothing at all — `ErrRosterTooLarge`
// appeared in no test in the tree.
//
// Every ceremony this repo tests has between two and nine parties, so the check sat one branch
// past every fixture. That is the shape C12's own sentence describes, and the phase close is where
// it was asked.
//
// # Why the cap matters rather than being tidiness
//
// D25 allocates signature pages from the roster length, so an unbounded roster is an unbounded
// page count on a document a convener supplies the roster for. D33 fixes it at 32 — six signature
// pages, "far past any real signing".
func TestARosterPastTheCapIsRefusedAtConvene(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	mk := func(n int) ConveneRequest {
		t.Helper()
		r := ConveneRequest{
			Intent:        "We agree to co-sign the lease",
			Expires:       now.Add(30 * 24 * time.Hour),
			HopBudget:     hopBudget,
			ConvenerSigns: true,
			Roster:        []Party{{Fingerprint: cfp, Label: "Convener", Signs: true}},
		}
		// n TOTAL parties, the convener included — `canonicalRoster` counts what it returns.
		for i := 1; i < n; i++ {
			_, _, fp := identity(t, "P")
			r.Roster = append(r.Roster, Party{Fingerprint: fp, Label: "P", Signs: true})
		}
		return r
	}

	// SETUP: a roster AT the cap convenes. Without this, the refusal below is equally true of a
	// function that refuses every roster, and C12's whole point is that a value inside the bound
	// proves nothing about enforcement.
	//
	// The deadline is the maximum, because `checkRosterText`'s sibling C20 check reserves one hop
	// budget per hop: 31 hops at 30 minutes needs more than a day, and a fixture that tripped THAT
	// bound would report the wrong refusal.
	if _, err := Convene(base, mk(MaxRoster), cert, key, now); err != nil {
		t.Fatalf("a roster of exactly %d parties was refused: %v — this test cannot show a cap "+
			"being enforced if the value inside it is refused too", MaxRoster, err)
	}

	_, err = Convene(base, mk(MaxRoster+1), cert, key, now)
	if err == nil {
		t.Fatalf("a roster of %d parties convened, past D33's maximum of %d. The roster is "+
			"externally supplied and D25 allocates signature pages from its length, so an "+
			"unbounded roster is an unbounded page count on a document somebody else's request "+
			"created", MaxRoster+1, MaxRoster)
	}
	if !errors.Is(err, ErrRosterTooLarge) {
		t.Errorf("the refusal is %v, which is not the cap error a caller can match on — and a "+
			"generic error here is indistinguishable from the half-dozen other ways a convene "+
			"can fail", err)
	}
	// It quotes both numbers, because a convener's action is to remove parties and they need to
	// know how many.
	msg := err.Error()
	if !strings.Contains(msg, "33") || !strings.Contains(msg, "32") {
		t.Errorf("the refusal does not name both the roster size and the limit: %q", msg)
	}
}

// TestTheRosterCapIsTheOneD33Names guards the number itself against a quiet edit, in the same
// shape as the law/tunable guard one package over: the cap is a decision, not a preference.
func TestTheRosterCapIsTheOneD33Names(t *testing.T) {
	if MaxRoster != 32 {
		t.Errorf("MaxRoster is %d; D33 fixes it at 32 — six signature pages, chosen as far past "+
			"any real signing. It is TUNABLE rather than law (D33 as amended), so a change is "+
			"allowed and is a decision somebody makes on purpose", MaxRoster)
	}
}
