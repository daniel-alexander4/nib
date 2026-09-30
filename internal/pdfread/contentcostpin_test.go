package pdfread

import (
	"os"
	"regexp"
	"testing"
)

// TestTheContentCostGateNamesThePdfcpuItRestates — passDecodesPageContent restates pdfcpu v0.13.0's own
// condition for decoding page content during its optimize pass (optimize.go:1647-1653, :997). An upgrade that
// moves that condition would leave the gate closed on a path that now decodes, and nothing would say so; this
// fails on the upgrade, so the lines are re-read before it lands (/pending 748).
func TestTheContentCostGateNamesThePdfcpuItRestates(t *testing.T) {
	if got := pdfcpuPin(t); got != "v0.13.0" {
		t.Errorf("go.mod pins pdfcpu %s, and passDecodesPageContent (contentcost.go) restates v0.13.0's "+
			"optimize.go:1647-1653 and :997 — re-read those lines in %s, update the gate, then this pin", got, got)
	}
}

// TestTheReferenceDoorNamesThePdfcpuItRestates — refgraph.go's edge table is pdfcpu v0.13.0's unguarded recursion,
// traced from `validate/` (ADR-069). An upgrade can add a recursion, or guard one, and nothing else would say so:
// re-trace the validator's call graph for the new version (every cycle, and whether it marks before recursing),
// update the table and its fixtures (testpdf/refloops.go), then this pin (/pending 764).
func TestTheReferenceDoorNamesThePdfcpuItRestates(t *testing.T) {
	if got := pdfcpuPin(t); got != "v0.13.0" {
		t.Errorf("go.mod pins pdfcpu %s, and the reference door (refgraph.go) restates v0.13.0's unguarded "+
			"recursion — re-trace %s's validate/ package, update the edge table, then this pin", got, got)
	}
}

// pdfcpuPin is the pdfcpu version go.mod requires.
func pdfcpuPin(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^\s*github\.com/pdfcpu/pdfcpu (v\S+)`).FindSubmatch(b)
	if m == nil {
		t.Fatal("setup: go.mod names no github.com/pdfcpu/pdfcpu requirement — this guard cannot see the pin")
	}
	return string(m[1])
}
