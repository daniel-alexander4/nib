package ceremony

import (
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// /pending 755: a page tree two readings order differently has no one order for DocHash to commit
// to. pdfops' own tests hold the predicate; these hold the ceremony surfaces to the sentence.

// TestConveneRefusesAPageTreeTwoReadingsOrderDifferently — on the ORIGINAL. DocHash is taken over
// the prepared copy, and the prepare's merge rewrites the tree so the ambiguity is gone by then:
// measured, without the pre-prepare check three of the four shapes convened silently in an order
// the merge chose, and (b) failed as "page 1 is unreadable: %!w(<nil>)".
func TestConveneRefusesAPageTreeTwoReadingsOrderDifferently(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	for name, pdf := range testpdf.AmbiguousPageTrees() {
		t.Run(name, func(t *testing.T) {
			_, err := Convene(pdf, conveneReq(t, cfp, afp), cert, key, now)
			if !errors.Is(err, pdfops.ErrPageTreeAmbiguous) {
				t.Fatalf("Convene = %v; want ErrPageTreeAmbiguous — the prepared copy would commit "+
					"DocHash to a page order the convener was never shown", err)
			}
			if !strings.Contains(err.Error(), pdfops.ErrPageTreeAmbiguous.Error()) {
				t.Errorf("the convener does not see the sentence verbatim: %v", err)
			}
		})
	}
}

func TestReadMirrorNamesAnAmbiguousPageTreeRatherThanCallingTheCopyDamaged(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	r.DocHash = strings.Repeat("0", 64)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, err := WriteMirror(root, r, testpdf.AmbiguousPageTrees()["a subtree count too small, root total right"]); err != nil {
		t.Fatal(err)
	}
	_, _, err := ReadMirror(root, r.ID, time.Now())
	if !errors.Is(err, pdfops.ErrPageTreeAmbiguous) {
		t.Fatalf("ReadMirror = %v; want ErrPageTreeAmbiguous", err)
	}
	if errors.Is(err, ErrMirrorDamaged) {
		t.Errorf("an intact copy of a malformed document reads as damaged: %v", err)
	}
}
