package pdfops

import (
	"fmt"
	"strings"
	"testing"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// TestAPageWrittenVerticallyIsReportedNotReadAsHorizontalLines — /pending 795 W5, /pending 503's sibling. An
// Identity-V run has an upright matrix, so it was not `rotated`, and the page's layout — the tag proposer's
// reader, which keeps no glyphs and so never had the run's face — grouped it as horizontal lines with no
// note. Reflow already refused it (`causeVertical`); the layout now says so too, and the same page under
// Identity-H says nothing.
func TestAPageWrittenVerticallyIsReportedNotReadAsHorizontalLines(t *testing.T) {
	cmap := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap\n1 begincodespacerange <0000> <FFFF> endcodespacerange\n" +
		"3 beginbfchar <0001> <0061> <0002> <0062> <0003> <0020> endbfchar\nendcmap CMapName currentdict /CMap defineresource pop end end"
	show := "BT /F1 10 Tf 14 TL 72 700 Td <000100030002> Tj T* <00020003 0001> Tj ET"
	layout := func(enc string) pageLayout {
		t.Helper()
		pdf := assembleFixture(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
			4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(show), show),
			5: "<< /Type /Font /Subtype /Type0 /BaseFont /V /Encoding /" + enc + " /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>",
			6: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /V /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor 8 0 R /CIDToGIDMap /Identity /DW 500 >>",
			8: "<< /Type /FontDescriptor /FontName /V /Flags 4 /FontBBox [0 -200 1000 800] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 >>",
			7: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap),
		})
		ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("%s: %v", enc, err)
		}
		l, err := readPageLayout(ctx, pageAt(ctx, nil, 1))
		if err != nil {
			t.Fatalf("%s: %v", enc, err)
		}
		if len(l.paragraphs) == 0 {
			t.Fatalf("setup: the %s page read as no text, so this asserts nothing", enc)
		}
		return l
	}
	if got := layout("Identity-V").unsupported; !strings.Contains(got, "2 run(s) of text are written vertically") {
		t.Errorf("a page of Identity-V text reports %q, want its two runs named as written vertically", got)
	}
	if got := layout("Identity-H").unsupported; got != "" {
		t.Errorf("the same page under Identity-H reports %q, want nothing", got)
	}
}
