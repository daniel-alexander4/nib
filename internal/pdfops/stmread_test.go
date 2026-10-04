package pdfops

import (
	"fmt"
	"testing"
)

// TestAMalformedStmIsReadOnceAndReported — /pending 644 #4.
//
// A `/Stm` that is present and not an indirect reference was 0, "the page's own stream", to the checker
// and the Tags panel, while the artifact edit re-read the key and refused the kid as form content. One
// reading now (`structKid.stmMalformed`), and the checker says the content cannot be found.
func TestAMalformedStmIsReadOnceAndReported(t *testing.T) {
	page := "/Fm0 Do\n"
	form := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (IN THE FORM) Tj ET\nEMC\n"
	build := func(stm string) []byte {
		return assembleFixture(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 " +
				"/Resources << /Font << /F1 5 0 R >> /XObject << /Fm0 11 0 R >> >> /Contents 4 0 R >>",
			4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page),
			5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
			7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 2 >>",
			8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K " +
				"<< /Type /MCR /Pg 3 0 R /Stm " + stm + " /MCID 0 >> >>",
			9: "<< /Nums [0 [] 1 [8 0 R]] >>",
			11: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /StructParents 1 "+
				"/Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
		})
	}
	// The control: a well-formed reference is not reported as malformed.
	if d := completeness(t, build("11 0 R")); defectsKeyed(d, "stm-not-a-reference") {
		t.Errorf("a well-formed /Stm is reported as not a reference: %v", d)
	}

	malformed := build("<< >>")
	mt, merr := readTree(t, malformed)
	if merr != nil {
		t.Fatalf("readTree: %v", merr)
	}
	flagged := false
	for _, e := range mt.elems {
		for _, k := range e.kids {
			flagged = flagged || k.stmMalformed
		}
	}
	if !flagged {
		t.Error("a /Stm that is not a reference was read as absent — the page's own stream")
	}
	if d := completeness(t, malformed); !defectsKeyed(d, "stm-not-a-reference") {
		t.Errorf("a /Stm that is not a reference was not reported; the checker read the kid as page "+
			"content while the artifact edit refused it as form content. Reported: %v", d)
	}
}
