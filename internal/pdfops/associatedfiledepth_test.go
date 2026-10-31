package pdfops

import (
	"errors"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestAnAssociatedFileTooDeepToFindIsRefusedNotSkipped — P02 close F01. dropAssociatedFileKeys used to stop at a depth
// and return quietly, and its caller then cleared the read mark that makes the write refuse an `/AF`: a file the user
// removed stayed in the document. The stimulus is asserted first (the deep `/AF` really is still there, and the
// shallow one really is stripped), so a green here is not about a document the walk never reached.
func TestAnAssociatedFileTooDeepToFindIsRefusedNotSkipped(t *testing.T) {
	inner := types.Dict{"AF": types.Array{}}
	var deep types.Object = inner
	for i := 0; i < maxAssociatedFileDepth+5; i++ {
		deep = types.Dict{"K": deep}
	}
	shallow := types.Dict{"K": types.Dict{"AF": types.Array{}}}
	xt := &model.XRefTable{Table: map[int]*model.XRefTableEntry{
		1: {Object: deep},
		2: {Object: shallow},
	}}

	err := dropAssociatedFileKeys(xt)

	if _, still := inner["AF"]; !still {
		t.Fatal("setup: the deep /AF was stripped, so the walk reached it and this test is about nothing")
	}
	if _, still := shallow["K"].(types.Dict)["AF"]; still {
		t.Fatal("setup: the shallow /AF was not stripped — the walk is not running")
	}
	if !errors.Is(err, errAssociatedFileTooDeep) {
		t.Errorf("a /AF nested past %d levels was skipped without a refusal (err = %v): the removal would report success with the file still in the document",
			maxAssociatedFileDepth, err)
	}
}

// TestAnAssociatedFileIsStrippedFromEveryContainerItCanSitIn — P02 close, redproof survivors 1 and 2: no fixture put an
// `/AF` inside a direct array or inside a stream's own dictionary, so neither descent arm of dropAssociatedFileKeys was
// ever exercised (each could be replaced by a no-op and every test stayed green).
func TestAnAssociatedFileIsStrippedFromEveryContainerItCanSitIn(t *testing.T) {
	inArray := types.Dict{"AF": types.Array{}}
	inStream := types.Dict{"AF": types.Array{}}
	xt := &model.XRefTable{Table: map[int]*model.XRefTableEntry{
		1: {Object: types.Dict{"K": types.Array{inArray}}},
		2: {Object: types.StreamDict{Dict: inStream}},
	}}
	if err := dropAssociatedFileKeys(xt); err != nil {
		t.Fatal(err)
	}
	if _, still := inArray["AF"]; still {
		t.Error("an /AF inside a direct array was left in the document")
	}
	if _, still := inStream["AF"]; still {
		t.Error("an /AF inside a stream's own dictionary was left in the document")
	}
}

// TestTheRetiredRulesAreExactlyTheOnesBelowTheCurrentRule — P02 close, redproof survivor 10. A bound written `<=` would
// call a readable rule retired, and ReadMirrorToEnd would then skip the DocHash comparison for every such mirror.
// Rule 7 is below the current rule 8 and still readable (/pending 927), so the retired ones are 5 and 6.
func TestTheRetiredRulesAreExactlyTheOnesBelowTheCurrentRule(t *testing.T) {
	cur := ContentDigestVersion
	for v, want := range map[int]bool{4: false, 5: true, 6: true, 7: false, cur: false, cur + 1: false} {
		if got := ContentDigestRuleRetired(v); got != want {
			t.Errorf("ContentDigestRuleRetired(%d) = %v, want %v (current rule %d)", v, got, want, cur)
		}
	}
	if !ContentDigestRuleReadable(cur) || !ContentDigestRuleReadable(7) || ContentDigestRuleReadable(6) {
		t.Errorf("only rules 7 and %d are readable", cur)
	}
}
