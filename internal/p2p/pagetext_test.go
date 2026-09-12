package p2p

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// extractedPageText reads a page's visible text with **poppler's `pdftotext`** — the reference
// extractor this repo already uses for exactly this question (`internal/pdfops/ocr_test.go`).
//
// # Why the hand-rolled extractor had to go, at P04.S05
//
// It read the raw content stream and decoded each `(...) Tj` literal as single-byte WinAnsi, which
// was correct while these pages were drawn in Base-14 core fonts. They are now drawn in an embedded
// Type0/CID face, so a literal holds two-byte glyph indices and that decode returns
// `\x007\x00M\x00K…` — garbage that contains no phrase at all.
//
// **The direction of the failure is the point.** Every claim assertion built on this text is a
// substring check, and the load-bearing ones are NEGATIVE — *"the page no longer says two people"*.
// A garbled extraction makes every negative assertion pass silently. The existing setup guard
// caught it, which is the one reason this was a visible failure rather than a quiet loss of four
// tests.
//
// The alternative was to parse the font's `/ToUnicode` CMap here. Rejected: the document is
// demonstrably correct — `pdftotext` reads the rendered readme in full — so the defect was in the
// instrument, and replacing a decoder that silently mis-reads with a second hand-written decoder is
// the same bet again.
func extractedPageText(t *testing.T, pdf []byte, page int) string {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("SKIP (not a pass): pdftotext (poppler) is not installed, so what nib's own pages " +
			"SAY is unchecked in this run — including the negative claims about prose that must " +
			"no longer appear")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "page.pdf")
	if err := os.WriteFile(in, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	p := strconv.Itoa(page)
	out, err := exec.Command("pdftotext", "-f", p, "-l", p, in, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext page %d: %v", page, err)
	}
	// Flattened: the page is drawn one wrapped line per run, so a phrase that straddles a line
	// break exists in neither. Joining and collapsing whitespace makes a claim assertion a
	// statement about the PAGE rather than about where the wrapper happened to break.
	return strings.Join(strings.Fields(string(out)), " ")
}
