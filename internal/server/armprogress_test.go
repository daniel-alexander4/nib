package server

import (
	"testing"
	"time"
)

// P06.S05 — what each tier of the connection ladder is doing, while it is doing it.

// TestTheArmProgressIsNotGatedOnTheBootstrap is the criterion's own defect.
//
// **The blank spinner was not hypothetical — it was the shipped state, and its cause was a gate.**
// `sessionStatus` fills `Diagnosis` only `if cer != nil && !inSession && cer.bootstrapDone.Load()`.
// That is right for a VERDICT: a cause computed before the DHT has had its chance would accuse the
// wrong tier. But under ADR-011 nothing bootstraps until the local link has had its window, and
// where a browse has answered that hold is `lanFirstBudget` — thirty seconds — so the product was
// deliberately silent for the longest stretch of an arm. D16 says that window must never be a
// blank spinner.
//
// So this drives the state the diagnosis cannot speak in: watching the link, bootstrap NOT done.
func TestTheArmProgressIsNotGatedOnTheBootstrap(t *testing.T) {
	cer := &ceremonyID{}
	cer.watchingLink(time.Now())

	// SETUP: the bootstrap really has not run, or this is not the window under test and the
	// assertion below would pass in a state the diagnosis could already describe.
	if cer.bootstrapDone.Load() {
		t.Fatal("setup: the bootstrap is already done, so this is not the pre-bootstrap window " +
			"the criterion is about")
	}

	p := cer.armProgressOf(time.Now())
	if p == nil {
		t.Fatal("an arm watching the link reports NOTHING before the bootstrap. That is the blank " +
			"spinner D16 forbids, and it is the longest part of a LAN arm by ADR-011's design")
	}
	if p.Link != "watching" {
		t.Errorf("link=%q, want watching", p.Link)
	}
	if p.DHT != "holding" {
		t.Errorf("dht=%q, want holding — the wait is not a failure and not a delay to apologise "+
			"for: it is the product deliberately not touching the public network until the link "+
			"has had its chance, and it is the state the screen has never had a word for", p.DHT)
	}
}

// TestTheLinkTierReportsFoundOnlyOnItsOwnPeersSighting.
//
// ADR-011's hold renews on evidence — *"every resolved sighting of its own expected peer renews the
// hold"* — and `linkSeenAt` is set by that path alone. A screen reporting "found" for any
// announcement would tell a user the ceremony is progressing when nothing of theirs has been seen.
func TestTheLinkTierReportsFoundOnlyOnItsOwnPeersSighting(t *testing.T) {
	cer := &ceremonyID{}
	cer.watchingLink(time.Now())
	if got := cer.armProgressOf(time.Now()).Link; got != "watching" {
		t.Fatalf("setup: link=%q before any sighting, want watching", got)
	}
	cer.noteLinkSighting(time.Now())
	if got := cer.armProgressOf(time.Now()).Link; got != "found" {
		t.Errorf("after a sighting of its own expected peer link=%q, want found", got)
	}
}

