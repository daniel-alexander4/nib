package server

import (
	"context"
	"testing"
	"time"

	"nib/internal/discovery"
	"nib/internal/sign"
	"nib/internal/vault"
)

// P07.S05e — the arm holds the DHT on EVIDENCE, and the evidence is its own peer on the link.
//
// S05d gave the DIAL side a decisive answer: `peerAddresses` browses before the race, so a LAN
// candidate is the link having already answered. The arm has no such one-shot answer — it is
// waiting, and in a relay it waits through the hops before its own. Measured at nine parties:
// instances 1 and 2 reached the DHT zero times and 3 through 9 reached it twice each, which is
// exactly the parties whose turn comes after a fixed window closes.

// TestTheSightingIsReportedBeforeTheAnswerRateLimit is the hook's placement, and the placement is
// the whole of it.
//
// The rate limit below the hook is `hopAnnounceWindow` and it is a rule about ANNOUNCING: a
// re-dial must not stack a second announcer. Observing is a different rate over the same stream.
// Hung off that gate, the hold would stop renewing during precisely the stretch in which the peer
// is most present on the link — the opposite of what the evidence says — so the count of sightings
// must EXCEED the count of answers.
func TestTheSightingIsReportedBeforeTheAnswerRateLimit(t *testing.T) {
	mine, _, err := sign.GenerateIdentity("Convener")
	if err != nil {
		t.Fatal(err)
	}
	myFP, _ := sign.Fingerprint(mine)
	pins := []vault.PinnedPeer{{Fingerprint: myFP, Label: "Convener"}}

	// Three sightings of the arm's own peer, all inside one announce window.
	br := &scriptedBrowser{done: make(chan struct{}), seen: []discovery.Seen{
		seenFrom(t, myFP, 5002),
		seenFrom(t, myFP, 5002),
		seenFrom(t, myFP, 5002),
	}}
	base := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-br.done
		cancel()
	}()

	var sightings, answers int
	answerLoop(ctx, br, pins, func() time.Time { return base }, nil, nil,
		func(time.Time) { sightings++ },
		func(candidate) bool { answers++; return true })

	// STIMULUS: the loop actually saw its peer. Without this, "sightings > answers" is satisfied
	// by 0 > 0 being false in a way that still reads as a placement bug rather than a dead loop —
	// and by a loop that never ran at all, which is how this file's sibling first went green.
	if sightings == 0 {
		t.Fatal("setup: the loop reported no sighting at all, so it never resolved its peer and " +
			"nothing below is a statement about the hook's placement")
	}
	if answers != 1 {
		t.Fatalf("the arm answered %d time(s) for three sightings inside one window, want 1 — the "+
			"rate limit is not doing its own job, so this test cannot say anything about the "+
			"hook sitting above it", answers)
	}
	if sightings <= answers {
		t.Fatalf("%d sighting(s) for %d answer(s): the hook is BELOW the answer rate limit, so the "+
			"DHT hold stops renewing during exactly the stretch the peer is most present on the "+
			"link (S05e)", sightings, answers)
	}
}

