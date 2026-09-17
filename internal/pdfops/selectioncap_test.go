package pdfops

import (
	"strings"
	"testing"
)

// TestASelectionCannotAmplifyTheDocumentItSelectsFrom is `/pending 526`'s reader. The two
// user-text doors (`splitPages`, `splitSel`) validate nothing beyond splitting on commas, so a
// repeated range buys pages of output for bytes of input — measured at 10.4 KiB of peak heap and
// 0.09 ms per output page, flat to 200,000 pages, against ~5 bytes of typed input per page.
//
// Three arms, and all three are needed. The refusal alone passes for a door that refuses
// everything; the two acceptances alone pass for a door that refuses nothing. The repeat arm is
// the one that separates "amplification is capped" from "repeats are banned" — repeats are how
// `DuplicatePage` works and they stay legal up to the ceiling.
func TestASelectionCannotAmplifyTheDocumentItSelectsFrom(t *testing.T) {
	pdf := pagesPDF(t, 10) // ceiling is 10*10 = 100 pages

	t.Run("a selection past the ceiling is refused before anything is cloned", func(t *testing.T) {
		sel := make([]string, 11) // 11 x "1-10" = 110 pages, past the 100 the ceiling allows
		for i := range sel {
			sel[i] = "1-10"
		}
		out, err := Collect(pdf, sel)
		if err == nil {
			t.Fatalf("a selection expanding to 110 pages of a 10-page document was accepted (%d bytes out)", len(out))
		}
		// The message must name the numbers, or the user cannot tell what to type instead.
		for _, want := range []string{"110", "10-page", "100"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal does not say %q: %v", want, err)
			}
		}
	})

	t.Run("a selection at the ceiling is accepted", func(t *testing.T) {
		sel := make([]string, 10) // 10 x "1-10" = exactly 100
		for i := range sel {
			sel[i] = "1-10"
		}
		out, err := Collect(pdf, sel)
		if err != nil {
			t.Fatalf("a selection expanding to exactly the ceiling was refused: %v", err)
		}
		if n, cerr := PageCount(out); cerr != nil || n != 100 {
			t.Fatalf("expected 100 pages out, got %d (%v)", n, cerr)
		}
	})

	t.Run("a small document keeps its floor", func(t *testing.T) {
		one := pagesPDF(t, 1) // 1*10 = 10 is below the floor, so 100 copies must still work
		sel := make([]string, 100)
		for i := range sel {
			sel[i] = "1"
		}
		out, err := Collect(one, sel)
		if err != nil {
			t.Fatalf("100 copies of a one-page document were refused: %v", err)
		}
		if n, cerr := PageCount(out); cerr != nil || n != 100 {
			t.Fatalf("expected 100 pages out, got %d (%v)", n, cerr)
		}
	})

	t.Run("the operations nib generates itself are unaffected", func(t *testing.T) {
		// Booklet emits exactly n and DuplicatePage n+1 — both an order of magnitude inside the
		// ceiling. This is the arm that fails if the factor is ever tightened to something a real
		// operation reaches.
		if _, err := Booklet(pdf, false); err != nil {
			t.Errorf("Booklet: %v", err)
		}
		if _, err := DuplicatePage(pdf, 3); err != nil {
			t.Errorf("DuplicatePage: %v", err)
		}
	})
}
