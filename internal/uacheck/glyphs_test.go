package uacheck

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// glyphDoc is one tagged page whose one text run is `show`, drawn in font object 10 (`font`), with `extra`
// objects from 20 up — tagged so the documents exercise 7.21.7 and not the divergences an untagged page
// reaches (/pending 674).
func glyphDoc(font, show string, extra map[int]string) []byte {
	return glyphPage("BT /F0 12 Tf 10 10 Td "+show+" ET", "", "", font, extra)
}

// glyphPage is glyphDoc with the whole tagged content written by the caller, and further entries for the page's
// /Font dictionary (`fonts`) and its /Resources (`res`).
func glyphPage(content, fonts, res, font string, extra map[int]string) []byte {
	o := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 5 0 R /Lang (en) >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /StructParents 0 /Resources << /Font << /F0 10 0 R " + fonts + " >> " + res + " >> >>",
		4:  spStream("", "/P <</MCID 0>> BDC "+content+" EMC"),
		5:  "<< /Type /StructTreeRoot /K [6 0 R] /ParentTree 7 0 R >>",
		6:  "<< /Type /StructElem /S /P /P 5 0 R /Pg 3 0 R /K [0] >>",
		7:  "<< /Nums [0 [6 0 R]] >>",
		10: font,
	}
	for k, v := range extra {
		o[k] = v
	}
	return buildPDF(o)
}

// toUni is a /ToUnicode stream holding `body`.
func toUni(body string) string {
	return spStream("", "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /U def "+body+
		" endcmap CMapName currentdict /CMap defineresource pop end end")
}

const (
	someTrueType  = "<< /Type /Font /Subtype /TrueType /BaseFont /SomeFace /FirstChar 0 /LastChar 255 /Widths 30 0 R "
	identityFont  = "<< /Type /Font /Subtype /Type0 /BaseFont /SomeCID /Encoding /Identity-H /DescendantFonts [11 0 R] "
	adobeIdentity = "<< /Type /Font /Subtype /CIDFontType0 /BaseFont /SomeCID /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor 12 0 R >>"
	cidDescriptor = "<< /Type /FontDescriptor /FontName /SomeCID /Flags 4 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 880 /Descent -120 /CapHeight 880 /StemV 80 >>"
)

// glyphFixture is one shape of 7.21.7, with veraPDF 1.30.2's verdicts for t1 and t2 ("-" is no glyph at all).
// `refused` is a shape nib deliberately answers CannotCheck on, with the words its reason must hold.
type glyphFixture struct {
	name    string
	pdf     []byte
	t1, t2  string
	refused string
}

