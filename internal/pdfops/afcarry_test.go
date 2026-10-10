package pdfops

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// associatedFilesPDF is a tagged one-page document carrying an associated file (ISO 32000-2 §14.13) in each of
// three places: `/AF` on the catalog, on the page and on a structure element, each naming its own file
// specification with an embedded stream. listed also enters the catalog's and the page's specifications in
// /Names /EmbeddedFiles, where PDF/A-3 requires every embedded file to be.
func associatedFilesPDF(version string, listed bool) []byte {
	spec := func(name string, stream int) string {
		return fmt.Sprintf("<< /Type /Filespec /F (%s) /UF (%s) /AFRelationship /Source /EF << /F %d 0 R >> >>", name, name, stream)
	}
	file := func(body string) string {
		return fmt.Sprintf("<< /Type /EmbeddedFile /Length %d >>\nstream\n%s\nendstream", len(body), body)
	}
	names := ""
	if listed {
		names = " /Names << /EmbeddedFiles << /Names [(catalog.csv) 20 0 R (page.csv) 21 0 R] >> >>"
	}
	content := "/P << /MCID 0 >> BDC BT /F1 12 Tf 20 100 Td (hi) Tj ET EMC"
	out := testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R /AF [20 0 R]" + names + " >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /StructParents 0 /Resources << /Font << /F1 5 0 R >> >> /AF [21 0 R] >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		20: spec("catalog.csv", 23),
		21: spec("page.csv", 24),
		22: spec("elem.csv", 25),
		23: file("CATALOG-BYTES"),
		24: file("PAGE-BYTES"),
		25: file("ELEM-BYTES"),
		30: "<< /Type /StructTreeRoot /K 31 0 R /ParentTree << /Nums [0 [31 0 R]] >> >>",
		31: "<< /Type /StructElem /S /P /P 30 0 R /Pg 3 0 R /K 0 /AF [22 0 R] >>",
	})
	return bytes.Replace(out, []byte("%PDF-1.7"), []byte("%PDF-"+version), 1)
}

// associatedFiles reports what the `/AF` on the catalog, the first page and the first structure element reaches
// in pdf, read WITHOUT the validator: the embedded stream's bytes, "dangling" for a key whose reference names no
// object in the file, or "no key".
func associatedFiles(t *testing.T, pdf []byte) (catalog, page, elem string) {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-reading the PDF: %v", err)
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	first := func(o types.Object) types.Dict {
		o, _ = xt.Dereference(o)
		if a, ok := o.(types.Array); ok && len(a) > 0 {
			o, _ = xt.Dereference(a[0])
		}
		d, _ := o.(types.Dict)
		return d
	}
	reach := func(holder types.Dict) string {
		raw, ok := holder.Find("AF")
		if !ok {
			return "no key"
		}
		spec := first(raw)
		if spec == nil {
			return "dangling"
		}
		ef, _ := xt.DereferenceDict(spec["EF"])
		sd, _, err := xt.DereferenceStreamDict(ef["F"])
		if err != nil || sd == nil || sd.Decode() != nil {
			return "specification with no stream"
		}
		return strings.TrimSpace(string(sd.Content))
	}
	pages, _ := xt.DereferenceDict(root["Pages"])
	str, _ := xt.DereferenceDict(root["StructTreeRoot"])
	return reach(root), reach(first(pages["Kids"])), reach(first(str["K"]))
}

// TestWhatARewriteDoesToAnAssociatedFile — `/pending 655`, MEASURED on a document that already carries `/AF` (the
// entry's own probe built the specification in the pass that wrote it). This test fixes nothing: it holds what
// was measured, so that the day pdfcpu's writer learns the key, the declaration on `pdfread.Write` is taken down
// with it.
//
// pdfcpu's writer serialises the catalog and each page dictionary whole and then writes what a FIXED LIST of
// their keys reaches (`write.go:276-322`, `writePages.go:75-102`, v0.13.0). `/AF` is on neither list, so the key
// is written and the object it names is not. The validator and the optimize pass are not the cause — the objects
// are in the context after `pdfread.ReadOptimized`, and pdfcpu's own unvalidated read followed by its write loses
// them the same way — so there is nothing for `pdfread.Write` to put back as it does a font (ADR-129).
func TestWhatARewriteDoesToAnAssociatedFile(t *testing.T) {
	noop := func(pdf []byte) ([]byte, error) { return writeMutated(pdf, func(*model.Context) error { return nil }) }
	facts := func(pdf []byte) string {
		c, p, e := associatedFiles(t, pdf)
		return "catalog: " + c + "; page: " + p + "; element: " + e
	}
	const whole = "catalog: CATALOG-BYTES; page: PAGE-BYTES; element: ELEM-BYTES"

	src := associatedFilesPDF("1.7", false)
	if got := facts(src); got != whole {
		t.Fatalf("setup: the fixture reads as %q, want %q", got, whole)
	}
	// Named by /AF alone: a structure element's file is carried (the structure tree is written by walking
	// it), the catalog's and the page's are lost, and each of those keys is left naming nothing.
	out, err := noop(src)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := facts(out), "catalog: dangling; page: dangling; element: ELEM-BYTES"; got != want {
		t.Errorf("after a rewrite that changes nothing: %q, measured %q — if the files are now carried, "+
			"pdfcpu's writer has changed and the declared gap on pdfread.Write is out of date", got, want)
	}
	for _, lost := range []string{"CATALOG-BYTES", "PAGE-BYTES", "catalog.csv", "page.csv"} {
		if bytes.Contains(out, []byte(lost)) {
			t.Errorf("%q is in the output's uncompressed bytes, measured absent", lost)
		}
	}

	// Entered in /Names /EmbeddedFiles as well — as PDF/A-3 requires of every embedded file — the same
	// specifications are written by that road and each /AF still reaches its file.
	out, err = noop(associatedFilesPDF("1.7", true))
	if err != nil {
		t.Fatal(err)
	}
	if got := facts(out); got != whole {
		t.Errorf("with the files also in /EmbeddedFiles: %q, want %q", got, whole)
	}

	// A PDF 2.0 document with /AF on its catalog is refused at the read, by name, so nothing rewrites it.
	if _, err := noop(associatedFilesPDF("2.0", false)); err == nil || !strings.Contains(err.Error(), `"AF" not supported`) {
		t.Errorf("a PDF 2.0 document with a catalog /AF: error %v, measured pdfcpu's refusal", err)
	}
}
