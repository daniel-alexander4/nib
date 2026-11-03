package docx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"nib/internal/docmodel"
	"nib/internal/docwrite/wtest"
)

// read is what a reader of the package finds, from the XML alone (R1's second oracle: it parses the written package,
// `w:t` runs per paragraph in document order, and never asks the writer what it meant).
type read struct {
	words      []string
	headings   map[string]int
	listParas  int
	numIDs     map[string]int
	tables     int
	cells      int
	vmerge     int
	drawings   int
	hyperlinks []string // r:id values
	bold       int
	italic     int
	rtl        int
}

func parse(t *testing.T, data []byte) read {
	t.Helper()
	r := read{headings: map[string]int{}, numIDs: map[string]int{}}
	d := xml.NewDecoder(bytes.NewReader(data))
	var para strings.Builder
	inP, inT, inList := 0, false, false
	var paraNum string
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			attr := func(n string) string {
				for _, a := range e.Attr {
					if a.Name.Local == n {
						return a.Value
					}
				}
				return ""
			}
			switch e.Name.Local {
			case "p":
				inP++
				para.Reset()
				inList = false
			case "pStyle":
				if strings.HasPrefix(attr("val"), "Heading") {
					r.headings[attr("val")]++
				}
			case "numId":
				if inP > 0 {
					inList, paraNum = true, attr("val")
				}
			case "tbl":
				r.tables++
			case "tc":
				r.cells++
			case "vMerge":
				r.vmerge++
			case "drawing":
				r.drawings++
			case "hyperlink":
				r.hyperlinks = append(r.hyperlinks, attr("id"))
			case "b":
				r.bold++
			case "i":
				r.italic++
			case "rtl", "bidi":
				r.rtl++
			case "t":
				inT = true
			case "tab", "br":
				para.WriteString(" ")
			}
		case xml.CharData:
			if inT {
				para.Write(e)
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "t":
				inT = false
			case "p":
				inP--
				if inList {
					r.listParas++
					r.numIDs[paraNum]++
				}
				txt := para.String()
				if !strings.HasPrefix(txt, "[Picture:") {
					r.words = append(r.words, strings.Fields(txt)...)
				}
			}
		}
	}
	return r
}

func TestDocxReadsBackAsTheModel(t *testing.T) {
	d := wtest.Hostile()
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	pkg := wtest.Open(t, out)
	pkg.WellFormed(t)
	r := parse(t, pkg.Files["word/document.xml"])

	if want := wtest.Words(d); !slices.Equal(r.words, want) {
		t.Errorf("the words differ from the model's\n got %q\nwant %q", r.words, want)
	}
	if !reflect.DeepEqual(r.headings, map[string]int{"Heading1": 1}) {
		t.Errorf("headings = %v, want one Heading1", r.headings)
	}
	if r.tables != 1 || r.cells != 4 {
		t.Errorf("tables = %d, cells = %d, want 1 and 4", r.tables, r.cells)
	}
	if r.listParas != 3 || r.numIDs["1"] != 1 || r.numIDs["2"] != 2 {
		t.Errorf("list paragraphs = %d by numId %v, want 3: one bullet (1), two decimal (2)", r.listParas, r.numIDs)
	}
	if r.drawings != 1 {
		t.Errorf("drawings = %d, want the one carried picture", r.drawings)
	}
	if len(r.hyperlinks) != 1 {
		t.Errorf("hyperlinks = %v, want only the https link", r.hyperlinks)
	}
	rels := string(pkg.Files["word/_rels/document.xml.rels"])
	if !strings.Contains(rels, `Target="https://example.com/a(b)c?x=1&amp;y=2" TargetMode="External"`) || strings.Contains(rels, "javascript") {
		t.Errorf("relationships:\n%s", rels)
	}
	doc := string(pkg.Files["word/document.xml"])
	for _, bad := range []string{"<script", "onerror=\"x\"", "\x00"} {
		if strings.Contains(doc, bad) {
			t.Errorf("document.xml holds %q", bad)
		}
	}
	if r.rtl < 2 {
		t.Errorf("the right-to-left paragraph is not marked (bidi/rtl seen %d times)", r.rtl)
	}
}

