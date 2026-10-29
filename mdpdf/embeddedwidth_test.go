package mdpdf

import (
	"context"
	"testing"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/font"
)

// The remembered advance is the one pdfcpu takes: an installed face measured through the cache is
// the width `font.TextWidth` answers, on the first call (every rune a miss) and the second (every
// rune a hit), at more than one size. The stimulus is asserted first — the face is installed and
// the cache holds its runes — so a cache that was never consulted cannot pass for a correct one.
func TestTheRememberedAdvanceIsTheWidthPdfcpuTakes(t *testing.T) {
	const name = "NotoSansThai-Regular"
	const text = "สวัสดีครับ hello, world"
	if _, err := ConvertWithFonts([]byte(text), []Font{{Name: name, Data: readTestFont(t), Covers: []*unicode.RangeTable{unicode.Thai}}}); err != nil {
		t.Fatalf("installing the face: %v", err)
	}
	runeAdvance.Lock()
	runeAdvance.m = map[runeKey]int{}
	runeAdvance.Unlock()

	for pass := 1; pass <= 2; pass++ {
		for _, size := range []int{9, 12, 18} {
			want, err := font.TextWidth(context.Background(), text, name, size)
			if err != nil {
				t.Fatalf("pdfcpu: %v", err)
			}
			if want <= 0 {
				t.Fatalf("setup: pdfcpu measured %q at %v", text, want)
			}
			if got := Width(text, name, size, true); got != want {
				t.Errorf("pass %d size %d: cached width %v, pdfcpu %v", pass, size, got, want)
			}
		}
	}
	runeAdvance.RLock()
	n := len(runeAdvance.m)
	runeAdvance.RUnlock()
	if n == 0 {
		t.Error("the cache holds nothing after four measurements — the embedded path does not use it")
	}
}
