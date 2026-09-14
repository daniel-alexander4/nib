package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// The structure editor's read door — `PLAN-accessibility.md` P09.S01.

// tableAndFigureODT is the table-and-figure truth measured at P09's phase open: LibreOffice writes a
// header row as `TH` with `/Scope /Column` and an image's title as a `Figure`'s `/Alt`.
func tableAndFigureODT(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{0x80, 0x80, 0x80, 0xff})
		}
	}
	var pic bytes.Buffer
	if err := png.Encode(&pic, img); err != nil {
		t.Fatal(err)
	}
	cell := func(s string) string {
		return `<table:table-cell office:value-type="string"><text:p>` + s + `</text:p></table:table-cell>`
	}
	body := `<text:h text:outline-level="1">Report</text:h><text:p>Intro paragraph.</text:p>` +
		`<table:table table:name="T1"><table:table-column table:number-columns-repeated="2"/>` +
		`<table:table-header-rows><table:table-row>` + cell("Name") + cell("Qty") + `</table:table-row></table:table-header-rows>` +
		`<table:table-row>` + cell("Apple") + cell("3") + `</table:table-row>` +
		`<table:table-row>` + cell("Pear") + cell("5") + `</table:table-row></table:table>` +
		`<text:p><draw:frame draw:name="img1" text:anchor-type="as-char" svg:width="2cm" svg:height="1.5cm">` +
		`<draw:image xlink:href="Pictures/a.png" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/>` +
		`<svg:title>A grey square</svg:title></draw:frame></text:p>`
	pdf, err := ConvertOfficeToPDF(odtDocument(t, "", body, odtPicture{"Pictures/a.png", "image/png", pic.Bytes()}), "odt")
	if err != nil {
		t.Fatalf("LibreOffice is present and could not convert the table-and-figure document: %v", err)
	}
	return pdf
}

// TestAnExistingTreeReadsBackAsItsOwnTruth — every element of every LibreOffice tree, in `/K` order,
// its text the truth reader's.
func TestAnExistingTreeReadsBackAsItsOwnTruth(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("LibreOffice is not installed, so there is no producer's tree to read; the inline and untagged cases still run")
	}
	docs := append(truthCorpus(t), runCorpusDoc{"a table and a figure", tableAndFigureODT(t)})
	for _, doc := range docs {
		v, err := readStructureView(doc.pdf)
		if err != nil {
			t.Fatalf("%s: %v", doc.name, err)
		}
		tree, _ := checkTree(t, doc.pdf)
		if len(v.elements) != tree.elements() || len(v.elements) == 0 {
			t.Fatalf("%s: the view holds %d element(s), the tree %d", doc.name, len(v.elements), tree.elements())
		}
		for i, e := range tree.elems {
			got := v.elements[i]
			if got.id != e.objNr || got.kind != e.kind {
				t.Errorf("%s: element %d reads %s obj %d, the tree holds %s obj %d", doc.name, i, got.kind, got.id, e.kind, e.objNr)
			}
			var want, have []int
			for _, k := range e.kids {
				if k.kind == kidElement && k.elem != nil {
					want = append(want, k.elem.objNr)
				}
			}
			for _, j := range got.kids {
				have = append(have, v.elements[j].id)
				if v.elements[j].parent != i {
					t.Errorf("%s: element %d lists kid %d, whose parent reads %d", doc.name, i, j, v.elements[j].parent)
				}
			}
			if !equalInts(want, have) {
				t.Errorf("%s: element %d's kids read %v, its /K holds %v", doc.name, i, have, want)
			}
		}
		// The truth reader's blocks, read out of the view by the truth reader's own rule.
		var blocks []truthBlock
		var walk func(i int)
		walk = func(i int) {
			e := v.elements[i]
			if e.standard == "LI" || isHeadingRole(e.standard) || e.marked {
				if text := squeeze(e.text); text != "" {
					blocks = append(blocks, truthBlock{heading: isHeadingRole(e.standard), text: text})
				}
				return
			}
			for _, j := range e.kids {
				walk(j)
			}
		}
		for i, e := range v.elements {
			if e.parent == -1 {
				walk(i)
			}
		}
		truth := readTruth(t, doc.pdf)
		if len(blocks) != len(truth) {
			t.Fatalf("%s: the view reads %d block(s), the truth reader %d", doc.name, len(blocks), len(truth))
		}
		for i := range truth {
			if blocks[i] != truth[i] {
				t.Errorf("%s: block %d reads %+v, the truth reader %+v", doc.name, i, blocks[i], truth[i])
			}
		}
	}
}

