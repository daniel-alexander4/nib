package testpdf

import (
	"bytes"
	"fmt"
	stdimage "image"
	"image/color"
	"image/jpeg"
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

// bidiMap is ExportBidi's /ToUnicode: three Hebrew letters and two text-direction controls (/pending 947).
const bidiMap = "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /Adobe-Identity-UCS def /CMapType 2 def " +
	"1 begincodespacerange <0000> <FFFF> endcodespacerange 5 beginbfchar <0001> <05D0> <0002> <05D1> <0003> <05D2> <0004> <202E> <0005> <2066> endbfchar " +
	"endcmap CMapName currentdict /CMap defineresource pop end end"

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
	linked := text("F2", 14, 72, 700, "A bold title") + "BT /F1 10 Tf 72 680 Td (plain words then a ) Tj (link) Tj ( here) Tj ET\n" + text("F1", 10, 72, 660, "after")
	annot := "<< /Type /Annot /Subtype /Link /F 4 /Rect [170 678 184 690] /A << /S /URI /URI (https://example.org/x) >> >>"
	return []ExportFixture{
		{"two-column", onePageWith(twoCol, fonts, "", base), "left column reads before the right"},
		{"right-to-left", onePageWith(rtl, "/Font << /F1 5 0 R >>", "", map[int]string{5: exportHebrew, 7: hebrewCID, 8: stream(hebrewMap)}), "no /Lang; Hebrew letters"},
		{"scanned", onePageWith(scan, "/XObject << /Im1 5 0 R >>", "", map[int]string{
			5: "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n\x00\xff\xff\x00\nendstream"}), "no text at all"},
		{"covered", onePageWith(covered, fonts, "", base), "'secret under the box' is drawn under an opaque box"},
		{"styled-linked", onePageWith(linked, fonts, "/Annots [7 0 R]", map[int]string{5: exportFont, 6: exportBold, 7: annot}), "bold title; a link over 'link'"},
	}
}

