package server

import (
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/rendezvous"
)

// TestAPublishedRecordOutlivesThePeersRace — P05.S04 T12, and the arithmetic is the point.
//
// The acceptance says the expiry is "the connect deadline PLUS MARGIN", and the one prior
// example in the tree is the anti-pattern the plan names: `nib rendezvous --self-test` uses
// `now + 5 minutes`, which is exactly `connectDeadline` with zero margin.
//
// **Zero margin is not merely tight, it is short by about 90 seconds.** The record must still
// be valid when the PEER verifies it, and two rendezvous budgets bracket the race: our publish
// traversal can take up to `PublishBudget` before the record is anywhere, and the peer's last
// fetch can take another before it is read. `Verify` checks `now.Before(c.Expires)` on every
// `gate.Accept`, so an expired record is refused mid-race and reads to the user as a peer who
// never published.
func TestAPublishedRecordOutlivesThePeersRace(t *testing.T) {
	life := candidateLife()

	// The floor: publish traversal + the peer's whole race + the peer's final fetch.
	floor := rendezvous.PublishBudget + connectDeadline + rendezvous.PublishBudget
	if life <= floor {
		t.Errorf("a published record claims %s of life against a floor of %s "+
			"(%s publish + %s race + %s fetch). It expires while the peer is still reading "+
			"it, and the peer sees a counterparty who never published.",
			life, floor, rendezvous.PublishBudget, connectDeadline, rendezvous.PublishBudget)
	}

	// **And the anti-pattern by name**, so a later edit cannot quietly arrive back at it.
	if life <= connectDeadline {
		t.Errorf("the record's life (%s) does not exceed the connect deadline (%s) — that is "+
			"the self-test's `now + 5 minutes` with the margin the acceptance asks for still "+
			"missing", life, connectDeadline)
	}

	// The ceiling is the READER's, and it is the only thing that caps a publisher's
	// generosity: `Verify` refuses anything claiming more than MaxCandidateLife ahead. A
	// record we publish and every peer refuses is worse than none.
	if life >= ceremony.MaxCandidateLife {
		t.Errorf("a published record claims %s of life, at or past the reader-side ceiling of "+
			"%s — every peer would refuse it as ErrCandidateExpired",
			life, ceremony.MaxCandidateLife)
	}

	// Clock skew between the two machines is real on this path — D19's fifth cause exists
	// because of it — so the margin must be more than the two budgets alone.
	if life < floor+time.Minute {
		t.Errorf("the margin over the floor is %s, which leaves nothing for the clock "+
			"disagreement D19 cause 5 is about", life-floor)
	}
}
