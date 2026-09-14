package uacheck

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// The first rule, against documents whose veraPDF verdict this repo already measured — P07.S01.

// TestStructTreeRootAgreesWithWhatP05AndP06Measured.
//
// This is law 5 in miniature, on one clause. The expected verdicts are not invented: each is what
// veraPDF reported about that exact document shape during P05 and P06, recorded in the plan and in
// the instrument inventory. P07.S05 generalises it to every clause over the corpus.
func TestStructTreeRootAgreesWithWhatP05AndP06Measured(t *testing.T) {
	plain, err := testpdf.Text("no structure here")
	if err != nil {
		t.Fatal(err)
	}
	tagged := committedProposal(t, plain)

	for _, c := range []struct {
		name string
		pdf  []byte
		want Verdict
	}{
		// veraPDF failed 7.1 t11 on every untagged nib document measured in P03–P06.
		{"an untagged page", plain, Fail},
		// And passed it on every document with a tree that reaches content — P05.S05 onward.
		{"a committed proposal", tagged, Pass},
	} {
		rep, err := Check(c.pdf)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		var got Result
		for _, r := range rep.Results {
			if r.Clause == "7.1 t11" {
				got = r
			}
		}
		if got.Clause == "" {
			t.Errorf("%s: 7.1 t11 is not in the report at all, so the registry is not being run",
				c.name)
			continue
		}
		if got.Verdict != c.want {
			t.Errorf("%s: 7.1 t11 reports %v (%s), want %v — this is the verdict veraPDF gave "+
				"this document shape throughout P05 and P06", c.name, got.Verdict, got.Why, c.want)
		}
		if c.want == Fail {
			if got.Why == "" {
				t.Errorf("%s: the failure carries no reason", c.name)
			}
			if got.Where == "" {
				t.Errorf("%s: the failure does not say WHERE. P01 spent a slice discovering that "+
					"veraPDF points at `xObject[0]/contentStream[0]/content[2]`, and a checker "+
					"that says only `7.1 t11 fails` reproduces the problem it exists to solve",
					c.name)
			}
		}
	}
}

// TestAClaimedStructureThatHoldsNothingFAILS — ADR-031 law 1's first case, read from the other side.
//
// A catalog naming a `/StructTreeRoot` whose tree is empty is `tagState.orphaned()` — the state P01's
// phase review caught being rated `carried`. The checker must call that a failure and not a
// pass-on-a-technicality: the clause asks for logical structure, and an empty root is a claim with
// nothing behind it.
func TestAClaimedStructureThatHoldsNothingFAILS(t *testing.T) {
	rep, err := Check(emptyRootFixture())
	if err != nil {
		t.Fatalf("the fixture could not be read: %v", err)
	}
	var got Result
	for _, r := range rep.Results {
		if r.Clause == "7.1 t11" {
			got = r
		}
	}
	if got.Verdict != Fail {
		t.Errorf("a catalog naming a structure root that holds nothing reports %v (%s), want Fail",
			got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "no children") && !strings.Contains(got.Why, "holds nothing") {
		t.Errorf("the reason does not say the root is empty: %q", got.Why)
	}
}

// TestADocumentThatCannotBeReadIsAnErrorNotAFailingReport.
//
// A checker that answered "fails every clause" for a file it could not parse would tell the user
// their document is inaccessible when what happened is that nib could not read it. Two different
// facts, and the second is not the user's to fix.
func TestADocumentThatCannotBeReadIsAnErrorNotAFailingReport(t *testing.T) {
	for _, c := range []struct {
		name string
		pdf  []byte
	}{
		{"nothing at all", nil},
		{"not a PDF", []byte("this is not a PDF at all, not even close")},
	} {
		rep, err := Check(c.pdf)
		if err == nil {
			t.Errorf("%s: Check returned no error and a report of %d result(s) — a document nib "+
				"cannot read must not be reported as a document that fails clauses",
				c.name, len(rep.Results))
		}
		if len(rep.Results) != 0 {
			t.Errorf("%s: an unreadable document produced %d verdict(s)", c.name, len(rep.Results))
		}
	}
}

// emptyRootFixture is a document whose catalog names a `/StructTreeRoot` with an empty `/K`.
//
// Generated rather than committed, for `corpus_test.go`'s stated reason: *"A checked-in binary
// fixture is opaque in review — you cannot see what changed, or what it contains, without opening
// it in something."*
func emptyRootFixture() []byte {
	return buildPDF(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: "<< /Length 44 >>\nstream\nBT /F1 24 Tf 72 700 Td (Untagged) Tj ET\nendstream",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [] >>",
	})
}

// buildPDF assembles numbered objects into a minimal valid PDF with a correct xref table.
//
// The same assembly `internal/pdfops/corpus_test.go` uses, written here rather than exported from
// there because a test helper exported across packages to save twenty lines is a dependency between
// two test suites — and this package's fixtures are about the checker, not about pdfops.
func buildPDF(objs map[int]string) []byte {
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
