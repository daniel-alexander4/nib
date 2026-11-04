package pdfops

import (
	"slices"
	"testing"
)

func TestDeclaredLimitsSayEachGapAndOnlyThatGap(t *testing.T) {
	if got := declaredLimits(true, ""); len(got) != 0 {
		t.Fatalf("both enforced yet %q", got)
	}
	if got := declaredLimits(false, ""); !slices.Equal(got, []string{ReapUnsupportedNote}) {
		t.Fatalf("no reaper: %q", got)
	}
	if got := declaredLimits(true, "ceiling gap"); !slices.Equal(got, []string{"ceiling gap"}) {
		t.Fatalf("no ceiling: %q", got)
	}
	if got := declaredLimits(false, "ceiling gap"); len(got) != 2 {
		t.Fatalf("both gaps: %q", got)
	}
}

// On the platform under test the public door reads the real constants: Linux enforces both, so it is silent there.
func TestDeclaredLimitsReadTheBuildTaggedConstants(t *testing.T) {
	got := DeclaredLimits()
	if CeilingEnforced && len(got) != 0 {
		t.Fatalf("an enforcing platform declared %q", got)
	}
	if !CeilingEnforced && !slices.Contains(got, CeilingUnenforcedNote) {
		t.Fatalf("the ceiling gap is declared by its constant yet DeclaredLimits says %q", got)
	}
}
