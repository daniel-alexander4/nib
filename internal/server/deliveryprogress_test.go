package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// /pending 370 — a delivery round in flight was unobservable.
//
// **The ceiling is over two hours, not the ten minutes the item was filed with.** A leg to a party
// that is not listening burns `connectDeadline` (300 s, measured at tier 4), and `ceremony.MaxRoster`
// is 32 — so 31 legs is 155 minutes of a synchronous POST that published nothing until it returned.
// A convener could not tell a working round from a hung process.
func TestTheRoundReportsTheLegItIsOn(t *testing.T) {
	s := &Server{}
	const id = "abc123"

	// Nothing running: the answer is a stated `false`, not a zero struct. A watcher that could not
	// tell "finished" from "never started" would show a stale leg forever.
	if l, ok := s.currentLeg(id); ok {
		t.Fatalf("a server with no round reports a leg in flight: %+v", l)
	}

	end := s.beginLeg(id, "BOB00FF", "Bob Landlord", 2, 4)
	got, ok := s.currentLeg(id)
	if !ok {
		t.Fatal("a round in flight reports no leg, which is the whole of /pending 370")
	}
	if got.Label != "Bob Landlord" || got.Index != 2 || got.Of != 4 {
		t.Errorf("the leg reports %q %d of %d, want \"Bob Landlord\" 2 of 4", got.Label, got.Index, got.Of)
	}
	if got.Started.IsZero() {
		t.Error("the leg carries no start time. Within one stalled leg the INDEX does not move — " +
			"that is the shape of the problem — so elapsed time is the only thing that ticks, and " +
			"a surface without it is as silent as none for the five minutes that matter")
	}

	// **A second ceremony's round is not this one's.** One map, keyed by id, and a reader asking
	// about a quiet ceremony must not be handed a busy one's leg.
	if _, other := s.currentLeg("different"); other {
		t.Error("a ceremony with no round in flight reports another ceremony's leg")
	}

	end()
	if l, ok := s.currentLeg(id); ok {
		t.Errorf("the leg survives the round that published it: %+v. `beginLeg` returns the clear "+
			"so a caller cannot take the publish and forget it — a round that returned early would "+
			"otherwise report a leg in flight forever", l)
	}
	// Idempotent: the closure is called on every path out of a leg, including ones that already
	// cleared it.
	end()
}

// The route, and the two states it must keep apart.
func TestTheDeliveryProgressRouteSeparatesRunningFromQuiet(t *testing.T) {
	ts, srv := startServerWith(t)
	c, _ := authedClient(t, ts)

	if r, err := c.Get(ts.URL + "/api/ceremony/delivery"); err == nil {
		defer r.Body.Close()
		if r.StatusCode != http.StatusBadRequest {
			t.Errorf("a request naming no ceremony = %d, want 400", r.StatusCode)
		}
	}

	const id = "deadbeef"
	get := func() deliveryProgressResponse {
		t.Helper()
		r, err := c.Get(ts.URL + "/api/ceremony/delivery?ceremony=" + id)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", r.StatusCode)
		}
		var out deliveryProgressResponse
		if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	if q := get(); q.Running {
		t.Error("a ceremony with no round running reports one")
	}

	end := srv.beginLeg(id, "CY00FF", "Cy Witness", 1, 3)
	defer end()
	run := get()
	if !run.Running {
		t.Fatal("a round in flight is reported as quiet")
	}
	if run.Label != "Cy Witness" || run.Index != 1 || run.Of != 3 {
		t.Errorf("reported %q %d of %d", run.Label, run.Index, run.Of)
	}
	// **The pair that answers "is this hung".** A rising elapsed against a stated ceiling is
	// progress even while the index is still, and the ceiling is what makes the number mean
	// something rather than just increase.
	if run.CeilingMs != int(connectDeadline/time.Millisecond) {
		t.Errorf("ceilingMs = %d, want %d — without the bound a rising number is not progress, "+
			"it is just a number going up", run.CeilingMs, connectDeadline/time.Millisecond)
	}
	time.Sleep(15 * time.Millisecond)
	later := get()
	if later.ElapsedMs < run.ElapsedMs {
		t.Errorf("elapsed went backwards: %d then %d", run.ElapsedMs, later.ElapsedMs)
	}
	if later.ElapsedMs == 0 {
		t.Error("elapsed is pinned at zero, so the one number that moves during a stall does not")
	}

	end()
	if q := get(); q.Running {
		t.Error("the round is reported as running after it ended")
	}
}

