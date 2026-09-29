package pdfops

import (
	"strings"
	"testing"
)

// TestUniqueNameNeverRepeatsASuffixedName — /pending 594.
//
// The base was the only thing recorded, so a batch whose own names already carried a suffix got
// the same name twice and the later part replaced the earlier: a mail-merge zip, a split folder or
// a `pagenum --continuous` output set one file short, with nothing said. Both orders are driven,
// because the suffix can arrive before or after the base it collides with, and case is folded
// because the names land on macOS/Windows filesystems.
func TestUniqueNameNeverRepeatsASuffixedName(t *testing.T) {
	for _, batch := range [][]string{
		{"A", "A", "A (2)"},
		{"A", "A (2)", "A"},
		{"A (2)", "A", "A"},
		{"a", "A (2)", "A", "A (3)", "a"},
		{"", "bookmark-2", ""},
	} {
		seen := map[string]int{}
		got := map[string]string{}
		for i, b := range batch {
			n := UniqueName(b, i+1, seen)
			if prev, dup := got[strings.ToLower(n)]; dup {
				t.Errorf("batch %q: %q returned twice (for %q and %q) — the later part replaces the earlier",
					batch, n, prev, b)
			}
			got[strings.ToLower(n)] = b
		}
	}
}