func TestDocxHoldsOnlyItsNamedPartsAndNoClaims(t *testing.T) {
	d := wtest.Hostile()
	out, _ := Write(d)
	pkg := wtest.Open(t, out)
	allowed := map[string]bool{"word/media/img-1.png": true}
	for _, p := range Parts {
		allowed[p] = true
	}
	for _, n := range pkg.Order {
		if !allowed[n] {
			t.Errorf("unexpected part %q", n)
		}
	}
	for _, n := range Parts {
		if _, ok := pkg.Files[n]; !ok {
			t.Errorf("missing part %q", n)
		}
	}
	core := string(pkg.Files["docProps/core.xml"])
	for _, bad := range []string{"creator", "lastModifiedBy", "dcterms", "pdfuaid", "conformsTo", "script>"} {
		if strings.Contains(core, bad) {
			t.Errorf("core.xml holds %q:\n%s", bad, core)
		}
	}
	if !strings.Contains(core, "<dc:language>und</dc:language>") {
		t.Errorf("a hostile language was not turned to und:\n%s", core)
	}
	if string(pkg.Files["docProps/app.xml"]) == "" || !strings.Contains(string(pkg.Files["docProps/app.xml"]), "<Application>Nib</Application>") {
		t.Error("app.xml names something other than Nib")
	}
}

func TestDocxTableSpansAreRealMerges(t *testing.T) {
	cell := func(s string, cs, rs int) docmodel.Cell {
		return docmodel.Cell{Paragraphs: [][]docmodel.Span{{{Text: s}}}, ColSpan: cs, RowSpan: rs}
	}
	d := docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Table, Page: 1, Rows: [][]docmodel.Cell{
		{cell("a", 2, 1), cell("b", 1, 2)},
		{cell("c", 1, 1), cell("d", 1, 1)},
	}}}}
	out, _ := Write(d)
	pkg := wtest.Open(t, out)
	doc := string(pkg.Files["word/document.xml"])
	// Row 2 holds c, d and a covered position under b: three cells, one vMerge continue and one restart in all.
	if strings.Count(doc, "<w:tc>") != 5 || strings.Count(doc, `<w:vMerge w:val="restart"/>`) != 1 || strings.Count(doc, "<w:vMerge/>") != 1 || !strings.Contains(doc, `<w:gridSpan w:val="2"/>`) {
		t.Errorf("spans:\n%s", doc)
	}
	if r := parse(t, pkg.Files["word/document.xml"]); !slices.Equal(r.words, []string{"a", "b", "c", "d"}) {
		t.Errorf("words = %q", r.words)
	}
}

func TestDocxOfNothingIsNil(t *testing.T) {
	if out, err := Write(docmodel.Document{}); out != nil || err != nil {
		t.Errorf("got %d bytes, %v", len(out), err)
	}
}

func TestDocxWhitespaceIsKept(t *testing.T) {
	d := docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Paragraph, Spans: []docmodel.Span{{Text: " lead "}, {Text: "bold", Bold: true}, {Text: "a\tb\nc  d"}}}}}
	out, _ := Write(d)
	r := parse(t, wtest.Open(t, out).Files["word/document.xml"])
	if want := []string{"lead", "bolda", "b", "c", "d"}; !slices.Equal(r.words, want) {
		t.Errorf("words = %q, want %q (bold and plain adjoin with no separator, as Word joins them)", r.words, want)
	}
}

// The confirmatory run (D22): LibreOffice opens the package and reads the model's words back in order.
func TestDocxOpensInLibreOffice(t *testing.T) {
	d := wtest.Hostile()
	out, _ := Write(d)
	got := wtest.LibreOfficeText(t, "hostile.docx", out)
	extra, ok := wtest.ReadBack(got, wtest.Words(d))
	// Extra: three rendered list labels and the six words of the placeholder line "[Picture: Scanned page 3 "quoted" <b>]".
	if !ok || extra != 9 {
		t.Errorf("LibreOffice does not read the model's words in order (ok=%v, %d extra, want 9):\n%s", ok, extra, got)
	}
}