// ExportPictures is the pictures fixtures (P02.S03b, W23): each document draws images and expects a named outcome.
// Every image is a 4x4 so none is under figureMinSide as drawn (60 pt boxes), and the page draws a line of text so it
// is not the scanned-page case.
func ExportPictures() []ExportFixture {
	fonts := "/Font << /F1 5 0 R >>"
	rgb := func(extra string) string {
		return "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceRGB /BitsPerComponent 8 " + extra + " /Length 12 >>\nstream\n\xff\x00\x00\x00\xff\x00\x00\x00\xff\xff\xff\x00\nendstream"
	}
	gray := "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n\x00\xff\xff\x00\nendstream"
	label := text("F1", 12, 72, 740, "a line of text")
	draw := func(name string, x, y int) string { return fmt.Sprintf("q 60 0 0 60 %d %d cm /%s Do Q\n", x, y, name) }
	page := func(content, xobj string, extra map[int]string) []byte {
		m := map[int]string{5: exportFont}
		for k, v := range extra {
			m[k] = v
		}
		return onePageWith(content, fonts+" /XObject << "+xobj+" >>", "", m)
	}
	var jb bytes.Buffer
	im := stdimage.NewRGBA(stdimage.Rect(0, 0, 8, 8))
	for i := 0; i < 8; i++ {
		im.Set(i, i, color.RGBA{255, 0, 0, 255})
	}
	if err := jpeg.Encode(&jb, im, nil); err != nil {
		panic(err)
	}
	jpegImage := fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream", jb.Len(), jb.String())
	return []ExportFixture{
		{"picture-plain", page(label+draw("Im1", 72, 600), "/Im1 6 0 R", map[int]string{6: rgb("")}), "one carried picture"},
		{"picture-twice", page(label+draw("Im1", 72, 600)+draw("Im1", 300, 600), "/Im1 6 0 R", map[int]string{6: rgb("")}), "two figures, one object, bytes shared"},
		{"picture-in-form", page(label+"/Fm1 Do\n", "/Fm1 7 0 R", map[int]string{
			6: gray,
			7: "<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /XObject << /Im1 6 0 R >> >> /Length 40 >>\nstream\nq 60 0 0 60 200 500 cm /Im1 Do Q\nendstream"}), "a picture drawn by a form is found"},
		{"picture-turned", page(label+"q 0 60 -60 0 200 600 cm /Im1 Do Q\n", "/Im1 6 0 R", map[int]string{6: rgb("")}), "placeholder turned-or-mirrored"},
		{"picture-mirrored", page(label+"q -60 0 0 60 200 600 cm /Im1 Do Q\n", "/Im1 6 0 R", map[int]string{6: rgb("")}), "placeholder turned-or-mirrored"},
		{"picture-softmask", page(label+draw("Im1", 72, 600), "/Im1 6 0 R", map[int]string{6: rgb("/SMask 7 0 R"), 7: gray}), "placeholder soft-masked"},
		{"picture-stencil", page(label+draw("Im1", 72, 600), "/Im1 6 0 R", map[int]string{
			6: "<< /Type /XObject /Subtype /Image /Width 8 /Height 1 /ImageMask true /BitsPerComponent 1 /Length 1 >>\nstream\n\xaa\nendstream"}), "placeholder stencil-mask"},
		{"picture-inline", page(label+"q 60 0 0 60 72 600 cm BI /W 2 /H 2 /CS /G /BPC 8 ID \x00\xff\xff\x00 EI Q\n", "", nil), "placeholder inline-image"},
		{"picture-jpeg", page(label+draw("Im1", 72, 600), "/Im1 6 0 R", map[int]string{6: jpegImage}), "carried as a JPEG that decodes"},
		{"picture-cmyk", page(label+draw("Im1", 72, 600), "/Im1 6 0 R", map[int]string{
			6: "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceCMYK /BitsPerComponent 8 /Length 4 >>\nstream\n\x00\x00\x00\x00\nendstream"}), "placeholder colour-space"},
		{"picture-stencil-beside-plain", page(label+draw("Im1", 72, 600)+draw("Im2", 300, 600), "/Im1 6 0 R /Im2 7 0 R", map[int]string{
			6: rgb(""),
			7: "<< /Type /XObject /Subtype /Image /Width 8 /Height 1 /ImageMask true /BitsPerComponent 1 /Length 1 >>\nstream\n\xaa\nendstream"}), "pdfcpu cannot render the page's pictures with a stencil: the stencil is named, the other is not-extracted, the export succeeds"},
		{"picture-masked", page(label+draw("Im1", 72, 600), "/Im1 6 0 R", map[int]string{6: rgb("/Mask [0 0 0 0 0 0]")}), "placeholder masked"},
		{"picture-ccitt", page(label+draw("Im1", 72, 600), "/Im1 6 0 R", map[int]string{
			6: "<< /Type /XObject /Subtype /Image /Width 8 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 1 /Filter /CCITTFaxDecode /DecodeParms << /K -1 /Columns 8 /Rows 1 >> /Length 2 >>\nstream\n\x00\x00\nendstream"}), "placeholder filter"},
		{"picture-skewed", page(label+"q 60 0.3 0 60 200 600 cm /Im1 Do Q\n", "/Im1 6 0 R", map[int]string{6: rgb("")}), "placeholder turned-or-mirrored"},
		{"picture-narrow", page(label+"q 4 0 0 60 72 600 cm /Im1 Do Q\n", "/Im1 6 0 R", map[int]string{6: rgb("")}), "no figure; counted small (width only)"},
		{"picture-small", page(label+"q 4 0 0 4 72 600 cm /Im1 Do Q\n", "/Im1 6 0 R", map[int]string{6: rgb("")}), "no figure; counted small"},
		{"picture-artifact", page(label+"/Artifact BMC\n"+draw("Im1", 72, 600)+"EMC\n", "/Im1 6 0 R", map[int]string{6: rgb("")}), "no figure; counted artifact"},
	}
}

// ExportPictureTwoPages draws one image object on two pages: the extraction offers it once per page, a figure each.
func ExportPictureTwoPages() []byte {
	im := "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n\x00\xff\xff\x00\nendstream"
	page := func(content int) string {
		return fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Im1 6 0 R >> >> /Contents %d 0 R >>", content)
	}
	draw := "q 60 0 0 60 72 600 cm /Im1 Do Q\n"
	return assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		3: page(7), 4: page(8), 6: im, 7: stream(draw), 8: stream(draw),
	})
}

// ExportBidi is a page of three Hebrew letters with three text-direction controls between them (U+202E twice and
// U+2066 once), drawn through a ToUnicode that maps the controls like any other code (/pending 947): the shape of the
// "invoice_‮fdp.exe" spoof, in a document the reader can open.
func ExportBidi() ExportFixture {
	rtl := "BT /F1 12 Tf 400 700 Td <000100040002000500030004> Tj ET\n"
	return ExportFixture{"bidi-controls", onePageWith(rtl, "/Font << /F1 5 0 R >>", "", map[int]string{5: exportHebrew, 7: hebrewCID, 8: stream(bidiMap)}), "three text-direction controls in a Hebrew line"}
}
