package pdfread

import (
	"math"
	"testing"
)

// The cap is on the area the file DECLARES, read as 64-bit integers: a product of two large sides wraps negative and
// would pass the test it exists to be (/pending 941 #4).
func TestPixelsOverRefusesWhatWrapsAndWhatIsMerelyLarge(t *testing.T) {
	const max = int64(100)
	for name, c := range map[string]struct {
		w, h int64
		over bool
	}{
		"exactly the cap":        {10, 10, false},
		"one over":               {10, 11, true},
		"wide and short":         {100, 1, false},
		"wider than the cap":     {101, 1, true},
		"tall and narrow":        {1, 100, false},
		"taller than the cap":    {1, 101, true},
		"zero width":             {0, 5, true},
		"negative height":        {5, -1, true},
		"the product wraps":      {4_000_000_000, 4_000_000_000, true},
		"the product wraps to 0": {1 << 32, 1 << 32, true},
		"the largest sides":      {math.MaxInt64, math.MaxInt64, true},
	} {
		if got := pixelsOver(c.w, c.h, max); got != c.over {
			t.Errorf("%s: pixelsOver(%d,%d,%d)=%v, want %v", name, c.w, c.h, max, got, c.over)
		}
	}
}

// With a real cap a side can be within it while the product is not representable: 2^30 × 2^40 is 2^70, which is 0 in
// 64 bits, and a multiplying test would let it through.
func TestPixelsOverCatchesAProductThatWrapsWithinTheCap(t *testing.T) {
	const max = int64(1) << 30
	if !pixelsOver(max, int64(1)<<40, max) {
		t.Error("a side within the cap whose product wraps to zero passed the cap")
	}
	if !pixelsOver(int64(1)<<31, int64(1)<<33, int64(1)<<40) {
		t.Error("2^31 × 2^33 (2^64, zero in 64 bits) passed a cap of 2^40")
	}
	if pixelsOver(max, 1, max) {
		t.Error("a picture exactly at the cap was refused")
	}
}
