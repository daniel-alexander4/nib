package pdfops

import (
	"errors"
	"strings"
	"testing"
)

// TestABookmarkOrASplitRangePastTheDocumentIsThePageRangeRefusal — `/pending 834` (2). `SetOutline` and
// `PageSpans` each tested a page against the page count for themselves and refused with a plain error, so
// neither was the package's page-range refusal and no caller could tell it from any other bad input. Each
// keeps its own words; under them is the one door's `PageRangeError`.
func TestABookmarkOrASplitRangePastTheDocumentIsThePageRangeRefusal(t *testing.T) {
	base := sizedPDF(t, 3, 120, 160)
	for _, page := range []int{0, 9} {
		_, err := SetOutline(base, []OutlineItem{{Title: "A", Page: page}})
		var pe *PageRangeError
		if !errors.As(err, &pe) || pe.Page != page || pe.Pages != 3 {
			t.Errorf("a bookmark on page %d of 3: err = %v (%#v), want the page-range refusal naming that page", page, err, pe)
		}
		if err == nil || !strings.Contains(err.Error(), `bookmark "A"`) {
			t.Errorf("a bookmark on page %d of 3: the refusal no longer names the bookmark: %v", page, err)
		}
	}
	for tok, page := range map[string]int{"0-2": 0, "2-9": 9, "9": 9} {
		_, err := PageSpans("ranges", "", tok, 5)
		var pe *PageRangeError
		if !errors.As(err, &pe) || pe.Page != page || pe.Pages != 5 {
			t.Errorf("range %q of 5: err = %v (%#v), want the page-range refusal naming page %d", tok, err, pe, page)
		}
		if err == nil || !strings.Contains(err.Error(), tok) {
			t.Errorf("range %q of 5: the refusal no longer names the range: %v", tok, err)
		}
	}
	// The control: input that is wrong for another reason is refused, and not as a page out of range.
	for _, tok := range []string{"4-2", "abc"} {
		if _, err := PageSpans("ranges", "", tok, 5); err == nil || errors.Is(err, ErrPageNotInDocument) {
			t.Errorf("range %q of 5: err = %v, want a refusal that is not the page-range one", tok, err)
		}
	}
}
