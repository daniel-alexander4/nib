package pdfops

import (
	"os"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestANamedPropertyListCarriesItsMCIDToEveryReader — /pending 644 #2.
//
// Three readers answered "does this marked content carry an MCID" three ways: the run walker resolved a
// property list NAMED in `/Properties`, while `carriesMCID` and the claim door's coverage scan looked for a
// literal `/MCID` operand. A form marking content through `/Span /MC0 BDC` therefore read as carrying none,
// and condition 4 of `structureCarriedCompletely` passed a doubly-drawn form having looked at nothing.
func TestANamedPropertyListCarriesItsMCIDToEveryReader(t *testing.T) {
	xt := &model.XRefTable{}
	res := types.Dict{"Properties": types.Dict{"MC0": types.Dict{"MCID": types.Integer(4)}}}

	named := []byte("/Span /MC0 BDC 0 0 m 10 10 l S EMC")
	if !carriesMCID(xt, res, named) {
		t.Error("carriesMCID: a stream marking content through a named property list reads as carrying no MCID")
	}
	if sp, _ := uncoveredDrawingSpans(xt, res, named, nil); len(sp) != 0 {
		t.Errorf("uncoveredDrawingSpans: a path inside a named-MCID sequence is counted uncovered (%d spans)", len(sp))
	}

	// The controls: the same stream WITHOUT the resources has nothing to resolve, and an inline list
	// still counts, and a list with no MCID does not.
	if carriesMCID(xt, nil, named) {
		t.Error("carriesMCID claims an MCID for a name it could not resolve")
	}
	if sp, _ := uncoveredDrawingSpans(xt, nil, named, nil); len(sp) != 1 {
		t.Errorf("uncoveredDrawingSpans without resources: %d spans, want 1", len(sp))
	}
	if !carriesMCID(xt, nil, []byte("/Span <</MCID 3>> BDC (x) Tj EMC")) {
		t.Error("carriesMCID lost the inline form")
	}
	if carriesMCID(xt, nil, []byte("/Span <</Lang (en)>> BDC (x) Tj EMC")) {
		t.Error("carriesMCID reads an MCID into a property list that has none")
	}
	noMCID := types.Dict{"Properties": types.Dict{"MC0": types.Dict{"Lang": types.StringLiteral("en")}}}
	if carriesMCID(xt, noMCID, named) {
		t.Error("carriesMCID reads an MCID into a named property list that has none")
	}
}

// TestOnlyTheDoorReadsAnMCIDOperand is ADR-009's guard for /pending 644 #2: a literal `/MCID` operand is
// read by `bdcMCID` and by `openerAround`, which REWRITES an opener's MCID rather than asking whether one
// exists, and nowhere else in this package.
func TestOnlyTheDoorReadsAnMCIDOperand(t *testing.T) {
	allowed := map[string]int{"textrun.go": 1, "flowtags.go": 1}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, rerr := os.ReadFile(n)
		if rerr != nil {
			t.Fatal(rerr)
		}
		if c := strings.Count(string(b), `"/MCID"`); c > 0 {
			seen[n] = c
		}
	}
	for n, c := range seen {
		if c > allowed[n] {
			t.Errorf("%s reads a literal /MCID operand %d time(s) — ask bdcMCID, which also resolves a "+
				"property list named in /Properties", n, c)
		}
	}
	for n := range allowed {
		if seen[n] == 0 {
			t.Errorf("%s no longer reads /MCID — the guard's population has moved", n)
		}
	}
}
