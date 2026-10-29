package pdfops

import (
	"reflect"
	"testing"
)

// TestTheWidthCacheAnswersAsPdfcpuDoes — P02 close F06. The 600-word stamp test is a wall-clock floor and says nothing
// about whether a cached fit equals an uncached one; a cache keyed wrongly would be fast and wrong. Every word is fitted
// twice, once asking pdfcpu directly (nil) and once through ONE shared cache, in an order that revisits runes and
// sizes, and the two must agree exactly. The stimulus is asserted first: the words must actually fit, or both arms
// return "not fitted" and agree about nothing.
func TestTheWidthCacheAnswersAsPdfcpuDoes(t *testing.T) {
	fontName := ocrFontFor("eng")
	words := []string{"Total", "amount", "due", "Total", "WWWW", "iiii", "Invoice", "no.", "4,082.17", "amount", "due", "Ünïcödé"}
	rects := [][4]float64{{10, 700, 70, 712}, {10, 600, 40, 620}, {10, 500, 200, 540}, {10, 400, 90, 411}}
	shared := newRuneWidths(fontName)
	fitted := 0
	for _, w := range words {
		for _, r := range rects {
			wantPts, wantFit, wantOK := fitWord(fontName, w, r, nil)
			gotPts, gotFit, gotOK := fitWord(fontName, w, r, shared)
			if wantOK {
				fitted++
			}
			if wantPts != gotPts || wantOK != gotOK || !reflect.DeepEqual(wantFit, gotFit) {
				t.Errorf("%q in %v: uncached (%d, %v, %v) vs cached (%d, %v, %v)", w, r, wantPts, wantFit, wantOK, gotPts, gotFit, gotOK)
			}
		}
	}
	if fitted < len(words) {
		t.Fatalf("setup: only %d of %d fits succeeded, so the comparison above is mostly 'not fitted' against 'not fitted'", fitted, len(words)*len(rects))
	}
}
