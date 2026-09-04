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

	end := s.beginLeg(id, "Bob Landlord", 2, 4)
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

	end := srv.beginLeg(id, "Cy Witness", 1, 3)
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
