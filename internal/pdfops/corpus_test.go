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

// claims counts the assertions a document makes about being tagged, **in the raw bytes**.
//
// **`/StructElem` here is nearly always 0 and that is not a finding** — pdfcpu writes the tree into
// a compressed object stream, so the literal never appears in the output. It is kept for DIAGNOSTIC
// output only: when a guard below fails, the raw keys are what a reader wants to see beside the
// parsed verdict. Nothing decides anything on this function's result, and
// `TestTheOracleParsesRatherThanCountingBytes` is what keeps that true.
func claims(pdf []byte) map[string]int {
	out := map[string]int{}
	for _, k := range []string{"/StructTreeRoot", "/MarkInfo", "/Marked", "/StructParents", "/StructElem"} {
		out[k] = bytes.Count(pdf, []byte(k))
	}
	return out
}

// fate classifies a document against law 1 and law 2's vocabulary, by parsing.
//
// The four values are the verdicts the tag-fate table declares, and this is what turns that table
// from a list of assertions into a list of CHECKED assertions:
//
//   - `dropped`  — no claim at all. Honest: a visible loss.
//   - `carried`  — a claim, a live tree, and every page with content described by it.
//   - `partial`  — a claim and a live tree that does not reach every page with content.
//   - `orphaned` — a claim over a tree that describes nothing in the document. **Law 1's violation.**
//
// **`orphaned` exists because the first version of this file had no word for it.** Its predicate was
// "claims tagging and has zero struct elements", which is the weakest possible reading of law 1 and
// cannot see the case that actually ships: `NUp` emits 45 elements, all of them pointing at pages
// that are no longer in the document. A census whose vocabulary cannot express the defect reports
// every operation as compliant.
func fate(pdf []byte) string {
	s := inspectTags(pdf)
	switch {
	case !s.readable:
		return "unreadable"
	case !s.claims():
		return "dropped"
	case s.orphaned():
		return "orphaned"
	case s.partial():
		return "partial"
	default:
		return "carried"
	}
}

// lies reports law 1's violation.
func lies(pdf []byte) bool { return inspectTags(pdf).orphaned() }
