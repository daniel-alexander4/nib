package server

import (
	"bytes"
	"fmt"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// The notice half of law 2's `dropped-with-notice` — `PLAN-accessibility.md` P01.S04, ADR-031.
//
// # What this is for
//
// P01.S03 made 33 operations drop a tagging claim honestly. Nobody was told. A user who brought in a
// tagged document from LibreOffice or Word and rotated a page shipped it stripped of its
// accessibility metadata having been given no sign — the FILE stopped lying and the PERSON was still
// in the dark, which is the "claims accessible, is not" family one step removed.
//
// # Why the comparison is at the commit door and these tests are about that door
//
// It is the one place a mutation's result becomes the document, so it holds both the before and the
// after. Asking each of 33 operations to report its own fate would be ADR-009's rule inverted.

// taggedDoc is a document that arrives already tagged — the only population this notice is about.
//
// Built here rather than imported: `internal/pdfops`' corpus is test-scoped to that package, and a
// second copy of six object definitions is cheaper than exporting a fixture builder into production
// so one test in another package can reach it.
func taggedDoc() []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9: "<< /Nums [0 [8 0 R]] >>",
	}
	content := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (Tagged) Tj ET\nEMC\n"
	objs[4] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offs := map[int]int{}
	for n := 1; n <= 9; n++ {
		body, ok := objs[n]
		if !ok {
			continue
		}
		offs[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 10\n0000000000 65535 f \n")
	for n := 1; n <= 9; n++ {
		if off, ok := offs[n]; ok {
			fmt.Fprintf(&b, "%010d 00000 n \n", off)
		} else {
			b.WriteString("0000000000 65535 f \n")
		}
	}
	fmt.Fprintf(&b, "trailer\n<< /Size 10 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return b.Bytes()
}

func TestDroppingATaggingClaimIsRecordedOnTheDocument(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	_ = c
	_ = csrf

	src := taggedDoc()
	// SETUP, and it is half the test: an untagged fixture would make every assertion below pass on
	// a build that never sets the flag at all.
	if !pdfops.ClaimsTagging(src) {
		t.Fatal("setup: the fixture is not tagged, so nothing below can observe a claim being dropped")
	}
	doc := &document{data: src}
	srv.mu.Lock()
	srv.registerLocked(doc)
	srv.mu.Unlock()

	// **`Collect`, not `Rotate`, and the difference is the correction this file carries.** Measured
	// by PARSING (a byte count cannot see a compressed object stream): `Rotate` and `Optimize` carry
	// the structure tree through intact, while `Collect` and `NUp` drop the claim and the content
	// together. An earlier version of this test used `Rotate` on the belief that every operation
	// destroyed tagging, which was an artefact of the byte count and not true of any real document.
	out, err := pdfops.Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if pdfops.ClaimsTagging(out) {
		t.Fatal("setup: Collect still claims tagging, so there is no loss for the notice to record " +
			"and this test is about the wrong thing")
	}
	if err := srv.commitMutation(doc, src, out, false); err != nil {
		t.Fatal(err)
	}
	if !doc.taggingDropped {
		t.Error("an operation removed this document's accessibility structure and nothing recorded " +
			"it. The file is honest — it no longer claims to be tagged — and the user who could " +
			"re-make it is told nothing, which is the whole of what law 2's `with-notice` half is")
	}
}

// TestAnUntaggedDocumentNeverRaisesTheNotice — the population that matters most, because it is
// nearly every document. A notice that fired on documents that never had tagging would be noise on
// every edit, and the user would learn to ignore the one time it mattered.
func TestAnUntaggedDocumentNeverRaisesTheNotice(t *testing.T) {
	_, srv := startServerWith(t)
	src, err := testpdf.Text("an ordinary untagged document")
	if err != nil {
		t.Fatal(err)
	}
	if pdfops.ClaimsTagging(src) {
		t.Fatal("setup: the plain fixture claims tagging")
	}
	doc := &document{data: src}
	srv.mu.Lock()
	srv.registerLocked(doc)
	srv.mu.Unlock()

	out, rerr := pdfops.Rotate(src, nil, 90)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if err := srv.commitMutation(doc, src, out, false); err != nil {
		t.Fatal(err)
	}
	if doc.taggingDropped {
		t.Error("a document that never had tagging raised the lost-tagging notice — which would " +
			"fire on nearly every edit in the product and teach the user to ignore it")
	}
}

// TestTheNoticeIsStickyAcrossLaterEdits.
//
// Six edits after the one that dropped the tagging, the tagging is still gone. A flag that cleared
// on the next commit would be gone before the user saved, which is the moment it exists for.
func TestTheNoticeIsStickyAcrossLaterEdits(t *testing.T) {
	_, srv := startServerWith(t)
	src := taggedDoc()
	doc := &document{data: src}
	srv.mu.Lock()
	srv.registerLocked(doc)
	srv.mu.Unlock()

	dropped, err := pdfops.Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.commitMutation(doc, src, dropped, false); err != nil {
		t.Fatal(err)
	}
	if !doc.taggingDropped {
		t.Fatal("setup: the first commit did not record the loss, so stickiness is untestable")
	}
	// A second, entirely innocent edit on a document that no longer claims anything.
	again, err := pdfops.Rotate(dropped, nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.commitMutation(doc, dropped, again, false); err != nil {
		t.Fatal(err)
	}
	if !doc.taggingDropped {
		t.Error("a later edit cleared the lost-tagging notice. The tagging is still gone; the flag " +
			"describes the document, not the last operation, and clearing it means the user stops " +
			"being told before the moment they save")
	}
}

// TestTheFlagReachesTheClient — a flag the client cannot read is a bool in a struct.
func TestTheFlagReachesTheClient(t *testing.T) {
	_, srv := startServerWith(t)
	doc := &document{data: taggedDoc(), taggingDropped: true}
	srv.mu.Lock()
	srv.registerLocked(doc)
	srv.mu.Unlock()
	if got := srv.docResponse(doc); !got.TaggingDropped {
		t.Error("docResponse does not carry taggingDropped, so the banner never renders and the " +
			"whole notice is a field nobody reads")
	}
}
