package pdfops

import (
	"testing"

	"nib/internal/testpdf"
)

// TestTheAnswerFactsSayWhenThePassIsSkipped — /pending 766 part 7. The server's one read of a document asks
// for pdfcpu's optimize pass and the door skips it where it is unaffordable; that skip is a fact about what
// every later write of the document produces, and `ReadAnswerFacts` carries it.
func TestTheAnswerFactsSayWhenThePassIsSkipped(t *testing.T) {
	f, err := ReadAnswerFacts(testpdf.PassSkipped())
	if err != nil {
		t.Fatalf("setup: the document does not read: %v", err)
	}
	if !f.Unoptimized {
		t.Error("the pass is skipped for this document and its facts do not say so; the window is told nothing")
	}
	plain, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	f, err = ReadAnswerFacts(plain)
	if err != nil {
		t.Fatal(err)
	}
	if f.Unoptimized {
		t.Error("an ordinary document's facts say its pass is skipped")
	}
}