// TestATableAndAFigureReadBackTheirScopeAndAlt — the two values P09's editor exists to correct.
func TestATableAndAFigureReadBackTheirScopeAndAlt(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("LibreOffice is not installed, so the table-and-figure document cannot be generated")
	}
	src := tableAndFigureODT(t)
	before := append([]byte(nil), src...)
	v, err := readStructureView(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(src, before) {
		t.Fatal("reading the structure changed the document's bytes")
	}
	var standards []string
	for _, e := range v.elements {
		standards = append(standards, e.standard)
		switch {
		case e.standard == "TH":
			if e.scope != "Column" {
				t.Errorf("header cell %q reads scope %q, LibreOffice wrote Column", squeeze(e.text), e.scope)
			}
		case e.scope != "":
			t.Errorf("a %s reads scope %q — only a header cell declares one here", e.standard, e.scope)
		}
		if e.standard == "Figure" {
			if !e.hasAlt || e.alt != "A grey square" {
				t.Errorf("the figure reads alt %q (present %v), the document says \"A grey square\"", e.alt, e.hasAlt)
			}
		} else if e.hasAlt {
			t.Errorf("a %s reads alt %q, and only the figure has one", e.standard, e.alt)
		}
		if e.page != 1 {
			t.Errorf("a %s reads page %d on a one-page document", e.standard, e.page)
		}
	}
	if got := strings.Join(standards, " "); got != "Document H1 P Table TR TH P TH P TR TD P TD P TR TD P TD P P Figure" {
		t.Errorf("standard types read %s", got)
	}
	if v.elements[0].kind != "Document" || v.unaddressable != 0 {
		t.Errorf("root %q, unaddressable %d", v.elements[0].kind, v.unaddressable)
	}
	var headers []string
	for _, e := range v.elements {
		if e.standard == "TH" {
			headers = append(headers, squeeze(e.text))
		}
	}
	if got := strings.Join(headers, ","); got != "Name,Qty" {
		t.Errorf("header cells read %q", got)
	}
}

// TestAnInlineElementIsReportedNotDropped — an element with no object number stays in the view and is
// counted, because an edit cannot name it.
func TestAnInlineElementIsReportedNotDropped(t *testing.T) {
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4: "<< /Length 1 >>\nstream\n \nendstream",
		7: "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8: "<< /Type /StructElem /S /Sect /Pg 3 0 R /K [<< /S /P /Alt (inline) >> 9 0 R] >>",
		9: "<< /Type /StructElem /S /P /Pg 3 0 R /A << /O /Table /Scope /Row >> >>",
	})
	v, err := readStructureView(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.elements) != 3 || v.unaddressable != 1 {
		t.Fatalf("the view holds %d element(s), %d unaddressable — want 3 and 1", len(v.elements), v.unaddressable)
	}
	sect, inline, last := v.elements[0], v.elements[1], v.elements[2]
	if sect.id != 8 || !equalInts(sect.kids, []int{1, 2}) {
		t.Errorf("the section reads obj %d, kids %v", sect.id, sect.kids)
	}
	if inline.id != 0 || inline.kind != "P" || inline.alt != "inline" || inline.parent != 0 {
		t.Errorf("the inline element reads %+v", inline)
	}
	if last.id != 9 || last.scope != "Row" || last.page != 1 {
		t.Errorf("the second paragraph reads %+v", last)
	}
}

// TestAPageAndAScopeAreReadOnlyWhereTheDocumentSaysThem — an element that names no page takes the page
// its first content is on, directly or through a kid; a `/Scope` counts only in a Table attribute object.
// LibreOffice names `/Pg` on every element and puts `/Scope` only under `/O /Table`, so its trees reach
// neither branch (both survived a mutation until this fixture).
func TestAPageAndAScopeAreReadOnlyWhereTheDocumentSaysThem(t *testing.T) {
	src := assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K [10 0 R 11 0 R] >>",
		10: "<< /Type /StructElem /S /P /K << /Type /MCR /Pg 3 0 R /MCID 0 >> /A << /O /Layout /Scope /Row >> >>",
		11: "<< /Type /StructElem /S /Div /K [12 0 R] >>",
		12: "<< /Type /StructElem /S /P /K << /Type /MCR /Pg 3 0 R /MCID 1 >> >>",
	})
	v, err := readStructureView(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.elements) != 3 {
		t.Fatalf("the view holds %d element(s), want 3", len(v.elements))
	}
	mcrOnly, div, inner := v.elements[0], v.elements[1], v.elements[2]
	if mcrOnly.page != 1 || !mcrOnly.marked {
		t.Errorf("a paragraph naming its page only through its MCR reads page %d, marked %v", mcrOnly.page, mcrOnly.marked)
	}
	if mcrOnly.scope != "" {
		t.Errorf("a /Scope under a Layout attribute object reads %q — it is not a table header's", mcrOnly.scope)
	}
	if div.page != 1 || div.marked {
		t.Errorf("a division naming no page, whose kid is on page 1, reads page %d, marked %v", div.page, div.marked)
	}
	if inner.page != 1 {
		t.Errorf("the division's paragraph reads page %d", inner.page)
	}
}

