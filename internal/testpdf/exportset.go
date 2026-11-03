package testpdf

import (
	"fmt"
	"strings"
)

// ExportFixture is one hand-built document of the export set (PLAN-import-export P02.S03a, W22): generated in-test,
// committed as code and never as binaries, and read by the document model's oracle (D34) once the model exists.
// Expect names what each is FOR, so a fixture cannot drift from the property it exists to hold.
type ExportFixture struct {
	Name   string
	PDF    []byte
	Expect string
}

var widths95 = strings.TrimSpace(strings.Repeat("500 ", 95))

var (
	exportFont = "<< /Type /Font /Subtype /TrueType /BaseFont /Arial /FirstChar 32 /LastChar 126 /Widths [" + widths95 + "] /Encoding /WinAnsiEncoding >>"
	exportBold = "<< /Type /Font /Subtype /TrueType /BaseFont /Arial-BoldMT /FirstChar 32 /LastChar 126 /Widths [" + widths95 + "] /Encoding /WinAnsiEncoding >>"
	// Hebrew letters alef, bet, gimel as codes 1-3 of a Type 0 font whose ToUnicode says so.
	exportHebrew = "<< /Type /Font /Subtype /Type0 /BaseFont /Hebrew /Encoding /Identity-H /ToUnicode 8 0 R /DescendantFonts [7 0 R] >>"
	hebrewCID    = "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Hebrew /DW 600 /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> " +
		"/FontDescriptor << /Type /FontDescriptor /FontName /Hebrew /Flags 4 /FontBBox [0 0 1000 1000] /Ascent 800 /Descent -200 /CapHeight 700 /ItalicAngle 0 /StemV 80 >> >>"
	hebrewMap = "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /Adobe-Identity-UCS def /CMapType 2 def " +
		"1 begincodespacerange <0000> <FFFF> endcodespacerange 3 beginbfchar <0001> <05D0> <0002> <05D1> <0003> <05D2> endbfchar " +
		"endcmap CMapName currentdict /CMap defineresource pop end end"
)

func stream(body string) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(body), body)
}

func onePageWith(content, fonts, pageExtra string, extra map[int]string) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << " + fonts + " >> /Contents 4 0 R " + pageExtra + ">>",
		4: stream(content),
	}
	for n, o := range extra {
		objs[n] = o
	}
	return assemble(objs)
}

func text(font string, size, x, y int, s string) string {
	return fmt.Sprintf("BT /%s %d Tf %d %d Td (%s) Tj ET\n", font, size, x, y, s)
}

// ExportSet is the fixtures the export oracle needs that need no structure tree: two columns, right-to-left text, a
// scan, text beneath a box, and styled text with links. A tagged document and a merged-cell table need the tree
// writers and are built with the model reader (P02.S03c).
func ExportSet() []ExportFixture {
	fonts := "/Font << /F1 5 0 R /F2 6 0 R >>"
	base := map[int]string{5: exportFont, 6: exportBold}
	twoCol := ""
	for i := 0; i < 6; i++ {
		y := 700 - i*14
		twoCol += text("F1", 10, 72, y, fmt.Sprintf("left column line %d runs on", i)) + text("F1", 10, 330, y, fmt.Sprintf("right column line %d runs on", i))
	}
	rtl := "BT /F1 12 Tf 400 700 Td <000300020001> Tj ET\n"
	scan := "q 612 0 0 792 0 0 cm /Im1 Do Q\n"
	covered := text("F1", 12, 100, 600, "visible heading") + text("F1", 12, 100, 500, "secret under the box") + "0 g 90 490 300 30 re f\n"
	linked := text("F2", 14, 72, 700, "A bold title") + text("F1", 10, 72, 680, "plain words then a link here") + text("F1", 10, 72, 660, "after")
	annot := "<< /Type /Annot /Subtype /Link /F 4 /Rect [200 678 260 690] /A << /S /URI /URI (https://example.org/x) >> >>"
	return []ExportFixture{
		{"two-column", onePageWith(twoCol, fonts, "", base), "left column reads before the right"},
		{"right-to-left", onePageWith(rtl, "/Font << /F1 5 0 R >>", "", map[int]string{5: exportHebrew, 7: hebrewCID, 8: stream(hebrewMap)}), "no /Lang; Hebrew letters"},
		{"scanned", onePageWith(scan, "/XObject << /Im1 5 0 R >>", "", map[int]string{
			5: "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n\x00\xff\xff\x00\nendstream"}), "no text at all"},
		{"covered", onePageWith(covered, fonts, "", base), "'secret under the box' is drawn under an opaque box"},
		{"styled-linked", onePageWith(linked, fonts, "/Annots [7 0 R]", map[int]string{5: exportFont, 6: exportBold, 7: annot}), "bold title; a link over 'link'"},
	}
}
