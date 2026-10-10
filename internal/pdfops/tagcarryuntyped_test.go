package pdfops

import (
	"fmt"
	"testing"
)

// untypedElementsFixture is two tagged pages whose structure elements carry no `/Type` when typed is false —
// the key is optional on an element (ISO 32000-1 table 323) — and are otherwise the same document.
func untypedElementsFixture(typed bool) []byte {
	ty := ""
	if typed {
		ty = "/Type /StructElem "
	}
	c1 := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (page one) Tj ET\nEMC\n"
	c2 := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (page two) Tj ET\nEMC\n"
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R 13 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c1), c1),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /ParentTreeNextKey 2 >>",
		8:  "<< " + ty + "/S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9:  "<< /Nums [0 [8 0 R] 1 [10 0 R]] >>",
		10: "<< " + ty + "/S /P /P 7 0 R /Pg 13 0 R /K [0] >>",
		13: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 14 0 R /StructParents 1 >>",
		14: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c2), c2),
	})
}

// TestTheCarryRepointsAnElementWithNoType — /pending 632. The n-up carry rewrote only a dictionary with an explicit
// `/Type`, while the reader (`readKid`) takes an untyped one under `/K` as the element it is: the same two pages
// measured `carried` with the key and `dropped` without it, through NUp and Booklet alike. Honest — the drop was
// declared — and all the tagging of any producer that omits an optional key.
func TestTheCarryRepointsAnElementWithNoType(t *testing.T) {
	for _, typed := range []bool{true, false} {
		src := untypedElementsFixture(typed)
		if d := completeness(t, src); len(d) > 0 {
			t.Fatalf("setup (typed=%v): the fixture is incomplete before the n-up:%s", typed, defectLines(d))
		}
		for name, op := range map[string]func() ([]byte, error){
			"NUp":     func() ([]byte, error) { return NUp(src, 2, false) },
			"Booklet": func() ([]byte, error) { return Booklet(src, false) },
		} {
			out, err := op()
			if err != nil {
				t.Fatalf("%s (typed=%v): %v", name, typed, err)
			}
			if got := fate(out); got != "carried" {
				t.Errorf("%s of elements with /Type=%v measures %q, want %q", name, typed, got, "carried")
				continue
			}
			if d := completeness(t, out); len(d) > 0 {
				t.Errorf("%s (typed=%v) is not completely carried:%s", name, typed, defectLines(d))
			}
		}
	}
}
