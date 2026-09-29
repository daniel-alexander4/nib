package mdpdf

import (
	"strings"
	"testing"
)

// TestWrapBreaksWhereItAlwaysHas pins where `Wrap` breaks lines, because the breaker became a shared door at
// `PLAN-text-reflow.md` P06.S03 (`BreakGreedy`, which reflow calls too) and nothing else in this package's tests sees a
// line break move: a mutation dropping the space between words left every other test green. These are the breaks
// HEAD (`71dc37be`) produced before the extraction, generated there and not written by hand (measured byte-identical across five full layouts and these three widths), so a
// change here is a change to every document mdpdf lays out and to every reflow — make it on purpose.
func TestWrapBreaksWhereItAlwaysHas(t *testing.T) {
	long := strings.Repeat("supercalifragilistic", 4)
	text := "The quick brown fox jumps over " + long + " the lazy dog"
	for _, c := range []struct {
		maxW float64
		want []string
	}{
		{80, []string{"The quick", "brown fox", "jumps over", "supercalifragilist", "icsupercalifragili", "sticsupercalifrag",
			"ilisticsupercalifr", "agilistic the lazy", "dog"}},
		{200, []string{"The quick brown fox jumps over", "supercalifragilisticsupercalifragilisticsupe",
			"rcalifragilisticsupercalifragilistic the lazy", "dog"}},
	} {
		if got := Wrap(text, "Helvetica", 11, c.maxW, false); strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("Wrap at %v:\n  got  %q\n  want %q", c.maxW, got, c.want)
		}
	}
}