// TestTheRouterTierKeepsItsThreeFailuresApart is D15's absence clause, and the arm most likely to
// ship unexercised.
//
// **Silence, refusal and unroutable are three different next actions**, which is why the state is
// not a boolean. Silence may mean there is no gateway to ask. A refusal means *"the router is the
// user's, is reachable, and said no"* (`mapRefused`'s own words). An unroutable answer means a
// second layer of NAT, and D9's advice diverges there to a VPN rather than a port-forward. A screen
// that said "no mapping" for all three would give one answer to three users who need different ones.
func TestTheRouterTierKeepsItsThreeFailuresApart(t *testing.T) {
	// Never asked: no mapper at all. Distinct from asking and hearing nothing.
	cer := &ceremonyID{}
	cer.watchingLink(time.Now())
	if got := cer.armProgressOf(time.Now()).Router; got != "" {
		t.Errorf("an arm that never asked for a mapping reports router=%q, want empty", got)
	}

	// Asked, nothing answered.
	silent := &ceremonyID{}
	silent.watchingLink(time.Now())
	silent.portMap = &portMapper{}
	if got := silent.armProgressOf(time.Now()).Router; got != "silent" {
		t.Errorf("router=%q for a mapper that obtained nothing, want silent", got)
	}

	// A gateway answered and said no.
	refused := &ceremonyID{}
	refused.watchingLink(time.Now())
	refused.portMap = &portMapper{}
	refused.mapRefused = true
	if got := refused.armProgressOf(time.Now()).Router; got != "refused" {
		t.Errorf("router=%q when a gateway answered with a refusal, want refused — the opposite "+
			"fact from silence, and D9 tells those two users different things", got)
	}

	// An answer that cannot be published.
	unroutable := &ceremonyID{}
	unroutable.watchingLink(time.Now())
	unroutable.portMap = &portMapper{}
	unroutable.mapUnroutable = true
	if got := unroutable.armProgressOf(time.Now()).Router; got != "unroutable" {
		t.Errorf("router=%q for an unpublishable answer, want unroutable — D9's advice diverges "+
			"here to a VPN rather than a port-forward", got)
	}
}

// TestAnArmWithNothingToSaySaysNothing.
//
// A progress view of four empty strings is a ladder with no rungs, and a client would render it as
// one. The manual and plain co-sign paths have no ladder to report on at all.
func TestAnArmWithNothingToSaySaysNothing(t *testing.T) {
	if p := (*ceremonyID)(nil).armProgressOf(time.Now()); p != nil {
		t.Errorf("a nil ceremony reports progress %+v", p)
	}
	if p := (&ceremonyID{}).armProgressOf(time.Now()); p != nil {
		t.Errorf("an arm that is watching nothing, has not bootstrapped and never asked for a "+
			"mapping reports %+v — four empty strings a client draws as an empty ladder", p)
	}
}

// TestTheStatusPublishesProgressBeforeTheBootstrap is where the criterion's defect actually lived.
//
// **The unit tests above grade `armProgressOf`; this grades the GATE.** `sessionStatus` fills
// `Diagnosis` only when `cer.bootstrapDone.Load()`, and the whole finding of this slice is that
// putting progress behind the same condition reproduces the blank spinner exactly — the tier view
// would arrive at the moment the diagnosis could already speak, which is after the window it exists
// for. So this asserts the status carries progress while the bootstrap has NOT happened.
//
// A test that only drove `armProgressOf` would stay green against a status that never published it.
func TestTheStatusPublishesProgressBeforeTheBootstrap(t *testing.T) {
	var se session
	cer := &ceremonyID{}
	cer.watchingLink(time.Now())
	se.arms[armInteractive] = &arm{cer: cer, addr: "127.0.0.1:9999"}

	// SETUP, both halves. The bootstrap has not run, and the DIAGNOSIS is therefore silent — which
	// is the state this whole slice is about. Without the second check this test could pass in a
	// window where the diagnosis was already speaking and prove nothing about the gate.
	if cer.bootstrapDone.Load() {
		t.Fatal("setup: the bootstrap is done, so this is not the window under test")
	}
	st := se.status()
	if st.Diagnosis != nil {
		t.Fatalf("setup: the diagnosis is already speaking (%+v), so a blank screen was never "+
			"the state here and the assertion below is not about the gate", st.Diagnosis)
	}

	if st.Progress == nil {
		t.Fatal("the status carries NO progress while the bootstrap has not run. That is the " +
			"blank spinner: the diagnosis is gated on bootstrapDone so it cannot speak yet, and " +
			"under ADR-011 the bootstrap waits for the local link — thirty seconds on a LAN by " +
			"lanFirstBudget. D16 says that window must never be blank, and it is the window")
	}
	if st.Progress.Link != "watching" || st.Progress.DHT != "holding" {
		t.Errorf("progress is %+v, want link=watching dht=holding", st.Progress)
	}
}
