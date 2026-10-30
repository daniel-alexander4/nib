package pdfread

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// expectedNames is, written out beside the table it checks, which nib refusal each of pdfcpu v0.16.2's bounded-walk
// errors (`model/recursion.go`) is named as — the explicit mapping is the test's own, so a row dropped from
// validatorRefusals, or moved to another sentence, is red for THAT error and not only for the count.
var expectedNames = map[error]error{
	model.ErrMaxRecursionDepthExceeded: ErrNestedTooDeep,

	model.ErrFormFieldCycle: ErrReferenceCycle, model.ErrNameTreeCycle: ErrReferenceCycle,
	model.ErrNumberTreeCycle: ErrReferenceCycle, model.ErrPageTreeCycle: ErrReferenceCycle,
	model.ErrStructureTreeCycle: ErrReferenceCycle, model.ErrFunctionCycle: ErrReferenceCycle,
	model.ErrHalftoneCycle: ErrReferenceCycle, model.ErrMediaClipCycle: ErrReferenceCycle,
	model.ErrPatternCycle: ErrReferenceCycle, model.ErrRenditionCycle: ErrReferenceCycle,
	model.ErrTargetCycle: ErrReferenceCycle,

	model.ErrFormFieldDuplicate: ErrNodeNamedTwice, model.ErrNameTreeDuplicate: ErrNodeNamedTwice,
	model.ErrNumberTreeDuplicate: ErrNodeNamedTwice, model.ErrPageTreeDuplicate: ErrNodeNamedTwice,
	model.ErrStructureTreeDuplicate: ErrNodeNamedTwice,
}

// unnamedByDecision are the validator's bounded-walk errors nib does NOT name, each with the reason. They pass
// through as pdfcpu's own words (F12, /pending 919 owns whether they should be classified). A new `Err…` in
// pdfcpu's recursion.go that is in neither this set nor expectedNames fails the census below, so a bump cannot add
// a refusal nobody decided about.
var unnamedByDecision = map[string]string{
	"ErrActionCycle":        "no pdfcpu v0.16.2 code returns it",
	"ErrBeadCycle":          "no pdfcpu v0.16.2 code returns it",
	"ErrAnnotationIRTCycle": "returned by the annotation walk; not named (F12)",
	"ErrCMapCycle":          "returned by the font walk; not named (F12)",
	"ErrColorSpaceCycle":    "returned by the colour-space walk; not named (F12)",
}

// TestEveryBoundedWalkRefusalIsNamedAsExpected probes validatorRefusals one error at a time (P02 red-proof: 15 of
// the 17 were unprobed). Each error, wrapped as the validator wraps it, must come back as its named refusal and
// still answer errors.Is for pdfcpu's own.
func TestEveryBoundedWalkRefusalIsNamedAsExpected(t *testing.T) {
	if len(expectedNames) != 17 {
		t.Fatalf("setup: the expectation holds %d errors, want the 17 the table names", len(expectedNames))
	}
	for pdfcpuErr, want := range expectedNames {
		wrapped := fmt.Errorf("prepare PDF context: validation error (obj#:7): %w", pdfcpuErr)
		got := nameValidatorRefusal(wrapped)
		if !errors.Is(got, want) {
			t.Errorf("%v: named %v, want %v", pdfcpuErr, got, want)
		}
		if !errors.Is(got, pdfcpuErr) {
			t.Errorf("%v: the named refusal lost pdfcpu's own error", pdfcpuErr)
		}
	}
	// And an error that is not one of them is left exactly as it was.
	other := errors.New("some other validation error")
	if got := nameValidatorRefusal(other); got != other {
		t.Errorf("an unrelated error came back as %v", got)
	}
	for _, e := range []error{model.ErrActionCycle, model.ErrBeadCycle, model.ErrAnnotationIRTCycle, model.ErrCMapCycle, model.ErrColorSpaceCycle} {
		if got := nameValidatorRefusal(e); got != e {
			t.Errorf("%v is declared unnamed and came back as %v", e, got)
		}
	}
}

// TestNoBoundedWalkRefusalIsLeftUndecided is the census over pdfcpu's own list: every `Err…` in
// `model/recursion.go` is named above or declared unnamed with a reason, so a pdfcpu bump that adds one fails here.
func TestNoBoundedWalkRefusalIsLeftUndecided(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/pdfcpu/pdfcpu").Output()
	if err != nil {
		t.Skipf("cannot locate the pdfcpu module: %v", err)
	}
	src, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), "pkg", "pdfcpu", "model", "recursion.go"))
	if err != nil {
		t.Skipf("cannot read pdfcpu's recursion.go: %v", err)
	}
	found := regexp.MustCompile(`(?m)^var (Err[A-Za-z]+) = errors\.New`).FindAllStringSubmatch(string(src), -1)
	if len(found) < 20 {
		t.Fatalf("stimulus: only %d errors found in recursion.go — the pattern no longer reads it", len(found))
	}
	named := map[string]bool{}
	for e := range expectedNames {
		for _, f := range found {
			if errorName(e) == f[1] {
				named[f[1]] = true
			}
		}
	}
	var undecided []string
	for _, f := range found {
		if !named[f[1]] && unnamedByDecision[f[1]] == "" {
			undecided = append(undecided, f[1])
		}
	}
	sort.Strings(undecided)
	if len(undecided) > 0 {
		t.Errorf("pdfcpu's recursion.go has bounded-walk errors nib neither names nor declares unnamed: %v", undecided)
	}
	for name := range unnamedByDecision {
		ok := false
		for _, f := range found {
			ok = ok || f[1] == name
		}
		if !ok {
			t.Errorf("%s is declared unnamed but pdfcpu no longer defines it — drop the row", name)
		}
	}
}

// errorName is the identifier of one of expectedNames' keys, found by comparing against the module's own variables.
func errorName(e error) string {
	for name, v := range map[string]error{
		"ErrMaxRecursionDepthExceeded": model.ErrMaxRecursionDepthExceeded,
		"ErrFormFieldCycle":            model.ErrFormFieldCycle, "ErrNameTreeCycle": model.ErrNameTreeCycle,
		"ErrNumberTreeCycle": model.ErrNumberTreeCycle, "ErrPageTreeCycle": model.ErrPageTreeCycle,
		"ErrStructureTreeCycle": model.ErrStructureTreeCycle, "ErrFunctionCycle": model.ErrFunctionCycle,
		"ErrHalftoneCycle": model.ErrHalftoneCycle, "ErrMediaClipCycle": model.ErrMediaClipCycle,
		"ErrPatternCycle": model.ErrPatternCycle, "ErrRenditionCycle": model.ErrRenditionCycle,
		"ErrTargetCycle":        model.ErrTargetCycle,
		"ErrFormFieldDuplicate": model.ErrFormFieldDuplicate, "ErrNameTreeDuplicate": model.ErrNameTreeDuplicate,
		"ErrNumberTreeDuplicate": model.ErrNumberTreeDuplicate, "ErrPageTreeDuplicate": model.ErrPageTreeDuplicate,
		"ErrStructureTreeDuplicate": model.ErrStructureTreeDuplicate,
	} {
		if v == e {
			return name
		}
	}
	return ""
}
