package server

import (
	"time"

	"nib/internal/p2p"
	"nib/internal/rendezvous"
)

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

	// bootstrapBudget bounds the DHT's first contact with the network.
	//
	// `rendezvous.Bootstrap` takes only a context and has **no internal budget of its own**,
	// unlike Publish, Fetch and ProbeSelf which each cap themselves. The only caller before
	// this was the CLI, which splits a user-supplied figure and says why: "one shared
	// deadline lets the first starve the second". A server has no such caller, so an
	// unbudgeted bootstrap on the connect deadline could consume the whole race before a
	// single candidate was fetched — and the failure would read as "none answered as the
	// pinned peer" about a machine that never finished bootstrapping.
	//
	// It runs AT ARM rather than inside the race, so the table is warm before anybody dials.
	bootstrapBudget = 20 * time.Second
	// lanFirstBudget is how long the link-local tier gets TO ITSELF before the DHT tier opens,
	// and it applies only when the browse actually found the peer on the link (P07.S05d).
	//
	// `browseWindow` is the wrong figure for that case and the measurement says so. It is how long
	// a browse LISTENS; using it as the DHT hold-off makes the criterion a race between a 2 s timer
	// and the hop, and a nine-party LAN relay lost that race often enough to emit 105 off-link
	// packets with the lazy bootstrap already in place. A hop that found its peer on the link
	// completes in 1–3 s wall including consent and signing, and the connection itself in well
	// under a second; 30 s is an order of magnitude past the worst observed hop and a tenth of
	// `connectDeadline`, so the DHT fallback is still comfortably inside the race it belongs to.
	//
	// It is a hold, never a refusal: a LAN candidate that turns out stale still falls through to
	// the DHT tier, which is what keeps this from trading D6's privacy for a ceremony that cannot
	// complete.
	lanFirstBudget = 30 * time.Second

	// portMapBudget is D16's clock-1 for the port-mapping request (PCP then NAT-PMP), 3 s and
	// STRICTLY ITS OWN. The plan-review clock-independence pin forbids implementing one clock
	// in terms of another, and the natural call site (`publishCandidates`) already runs under
	// the 45 s `rendezvousPublishBudget`; using that context for the mapping would both nest
	// the mapping budget inside the publish budget and let a silent gateway eat up to 45 s of
	// publish time. So the mapping call derives cancellation from the arm's context but bounds
	// itself here. (S06.T04.)
	portMapBudget = 3 * time.Second

	// rendezvousPublishBudget mirrors rendezvous.PublishBudget, which bounds a publish AND a
	// fetch. Named here because candidateLife is arithmetic over it and a figure used in a
	// sum should be visible beside the other clocks rather than reached for through another
	// package.
	rendezvousPublishBudget = rendezvous.PublishBudget

	// candidateFetchEvery is the STARTING cadence at which a side re-fetches its peer's
	// record. `rendezvousInterval` steps it down from here; see that function for why.
	//
	// The peer may not have published yet — that is the ordinary case, not a failure — so
	// this is a poll. Five seconds gives roughly a dozen attempts inside `connectDeadline`
	// while leaving the 45 s fetch budget room to be the thing that bounds each one, and it
	// is the cadence D16's "nothing emits at full rate for the whole deadline" is about at
	// this tier. The punch's step-down is S08's and is a different cadence.
	//
	// **It used to say "the dialing side", and that stopped being the whole truth at
	// P05.S09b** (/pending 256): the receive arm runs the same loop bounded by
	// `MaxCeremonyLife` instead of `connectDeadline`, so the same constant that gives a dozen
	// polls in a 300 s race gives hundreds of thousands over thirty days.
	candidateFetchEvery = 5 * time.Second
)

// ceremonyHopBudget is the worst-case wall-clock ONE ceremony hop can consume: bootstrap,
// the connect race, and the whole session.
//
// # Why it lives here, which is not where anyone would look for it
//
// It sums four terms and **no other package can see all four.** `connectDeadline` and
// `bootstrapBudget` are unexported constants in this file; `p2p.SessionBudget()` is derived
// inside the package that arms the deadlines. So `internal/ceremony` cannot compute it, and
// `internal/p2p` cannot either — which is exactly why the two panels that derived a per-hop
// figure at P07's planning both produced a number without checking which package could
// arrive at it. The convene door therefore takes the budget as a PARAMETER rather than
// reaching for it, and this is the one place that fills it in.
//
// The clock-independence pin above forbids implementing one clock in terms of another; it
// does not forbid SUMMING them, which is what `candidateLife` already does over
// `rendezvousPublishBudget`.
//
// # The measurement, and the number it replaces
//
//	bootstrapBudget       20s   the DHT table is warm before anybody dials
//	connectDeadline      300s   the race
//	p2p.SessionBudget()   24m   verification + a 128 MiB write + both of the peer's gates
//	                    -------
//	                     29m20s
//
// P07's plan quotes ~23 minutes for this and that figure is **refuted** — it omits the second
// `exchangeDeadline` arm, the one covering the document write. The direction of C20 stands and
// its arithmetic did not.
func ceremonyHopBudget() time.Duration {
	return bootstrapBudget + connectDeadline + p2p.SessionBudget()
}
