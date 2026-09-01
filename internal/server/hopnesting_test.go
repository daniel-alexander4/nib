package server

import (
	"testing"

	"nib/internal/p2p"
)

// TestTheHopBudgetNestsTheReceiversWorstCaseLag — P08.S04a's arithmetic, made falsifiable.
//
// # Why this exists
//
// P08.S04a's receiver-side deadline check reserves NOTHING: `checkArrival` refuses only a deadline
// already past. That is only safe if the convener's own admission rule already implies it — the
// convener admits a hop when `Expires > t0 + ceremonyHopBudget()` — unchanged by P08.S05b, whose
// delivery term is reserved at Convene and only makes the up-front deadline larger. The signer's gate runs at
// worst `t0 + ReceiveArrivalLag() + bootstrap + connect`. If that sum ever exceeds the hop budget,
// a hop the convener CORRECTLY admitted is refused at the far end, for arithmetic reasons, with no
// party at fault.
//
// **The arithmetic was got wrong twice before it was re-derived at the line.** A deepdive summed
// `20s + 300s + 360s` = 11m20s by counting one of `Receive`'s two pre-gate `exchangeDeadline` arms
// and omitting the spoken-check gate entirely; its grill corrected the omission and then miscounted
// `Receive`'s arms as three. Neither error was visible to any test, because the budget was a number
// in a comment. This is the check that makes it a number in a build.
//
// # What the surplus IS
//
// The surplus is not slack — it is **the tolerable clock skew between two machines with no protocol
// time sync**. Nothing in the ceremony synchronises clocks, so a signer whose clock runs fast by
// more than this refuses an honest hop, and that is the residual risk this figure bounds. Reserving
// a budget at the receiver spends it: at eight minutes the tolerance is MINUS one minute.
func TestTheHopBudgetNestsTheReceiversWorstCaseLag(t *testing.T) {
	hop := ceremonyHopBudget()
	lag := bootstrapBudget + connectDeadline + p2p.ReceiveArrivalLag()

	// STIMULUS: both sides must be non-zero, or the inequality below is satisfied by a constant
	// that has been deleted rather than by a budget that nests.
	if hop <= 0 || lag <= 0 {
		t.Fatalf("setup: hop=%s lag=%s — one of these constants is gone, and the comparison "+
			"below would pass over nothing", hop, lag)
	}

	surplus := hop - lag
	if surplus < 0 {
		t.Errorf("the hop budget (%s) does NOT cover the receiver's worst-case lag (%s): a hop the "+
			"convener correctly admitted is refused at the signer's gate by %s, for arithmetic "+
			"reasons, with neither party at fault.\n"+
			"  bootstrap %s + connect %s + ReceiveArrivalLag %s = %s\n"+
			"  ceremonyHopBudget = %s",
			hop, lag, -surplus, bootstrapBudget, connectDeadline, p2p.ReceiveArrivalLag(), lag, hop)
	}
	// The figure is printed rather than asserted at a literal: it is a DERIVED tolerance and
	// pinning it would make every deadline change look like a skew regression. What must not
	// change silently is its sign.
	t.Logf("clock-skew tolerance between convener and signer: %s "+
		"(hop budget %s − worst-case lag %s). Nothing measures real skew on this path.",
		surplus, hop, lag)
}