// TestAnElementCarriesTheBoxOfWhatItDraws — P09.S06a: the Tags panel outlines an element from this box,
// and the exported door carries the view unchanged.
func TestAnElementCarriesTheBoxOfWhatItDraws(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("LibreOffice is not installed, so the table-and-figure document cannot be generated")
	}
	table := tableAndFigureODT(t)
	v, err := readStructureView(table)
	if err != nil {
		t.Fatal(err)
	}
	inside := func(r, b [4]float64) bool {
		return r[0] < r[2] && r[1] < r[3] && r[0] >= b[0] && r[2] <= b[2] && r[1] >= b[1] && r[3] <= b[3]
	}
	tableIdx := -1
	var headers []viewElement
	for i, e := range v.elements {
		switch {
		case e.standard == "Figure":
			if e.hasRect {
				t.Errorf("the figure draws no text and reads a box %v", e.rect)
			}
		case squeeze(e.text) != "":
			if !e.hasRect || !inside(e.rect, e.pageBox) {
				t.Errorf("a %s drawing %q reads box %v (present %v) on page box %v", e.standard, e.text, e.rect, e.hasRect, e.pageBox)
			}
		}
		if e.standard == "Table" {
			tableIdx = i
		}
		if e.standard == "TH" {
			headers = append(headers, e)
		}
	}
	if tableIdx < 0 || len(headers) != 2 {
		t.Fatalf("setup: table %d, %d header cells", tableIdx, len(headers))
	}
	tb := v.elements[tableIdx].rect
	for _, e := range v.elements {
		if e.standard == "TH" || e.standard == "TD" {
			if r := e.rect; r[0] < tb[0] || r[1] < tb[1] || r[2] > tb[2] || r[3] > tb[3] {
				t.Errorf("cell %q reads %v, outside its table's box %v", e.text, r, tb)
			}
		}
	}
	if headers[0].rect[2] > headers[1].rect[0] {
		t.Errorf("header %q ends at %.1f and %q starts at %.1f — the cells' boxes overlap or are swapped", headers[0].text, headers[0].rect[2], headers[1].text, headers[1].rect[0])
	}

	tree, err := ReadStructure(table)
	if err != nil {
		t.Fatal(err)
	}
	if !tree.Tagged || len(tree.Elements) != len(v.elements) || tree.Unaddressable != v.unaddressable {
		t.Fatalf("ReadStructure reads tagged %v with %d element(s); the view has %d", tree.Tagged, len(tree.Elements), len(v.elements))
	}
	for i, e := range tree.Elements {
		ve := v.elements[i]
		if e.Kids == nil || e.ID != ve.id || e.Parent != ve.parent || e.Rect != ve.rect || e.PageBox != ve.pageBox || e.Standard != ve.standard || e.Scope != ve.scope || e.Alt != ve.alt {
			t.Errorf("element %d reads %+v, the view %+v", i, e, ve)
		}
	}
	src, err := untaggedMarkdown([]byte("# A title\n\nA paragraph.\n"))
	if err != nil {
		t.Fatal(err)
	}
	none, err := ReadStructure(src)
	if err != nil || none.Tagged || none.Elements == nil || len(none.Elements) != 0 {
		t.Errorf("an untagged document reads %+v, %v — want untagged with an empty, non-nil list", none, err)
	}
}

// TestAnElementSpanningPagesIsOutlinedWhereItStarts — the box is on the element's own page. A section
// whose paragraphs are on two pages is outlined around its first-page paragraph only; a union across
// pages would describe a region of page 1 that page 2's text happens to occupy.
func TestAnElementSpanningPagesIsOutlinedWhereItStarts(t *testing.T) {
	page := func(n int, contents int) string {
		return fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents %d 0 R >>", contents)
	}
	src := assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R 11 0 R] /Count 2 >>",
		3:  page(1, 4),
		4:  streamObject("", "/P <</MCID 0>> BDC BT /F1 12 Tf 72 700 Td (Short) Tj ET EMC"),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8:  "<< /Type /StructElem /S /Sect /P 7 0 R /K [9 0 R 10 0 R] >>",
		9:  "<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K 0 >>",
		10: "<< /Type /StructElem /S /P /P 8 0 R /Pg 11 0 R /K 0 >>",
		11: page(2, 12),
		12: streamObject("", "/P <</MCID 0>> BDC BT /F1 12 Tf 72 700 Td (A much longer line of text on the second page) Tj ET EMC"),
	})
	v, err := readStructureView(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.elements) != 3 {
		t.Fatalf("setup: %d element(s)", len(v.elements))
	}
	sect, first, second := v.elements[0], v.elements[1], v.elements[2]
	if first.page != 1 || second.page != 2 || !first.hasRect || !second.hasRect || second.rect[2] <= first.rect[2] {
		t.Fatalf("setup: the paragraphs read page %d box %v and page %d box %v", first.page, first.rect, second.page, second.rect)
	}
	if sect.page != 1 || sect.rect != first.rect {
		t.Errorf("the section reads page %d box %v — want page 1 and exactly its first-page paragraph's box %v", sect.page, sect.rect, first.rect)
	}
}

// TestAnUntaggedDocumentHasNoStructureView.
func TestAnUntaggedDocumentHasNoStructureView(t *testing.T) {
	src, err := untaggedMarkdown([]byte("# A title\n\nA paragraph.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, verr := readStructureView(src); !errors.Is(verr, errNoStructTree) {
		t.Fatalf("an untagged document: err = %v, want errNoStructTree", verr)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