func glyphFixtures() []glyphFixture {
	widths := map[int]string{30: "[" + repeatStr("500 ", 256) + "]"}
	with := func(m map[int]string, k int, v string) map[int]string {
		out := map[int]string{}
		for a, b := range m {
			out[a] = b
		}
		out[k] = v
		return out
	}
	cid := map[int]string{11: adobeIdentity, 12: cidDescriptor}
	return []glyphFixture{
		{"a TrueType face with no encoding and no program", glyphDoc(someTrueType+">>", "(a) Tj", widths), "fail", "pass", ""},
		{"a TrueType face under WinAnsi", glyphDoc(someTrueType+"/Encoding /WinAnsiEncoding >>", "(a) Tj", widths), "pass", "pass", ""},
		{"WinAnsi's code 0x81 is bullet, not .notdef", glyphDoc(someTrueType+"/Encoding /WinAnsiEncoding >>", "<81> Tj", widths), "pass", "pass", ""},
		{"a non-standard face NAMING StandardEncoding", glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /SomeFace /Encoding /StandardEncoding /FirstChar 0 /LastChar 255 /Widths 30 0 R >>", "(a) Tj", widths), "fail", "pass", ""},
		{"Helvetica with no encoding", glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", "(abc) Tj", nil), "pass", "pass", ""},
		{"Helvetica's undefined code 0x01", glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", "<01> Tj", nil), "pass", "fail", ""},
		{"Helvetica naming StandardEncoding", glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /StandardEncoding >>", "(a) Tj", nil), "pass", "pass", ""},
		{"a Differences name outside the glyph list", glyphDoc(someTrueType+"/Encoding << /Differences [97 /uni0041] >> >>", "(a) Tj", widths), "fail", "pass", ""},
		{"a Differences name in the glyph list", glyphDoc(someTrueType+"/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [97 /alpha] >> >>", "(a) Tj", widths), "pass", "pass", ""},
		{"a Differences name only ZapfDingbats knows", glyphDoc(someTrueType+"/Encoding << /Differences [97 /a20] >> >>", "(a) Tj", widths), "pass", "pass", ""},
		{"a Differences .notdef", glyphDoc(someTrueType+"/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [97 /.notdef] >> >>", "(a) Tj", widths), "pass", "fail", ""},
		{"Symbol with no encoding", glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /Symbol >>", "(a) Tj", nil), "fail", "pass", ""},
		{"Symbol under WinAnsi", glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /Symbol /Encoding /WinAnsiEncoding >>", "(a) Tj", nil), "pass", "pass", ""},
		{"ZapfDingbats with a Differences name", glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /ZapfDingbats /Encoding << /Differences [52 /a20] >> >>", "(4) Tj", nil), "pass", "pass", ""},
		{"a ToUnicode destination of U+0000", glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode 20 0 R >>", "(a) Tj", map[int]string{20: toUni("1 beginbfchar <61> <0000> endbfchar")}), "pass", "fail", ""},
		{"a ToUnicode destination holding U+FEFF after a letter", glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode 20 0 R >>", "(a) Tj", map[int]string{20: toUni("1 beginbfchar <61> <0041FEFF> endbfchar")}), "pass", "fail", ""},
		{"\" draws no glyph", glyphDoc(someTrueType+">>", "0 0 (a) \"", widths), "-", "-", ""},
		{"' draws its glyph", glyphDoc(someTrueType+">>", "(a) '", widths), "fail", "pass", ""},
		{"a TJ array's second string is unmapped", glyphDoc(someTrueType+"/ToUnicode 20 0 R >>", "[(a) 120 (b)] TJ", with(widths, 20, toUni("1 beginbfchar <61> <0061> endbfchar"))), "fail", "pass", ""},
		{"a two-byte ToUnicode key maps a one-byte code", glyphDoc(someTrueType+"/ToUnicode 20 0 R >>", "(A) Tj", with(widths, 20, toUni("1 beginbfchar <0041> <0041> endbfchar"))), "pass", "pass", ""},
		{"only the count of a bfchar list is read", glyphDoc(someTrueType+"/ToUnicode 20 0 R >>", "(b) Tj", with(widths, 20, toUni("1 beginbfchar <61> <0061> <62> <0062> endbfchar"))), "fail", "pass", ""},
		{"Identity-H over Adobe-Identity, no ToUnicode", glyphDoc(identityFont+">>", "<0001> Tj", cid), "fail", "pass", ""},
		{"Identity-H, the second code unmapped", glyphDoc(identityFont+"/ToUnicode 20 0 R >>", "<00010002> Tj", with(cid, 20, toUni("1 beginbfchar <0001> <0041> endbfchar"))), "fail", "pass", ""},
		{"Identity-H, a string ending inside a code, 00FF mapped", glyphDoc(identityFont+"/ToUnicode 20 0 R >>", "<000100> Tj", with(cid, 20, toUni("2 beginbfchar <0001> <0041> <00FF> <0042> endbfchar"))), "pass", "pass", ""},
		{"Identity-H, a string ending inside a code, 00FF unmapped", glyphDoc(identityFont+"/ToUnicode 20 0 R >>", "<000100> Tj", with(cid, 20, toUni("1 beginbfchar <0001> <0041> endbfchar"))), "fail", "pass", ""},
		{"a ToUnicode NAMING Identity-H, code 0", glyphDoc(identityFont+"/ToUnicode /Identity-H >>", "<0000> Tj", cid), "pass", "fail", ""},
		{"a ToUnicode NAMING Identity-H, code 0041", glyphDoc(identityFont+"/ToUnicode /Identity-H >>", "<0041> Tj", cid), "pass", "pass", ""},
		{"a full-plane bfrange is 256 codes", glyphDoc(identityFont+"/ToUnicode 20 0 R >>", "<0100> Tj", with(cid, 20, toUni("1 beginbfrange <0000> <FFFF> <0041> endbfrange"))), "fail", "pass", ""},
		{"an embedded one-byte CMap, both codes mapped", glyphDoc("<< /Type /Font /Subtype /Type0 /BaseFont /SomeCID /Encoding 21 0 R /DescendantFonts [11 0 R] /ToUnicode 20 0 R >>", "(AB) Tj",
			map[int]string{11: adobeIdentity, 12: cidDescriptor, 20: toUni("2 beginbfchar <41> <0041> <42> <0042> endbfchar"), 21: oneByteCMap}), "pass", "pass", ""},
		{"an embedded one-byte CMap, one code unmapped", glyphDoc("<< /Type /Font /Subtype /Type0 /BaseFont /SomeCID /Encoding 21 0 R /DescendantFonts [11 0 R] /ToUnicode 20 0 R >>", "(AB) Tj",
			map[int]string{11: adobeIdentity, 12: cidDescriptor, 20: toUni("1 beginbfchar <41> <0041> endbfchar"), 21: oneByteCMap}), "fail", "pass", ""},
		{"invisible text is not exempt", glyphDoc(someTrueType+">>", "3 Tr (a) Tj", widths), "fail", "pass", ""},
		{"a malformed ToUnicode", glyphDoc(someTrueType+"/Encoding /WinAnsiEncoding /ToUnicode 20 0 R >>", "(a) Tj", with(widths, 20, toUni("2 beginbfchar <61> <0041> endbfchar"))), "pass", "pass", "wrong kind"},
		// The P07.S02 review's shapes, each measured on veraPDF 1.30.2 before being pinned.
		{"WinAnsi's code 0x01 is .notdef", glyphDoc(someTrueType+"/Encoding /WinAnsiEncoding >>", "<01> Tj", widths), "pass", "fail", ""},
		{"a ToUnicode destination of U+FFFE", glyphDoc(helvToUni, "(a) Tj", map[int]string{20: toUni("1 beginbfchar <61> <FFFE> endbfchar")}), "pass", "fail", ""},
		{"a one-byte ToUnicode destination is ISO-8859-1", glyphDoc(helvToUni, "(a) Tj", map[int]string{20: toUni("1 beginbfchar <61> <00> endbfchar")}), "pass", "fail", ""},
		{"a range whose destination starts with 00 reads its last byte", glyphDoc(helvToUni, "(a) Tj", map[int]string{20: toUni("1 beginbfrange <61> <61> <00000041> endbfrange")}), "pass", "pass", ""},
		{"a form rebinding the font name is judged by the Tf-bound font (Zapf at Tf)", glyphPage("BT /F0 12 Tf ET /Fm0 Do", "", "/XObject << /Fm0 21 0 R >>",
			"<< /Type /Font /Subtype /Type1 /BaseFont /ZapfDingbats >>", map[int]string{21: rebindingForm, 22: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}), "fail", "pass", ""},
		{"a form rebinding the font name is judged by the Tf-bound font (Helvetica at Tf)", glyphPage("BT /F0 12 Tf ET /Fm0 Do", "", "/XObject << /Fm0 21 0 R >>",
			"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", map[int]string{21: rebindingForm, 22: "<< /Type /Font /Subtype /Type1 /BaseFont /ZapfDingbats >>"}), "pass", "pass", ""},
		{"a ToUnicode stream NAMED Identity-H answers the code", glyphDoc(helvToUni, "<00> Tj", map[int]string{20: spStream("/CMapName /Identity-H", "1 beginbfchar <00> <0041> endbfchar")}), "pass", "fail", ""},
		{"a ToUnicode stream NAMED Identity-H ignores its entries", glyphDoc(helvToUni, "(A) Tj", map[int]string{20: spStream("/CMapName /Identity-H", "1 beginbfchar <41> <0000> endbfchar")}), "pass", "pass", ""},
		{"a ToUnicode's /UseCMap overwrites its entries", glyphDoc(helvToUni, "(A) Tj", map[int]string{20: spStream("/UseCMap 23 0 R", "1 beginbfchar <41> <0041> endbfchar"),
			23: toUni("1 beginbfchar <41> <0000> endbfchar")}), "pass", "fail", ""},
		{"an embedded CMap NAMED Identity-H falls back through the descendant (Adobe-Identity)", glyphDoc(embeddedNamedIdentity, "<0022> Tj",
			map[int]string{11: adobeIdentity, 12: cidDescriptor, 21: namedIdentityCMap("Japan1")}), "fail", "pass", ""},
		{"an embedded CMap NAMED Identity-H falls back through the descendant (Adobe-Japan1)", glyphDoc(embeddedNamedIdentity, "<0022> Tj",
			map[int]string{11: strings.Replace(adobeIdentity, "(Identity)", "(Japan1)", 1), 12: cidDescriptor, 21: namedIdentityCMap("Identity")}), "pass", "pass", "Adobe-Japan1-UCS2"},
		{"usecmap Identity-H before a one-byte range drops the range", glyphDoc(type0Embedded, "(AB) Tj",
			map[int]string{11: adobeIdentity, 12: cidDescriptor, 20: toUni("1 beginbfchar <4142> <0041> endbfchar"), 21: identityThenOneByte}), "pass", "pass", ""},
		{"a four-byte range at or above 0x80000000 maps nothing", glyphDoc(type0Embedded, "<81308131> Tj",
			map[int]string{11: adobeIdentity, 12: cidDescriptor, 20: toUni("1 beginbfrange <81308130> <813081FF> <4E00> endbfrange"), 21: fourByteCMap}), "fail", "pass", ""},
		{"a four-byte bfchar at or above 0x80000000 maps", glyphDoc(type0Embedded, "<81308131> Tj",
			map[int]string{11: adobeIdentity, 12: cidDescriptor, 20: toUni("1 beginbfchar <81308131> <4E01> endbfchar"), 21: fourByteCMap}), "pass", "pass", ""},
		{"a Type 1 font's program under /FontFile2 is no program", glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /SomeFace /FirstChar 0 /LastChar 255 /Widths 30 0 R /FontDescriptor 24 0 R >>", "(a) Tj",
			with(with(widths, 24, someDescriptor+"/FontFile2 25 0 R >>"), 25, spStream("", "not a font"))), "fail", "pass", ""},
		{"text drawn only invisibly in a font that does not resolve", glyphPage("BT /F9 12 Tf 3 Tr 10 10 Td (a) Tj ET", "", "", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", nil), "-", "-", "does not resolve"},
		// The P07.S02 re-review's shapes: the font a nested stream inherits, an ExtGState's font, and a CIDFont's program key.
		// The page also shows `(x)` in the same font: a font used ONLY inside a pattern is outside nib's font
		// population (/pending 678), and this row is about the pattern's inherited glyph, not that gap.
		{"a tiling pattern shows text in the font its invoking form began with", glyphPage("BT /F0 12 Tf 10 10 Td (x) Tj ET /Fm0 Do", "", "/XObject << /Fm0 21 0 R >>",
			"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", map[int]string{21: patternForm, 23: tilingPattern}), "pass", "fail", ""},
		{"a Type 3 glyph procedure shows text in its own font", glyphDoc(type3Font, "(a) Tj", map[int]string{26: spStream("", "10 0 d0 BT <01> Tj ET")}), "fail", "pass", ""},
		{"an ExtGState /Font replaces the Tf font", glyphPage("BT /F0 12 Tf /GS0 gs 10 10 Td <01> Tj ET", "", "/ExtGState << /GS0 << /Font [22 0 R 12] >> >>", helvToUni,
			map[int]string{20: toUni("1 beginbfchar <01> <0041> endbfchar"), 22: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>"}), "pass", "fail", ""},
		{"a CIDFontType2 whose program is under /FontFile", glyphDoc(identityFont+"/ToUnicode 20 0 R >>", "<0001> Tj",
			map[int]string{11: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SomeCID /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /CIDToGIDMap /Identity /FontDescriptor 12 0 R >>",
				12: strings.Replace(cidDescriptor, ">>", "/FontFile 25 0 R >>", 1), 25: spStream("", "not a font"), 20: toUni("1 beginbfchar <0001> <0041> endbfchar")}), "pass", "pass", ""},
		// The third review pass: a pattern entered under two fonts is read once, under the first; an ExtGState /Font
		// that is not a font of a type veraPDF builds leaves the font in force.
		{"a pattern entered under two fonts is judged in the first (the mapping one)", glyphPage("BT /F0 12 Tf 10 10 Td (x) Tj ET /Fm0 Do BT /F1 12 Tf 10 30 Td (x) Tj ET /Fm0 Do", "/F1 22 0 R", "/XObject << /Fm0 21 0 R >>",
			helvToUni, map[int]string{20: toUni("1 beginbfchar <01> <0041> endbfchar"), 21: patternForm, 22: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>", 23: tilingPattern}), "pass", "pass", ""},
		{"a pattern entered under two fonts is judged in the first (the unmapping one)", glyphPage("BT /F1 12 Tf 10 10 Td (x) Tj ET /Fm0 Do BT /F0 12 Tf 10 30 Td (x) Tj ET /Fm0 Do", "/F1 22 0 R", "/XObject << /Fm0 21 0 R >>",
			helvToUni, map[int]string{20: toUni("1 beginbfchar <01> <0041> endbfchar"), 21: patternForm, 22: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>", 23: tilingPattern}), "pass", "fail", ""},
		{"an ExtGState /Font of an unknown subtype leaves the Tf font", glyphPage("BT /F0 12 Tf /GS0 gs 10 10 Td <01> Tj ET", "", "/ExtGState << /GS0 << /Font [22 0 R 12] >> >>", helvToUni,
			map[int]string{20: toUni("1 beginbfchar <01> <0041> endbfchar"), 22: "<< /Type /Font /Subtype /Bogus /BaseFont /Helvetica-Bold >>"}), "pass", "pass", ""},
		{"an ExtGState /Font that is not a dictionary leaves the Tf font", glyphPage("BT /F0 12 Tf /GS0 gs 10 10 Td <01> Tj ET", "", "/ExtGState << /GS0 << /Font [5 12] >> >>", helvToUni,
			map[int]string{20: toUni("1 beginbfchar <01> <0041> endbfchar")}), "pass", "pass", ""},
		// The control for the pattern rows: selected on the PAGE after a `Tf`, the pattern starts from the page's
		// starting state, which has no font — its text is no glyph veraPDF judges, whatever the page's `Tf` said.
		{"a pattern selected on the page starts from the page's state, not the Tf before it", glyphPage("BT /F0 12 Tf 10 10 Td (x) Tj ET /Pattern cs /P0 scn 0 0 50 50 re f", "",
			"/Pattern << /P0 23 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", map[int]string{23: tilingPattern}), "pass", "pass", ""},
		// The red-proof's survivors, closed: a nested array inside a TJ array is not a string of it, and a ToUnicode
		// program that `usecmap`s a UCS2 CMap veraPDF merges and nib does not carry is refused.
		{"a string nested in a TJ array draws nothing", glyphDoc(someTrueType+"/ToUnicode 20 0 R >>", "[(a) [(b)]] TJ", with(widths, 20, toUni("1 beginbfchar <61> <0061> endbfchar"))), "pass", "pass", ""},
		{"a ToUnicode program using Adobe-Japan1-UCS2", glyphDoc(helvToUni, "(A) Tj", map[int]string{20: toUni("/Adobe-Japan1-UCS2 usecmap 1 beginbfchar <41> <0041> endbfchar")}), "pass", "pass", "Adobe-Japan1-UCS2"},
	}
}

const (
	helvToUni      = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode 20 0 R >>"
	type0Embedded  = "<< /Type /Font /Subtype /Type0 /BaseFont /SomeCID /Encoding 21 0 R /DescendantFonts [11 0 R] /ToUnicode 20 0 R >>"
	someDescriptor = "<< /Type /FontDescriptor /FontName /SomeFace /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 "
	// embeddedNamedIdentity is a Type 0 font whose Encoding is an EMBEDDED CMap whose /CMapName is Identity-H.
	embeddedNamedIdentity = "<< /Type /Font /Subtype /Type0 /BaseFont /SomeCID /Encoding 21 0 R /DescendantFonts [11 0 R] >>"
)

// patternForm has its own /Resources holding only a tiling pattern, and paints with it — the pattern's text shows no
// `Tf`, so it is judged in the font the FORM was entered with.
var patternForm = spStream("/Type /XObject /Subtype /Form /BBox [0 0 200 200] /Resources << /Pattern << /P0 23 0 R >> >>", "/Pattern cs /P0 scn 0 0 50 50 re f")

var tilingPattern = spStream("/Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources << >>", "BT 1 1 Td <01> Tj ET")

// type3Font draws `/a` with a procedure that itself shows code 01 with no `Tf`.
const type3Font = "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.001 0 0 0.001 0 0] /CharProcs << /a 26 0 R >> " +
	"/Encoding << /Type /Encoding /Differences [97 /a] >> /FirstChar 97 /LastChar 97 /Widths [10] /Resources << >> >>"

// rebindingForm draws `(A)` under its OWN /F0 (object 22) without a `Tf` — so the font it is judged in is the one
// the page bound, not the one its resources name.
var rebindingForm = spStream("/Type /XObject /Subtype /Form /BBox [0 0 200 200] /Resources << /Font << /F0 22 0 R >> >>", "BT 10 10 Td (A) Tj ET")

func namedIdentityCMap(ordering string) string {
	info := "/CIDSystemInfo << /Registry (Adobe) /Ordering (" + ordering + ") /Supplement 0 >>"
	return spStream("/Type /CMap /CMapName /Identity-H "+info, "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /Identity-H def "+
		"1 begincodespacerange <0000> <FFFF> endcodespacerange 1 begincidrange <0000> <FFFF> 0 endcidrange endcmap CMapName currentdict /CMap defineresource pop end end")
}

// identityThenOneByte merges Identity-H BEFORE declaring a one-byte range, which the merged range then overlaps.
var identityThenOneByte = spStream("/Type /CMap /CMapName /Mixed /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>",
	"/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /Mixed def /Identity-H usecmap 1 begincodespacerange <00> <7F> endcodespacerange "+
		"1 begincidrange <00> <7F> 0 endcidrange endcmap CMapName currentdict /CMap defineresource pop end end")

// fourByteCMap has GB18030's four-byte range.
var fourByteCMap = spStream("/Type /CMap /CMapName /Four /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>",
	"/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /Four def 1 begincodespacerange <81308130> <FE39FE39> endcodespacerange "+
		"1 begincidrange <81308130> <813081FF> 1 endcidrange endcmap CMapName currentdict /CMap defineresource pop end end")

// oneByteCMap is an embedded CMap whose one codespace range is a single byte, over Adobe-Identity.
var oneByteCMap = spStream("/Type /CMap /CMapName /OneByte /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>",
	"/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /OneByte def 1 begincodespacerange <00> <FF> endcodespacerange "+
		"1 begincidrange <00> <FF> 0 endcidrange endcmap CMapName currentdict /CMap defineresource pop end end")

func repeatStr(s string, n int) string { return strings.Repeat(s, n) }

// TestTheGlyphClausesAgreeWithVeraPDF — every shape of 7.21.7 above, each run on veraPDF 1.30.2 before it was
// pinned (`--passed`, so a clause with no glyph reads "-"). The oracle asks veraPDF again on every run
// (`oracleCorpus`); this table is what the package asserts when veraPDF is not installed.
//
// One row differs, deliberately: a /ToUnicode veraPDF-parser throws on is discarded WHOLE and the font falls back
// to its encoding (veraPDF passes it), and nib — whose tokenizer is not veraPDF's PostScript interpreter —
// refuses rather than claim to know which CMaps veraPDF throws on.
func TestTheGlyphClausesAgreeWithVeraPDF(t *testing.T) {
	v := map[string]Verdict{"pass": Pass, "fail": Fail, "-": NotApplicable}
	for _, f := range glyphFixtures() {
		for _, c := range []struct{ clause, want string }{{"7.21.7 t1", f.t1}, {"7.21.7 t2", f.t2}} {
			want := v[c.want]
			got := verdictOf(t, f.pdf, c.clause)
			if f.refused != "" {
				if got.Verdict != CannotCheck || !strings.Contains(got.Why, f.refused) {
					t.Errorf("%s: %s reports %v (%s), want CannotCheck naming %q (veraPDF %s)", f.name, c.clause, got.Verdict, got.Why, f.refused, c.want)
				}
				continue
			}
			if got.Verdict != want {
				t.Errorf("%s: %s reports %v (%s), veraPDF %s", f.name, c.clause, got.Verdict, got.Why, c.want)
			}
		}
	}
}

// toUnicodeRewritten rewrites the first `bfchar` entry of every /ToUnicode CMap in nib's own conversion — its
// DESTINATION to `<0000>` (`dest`), or its SOURCE to `<FFFF>`, a code nothing draws. `/pending 657` filed the
// first as a 7.21.7 t1 failure; measured on veraPDF it is t2's (a `<0000>` destination is the character U+0000,
// never an absence), and the second — the drawn code left with no entry — is t1's.
func toUnicodeRewritten(t *testing.T, pdf []byte, dest bool) []byte {
	t.Helper()
	// Validated, as `ReadContextFile` does: an unvalidated context holds only what was dereferenced, and
	// writing it back emits a two-kilobyte shell.
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err == nil {
		err = api.ValidateContext(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(beginbfchar\s*)(<[0-9A-Fa-f]+>)(\s*)(<[0-9A-Fa-f]+>)`)
	n := 0
	for _, e := range ctx.XRefTable.Table {
		if e == nil {
			continue
		}
		sd, ok := e.Object.(types.StreamDict)
		if !ok || sd.Decode() != nil || !bytes.Contains(sd.Content, []byte("beginbfchar")) {
			continue
		}
		repl := "${1}<FFFF>${3}${4}"
		if dest {
			repl = "${1}${2}${3}<0000>"
		}
		sd.Content = re.ReplaceAll(sd.Content, []byte(repl))
		if err := sd.Encode(); err != nil {
			t.Fatal(err)
		}
		e.Object = sd
		n++
	}
	if n == 0 {
		t.Fatal("the conversion carries no bfchar CMap — the fixture does not carry the defect")
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestShownCodesPastTheBudgetAreCannotCheck — a string operand is attacker-sized and one `Tj` is one operator,
// so the operator ceiling does not bound the codes read: a form drawn N times re-reads its string N times.
// Past `maxGlyphCodes` the walk stops and both 7.21.7 clauses refuse, never answering over codes nib did not
// read. The control draws the same form fewer times and is judged — the stimulus before the response.
func TestShownCodesPastTheBudgetAreCannotCheck(t *testing.T) {
	const chunk = 1 << 18 // 262,144 codes per draw
	doc := func(draws int) []byte {
		show := "(" + repeatStr("a", chunk) + ") Tj"
		page := repeatStr("/Fm0 Do ", draws)
		return buildPDF(map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R >>",
			2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << /XObject << /Fm0 5 0 R >> >> >>",
			4:  spStream("", page),
			5:  spStream("/Type /XObject /Subtype /Form /BBox [0 0 200 200] /Resources << /Font << /F0 10 0 R >> >>", "BT /F0 12 Tf "+show+" ET"),
			10: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		})
	}
	under := doc(maxGlyphCodes/chunk - 1)
	if got := verdictOf(t, under, "7.21.7 t1"); got.Verdict != Pass {
		t.Fatalf("control: %d codes, under the budget, report %v (%s), want Pass", (maxGlyphCodes/chunk-1)*chunk, got.Verdict, got.Why)
	}
	over := doc(maxGlyphCodes/chunk + 1)
	for _, clause := range []string{"7.21.7 t1", "7.21.7 t2"} {
		got := verdictOf(t, over, clause)
		if got.Verdict != CannotCheck || !strings.Contains(got.Why, "character codes") {
			t.Errorf("%s past the code budget reports %v (%s), want CannotCheck naming the budget", clause, got.Verdict, got.Why)
		}
	}
}

// TestDistinctGlyphsPastTheBudgetAreCannotCheck — the code ceiling bounds the READING and not what is held: every
// code of N Identity-H fonts is N×65,536 distinct glyphs, and 256 fonts took 16 s and 10.7 GB before
// `maxDistinctGlyphs` (the P07.S02 review). Sixteen fonts fill the ceiling exactly and are judged; seventeen refuse.
func TestDistinctGlyphsPastTheBudgetAreCannotCheck(t *testing.T) {
	var all strings.Builder
	all.WriteString("<")
	for c := 0; c < 65536; c++ {
		fmt.Fprintf(&all, "%04X", c)
	}
	all.WriteString("> Tj")
	doc := func(fonts int) []byte {
		var names, page strings.Builder
		objs := map[int]string{
			11: adobeIdentity, 12: cidDescriptor,
			13: toUni("1 beginbfrange <0000> <00FF> <0041> endbfrange"),
			15: spStream("/Type /XObject /Subtype /Form /BBox [0 0 200 200]", "BT 10 10 Td "+all.String()+" ET"),
		}
		for i := 0; i < fonts; i++ {
			fmt.Fprintf(&names, "/G%d %d 0 R ", i, 100+i)
			// Distinct faces: pdfcpu's optimizer merges identical font objects, and seventeen identical fonts are one.
			objs[100+i] = strings.Replace(identityFont, "/SomeCID", fmt.Sprintf("/SomeCID%d", i), 1) + "/ToUnicode 13 0 R >>"
			fmt.Fprintf(&page, "BT /G%d 12 Tf ET /X0 Do ", i)
		}
		return glyphPage(page.String(), names.String(), "/XObject << /X0 15 0 R >>", identityFont+">>", objs)
	}
	if got := verdictOf(t, doc(maxDistinctGlyphs/65536), "7.21.7 t1"); got.Verdict != Fail {
		t.Fatalf("control: %d fonts fill the ceiling exactly and report %v (%s), want Fail (codes past 00FF are unmapped)",
			maxDistinctGlyphs/65536, got.Verdict, got.Why)
	}
	for _, clause := range []string{"7.21.7 t1", "7.21.7 t2"} {
		if got := verdictOf(t, doc(maxDistinctGlyphs/65536+1), clause); got.Verdict != CannotCheck || !strings.Contains(got.Why, "distinct glyphs") {
			t.Errorf("%s past the distinct-glyph ceiling reports %v (%s), want CannotCheck naming it", clause, got.Verdict, got.Why)
		}
	}
}

// TestTheFailingGlyphsAreTheOnesVeraPDFCounts — P07.S02's first acceptance clause, which a verdict cannot show:
// `7.21.7-t01-fail-a` fails 7.21.7 t1 on EIGHT distinct glyphs in veraPDF 1.30.2 (codes 3, 40, 55, 68, 69, 76, 79
// and 82 of JAPTCA+AboriginalSerif, read from its report), and nib must fail exactly those — not one, not all it drew.
func TestTheFailingGlyphsAreTheOnesVeraPDFCounts(t *testing.T) {
	path := filepath.Join(corpusDir(), "7.21 Fonts", "7.21.7 Unicode character maps", "7.21.7-t01-fail-a.pdf")
	pdf, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("the veraPDF corpus is not on this machine (%v) — TestTheCheckerAgreesWithVeraPDFsOwnCorpus reports the same absence", err)
	}
	d, err := open(pdf)
	if err != nil {
		t.Fatal(err)
	}
	glyphs, why := d.glyphsDrawn()
	if why != "" || len(glyphs) == 0 {
		t.Fatalf("the population is empty or refused (%q) — the stimulus is missing", why)
	}
	var failing []int
	for _, g := range glyphs {
		if _, st, _ := d.toUnicode(g); st == uniNull {
			failing = append(failing, g.code)
		}
	}
	sort.Ints(failing)
	if want := []int{3, 40, 55, 68, 69, 76, 79, 82}; !slices.Equal(failing, want) {
		t.Errorf("nib fails glyphs %v of the %d it read, veraPDF fails %v", failing, len(glyphs), want)
	}
}

// TestAWalkThatDidNotFinishIsNotAWalkThatFoundNothing — the walk records that it STARTED before it runs and that it
// FINISHED only on return, so a walk a panic cut short (recovered per rule by `runOne`) reads as unread for every
// later rule instead of as a population that happens to be small. No input panics it today, so the state is set by
// hand: started, not finished, no error recorded.
func TestAWalkThatDidNotFinishIsNotAWalkThatFoundNothing(t *testing.T) {
	d, err := open(glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", "(a) Tj", nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, why := d.contentEvents(); why != "" || !d.contentFinished {
		t.Fatalf("control: a walk that returned reports %q (finished %v)", why, d.contentFinished)
	}
	d.contentFinished = false
	if _, why := d.contentEvents(); !strings.Contains(why, "stopped part-way") {
		t.Errorf("a walk that never returned reads as complete (%q)", why)
	}
}
