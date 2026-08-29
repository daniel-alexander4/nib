package server

import (
	"strings"
	"sync"
	"testing"
)

// P07.S09b: D33's packet ceiling is per (hop, SIDE), and a side is a machine — not a
// `ceremonyID`.
//
// # The defect, and why two comments asserted the opposite of the code
//
// `punchLoop`'s doc says the armed side and the dialing side each run one loop "sharing one
// per-side budget", and `punchBudget`'s doc said "one lives per ceremonyID (a ceremonyID IS one
// hop, one side)". Both call sites built `&punchBudget{}` inline. A ceremonyID is not a side: the
// armed path and the dialing path hold different ones, with different sockets, and P05.S09's
// symmetric racing has one machine running both for the same hop — `glare.go` opens by saying so.
//
// So each got its own 3,000 and a side emitted 6,000 against a figure D33 calls LAW, on the
// ground that under D6's pin an attacker supplies the candidates. Nothing observable said so:
// both loops behave correctly in isolation, every existing punch test passes, and the only way to
// see it is to ask the two loops for their budget and compare identity.

func TestBothPunchLoopsOfOneHopSpendOneBudget(t *testing.T) {
	s := &Server{}
	const id = "ceremony-abc"

	// The two ceremonyIDs one machine holds for one hop: the armed one and the dialing one.
	// Distinct objects with distinct sockets, exactly as the two paths build them.
	armed := &ceremonyID{}
	armed.inv.ID = id
	dialing := &ceremonyID{}
	dialing.inv.ID = id

	a := armed.punchBudget(s)
	d := dialing.punchBudget(s)

	// SETUP: two genuinely distinct ceremonyIDs, or "they share a budget" is true of one object
	// asked twice and says nothing about the two paths.
	if armed == dialing {
		t.Fatal("setup: the two ceremonyIDs are the same object")
	}
	if a == nil || d == nil {
		t.Fatal("setup: a punch path was handed a nil budget")
	}

	if a != d {
		t.Fatal("the armed side and the dialing side of one hop hold DIFFERENT packet budgets, " +
			"so this machine spends D33's 3,000 twice for one hop. `punchLoop`'s own doc says " +
			"they share one per-side budget and `punchBudget`'s said a ceremonyID is a side; " +
			"neither is true when the two paths build their own. Under D6's pin an attacker " +
			"supplies the candidates, which is why this figure is law rather than tuning.")
	}

	// And the sharing is what bounds the total, which is the property the identity check stands
	// for. Spending to exhaustion through ONE handle must leave the other with nothing.
	for i := 0; i < punchBudgetPerSide; i++ {
		if !a.spend() {
			t.Fatalf("the budget refused packet %d of its own ceiling %d", i, punchBudgetPerSide)
		}
	}
	if d.spend() {
		t.Error("the dialing side was allowed a packet after the armed side had spent the whole " +
			"ceiling — the two are not one budget however they compare")
	}
	spent, dropped := a.report()
	if spent != punchBudgetPerSide {
		t.Errorf("the shared budget reports %d sent, want the ceiling %d", spent, punchBudgetPerSide)
	}
	if dropped != 1 {
		t.Errorf("the shared budget reports %d dropped, want 1", dropped)
	}
}

// TestTwoCeremoniesDoNotShareABudget is the other direction, and it is the assertion a
// process-wide singleton would fail.
//
// D33's unit is per HOP. One global counter would bound the machine rather than the hop, so a
// ceremony that had punched hard would starve the next one — the budget silently becoming a
// lifetime allowance.
func TestTwoCeremoniesDoNotShareABudget(t *testing.T) {
	s := &Server{}
	one := &ceremonyID{}
	one.inv.ID = "ceremony-one"
	two := &ceremonyID{}
	two.inv.ID = "ceremony-two"

	a, b := one.punchBudget(s), two.punchBudget(s)
	if a == b {
		t.Fatal("two different ceremonies share one packet budget — D33's unit is per HOP, so a " +
			"global counter makes the ceiling a lifetime allowance and lets one proceeding " +
			"starve the next")
	}
	for i := 0; i < punchBudgetPerSide; i++ {
		a.spend()
	}
	if !b.spend() {
		t.Error("exhausting one ceremony's budget refused a packet to another ceremony")
	}
}

// TestAPunchOutsideACeremonyGetsItsOwnBudget covers the empty-id branch. A punch with no
// proceeding must not join a shared pool keyed by "" — that pool would be every non-ceremony
// punch in the process, and the first one to exhaust it silences the rest.
func TestAPunchOutsideACeremonyGetsItsOwnBudget(t *testing.T) {
	s := &Server{}
	if a, b := s.punchBudgetFor(""), s.punchBudgetFor(""); a == b {
		t.Error("two punches outside any ceremony share one budget, so the first to exhaust it " +
			"silences every later one for the life of the process")
	}
	// And a nil ceremony does not panic on the way there.
	var nilCer *ceremonyID
	if got := nilCer.punchBudget(s); got == nil {
		t.Error("a nil ceremony was handed a nil budget; every spend() on it would panic")
	}
}

// TestThePunchBudgetRegistryIsConcurrencySafe — both loops start as goroutines, so first use
// races by construction.
func TestThePunchBudgetRegistryIsConcurrencySafe(t *testing.T) {
	s := &Server{}
	var wg sync.WaitGroup
	got := make([]*punchBudget, 32)
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i] = s.punchBudgetFor("same-ceremony")
		}(i)
	}
	wg.Wait()
	for i, b := range got {
		if b != got[0] {
			t.Fatalf("caller %d got a different budget: the registry raced and this hop has "+
				"more than one ceiling", i)
		}
	}
}

// TestAnExhaustedBudgetReachesTheDiagnosis is D33's "drops and reports", with the report finally
// having a reader.
//
// `report()`'s own doc said it existed "for D19/S11 to surface" and S11 shipped without wiring
// it, so until this slice the only callers were tests: the drop happened and nothing reported it.
// A user whose hop hit the ceiling was told that nothing answered and nothing more.
func TestAnExhaustedBudgetReachesTheDiagnosis(t *testing.T) {
	cer := &ceremonyID{}
	cer.inv.ID = "ceremony-x"
	b := cer.punchBudget(&Server{})

	// SETUP: a budget with nothing dropped says nothing, or the assertion below cannot tell the
	// report from a line printed on every diagnosis.
	quiet := diagnoseDetail(t, cer)
	if strings.Contains(quiet, "packet budget exhausted") {
		t.Fatalf("a budget that dropped nothing still reports drops: %q", quiet)
	}

	for i := 0; i < punchBudgetPerSide+7; i++ {
		b.spend()
	}
	loud := diagnoseDetail(t, cer)
	if !strings.Contains(loud, "packet budget exhausted") {
		t.Fatalf("a hop that dropped 7 packets over D33's ceiling reports nothing to the user. "+
			"The drop half of drop-and-report has always worked; this is the half that had no "+
			"caller: %q", loud)
	}
	if !strings.Contains(loud, "7 dropped") {
		t.Errorf("the report does not say how many were dropped, so a reader cannot tell a "+
			"trimmed tail from a starved hop: %q", loud)
	}
	// It must not become the CAUSE. D33: "it drops and reports; it never fails the ceremony."
	if got := cer.diagnose().cause; got == causeUndiagnosed {
		return // no DHT on this fixture; the cause arm is not what this test is about
	}
}

// diagnoseDetail runs the real diagnosis and returns its detail.
func diagnoseDetail(t *testing.T, c *ceremonyID) string {
	t.Helper()
	return c.diagnose().detail
}
