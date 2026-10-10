package pdfops

import "testing"

// TestABooleanInAPropertyListDoesNotUncoverWhatItTags — /pending 632, measured before the tokenizer called a
// boolean an operand: `/P <</MCID 3 /X true>> BDC` read as a sequence with no MCID and `/Artifact <<… true>> BDC`
// as no artifact, so the path each covers was returned as uncovered — one span each. The first row is the control.
func TestABooleanInAPropertyListDoesNotUncoverWhatItTags(t *testing.T) {
	for _, src := range []string{
		"/P <</MCID 3>> BDC 0 0 m 10 10 l S EMC",
		"/P <</MCID 3 /X true>> BDC 0 0 m 10 10 l S EMC",
		"/P <</X null /MCID 3 /Y false>> BDC 0 0 m 10 10 l S EMC",
		"/Artifact <</Attached [/Top] /X true>> BDC 0 0 m 10 10 l S EMC",
	} {
		if sp, _ := uncoveredDrawingSpans(nil, nil, []byte(src), nil); len(sp) != 0 {
			t.Errorf("%d uncovered span(s) in %q, want none: the path is inside a marked sequence", len(sp), src)
		}
	}
	if sp, _ := uncoveredDrawingSpans(nil, nil, []byte("0 0 m 10 10 l S"), nil); len(sp) != 1 {
		t.Fatalf("setup: a bare path reads as %d uncovered span(s), want 1 — the rows above would pass on a reader "+
			"that sees nothing", len(sp))
	}
}
