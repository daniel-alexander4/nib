package server

import "time"

// D16's clocks, and the two figures that are law rather than tuning.
//
// # Why this block exists at all
//
// D16 fixes the ladder's budgets in a table and says of it: *"named together in one constant
// block and tunable, not law — the structure above is the law"*. Until P05.S03 no such block
// existed in Go; the only dial budgets in the tree were `lanDialBudget` and `lanDialTimeout`,
// both sized for a serial walk over one tier's candidates, and D16's 300 s connect deadline
// had no constant anywhere. A criterion that says "letting the connect deadline elapse in
// full" cannot be tested against a number that does not exist.
//
// # What is NOT here, deliberately
//
// D16's table also names budgets for the port-mapping request, the DHT self-address probe,
// the candidate fetch and the punch cadence. Those belong to S06, S04 and S08, and writing
// them now would put four constants in the tree with no reader — the dead-counter defect this
// repo has already deleted once (`lanAnnouncer`'s Sent/Failed). Each slice adds its own to
// this block when it has something that reads it.
const (
	// connectDeadline is D16's clock 2: one budget for the whole race for a channel.
	//
	// **300 s is Dan's call (D16, 2026-08-16)** and the reasoning is human rather than
	// technical: two people are on a phone call when this runs, and a ceremony they
	// scheduled is worth more than a fast failure.
	//
	// It is NOT the arm window and must stay strictly below it — see
	// `TestTheConnectDeadlineFitsInsideThePeersArmWindow`.
	connectDeadline = 300 * time.Second

	// maxRaceCandidates bounds the DISTINCT candidates one race may dial, across every
	// tier. **Law, not tuning** (D16's plan-review pin, 2026-08-17): the backoff bounds how
	// fast the race emits and nothing bounded how much, and under D6 an attacker supplies
	// the candidates. Exceeding it drops the excess and reports it rather than failing the
	// ceremony.
	//
	// Sixteen rather than eight, and the reason is that eight is already spoken for twice:
	// `maxLANCandidates` bounds what ONE browse may hand over and `ceremony.MaxCandidates`
	// bounds what ONE record may carry. A per-ceremony cap equal to either would let the
	// first tier to answer consume the whole budget and starve the rest — which is the
	// per-source reservation problem stated as a number.
	maxRaceCandidates = 16

	// maxConcurrentDials bounds how many dials are in flight at once.
	//
	// Eight, against the peer's `maxConcurrentHandshakes` of 16: our own racer must not be
	// able to occupy a peer's whole handshake pool, and half is a margin that survives a
	// second Nib dialling the same peer. It is a concurrency bound and NOT a cap on how many
	// candidates are tried — `maxRaceCandidates` is that, and conflating the two produces
	// refusals a user cannot act on (ADR-005's own warning).
	maxConcurrentDials = 8
)