// TestTheDHTHoldRenewsOnEvidenceAndLapsesWithout is the hold itself, four-armed.
//
// Three arms are the property and the fourth is the control. A hold that never lapses is not a
// fix, it is a ceremony that can never fall back; a hold that a stranger can renew is a lever a
// stranger should not have; and a hold that applies with nothing heard at all is an outage.
func TestTheDHTHoldRenewsOnEvidenceAndLapsesWithout(t *testing.T) {
	// `base` is small so the arms below cost milliseconds; `lanFirstBudget` is the real figure and
	// is what the sighting arithmetic uses, so the two are not interchangeable and the test says
	// which is which.
	const base = 20 * time.Millisecond

	t.Run("nothing heard: the DHT tier opens after base", func(t *testing.T) {
		cer := &ceremonyID{}
		// Bounded, so a hold that never releases FAILS here instead of hanging until the package
		// timeout — a guard that hangs is read as a flake and gets rerun rather than fixed.
		ctx, cancel := context.WithTimeout(context.Background(), lanFirstBudget/4)
		defer cancel()
		started := time.Now()
		if !cer.holdDHT(ctx, base) {
			t.Fatal("the hold never released with nothing ever heard on the link")
		}
		if took := time.Since(started); took >= lanFirstBudget {
			t.Fatalf("nothing was ever heard on the link and the hold still waited %s: an arm "+
				"with no LAN peer must pay `base` and nothing more, or this is an outage rather "+
				"than a fix", took)
		}
	})

	t.Run("a fresh sighting holds past base", func(t *testing.T) {
		cer := &ceremonyID{}
		cer.noteLinkSighting(time.Now())
		ctx, cancel := context.WithTimeout(context.Background(), base*4)
		defer cancel()
		if cer.holdDHT(ctx, base) {
			t.Fatalf("the peer was sighted on the link just now and the hold released anyway — "+
				"it is not reading the sighting, so an arm waiting for a hop that is coming over "+
				"the link publishes to the public DHT regardless (S05e). Budget is %s.",
				lanFirstBudget)
		}
	})

	t.Run("a stale sighting lapses", func(t *testing.T) {
		cer := &ceremonyID{}
		// Sighted, but longer ago than the budget: the link went quiet and stayed quiet.
		cer.noteLinkSighting(time.Now().Add(-2 * lanFirstBudget))
		// Bounded for the same reason as the arm above: a hold that cannot lapse must fail, not
		// hang. The bound is well under the budget, so releasing here is itself the lapse.
		ctx, cancel := context.WithTimeout(context.Background(), lanFirstBudget/4)
		defer cancel()
		started := time.Now()
		if !cer.holdDHT(ctx, base) {
			t.Fatalf("the last sighting was %s ago and the hold never lapsed: a hold that does "+
				"not lapse is a DHT tier that never opens", 2*lanFirstBudget)
		}
		if took := time.Since(started); took >= lanFirstBudget {
			t.Fatalf("the last sighting was %s ago and the hold still waited %s: a hold that does "+
				"not lapse is a DHT tier that never opens", 2*lanFirstBudget, took)
		}
	})

	t.Run("a stranger never reaches the hold at all", func(t *testing.T) {
		mine, _, err := sign.GenerateIdentity("Convener")
		if err != nil {
			t.Fatal(err)
		}
		myFP, _ := sign.Fingerprint(mine)
		other, _, err := sign.GenerateIdentity("Stranger")
		if err != nil {
			t.Fatal(err)
		}
		otherFP, _ := sign.Fingerprint(other)
		// STIMULUS: the identities really differ, or "only mine renewed" is trivially true.
		if string(myFP) == string(otherFP) {
			t.Fatal("setup: the two identities are the same, so this cannot discriminate")
		}

		cer := &ceremonyID{}
		br := &scriptedBrowser{done: make(chan struct{}), seen: []discovery.Seen{
			seenFrom(t, otherFP, 5001), // a stranger, twice
			seenFrom(t, otherFP, 5001),
		}}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-br.done
			cancel()
		}()
		base := time.Now()
		answerLoop(ctx, br, []vault.PinnedPeer{{Fingerprint: myFP, Label: "Convener"}},
			func() time.Time { return base }, nil, nil,
			cer.noteLinkSighting, func(candidate) bool { return true })

		// A COUNT cannot see this — a stranger's sighting and the peer's produce the same one,
		// which is why `answerLoop`'s own first version was green. The claim is about WHOSE.
		if cer.linkSeenAt.Load() != 0 {
			t.Fatal("a stranger's announcement renewed the DHT hold: any host on the link could " +
				"then delay another party's fallback for as long as it kept shouting. The screen " +
				"is pins and not wire bytes (L1), and the hook sits past `resolve` for that reason")
		}
	})
}

