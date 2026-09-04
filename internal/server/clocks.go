package server

import (
	"context"
	"time"

	"nib/internal/ceremony"
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
	// **DERIVED, not eight (P07.S09a, D33's discharge).** It is what each source is already
	// bounded to upstream — `maxLANCandidates` is 8 per browse and `ceremony.MaxCandidates` is
	// 8 per record — so this figure does not narrow any honest source; it only stops a source
	// exceeding the bound it already has, at the one place that can see more than one of them.
	//
	// It was written as a bare `8` with that sentence beside it, which put **D33's law figure
	// inside the tunable block by hand-copy**. D33's discharge is a guard that fails "if either
	// law figure is reachable from the tunable block", and a literal an operator can edit here
	// is precisely that: raise `ceremony.MaxCandidates` and this silently stays behind, capping
	// an honest DHT source below the bound its own record is allowed. The comment claiming the
	// two agree is not a mechanism, and `NominalBlockRect` is this repo's standing example of
	// what a hand-copy plus a comment costs.
	//
	// `max` rather than either one: the rule is "no source may exceed the bound it already has",
	// so the cap has to admit the most generous honest source. If the two diverge, taking the
	// smaller would silently narrow the other — the same defect one direction over.
	//
	// **P05.S03's acceptance asked for this and shipped without it**, with the ledger
	// reporting only "the size half met"; the defect was invisible because there was one
	// source, and one source makes a global cap and a per-source cap the same thing.
	maxCandidatesPerSource = max(maxLANCandidates, ceremony.MaxCandidates)

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

	// closeOutGrace is how long a ceremony's directory stays live past its own deadline before
	// the sweep concludes nothing more is coming and closes it out (D29, P08.S06).
	//
	// **DERIVED from `ceremony.MaxCeremonyLife`, not a hand-copied duration.** This is the
	// `maxCandidatesPerSource` rule above, for the same reason: a literal here plus a comment
	// saying it agrees with the ceiling is not a mechanism, and if the ceiling moves this
	// silently stays behind — closing out ceremonies the record still considers live.
	//
	// **It is a tunable and NOT one of D33's law figures.** That guard's `lawFigures` list is a
	// deliberate two-name whitelist and this must not join it: a law figure is one a peer relies
	// on, and nothing on the wire depends on how long this machine keeps a directory.
	//
	// A tenth, which is 3 days against the 30-day ceiling. What the grace has to cover is the
	// delivery round starting AFTER the deadline — the convener's own arm runs to
	// `MaxCeremonyLife` and a round that begins at the last moment still has to walk N parties
	// at `connectDeadline` each — plus the machine being off. Three days covers a long weekend,
	// which is the outage the sweep's own trigger rule is written around: every ceremony route
	// is behind `requireUnlocked`, so a machine left locked prunes nothing while wall time runs.
	closeOutGrace = ceremony.MaxCeremonyLife / 10
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

// ceremonyDeliveryLegBudget is the worst-case wall-clock ONE leg of the delivery round can
// consume, and it is the same three terms as a hop for the same reason (P08.S05b).
//
//	bootstrapBudget                              20s   the DHT table is warm before anybody dials
//	connectDeadline                             300s   the race to reach the party
//	p2p.DeliveryLegBudget(PeerGatesUnattended)   14m   what SendDocument arms on this leg
//	                                          -------
//	                                           19m20s
//
// **It was equal to `ceremonyHopBudget()` and is no longer, which is why it was kept separate.**
// The two matched only because the transfer path and the co-sign path armed the same three
// deadlines; they are different rules about different functions, and P08.S05d shrank the p2p half
// of this one by making both of the leg's gates non-interactive — 29m20s to 19m20s. A caller that
// had reserved `ceremonyHopBudget()` for a delivery leg would now be over-reserving by ten minutes
// a leg while restating a rule it cannot see, which is the shape `p2p.SessionBudget`'s own doc
// grades critical.
//
// **The gates are named at the call, not assumed.** `PeerGatesUnattended` is a claim about the far
// side, and it is true here because the round arms that side itself (S05g). Passing it where the
// receiver is a person would reserve ten minutes less than the leg can spend.
//
// **Why it is not the 7m20s the slice was firmed with.** That figure was
// `bootstrapBudget + connectDeadline + postConsentDeadline`, on the reasoning that a delivery leg
// runs no `Confirmer` so it carries none of the consent budget. A non-interactive verifier removes
// the local WAIT and removes nothing from any deadline the code ARMS — and `ReceiveDocument`'s
// `Accepter` is still a five-minute human gate on the production path. It was short by 22 minutes
// per leg, against a rule this package already states: the outer clock reserves the inner one's
// worst case rather than merely exceeding it.
func ceremonyDeliveryLegBudget() time.Duration {
	return bootstrapBudget + connectDeadline + p2p.DeliveryLegBudget(p2p.PeerGatesUnattended)
}

// The re-race pacing (/pending 369). `runCeremonyReceive`'s pre-signing arm re-races on a
// transport loss and its post-signing arm retries a failed promote, and until v1.117.353 both were
// a bare `continue` — no delay, no attempt count, no bound on how many times either could go round.
//
// **The item that filed this asked for the RATE to be measured first, and a bound removes the
// question instead.** The measurement it wanted needs a counterpart that completes the p2p
// handshake and drops it repeatedly, which is a tier-4 harness this repo does not have; meanwhile
// the loop's continue condition is a REMOTE peer failing, so how fast it spins was never ours to
// know. A cap answers it by construction: once warmed, the arm cannot attempt more often than
// `reraceCap`, whatever the peer does. That is the same shape as `maxRaceCandidates` one level out
// — D16's pin says "the backoff bounds how fast the race emits and nothing bounded how much", and
// nothing bounded how often the race is RE-ENTERED.
const (
	// reraceBase is the first wait, and it is deliberately small enough to be free for the
	// honest case: a peer that dropped mid-handshake needs longer than this to come back, so a
	// legitimate re-race is not slowed by it in any way a person could perceive.
	reraceBase = 50 * time.Millisecond
	// reraceCap is the ceiling. Two seconds bounds a flapping peer to half an attempt per second
	// for the rest of the ceremony's life — which under `ceremony.MaxCeremonyLife` is up to
	// thirty days, and is the reason a ceiling and not merely a delay.
	reraceCap = 2 * time.Second
)

// reraceWait decides whether a lost channel is re-raced, and how long to wait first.
//
// **One door for both arms (ADR-009), and it carries the DEADLINE check as well as the delay.**
// Those two were separate before — the pre-signing site tested `time.Now().Before(deadline)` inline
// and the post-signing site tested nothing at all — and splitting them is how one site gets a bound
// the other does not. A caller asks one question and gets one answer.
//
// `attempt` is how many re-races have already happened on this arm, so the first is 0.
//
// The wait never overshoots the deadline: a delay that outlived the window it is pacing would turn
// a bounded arm into one that sleeps past its own end and wakes to discover it. Where the remaining
// time is shorter than the backoff, the caller gets what is left; where nothing is left, it gets
// `false` and stops.
func reraceWait(attempt int, now, deadline time.Time) (time.Duration, bool) {
	remaining := deadline.Sub(now)
	if remaining <= 0 {
		return 0, false
	}
	d := reraceBase
	for i := 0; i < attempt && d < reraceCap; i++ {
		d *= 2
	}
	if d > reraceCap {
		d = reraceCap
	}
	if d > remaining {
		d = remaining
	}
	return d, true
}

// sleepOrDone waits d, or returns false the moment ctx is done.
//
// **A bare `time.Sleep` in a retry loop is a cancel the arm does not honour**: a disarmed or
// shut-down session would keep the goroutine alive for the rest of the wait, and under `reraceCap`
// that is two seconds per attempt at teardown. The timer is stopped either way.
func sleepOrDone(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
