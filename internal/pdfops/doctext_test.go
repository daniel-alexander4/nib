package pdfops

import (
	"math"
	"testing"
)

// twoPageDoc is a document whose pages differ in every way ListPages reports: page 1 is plain, page 2 is turned a quarter,
// has a crop box inside its media box, and sets its text in a font whose /Differences name a glyph the list does not carry.
func twoPageDoc() []byte {
	font1 := "<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+NotACoreFont /Encoding /WinAnsiEncoding" + widths95 + " >>"
	font2 := "<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+OtherFont /Encoding << /Type /Encoding /BaseEncoding /WinAnsiEncoding /Differences [66 /g123] >>" + widths95 + " >>"
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 6 0 R] /Count 2 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: streamObj("BT /F1 20 Tf 72 700 Td (Hello) Tj 0 -30 Td (World) Tj ET\n"),
		5: font1,
		6: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /CropBox [50 100 450 700] /Rotate 90 /Resources << /Font << /F1 8 0 R >> >> /Contents 7 0 R >>",
		7: streamObj("BT /F1 20 Tf 100 600 Td (ABA) Tj ET\n"),
		8: font2,
	})
}

// TestListPagesGivesTheSizeAsShown — a person measures the visible box with /Rotate applied, and the boxes come back as the
// file gives them. Page 2: crop 400×600, turned a quarter, so shown 600×400.
func TestListPagesGivesTheSizeAsShown(t *testing.T) {
	got, err := ListPages(twoPageDoc())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d pages, want 2", len(got))
	}
	want := []PageInfo{
		{Page: 1, Width: 612, Height: 792, Rotate: 0, Media: [4]float64{0, 0, 612, 792}, Crop: [4]float64{0, 0, 612, 792}},
		{Page: 2, Width: 600, Height: 400, Rotate: 90, Media: [4]float64{0, 0, 600, 800}, Crop: [4]float64{50, 100, 450, 700}},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("page %d: %+v, want %+v", i+1, got[i], want[i])
		}
	}
}

// TestDocumentTextIsEachPagesMapInOnePass — the whole-document read is the page map's, page by page: the same runs, in
// points on the displayed page, with the unread glyphs carried through (/pending 924, 925).
func TestDocumentTextIsEachPagesMapInOnePass(t *testing.T) {
	pdf := twoPageDoc()
	got, err := DocumentText(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d pages, want 2", len(got))
	}
	for i, pt := range got {
		m, err := MapPage(pdf, i+1)
		if err != nil {
			t.Fatal(err)
		}
		if pt.Page != i+1 || pt.Width != m.Width || pt.Height != m.Height || pt.Unread != m.Unread || pt.NoText != m.NoText || len(pt.Runs) != len(m.Text) {
			t.Fatalf("page %d: %+v is not the map's %+v", i+1, pt, m)
		}
		for j, r := range pt.Runs {
			mt := m.Text[j]
			wantRect := [4]float64{mt.Rect[0] * m.Width, mt.Rect[1] * m.Height, mt.Rect[2] * m.Width, mt.Rect[3] * m.Height}
			if r.Text != mt.Text || r.Rect != wantRect || math.Abs(r.Size-mt.Size*m.Height) > 1e-9 {
				t.Errorf("page %d run %d: %+v, map %+v", i+1, j, r, mt)
			}
		}
	}
	if got[0].Unread != 0 || len(got[0].Runs) != 2 || got[0].Runs[0].Text != "Hello" || got[0].Runs[1].Text != "World" {
		t.Errorf("page 1: %+v, want Hello and World with nothing unread", got[0])
	}
	// Page 2: code 'B' is renamed to /g123, which the glyph list cannot read; 'A' reads by the base.
	if got[1].Unread != 1 || len(got[1].Runs) != 1 || got[1].Runs[0].Text != "AA" {
		t.Errorf("page 2: %+v, want AA with one glyph unread", got[1])
	}
	// The run's size is the 20 pt font at the page's own scale: a turned page does not change what 20 pt is.
	if s := got[0].Runs[0].Size; math.Abs(s-20) > 0.01 {
		t.Errorf("page 1 run size %.3f pt, want 20", s)
	}
}

func TestADocumentWithNoPagesListsNone(t *testing.T) {
	pdf := assembleFixture(map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>", 2: "<< /Type /Pages /Kids [] /Count 0 >>"})
	if got, err := ListPages(pdf); err == nil && len(got) != 0 {
		t.Errorf("listed %d pages of a document with none", len(got))
	}
}
