package pdfops

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// Booklet imposition (`/pending 398`).
//
// # Why the ORDER is tested on its own, separately from the imposition
//
// The permutation is the whole feature and it is the part nobody can eyeball. `n, 1, 2, n-1, …` is
// not an obvious sequence; the way it goes wrong is subtle (every other sheet's back face in the
// wrong pairing) and the symptom only appears after somebody has printed and folded a stack of
// paper. So the sequence is asserted directly, against the property that defines it: **fold the
// printed stack in half and the pages read 1, 2, 3, … in order.**
//
// That last sentence is the oracle, and `readsInOrderWhenFolded` is it in code — rather than a
// table of expected numbers, which would only restate whatever the implementation happens to do.

func TestTheBookletOrderReadsInSequenceOnceFolded(t *testing.T) {
	for _, n := range []int{4, 8, 12, 16, 40} {
		order := bookletOrder(n)
		if len(order) != n {
			t.Errorf("n=%d produced %d placements, want %d — a page is missing or duplicated, and "+
				"neither is recoverable once it is printed", n, len(order), n)
			continue
		}
		if err := readsInOrderWhenFolded(order, n); err != nil {
			t.Errorf("n=%d: %v", n, err)
		}
	}
}

// readsInOrderWhenFolded applies the definition of a saddle stitch to the sequence.
//
// Pages are placed two to a side, four to a sheet. Sheet s holds placements 4s..4s+3, which print
// as: front-left, front-right, back-left, back-right. Fold the stack and read: the front of the
// OUTERMOST sheet is the last leaf and the first leaf, the back is the second and second-to-last,
// and so on inward. So for sheet s the four placements must be exactly
// (n-2s, 1+2s, 2+2s, n-1-2s).
func readsInOrderWhenFolded(order []string, n int) error {
	for s := 0; s*4 < n; s++ {
		want := []int{n - 2*s, 1 + 2*s, 2 + 2*s, n - 1 - 2*s}
		for k, w := range want {
			i := s*4 + k
			if i >= len(order) {
				return errf("sheet %d placement %d is missing", s+1, k+1)
			}
			if order[i] != itoa(w) {
				return errf("sheet %d placement %d is page %s, want %d — folded, the booklet does "+
					"not read in order", s+1, k+1, order[i], w)
			}
		}
	}
	return nil
}

// TestABookletIsPaddedToAWholeSheet.
//
// A sheet is four pages. A five-page document that imposed five placements would leave a sheet with
// three empty faces and no way to fold it, so the pad is not politeness — it is what makes the
// output printable at all. Blanks go at the END, because a reader expects a booklet to finish with
// blank leaves and putting them at the front shifts every page number the document itself prints.
func TestABookletIsPaddedToAWholeSheet(t *testing.T) {
	for _, tc := range []struct{ pages, sheets int }{{1, 1}, {2, 1}, {4, 1}, {5, 2}, {9, 3}} {
		src, err := testpdf.Text(strings.Repeat("booklet ", 3))
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < tc.pages; i++ {
			if src, err = InsertBlank(src, i); err != nil {
				t.Fatal(err)
			}
		}
		if got, cerr := PageCount(src); cerr != nil || got != tc.pages {
			t.Fatalf("setup: fixture has %d pages (err %v), want %d", got, cerr, tc.pages)
		}
		out, berr := Booklet(src, false)
		if berr != nil {
			t.Errorf("%d pages: %v", tc.pages, berr)
			continue
		}
		got, cerr := PageCount(out)
		if cerr != nil {
			t.Errorf("%d pages: reading the result back: %v", tc.pages, cerr)
			continue
		}
		// Two placements per side, so the sheet count is the SIDE count halved: 4 pages -> 2 sides.
		want := tc.sheets * 2
		if got != want {
			t.Errorf("%d source pages imposed to %d sides, want %d (%d sheet(s) of four pages). A "+
				"partial sheet cannot be folded, and the fault only shows after somebody has "+
				"printed a stack of paper", tc.pages, got, want, tc.sheets)
		}
	}
}

func errf(format string, a ...any) error { return fmt.Errorf(format, a...) }

func itoa(n int) string { return strconv.Itoa(n) }