// TestAnArmThatIsWatchingButHasHeardNothingStillHolds is the race the sighting alone could not
// close, and it is the difference between "never heard" and "not looking".
//
// Announcements arrive at 2/s and the base wait is two seconds, so a first sighting that lands at
// 2.1 s would have missed: the feed releases, the arm publishes, and the peer it was about to hear
// arrives a tenth of a second later. Measuring from when the WATCH began removes the race, and it
// is also what makes the acceptance clause true as written — an arm with nothing on the link
// reaches the DHT within `lanFirstBudget`, not within `base`.
//
// The control is the arm below it: the dial side sets neither field and must be untouched, because
// a hold that applied there would be a DHT tier that never starts.
func TestAnArmThatIsWatchingButHasHeardNothingStillHolds(t *testing.T) {
	const base = 20 * time.Millisecond

	t.Run("watching, nothing heard yet: still holds", func(t *testing.T) {
		cer := &ceremonyID{}
		cer.watchingLink(time.Now())
		// STIMULUS: the watch registered. Without this the hold below could be releasing for the
		// dial-side reason — neither field set — and pass for the wrong one.
		if cer.linkWatchAt.Load() == 0 {
			t.Fatal("setup: the watch did not register, so this arm is indistinguishable from the " +
				"dial side and nothing below is about the race")
		}
		if cer.linkSeenAt.Load() != 0 {
			t.Fatal("setup: something was already sighted, so this is the renewal test, not this one")
		}
		ctx, cancel := context.WithTimeout(context.Background(), base*4)
		defer cancel()
		if cer.holdDHT(ctx, base) {
			t.Fatalf("the arm is watching the link and simply has not heard its peer YET, and the "+
				"hold released after %s anyway — so whether it publishes is a race between its "+
				"answer loop starting and this wait ending (S05e)", base)
		}
	})

	t.Run("not watching at all: the dial side is untouched", func(t *testing.T) {
		cer := &ceremonyID{}
		ctx, cancel := context.WithTimeout(context.Background(), lanFirstBudget/4)
		defer cancel()
		started := time.Now()
		if !cer.holdDHT(ctx, base) {
			t.Fatal("a ceremony with nobody watching the link held the DHT tier: the dial side " +
				"writes neither field, so this is the arm's rule leaking onto it")
		}
		if took := time.Since(started); took >= lanFirstBudget {
			t.Fatalf("the dial side waited %s: it has its own decisive answer from the browse and "+
				"must not pay the arm's budget", took)
		}
	})
}

// TestTheRemoteArmsCostIsMeasured is acceptance clause 4: what an arm with nothing on the link
// pays, as a number rather than an assumption.
//
// The cost is one-sided by construction and this is the side that pays. A ceremony the link
// carries emits nothing and waits only as long as its peer keeps announcing; a genuinely remote
// arm hears nothing, pays exactly one `lanFirstBudget` from the moment it began watching, and then
// publishes as it always did. Against a 300 s connect deadline and D33's thirty-day arm, that is
// the trade the ADR claims — so it is printed here rather than asserted in prose.
func TestTheRemoteArmsCostIsMeasured(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the arm's link budget")
	}
	cer := &ceremonyID{}
	cer.watchingLink(time.Now())

	ctx, cancel := context.WithTimeout(context.Background(), 2*lanFirstBudget)
	defer cancel()
	started := time.Now()
	if !cer.holdDHT(ctx, browseWindow) {
		t.Fatalf("a remote arm never reached the DHT tier at all within %s: the hold defers the "+
			"tier instead of delaying it, and a ceremony with no LAN peer cannot complete",
			2*lanFirstBudget)
	}
	took := time.Since(started)
	t.Logf("no peer on the link: the arm reaches the DHT tier %s after it starts watching "+
		"(budget %s, base %s) — the cost this slice adds to a genuinely remote ceremony, against "+
		"a %s connect deadline", took.Round(time.Millisecond), lanFirstBudget, browseWindow,
		connectDeadline)

	if took < lanFirstBudget {
		t.Fatalf("the arm reached the DHT %s in, short of its %s budget: it is not actually "+
			"waiting, so a peer arriving on the link a moment later finds the record already "+
			"published", took, lanFirstBudget)
	}
	if took > lanFirstBudget+browseWindow+time.Second {
		t.Fatalf("the arm took %s, past one budget plus the base: the hold is compounding rather "+
			"than measuring from the watch", took)
	}
}
