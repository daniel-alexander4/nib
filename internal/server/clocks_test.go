package server

import (
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
)

// /pending 420 — the budget table in clocks.go is COMPUTED, not copied.
//
// It tabulated a 14m leg and a 19m20s total for four commits after ADR-028's role frame took
// `DeliveryLegBudget` to 14m30s. A table copied by hand from figures computed elsewhere drifts
// the moment one of them moves, and nothing noticed — which is the same failure as a comment
// that names the day it should have gone red and does not.
//
// So the numbers are derived here rather than read off the comment, and the comment is required
// to agree with them. It is a doc check and it says so: what it cannot see is whether the
// SUM is the right set of terms, only that the terms it names are current.
func TestTheBudgetTableAgreesWithTheBudgets(t *testing.T) {
	src, err := os.ReadFile("clocks.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)

	leg := p2p.DeliveryLegBudget(p2p.PeerGatesUnattended)
	total := leg + 20*time.Second + 300*time.Second // bootstrapBudget + connectDeadline, as tabulated

	// STIMULUS: the table is still there. Without this the assertions pass over a file that no
	// longer carries the thing they check.
	from := strings.Index(s, "p2p.DeliveryLegBudget(PeerGatesUnattended)")
	if from < 0 {
		t.Fatal("the budget table is gone from clocks.go — this guard would pass over a file " +
			"with nothing to disagree with")
	}
	// **Scoped to the TABLE, not the file.** The first cut searched the whole file, and the
	// prose below the table explains that the leg moved 14m -> 14m30s — so the mutation that
	// put the stale figures back in the table left it green, satisfied by the sentence
	// describing the very drift it was written to catch.
	end := strings.Index(s[from:], "\n//\n")
	if end < 0 {
		end = len(s) - from
	}
	table := s[from : from+end]
	for _, want := range []string{leg.String(), total.String()} {
		if !strings.Contains(table, want) {
			t.Errorf("the budget table does not carry %s. DeliveryLegBudget is %s and the "+
				"tabulated total is %s; the table said 14m and 19m20s for four commits after "+
				"ADR-028 moved the leg, and nothing noticed (/pending 420).", want, leg, total)
		}
	}
}
