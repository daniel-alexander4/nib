package pdfops

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// /pending 755: ContentDigest's fast path (`collectLeaves`) is checked against pdfcpu's reading
// (`pdfread.Pages`) wherever it would be taken, and a tree the two order differently is refused as
// ErrPageTreeAmbiguous rather than hashed in an order no reader shows.

func ambPage(parent int, extra string) string {
	return fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 612 792] %s >>", parent, extra)
}

// agreeingTrees are malformed shapes the two readings order the same way (or collectLeaves refuses
// and the per-page fallback hashes) — none may be refused.
func agreeingTrees() map[string]map[int]string {
	deep := map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>"}
	const depth = 60
	for i := 0; i < depth; i++ {
		n := 2 + i
		parent := ""
		if i > 0 {
			parent = fmt.Sprintf("/Parent %d 0 R ", n-1)
		}
		deep[n] = fmt.Sprintf("<< /Type /Pages %s/Kids [%d 0 R] /Count 1 >>", parent, n+1)
	}
	deep[2+depth] = ambPage(1+depth, "")
	return map[string]map[int]string{
		// pdfcpu's parse refuses a missing /Count, so "absent" is one IntEntry cannot read: an
		// indirect one. pdfcpu then ENTERS the subtree rather than skipping it.
		"a subtree without a count": {
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
			3: "<< /Type /Pages /Parent 2 0 R /Kids [4 0 R] /Count 6 0 R >>",
			6: "1",
			4: ambPage(3, ""),
			5: ambPage(2, "/Rotate 90"),
		},
		"a subtree count too large": {
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
			3: "<< /Type /Pages /Parent 2 0 R /Kids [4 0 R] /Count 7 >>",
			4: ambPage(3, ""),
			5: ambPage(2, "/Rotate 90"),
		},
		fmt.Sprintf("a tree %d deep", depth): deep,
	}
}

// TestAnAttributePdfcpuRefusesIsNotCalledAmbiguous — an attribute pdfread's walk declines (a /Rotate
// that is not a multiple of 90, a float, an indirect; a short or long /MediaBox; a /Resources that is
// not a dictionary) never reaches the page-tree comparison: measured, pdfcpu's validation refuses every
// one inside ReadOptimized. What must hold is that the refusal stays pdfcpu's and is not reworded as a
// page-tree ambiguity.
func TestAnAttributePdfcpuRefusesIsNotCalledAmbiguous(t *testing.T) {
	for _, a := range []string{"/Rotate 45", "/Rotate 90.0", "/MediaBox [0 0 612]", "/Resources 8 0 R"} {
		pdf := testpdf.Assemble(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
			3: ambPage(2, a),
			4: ambPage(2, ""),
			8: "[1 2]",
		})
		if _, err := ContentDigest(pdf); errors.Is(err, ErrPageTreeAmbiguous) {
			t.Errorf("%s: an attribute refusal was reported as a page-tree ambiguity: %v", a, err)
		}
	}
}

func TestContentDigestRefusesAPageTreeTwoReadingsOrderDifferently(t *testing.T) {
	for name, pdf := range testpdf.AmbiguousPageTrees() {
		t.Run(name, func(t *testing.T) {
			d, err := ContentDigest(pdf)
			if !errors.Is(err, ErrPageTreeAmbiguous) {
				t.Fatalf("ContentDigest = %q, %v; want ErrPageTreeAmbiguous — the two page-tree readings "+
					"disagree, so a hash commits to an order no reader shows", short16(d), err)
			}
			if !strings.Contains(err.Error(), "re-save it") {
				t.Errorf("the refusal does not tell the user what to do: %v", err)
			}
			if err := CheckPageOrder(pdf); !errors.Is(err, ErrPageTreeAmbiguous) {
				t.Errorf("CheckPageOrder = %v, and ContentDigest refused — the two callers disagree", err)
			}
		})
	}
	for name, objs := range agreeingTrees() {
		if err := CheckPageOrder(testpdf.Assemble(objs)); err != nil {
			t.Errorf("%s: CheckPageOrder refused a tree both readings agree on: %v", name, err)
		}
	}
}

// TestAPageThatPdfcpuAnswersAsNothingIsNamed — the per-page fallback's nil-dict-with-nil-error
// branch. Shape (b) — a /Page carrying a direct /Kids — appended to by the merge a ceremony's
// prepare uses: the one walk's count no longer matches, so the fallback asks PageDict, which answers
// page 1 with nothing and no error. The sentence printed `%!w(<nil>)` there (measured through
// Convene before /pending 755's pre-prepare check).
func TestAPageThatPdfcpuAnswersAsNothingIsNamed(t *testing.T) {
	tail, err := testpdf.Text("appended")
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := Append(testpdf.AmbiguousPageTrees()["a page with a direct kids array"], tail)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ContentDigest(pdf)
	if err == nil {
		t.Fatal("setup: the document hashed, so the nil-dict branch was not reached")
	}
	if strings.Contains(err.Error(), "%!") || !strings.Contains(err.Error(), "no page at that number") {
		t.Errorf("the nil-dict refusal reads %q — want its own sentence, not a formatted nil", err)
	}
}

func TestContentDigestHashesAMalformedTreeTheTwoReadingsAgreeOn(t *testing.T) {
	for name, objs := range agreeingTrees() {
		t.Run(name, func(t *testing.T) {
			pdf := testpdf.Assemble(objs)
			d, err := ContentDigest(pdf)
			if err != nil {
				t.Fatalf("ContentDigest refused a tree both readings order the same way: %v", err)
			}
			if want, err := referenceContentDigest(pdf); err != nil || d != want {
				t.Errorf("digest %s, reference walk %s (%v) — the value moved", short16(d), short16(want), err)
			}
		})
	}
}
