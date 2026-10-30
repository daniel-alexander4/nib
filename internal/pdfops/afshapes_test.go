package pdfops

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// pageAFPDF is a one-page document whose page names af as its `/AF` value, with extra objects added beside the
// usual two file specifications (21 0 R is listed in /EmbeddedFiles when listed is true; 23 0 R is its stream).
func pageAFPDF(af string, extra map[int]string, listed bool) []byte {
	names := ""
	if listed {
		names = " /Names << /EmbeddedFiles << /Names [(page.csv) 21 0 R] >> >>"
	}
	body := "BT /F1 12 Tf 20 100 Td (hi) Tj ET"
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R" + names + " >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> /AF " + af + " >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(body), body),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		21: "<< /Type /Filespec /F (page.csv) /UF (page.csv) /AFRelationship /Source /EF << /F 23 0 R >> >>",
		23: "<< /Type /EmbeddedFile /Length 10 >>\nstream\nPAGE-BYTES\nendstream",
	}
	for n, o := range extra {
		objs[n] = o
	}
	return testpdf.Assemble(objs)
}

// TestWhichPageAFShapesAreRefused holds associatedFileLost's three clauses one at a time (P02 red-proof survivors
// 5 and 6): each row is a page `/AF` whose every other clause is false, so only its own can refuse, beside a
// control that differs from it by the one thing the clause reads. A refusal is graded by a rotate, the ordinary
// rewrite.
func TestWhichPageAFShapesAreRefused(t *testing.T) {
	cases := []struct {
		name   string
		af     string
		extra  map[int]string
		listed bool
		refuse bool
	}{
		// The array is its own object: the writer drops it even though the specification it names is written.
		{"an array that is its own object", "50 0 R", map[int]string{50: "[21 0 R]"}, true, true},
		{"the same array written in place", "[21 0 R]", nil, true, false},
		// A specification written in the array that names an embedded stream: the stream is not written.
		{"an inline specification holding /EF", "[<< /Type /Filespec /F (a.csv) /EF << /F 23 0 R >> >>]", nil, true, true},
		{"an inline specification with no /EF", "[<< /Type /Filespec /F (a.csv) >>]", nil, true, false},
		// A reference to a specification no road of the writer reaches.
		{"a specification nothing else names", "[21 0 R]", nil, false, true},
	}
	for _, c := range cases {
		src := pageAFPDF(c.af, c.extra, c.listed)
		_, err := Rotate(src, []string{"1"}, 90)
		refused := errors.Is(err, pdfread.ErrAssociatedFileLost)
		if err != nil && !refused {
			t.Errorf("%s: a rotate answered %v, want success or the associated-file refusal", c.name, err)
			continue
		}
		if refused != c.refuse {
			t.Errorf("%s: refused=%v, want %v", c.name, refused, c.refuse)
		}
	}
}

// TestAFileOnTheStructureRootDoesNotRideOutInASubset — P02 red-proof survivor 4. `structcarry.go` drops the tree
// root's `/AF` because the root cannot be pruned to the pages kept, and pdfcpu v0.16.2 writes it with the files it
// names. The control reads the fixture before the subset: the file is reachable from the trailer.
func TestAFileOnTheStructureRootDoesNotRideOutInASubset(t *testing.T) {
	src := []byte(strings.Replace(string(associatedFilesShape("1.7", false, "")),
		"/Type /StructTreeRoot /K 31 0 R", "/Type /StructTreeRoot /K 31 0 R /AF [20 0 R]", 1))
	if !strings.Contains(string(src), "/StructTreeRoot /K 31 0 R /AF [20 0 R]") {
		t.Fatal("setup: the fixture's tree root was not given an /AF (the object may be compressed)")
	}
	if got := strings.Join(filesReachable(t, src), ","); !strings.Contains(got, "CATALOG-BYTES") {
		t.Fatalf("setup: the tree root's file is not reachable in the source (%q)", got)
	}
	out, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := filesReachable(t, out); len(got) != 0 {
		t.Errorf("a subset carries the tree root's associated file: %v", got)
	}
}
