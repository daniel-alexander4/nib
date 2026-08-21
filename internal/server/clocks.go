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
	// **It is not the arm window, and it does NOT fit inside one.** This used to say it
	// "must stay strictly below" `sessionAcceptTimeout` and cite
	// `TestTheConnectDeadlineFitsInsideThePeersArmWindow` — a test that has never existed
	// anywhere in this tree, guarding a property that is false (both figures are 5 min)
	// and, more to the point, is not the property that matters.
	//
	// The two clocks start at DIFFERENT INSTANTS: the peer arms, the two humans talk, and
	// only then does this side dial. So no inequality between two constants can make a
	// full-length race end inside the peer's window — by the length of the conversation,
	// it cannot. The tail of a long race dials a listener whose timer has already fired,
	// and the user is told "none answered as the pinned peer" about a peer who was there.
	//
	// The fix is criterion 16 — the armed listener's wait bounded by the CEREMONY rather
	// than by a five-minute constant — and it is due in the slice where a signed Record
	// first reaches the server (P05.S04), because until then there is no ceremony deadline
	// to bound it with. Recorded here rather than left as a comparison of two literals,
	// which is what the missing test would have been: green with neither constant read.
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

	// maxCandidatesPerSource bounds what ONE tier may spend of the race's budget.
	//
	// `maxRaceCandidates` above is the law and stays; this is what stops one source
	// spending all of it. A global first-come cap is won by whoever emits fastest, and
	// under D6 an attacker supplies one of the sources — so with two tiers feeding one
	// channel, the flooding one takes all sixteen slots and the genuine peer is never
	// dialled. That is the capture attack `maxLANCandidates` closed at the browse level
	// (`discover.go`), re-opened one layer up.
	//
	// Eight, because it is what each source is already bounded to upstream:
	// `maxLANCandidates` is 8 per browse and `ceremony.MaxCandidates` is 8 per record. So
	// this figure does not narrow any honest source — it only stops a source exceeding
	// the bound it already has, at the one place that can see more than one of them.
	//
	// **P05.S03's acceptance asked for this and shipped without it**, with the ledger
	// reporting only "the size half met"; the defect was invisible because there was one
	// source, and one source makes a global cap and a per-source cap the same thing.
	maxCandidatesPerSource = 8
)
