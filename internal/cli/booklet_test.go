package cli

import (
	"strings"
	"testing"

	"nib/internal/pdfops"
)

// TestBookletSendsItsInstructionToTheReaderAndTheDocumentToStdout — `/pending 508`.
//
// `nib booklet in.pdf -o - > out.pdf` printed the folding instruction with `fmt.Println` after the
// PDF had already been written to stdout, so the one command whose output carries a printing
// instruction was the one that corrupted the file it described: the sentence landed after `%%EOF`,
// in the bytes.
//
// Both halves are asserted, because sending it nowhere would satisfy the first on its own and the
// instruction is not decoration — nothing in the PDF can say which way a printer will flip, so this
// sentence is the only thing standing between the user and a booklet with every back face inverted.
func TestBookletSendsItsInstructionToTheReaderAndTheDocumentToStdout(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf", "one", "two", "three")

	var out string
	var code int
	stderr := captureStderr(t, func() {
		out, code = captureStdout(t, func() int { return cmdBooklet([]string{in, "-o", "-"}) })
	})
	if code != 0 {
		t.Fatalf("nib booklet -o - exited %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "SHORT edge") {
		t.Errorf("the folding instruction reached neither stream — it is the only place that can say "+
			"which way to flip. stderr was:\n%s", stderr)
	}
	if strings.Contains(out, "SHORT edge") {
		t.Errorf("stdout carries the folding instruction as well as the PDF, so `nib booklet in.pdf "+
			"-o - > out.pdf` writes a corrupt document (%d bytes, ending %q)",
			len(out), out[max(0, len(out)-80):])
	}
	// And it is still a PDF that a reader gets through, which is the thing the appended sentence
	// took away.
	if n, err := pdfops.PageCount([]byte(out)); err != nil || n == 0 {
		t.Errorf("the piped output is not a readable PDF: %d page(s), %v", n, err)
	}
}
