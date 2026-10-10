package pdfops

import (
	"encoding/binary"
	"testing"

	"nib/internal/pdfread"
)

// `/pending 771` — residue of the returned-document P01 phase-close review.

// TestAnImageDrawnByAFormWithNoResourcesOfItsOwnIsCounted — a form XObject with no `/Resources` draws with
// its caller's. `countDrawings` read it as having none, so the image it drew by the PAGE's name was not a
// drawing at all and a document carrying it could be claimed tagged; the run reader had always inherited.
func TestAnImageDrawnByAFormWithNoResourcesOfItsOwnIsCounted(t *testing.T) {
	pdf := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 612 792] >>",
		3: "<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /XObject << /Fm 5 0 R /Im 6 0 R >> >> >>",
		4: stream("/Fm Do"),
		5: "<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Length 9 >>\nstream\nq /Im Do Q\nendstream",
		6: "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 1 >>\nstream\nA\nendstream",
	})
	n, err := uncoveredDrawings(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("uncoveredDrawings = %d, want 1 — the image the form draws with the page's resources, "+
			"which no marked-content sequence covers", n)
	}
}

// TestManyImagesMakeOneDocumentInTheirOrder — `ImagesToPDF` failed from about a hundred pages up: each
// part merged in put the page tree one level deeper, and the result was refused as too deep to read. The
// sizes are all different, so a page out of its place is seen too.
func TestManyImagesMakeOneDocumentInTheirOrder(t *testing.T) {
	const n = 600 // three passes of pdfread.MergeRaw's grouping
	pages := make([]RasterPage, n)
	for i := range pages {
		pages[i] = rasterPage(t, float64(20+i), 30)
	}
	pdf, err := ImagesToPDF(pages)
	if err != nil {
		t.Fatalf("ImagesToPDF over %d images: %v", n, err)
	}
	got := pdfread.Pages(readCtx(t, pdf))
	if len(got) != n {
		t.Fatalf("the document has %d pages, want %d", len(got), n)
	}
	for i, pg := range got {
		if pg.Attrs == nil || pg.Attrs.MediaBox == nil || pg.Attrs.MediaBox.Width() != float64(20+i) {
			t.Fatalf("page %d is not image %d: its attributes are %+v", i+1, i+1, pg.Attrs)
		}
	}
}

// TestARenamedFacesHeadIsSummedWithItsAdjustmentAsZero — the table directory's checksum for `head` is
// defined over the table with `checkSumAdjustment` zero. `sfntBytes` summed it with the source file's
// adjustment still in the table, so the directory entry was off by exactly that word.
func TestARenamedFacesHeadIsSummedWithItsAdjustmentAsZero(t *testing.T) {
	src, err := ocrFontFS.ReadFile("fonts/LiberationMono-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	tables, _, err := sfntTables(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables["head"]) < 12 || binary.BigEndian.Uint32(tables["head"][8:]) == 0 {
		t.Fatal("setup: the source's head carries no checkSumAdjustment, so nothing distinguishes the two sums")
	}
	renamed, err := renamedFace(src)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < int(binary.BigEndian.Uint16(renamed[4:])); i++ {
		rec := renamed[12+i*16 : 12+i*16+16]
		if string(rec[:4]) != "head" {
			continue
		}
		off, l := binary.BigEndian.Uint32(rec[8:]), binary.BigEndian.Uint32(rec[12:])
		head := append([]byte(nil), renamed[off:off+l]...)
		binary.BigEndian.PutUint32(head[8:], 0)
		if got, want := binary.BigEndian.Uint32(rec[4:]), sfntChecksum(head); got != want {
			t.Errorf("head's directory checksum is %#x, want %#x — the table summed with its checkSumAdjustment as zero", got, want)
		}
		return
	}
	t.Fatal("the renamed face has no head table")
}