func TestDocxMarksHeaderRowsAndPadsShortRows(t *testing.T) {
	cell := func(s string) docmodel.Cell {
		return docmodel.Cell{Paragraphs: [][]docmodel.Span{{{Text: s}}}, ColSpan: 1, RowSpan: 1}
	}
	d := docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Table, Page: 1, HeaderRows: 1, Rows: [][]docmodel.Cell{
		{cell("h1"), cell("h2"), cell("h3")}, {cell("a")}, {cell("b"), cell("c")},
	}}}}
	out, _ := Write(d)
	doc := string(wtest.Open(t, out).Files["word/document.xml"])
	if strings.Count(doc, "<w:tblHeader/>") != 1 {
		t.Errorf("header rows marked %d times, want once:\n%s", strings.Count(doc, "<w:tblHeader/>"), doc)
	}
	if strings.Count(doc, "<w:tc>") != 9 || strings.Count(doc, "<w:gridCol ") != 3 {
		t.Errorf("a short row is not padded to the grid: %d cells, %d columns", strings.Count(doc, "<w:tc>"), strings.Count(doc, "<w:gridCol "))
	}
	d.Blocks[0].HeaderRows = 0
	out, _ = Write(d)
	if strings.Contains(string(wtest.Open(t, out).Files["word/document.xml"]), "tblHeader") {
		t.Error("a table with no header rows marks one")
	}
}

func TestDocxTablePropertiesAreInSchemaOrderAndNothingWrittenIsNil(t *testing.T) {
	d := docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Table, Dir: "rtl", Rows: [][]docmodel.Cell{{{Paragraphs: [][]docmodel.Span{{{Text: "x"}}}, ColSpan: 1, RowSpan: 1}}}}}}
	out, _ := Write(d)
	doc := string(wtest.Open(t, out).Files["word/document.xml"])
	if strings.Index(doc, "<w:bidiVisual/>") < 0 || strings.Index(doc, "<w:bidiVisual/>") > strings.Index(doc, "<w:tblW ") {
		t.Errorf("bidiVisual must precede tblW in tblPr:\n%s", doc)
	}
	if out, _ := Write(docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Figure}, {Kind: docmodel.Paragraph}}}); out != nil {
		t.Error("blocks that render nothing must not become a blank 200")
	}
	big := docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Figure, Image: &docmodel.Image{Ext: "png", Data: []byte{1}, Width: 1000, Height: 300000000}}}}
	out, _ = Write(big)
	if strings.Contains(string(wtest.Open(t, out).Files["word/document.xml"]), `cy="-`) {
		t.Error("extent overflowed")
	}
}

func TestDocxCarriesAPictureAReaderCanDecode(t *testing.T) {
	out, _ := Write(wtest.Hostile())
	wtest.Open(t, out).DecodesAsPNG(t, "word/media/img-1.png")
}

func TestDocxPictureExtentStaysInsideWordsTwentyTwoInches(t *testing.T) {
	d := docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Figure, Image: &docmodel.Image{Ext: "png", Data: []byte{1}, Width: 100, Height: 20000}}}}
	out, _ := Write(d)
	doc := string(wtest.Open(t, out).Files["word/document.xml"])
	i := strings.Index(doc, `<wp:extent cx="`)
	if i < 0 {
		t.Fatalf("no extent:\n%s", doc)
	}
	var cx, cy int64
	if _, err := fmt.Sscanf(doc[i:], `<wp:extent cx="%d" cy="%d"`, &cx, &cy); err != nil {
		t.Fatal(err)
	}
	const limit = 22 * 914400
	if cx > limit || cy > limit || cy < 1 {
		t.Errorf("extent %d x %d EMU passes Word's 22 inches (%d)", cx, cy, limit)
	}
}
