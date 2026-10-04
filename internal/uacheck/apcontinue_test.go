package uacheck

import (
	"strconv"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestAnUnreadAppearanceDoesNotEndTheWalkOfTheNext — /pending 727. One annotation whose /AP nib cannot
// read used to `return` from the appearance walk, so every annotation after it went unread, and a rule
// that fails on what WAS walked before it reads `contentErr` lost the failure: 7.2 t29 answered
// CannotCheck on a document whose second appearance writes a marked-content /Lang veraPDF fails.
//
// The first annotation's /AP names an object the file does not hold; pdfcpu's validator refuses every
// other non-dictionary /AP at open, so this is the shape that reaches the branch today.
func TestAnUnreadAppearanceDoesNotEndTheWalkOfTheNext(t *testing.T) {
	const ap = "/Span << /Lang (en_US!) >> BDC EMC"
	pdf := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Lang (en) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Annots [5 0 R 6 0 R] >>",
		4: "<< /Length 0 >>\nstream\n\nendstream",
		5: "<< /Type /Annot /Subtype /Square /Rect [0 0 10 10] /AP 99 0 R >>",
		6: "<< /Type /Annot /Subtype /Square /Rect [20 20 30 30] /AP << /N 7 0 R >> >>",
		7: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length " + strconv.Itoa(len(ap)) + " >>\nstream\n" + ap + "\nendstream",
	})
	rep, err := Check(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var got *Result
	unread := false
	for i := range rep.Results {
		r := &rep.Results[i]
		if r.Clause == "7.2 t29" {
			got = r
		}
		if r.Verdict == CannotCheck && strings.Contains(r.Why, "not a dictionary") {
			unread = true
		}
	}
	if got == nil {
		t.Fatal("setup: 7.2 t29 is not in the report")
	}
	// STIMULUS: the first annotation's appearance really was unread, or this document tests nothing.
	if !unread {
		t.Fatal("setup: no rule reports the first annotation's /AP as unread")
	}
	if got.Verdict != Fail || !strings.Contains(got.Why, "en_US!") {
		t.Errorf("7.2 t29 = %v (%s) — the second annotation's appearance, past one nib could not read, was "+
			"never walked, so the /Lang veraPDF fails was never seen", got.Verdict, got.Why)
	}
}
