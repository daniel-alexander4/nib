package pdfops

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// bigMarkdown is a long agreement — the shape /pending 488 was opened against.
//
// It matters that this goes through `tagMarkdown` rather than `testpdf.Text`: the authoring path
// EMBEDS its faces (Liberation/Roboto), and re-decoding those embedded programs once per page was
// 74.6% of the digest's cost. A core-font document has nothing to re-decode and would have made
// the whole item look imaginary.
func bigMarkdown(clauses int) []byte {
	var b strings.Builder
	b.WriteString("# Master Agreement\n\n")
	for i := 1; i <= clauses; i++ {
		fmt.Fprintf(&b, "## Clause %d\n\nThe party of the first part shall, in clause %d, "+
			"observe every obligation set out in the schedule annexed hereto.\n\n", i, i)
	}
	return []byte(b.String())
}

// TestDigestScale is the stopwatch behind /pending 488's numbers, kept so they can be re-taken
// rather than quoted.
//
// **It is skipped unless asked for, and that is deliberate.** It is a measurement, not an
// assertion: on a shared machine the figures move with the load, so a threshold here would be a
// flake. The falsifiable checks for the same change are counters, in `digestfastpath_test.go`.
//
// Measured 2026-09-16, load average ~5-6 on 8 cores, median of three runs per size, before and
// after taken in the same session and alternating:
//
//	pages   before    after
//	  500   1.211s    0.456s    2.7x
//	 1000   2.991s    0.994s    3.0x
//	 2000   6.751s    1.649s    4.1x
//
// The ratio rises with the page count because what was removed is quadratic in pages and what is
// left is not. The page tree pdfcpu writes is FLAT — one interior node with a kid per page,
// measured — which is what makes `ctx.PageDict(i)` O(pages) per call.
//
//	NIB_MEASURE=1 NIB_MEASURE_CLAUSES=16000 go test ./internal/pdfops/ -run TestDigestScale -v
func TestDigestScale(t *testing.T) {
	if os.Getenv("NIB_MEASURE") == "" {
		t.Skip("a measurement, not an assertion — set NIB_MEASURE=1 to take it")
	}
	n := 8000
	if v := os.Getenv("NIB_MEASURE_CLAUSES"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("NIB_MEASURE_CLAUSES=%q: %v", v, err)
		}
		n = parsed
	}
	t0 := time.Now()
	pdf, err := tagMarkdown(bigMarkdown(n), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	pages, err := PageCount(pdf)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("built %d clauses -> %d bytes, %d pages, in %s", n, len(pdf), pages,
		time.Since(t0).Round(time.Millisecond))
	for i := 0; i < 3; i++ {
		t1 := time.Now()
		d, st, err := contentDigest(pdf)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("run %d: %s  %d decodes  fastPath=%v  %s", i+1,
			time.Since(t1).Round(time.Millisecond), st.decodes, st.fastPath, d[:16])
	}
}
