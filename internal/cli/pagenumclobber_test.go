package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
)

// TestContinuousPagenumNeverWritesOverALaterInput — /pending 663.
//
// # The defect, reproduced by the P06 phase-close review (R7 #1) before this existed
//
// `nib pagenum --continuous x/a.pdf out/a.pdf --out-dir out`: input 1's output name is
// `out/a.pdf`, which IS input 2. The loop wrote it before reading input 2, so the 3-page
// document vanished, both outputs held one page, and the command exited 0. `UniqueName` could
// not see it — it dedupes outputs against outputs, and the two inputs live in different folders.
//
// # The observable is input 2's bytes, not the exit code
//
// Refusing after the first write would already have destroyed it. Both --total (which counts
// every input first, then re-reads each while stamping) and the plain run are driven, because
// the counting pass reading input 2 intact does not save it: the stamping pass re-reads it from
// disk after input 1's output has landed.
func TestContinuousPagenumNeverWritesOverALaterInput(t *testing.T) {
	for _, total := range []bool{false, true} {
		root := t.TempDir()
		x := filepath.Join(root, "x")
		out := filepath.Join(root, "out")
		for _, d := range []string{x, out} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		first := writePDF(t, x, "a.pdf", "first-doc-p1")
		second := writePDF(t, out, "a.pdf", "second-doc-p1", "second-doc-p2", "second-doc-p3")
		original := readPDF(t, second)

		code := runContinuousPagenum([]string{first, second},
			pdfops.PageNumberStyle{Start: 1, Size: 11, OfTotal: total}, false, out, "")

		if got := readPDF(t, second); !bytes.Equal(got, original) {
			n, _ := pdfops.PageCount(got)
			t.Errorf("--total=%v: input 2 (%s, 3 pages) was overwritten before it was read — "+
				"it now holds %d page(s)", total, second, n)
		}
		if code == 0 {
			t.Errorf("--total=%v: an output naming another input must be refused, got exit 0", total)
		}
		ents, err := os.ReadDir(out)
		if err != nil {
			t.Fatal(err)
		}
		if len(ents) != 1 {
			t.Errorf("--total=%v: the refusal must come before the first write; %s holds %d entries",
				total, out, len(ents))
		}
	}
}

// TestContinuousPagenumStillRewritesAnInputOverItself — the SELF-OVERWRITE EXEMPT the fix keeps.
// An input whose own output lands on it is "stamp these and put them back" and must still work;
// the refusal above is for ANOTHER input only.
func TestContinuousPagenumStillRewritesAnInputOverItself(t *testing.T) {
	dir := t.TempDir()
	a := writePDF(t, dir, "a.pdf", "one", "two")
	b := writePDF(t, dir, "b.pdf", "three")
	before := readPDF(t, a)
	if code := runContinuousPagenum([]string{a, b}, pdfops.PageNumberStyle{Start: 1, Size: 11}, false, dir, ""); code != 0 {
		t.Fatalf("stamping inputs back into their own folder must succeed, got exit %d", code)
	}
	after := readPDF(t, a)
	if bytes.Equal(before, after) {
		t.Error("a.pdf was not stamped in place")
	}
	if n, _ := pdfops.PageCount(after); n != 2 {
		t.Errorf("a.pdf holds %d pages after stamping, want 2", n)
	}
}
