package pdfops

import (
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// unreadContentDoc is a tagged two-page document whose first page is described by its tree and whose
// second page's content stream cannot be decoded — present, so not `ErrNoContent`, and unreadable. Page 2 names a
// form XObject: a page naming none draws no form whatever its content says, so `formDrawCountsOn` rightly skips it
// unread (/pending 763) and its counts would be complete.
func unreadContentDoc() []byte {
	const mc = "/P << /MCID 0 >> BDC EMC"
	return testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 8 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 5 0 R /StructParents 0 >>",
		4:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X 7 0 R >> >> /Contents 6 0 R >>",
		5:  "<< /Length 24 >>\nstream\n" + mc + "\nendstream",
		6:  "<< /Filter /FlateDecode /Length 9 >>\nstream\nnot-flate\nendstream",
		7:  "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream",
		8:  "<< /Type /StructTreeRoot /K 9 0 R /ParentTree 10 0 R >>",
		9:  "<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K 0 >>",
		10: "<< /Nums [0 [9 0 R]] >>",
	})
}

// TestAPageWhoseContentCannotBeReadIsNotABlankPage — /pending 727. Two readers of page content
// exempted `ErrNoContent` in their comments and every error in their code, so a page whose content
// could not be decoded read as a blank page: `inspectTags` did not count it undescribed, and
// `formDrawCounts` counted the forms of part of the document with no error.
func TestAPageWhoseContentCannotBeReadIsNotABlankPage(t *testing.T) {
	pdf := unreadContentDoc()

	s := inspectTags(pdf)
	if !s.readable || !s.tree {
		t.Fatalf("setup: the fixture reads as readable=%v tree=%v", s.readable, s.tree)
	}
	if s.undescribed != 1 {
		t.Errorf("undescribed = %d, want 1 — page 2's content could not be read, and an unknown page "+
			"was counted as the blank page it may not be", s.undescribed)
	}

	ctx, err := inspectionRead(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if _, derr := formDrawCounts(ctx); derr == nil || !strings.Contains(derr.Error(), "page 2") {
		t.Errorf("formDrawCounts over a page it could not read returned %v, want an error naming page 2 — "+
			"its counts are of part of the document", derr)
	}
}