// TestTheLegIsPublishedBeforeItIsAttempted is the ordering, and it is the whole point of the row.
//
// **A mutation proved this uncovered.** Moving `beginLeg` to after `deliverToParty` compiles, and
// nothing went red: the jsdom test stubs the progress route with a canned answer, so it never
// exercises the server's ordering, and the unit test above drives `beginLeg` directly. A leg
// published after the dial names every party exactly once it has stopped being the one anybody is
// waiting on — which is a progress surface that reports only the past.
//
// **Structural, in this package's own idiom** (see rearm_test.go's `armWindowFor` guard), because
// the alternative is a delivery round with a slow counterpart in-process and this repo does not
// have one. Stated rather than implied: this proves the call precedes the dial in the source, not
// that the round behaves.
func TestTheLegIsPublishedBeforeItIsAttempted(t *testing.T) {
	b, err := os.ReadFile("delivery.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(b)
	const fn = "func (s *Server) runDeliveryRound("
	i := strings.Index(code, fn)
	if i < 0 {
		t.Fatalf("cannot find %s — this guard is pinned to a function that no longer exists under "+
			"that name, so its clean result says nothing", fn)
	}
	body := funcBodyFrom(code, i)
	if body == "" {
		t.Fatal("the brace matcher read an empty body")
	}
	begin := strings.Index(body, "s.beginLeg(")
	dial := strings.Index(body, "s.deliverToParty(")
	if begin < 0 {
		t.Fatal("runDeliveryRound publishes no leg at all, so nothing can watch a round that is " +
			"synchronous and can run for over two hours")
	}
	if dial < 0 {
		t.Fatal("runDeliveryRound no longer calls deliverToParty — this guard is pinned to a shape " +
			"that has moved")
	}
	if begin > dial {
		t.Error("the leg is published AFTER the dial it describes. `deliverToParty` is the call " +
			"that can burn the connect deadline, so publishing after it names each party only " +
			"once it has stopped being the one the convener is waiting on — a progress surface " +
			"reporting exclusively the past")
	}
}

// TestTwoLegsOfOneCeremonyDoNotSharaOneRecord — the leg map was keyed on the ceremony id alone.
//
// # The defect, and why the serial walk hides it completely
//
// `s.legs` held one entry per CEREMONY. That is unique only because `runDeliveryRound` walks one
// party at a time — a property of today's walk, not of a leg. Under `/pending 376`'s W concurrent
// legs every leg of one ceremony writes the same entry, and the FIRST to finish deletes the record
// the others are still represented by: the watcher then reports a quiet ceremony while W-1 legs
// are still burning `connectDeadline`, which is the exact silence /pending 370 built this surface
// to end.
//
// Found by this session's `/deepdive` while tracing the delivery round's concurrency seams. It is
// filed and fixed independently of 376 because the key is wrong whether or not 376 ever lands —
// the serial walk is what makes it *harmless*, not what makes it right.
//
// # Why this test drives beginLeg directly
//
// There is no slow counterpart in-process to make a real round overlap its own legs (the same
// reason `TestTheLegIsPublishedBeforeItIsAttempted` is structural). Driving the two publishes by
// hand is what the round would do under W>1, and it is the only shape that can go red here.
func TestTwoLegsOfOneCeremonyDoNotShareOneRecord(t *testing.T) {
	s := &Server{}
	const id = "ceremony-1"

	// Two legs of ONE ceremony, as a concurrent round would have. Distinct start times so the
	// oldest is unambiguous — the tie-break is asserted separately below.
	endBob := s.beginLeg(id, "BB00", "Bob Landlord", 1, 3)
	time.Sleep(2 * time.Millisecond)
	endCy := s.beginLeg(id, "CC00", "Cy Witness", 2, 3)

	// The OLDEST is reported: the surface exists to show a stall, and the leg nearest its ceiling
	// is the one a convener is actually waiting on. Reporting the newest would reset the clock
	// every time a sibling started.
	got, ok := s.currentLeg(id)
	if !ok {
		t.Fatal("two legs are in flight and the ceremony reports none")
	}
	if got.Label != "Bob Landlord" {
		t.Errorf("with two legs live the round reports %q; want the OLDEST, \"Bob Landlord\". "+
			"Reporting the newest resets the elapsed clock whenever a sibling starts, which is a "+
			"progress surface that cannot show the stall it exists for", got.Label)
	}

	// **The defect itself.** Ending one leg must leave the other reported. Under the old key both
	// legs were one entry, so this delete emptied it and the ceremony went quiet with a leg still
	// running.
	endBob()
	got, ok = s.currentLeg(id)
	if !ok {
		t.Fatal("ending one leg cleared the other: the map is keyed so that two legs of one " +
			"ceremony share a record, so the first leg to finish makes a running round look " +
			"finished. That is the key this test exists for (`legKey` carries the party)")
	}
	if got.Label != "Cy Witness" {
		t.Errorf("after the first leg ended the round reports %q, want the surviving leg "+
			"\"Cy Witness\"", got.Label)
	}

	endCy()
	if l, ok := s.currentLeg(id); ok {
		t.Errorf("both legs ended and the ceremony still reports one: %+v", l)
	}
}

// The tie-break, asserted on its own because a map range without one is the same class of defect
// as the key it was fixed beside: an answer that varies run to run for no reason a reader can see.
func TestSimultaneousLegsAreOrderedDeterministically(t *testing.T) {
	const id = "ceremony-2"
	// Legs whose Started is identical — ordinary on a coarse clock. Written straight into the map
	// so the times really are equal; `beginLeg` takes its own `time.Now()` and could not produce
	// this reliably.
	at := time.Now()
	first := ""
	for i := 0; i < 12; i++ {
		s := &Server{legs: map[legKey]deliveryLeg{
			{ceremony: id, party: "cc00"}: {Label: "Cy Witness", Index: 3, Of: 3, Started: at},
			{ceremony: id, party: "aa00"}: {Label: "Ann Signer", Index: 2, Of: 3, Started: at},
			{ceremony: id, party: "bb00"}: {Label: "Bob Landlord", Index: 2, Of: 3, Started: at},
		}}
		got, ok := s.currentLeg(id)
		if !ok {
			t.Fatal("three legs in the map and the ceremony reports none")
		}
		if first == "" {
			first = got.Label
		}
		if got.Label != first {
			t.Fatalf("the same three simultaneous legs reported %q and then %q — the reader is "+
				"ranging the map without a tie-break, so the watcher's answer changes for reasons "+
				"nothing in the round did", first, got.Label)
		}
	}
	// Lowest Index wins the tie, then the party, so the answer is also the RIGHT one rather than
	// merely stable: Ann is index 2 and sorts before Bob's identical index.
	if first != "Ann Signer" {
		t.Errorf("simultaneous legs resolved to %q; want \"Ann Signer\" — lowest index first, then "+
			"party, so a stable answer is also an explicable one", first)
	}
}
