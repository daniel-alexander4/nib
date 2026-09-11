package pdfops

import (
	"bytes"
	"fmt"
)

// The tag corpus — `PLAN-accessibility.md` D12.
//
// # Why a corpus at all
//
// D12: *"This plan's fatal-bug category is 'claims accessible, is not'. Its correctness oracle is a
// golden corpus: tagged and untagged fixtures, one per structure kind and one per known-destructive
// operation, with expected verdicts."* P01.S03's acceptance asks for the fixtures to live here
// rather than inline in one test, so that every guard reads the same documents and a new guard does
// not start by inventing its own idea of what "tagged" means.
//
// # Why these are GENERATED and not committed binaries
//
// `internal/pdfops/fixtures.go`'s sibling reasoning applies: *"A checked-in binary fixture is opaque
// in review — you cannot see what changed, or what it contains, without opening it in something."*
// Every byte below is readable. The cost is stated honestly at `taggedFixture`.

// taggedFixture is a minimal but genuinely tagged PDF: `/MarkInfo /Marked true`, a `/StructTreeRoot`
// with one `/StructElem`, one `/P <</MCID 0>> BDC … EMC` run, `/StructParents` on the page, and a
// `/ParentTree` linking them.
//
// **What it is adequate for, and what it is not.** It drives law 1 — *does an operation emit a claim
// over content nothing describes* — and any tagged input exercises that. It was NOT adequate for the
// question P01.S02 asked, which was whether pdfcpu itself carries a tree: a hand-built document
// cannot tell "pdfcpu drops trees" from "this fixture is malformed", and that measurement was
// re-run against a real LibreOffice-produced PDF for exactly that reason. The recipe is recorded at
// D9 in the plan and is one `libreoffice --headless --convert-to` away; it is deliberately not
// committed, because a 28 KB producer blob is the opaque fixture this package's own comment refuses.
func taggedFixture() []byte {
	content := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (Tagged heading) Tj ET\nEMC\n"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9: "<< /Nums [0 [8 0 R]] >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offs := map[int]int{}
	maxN := 0
	for n := range objs {
		if n > maxN {
			maxN = n
		}
	}
	for n := 1; n <= maxN; n++ {
		body, ok := objs[n]
		if !ok {
			continue
		}
		offs[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", maxN+1)
	for n := 1; n <= maxN; n++ {
		if off, ok := offs[n]; ok {
			fmt.Fprintf(&b, "%010d 00000 n \n", off)
		} else {
			b.WriteString("0000000000 65535 f \n")
		}
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", maxN+1, xref)
	return b.Bytes()
}

// claims counts the assertions a document makes about being tagged, and the structure that would
// have to be there for those assertions to be true.
func claims(pdf []byte) map[string]int {
	out := map[string]int{}
	for _, k := range []string{"/StructTreeRoot", "/MarkInfo", "/Marked", "/StructParents", "/StructElem"} {
		out[k] = bytes.Count(pdf, []byte(k))
	}
	return out
}

// lies reports law 1's violation: a document that SAYS it is tagged while carrying no structure.
//
// **This is the whole predicate, and it is deliberately not "did the tree survive".** A visible loss
// is honest; a false claim is not, and it is worse than no tagging at all because it defeats the
// reader's own check. An operation that drops everything passes; an operation that carries
// everything passes; only the middle — claim without content — fails.
func lies(pdf []byte) bool {
	c := claims(pdf)
	claimed := c["/StructTreeRoot"] > 0 || c["/Marked"] > 0
	return claimed && c["/StructElem"] == 0
}
