package odt

import (
	"bytes"
	"encoding/xml"
	"io"
	"slices"
	"strings"
	"testing"

	"nib/internal/docmodel"
	"nib/internal/docwrite/wtest"
)

// read is what a reader of content.xml finds, from the XML alone (R1's second oracle).
type read struct {
	words      []string
	headings   map[string]int
	lists      map[string]int // list style -> lists
	items      int
	tables     int
	cells      int
	covered    int
	headRows   int
	links      []string
	frames     int
	spanBold   int
	spanItalic int
	rtl        int
}

func parse(t *testing.T, data []byte) read {
	t.Helper()
	r := read{headings: map[string]int{}, lists: map[string]int{}}
	d := xml.NewDecoder(bytes.NewReader(data))
	var para strings.Builder
	depth := 0 // text:p / text:h nesting
	desc := 0  // inside svg:desc: a picture's description is not text of the document
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
			case "p", "h":
				if depth == 0 {
					para.Reset()
				}
				depth++
				if e.Name.Local == "h" {
					r.headings[attr("outline-level")]++
				}
				if strings.HasSuffix(attr("style-name"), "R") && attr("style-name") != "" {
					r.rtl++
				}
			case "list":
				r.lists[attr("style-name")]++
			case "list-item":
				r.items++
			case "table":
				r.tables++
			case "table-cell":
				r.cells++
			case "covered-table-cell":
				r.covered++
			case "table-header-rows":
				r.headRows++
			case "a":
				r.links = append(r.links, attr("href"))
			case "frame":
				r.frames++
			case "span":
				switch attr("style-name") {
				case "TB", "TBI":
					r.spanBold++
				}
				if s := attr("style-name"); s == "TI" || s == "TBI" {
					r.spanItalic++
				}
			case "desc":
				desc++
			case "tab", "line-break":
				para.WriteString(" ")
			case "s":
				n := 1
				for _, a := range e.Attr {
					if a.Name.Local == "c" {
						n = 0
						for _, c := range a.Value {
							n = n*10 + int(c-'0')
						}
					}
				}
				para.WriteString(strings.Repeat(" ", n))
			}
		case xml.CharData:
			if depth > 0 && desc == 0 {
				para.Write(e)
			}
		case xml.EndElement:
			if e.Name.Local == "desc" {
				desc--
			}
			if e.Name.Local == "p" || e.Name.Local == "h" {
				depth--
				if depth == 0 && !strings.HasPrefix(para.String(), "[Picture:") {
					r.words = append(r.words, strings.Fields(para.String())...)
				}
			}
		}
	}
	return r
}

func TestODTReadsBackAsTheModel(t *testing.T) {
	d := wtest.Hostile()
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	pkg := wtest.Open(t, out)
	pkg.WellFormed(t)
	r := parse(t, pkg.Files["content.xml"])
	if want := wtest.Words(d); !slices.Equal(r.words, want) {
		t.Errorf("the words differ from the model's\n got %q\nwant %q", r.words, want)
	}
	if r.headings["1"] != 1 || len(r.headings) != 1 {
		t.Errorf("headings = %v", r.headings)
	}
	if r.tables != 1 || r.cells != 4 || r.headRows != 1 {
		t.Errorf("tables = %d, cells = %d, header groups = %d", r.tables, r.cells, r.headRows)
	}
	if r.items != 3 {
		t.Errorf("list items = %d, want 3", r.items)
	}
	if r.lists["L1"] != 1 || r.lists["L2"] != 1 {
		t.Errorf("lists = %v, want the numbered list (L2) and the bullet list nested in it (L1)", r.lists)
	}
	if len(r.links) != 1 || r.frames != 1 {
		t.Errorf("links = %v, frames = %d", r.links, r.frames)
	}
	if r.spanBold != 3 || r.rtl == 0 {
		t.Errorf("bold spans = %d (the word and two header cells), rtl = %d", r.spanBold, r.rtl)
	}
	content := string(pkg.Files["content.xml"])
	for _, bad := range []string{"<script", `xlink:href="javascript`, "\x00"} {
		if strings.Contains(content, bad) {
			t.Errorf("content.xml holds %q", bad)
		}
	}
}

func TestODTMimetypeIsFirstStoredAndAtByte38(t *testing.T) {
	out, _ := Write(wtest.Hostile())
	if string(out[:2]) != "PK" || out[8] != 0 || out[9] != 0 {
		t.Errorf("first member is not stored: % x", out[:10])
	}
	if got := string(out[38 : 38+len(mime)]); got != mime {
		t.Errorf("byte 38 holds %q, want the media type", got)
	}
	if pkg := wtest.Open(t, out); pkg.Order[0] != "mimetype" {
		t.Errorf("first entry %q", pkg.Order[0])
	}
}

func TestODTHoldsOnlyItsNamedPartsAndNoClaims(t *testing.T) {
	out, _ := Write(wtest.Hostile())
	pkg := wtest.Open(t, out)
	allowed := map[string]bool{"Pictures/img-1.png": true}
	for _, p := range Parts {
		allowed[p] = true
	}
	for _, n := range pkg.Order {
		if !allowed[n] {
			t.Errorf("unexpected part %q", n)
		}
	}
	meta := string(pkg.Files["meta.xml"])
	for _, bad := range []string{"initial-creator", "creation-date", "user-defined", "conformsTo", "pdfuaid", "<script"} {
		if strings.Contains(meta, bad) {
			t.Errorf("meta.xml holds %q:\n%s", bad, meta)
		}
	}
	if !strings.Contains(string(pkg.Files["META-INF/manifest.xml"]), `full-path="Pictures/img-1.png" manifest:media-type="image/png"`) {
		t.Error("the picture is not in the manifest")
	}
}

func TestODTTableSpansAreRealCovers(t *testing.T) {
	cell := func(s string, cs, rs int) docmodel.Cell {
		return docmodel.Cell{Paragraphs: [][]docmodel.Span{{{Text: s}}}, ColSpan: cs, RowSpan: rs}
	}
	d := docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Table, Page: 1, Rows: [][]docmodel.Cell{
		{cell("a", 2, 1), cell("b", 1, 2)},
		{cell("c", 1, 1), cell("d", 1, 1)},
	}}}}
	out, _ := Write(d)
	r := parse(t, wtest.Open(t, out).Files["content.xml"])
	// a covers one position to its right; row 2 holds c, d and the position b covers.
	if r.cells != 4 || r.covered != 2 || !slices.Equal(r.words, []string{"a", "b", "c", "d"}) {
		t.Errorf("cells = %d, covered = %d, words = %q", r.cells, r.covered, r.words)
	}
}

func TestODTSpacesAndOfNothing(t *testing.T) {
	if out, err := Write(docmodel.Document{}); out != nil || err != nil {
		t.Errorf("got %d bytes, %v", len(out), err)
	}
	d := docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Paragraph, Spans: []docmodel.Span{{Text: "  a   b "}, {Text: "c"}}}}}
	out, _ := Write(d)
	content := string(wtest.Open(t, out).Files["content.xml"])
	if !strings.Contains(content, `<text:s text:c="2"/>a <text:s text:c="2"/>b `) {
		t.Errorf("collapsing spaces are not counted:\n%s", content)
	}
}

// The confirmatory run (D22): LibreOffice opens the package and reads the model's words back in order.
func TestODTOpensInLibreOffice(t *testing.T) {
	d := wtest.Hostile()
	out, _ := Write(d)
	got := wtest.LibreOfficeText(t, "hostile.odt", out)
	extra, ok := wtest.ReadBack(got, wtest.Words(d))
	// Extra: three rendered list labels and the six words of the placeholder line "[Picture: Scanned page 3 "quoted" <b>]".
	if !ok || extra != 9 {
		t.Errorf("LibreOffice does not read the model's words in order (ok=%v, %d extra, want 9):\n%s", ok, extra, got)
	}
}

func TestODTTwoListsAtOneDepthRestartAndNestingStaysInsideItsItem(t *testing.T) {
	item := func(list, level int, marker, s string) docmodel.Block {
		return docmodel.Block{Kind: docmodel.ListItem, List: list, Level: level, Marker: marker, Spans: []docmodel.Span{{Text: s}}}
	}
	d := docmodel.Document{Blocks: []docmodel.Block{
		item(1, 1, "1.", "a"), item(1, 2, "-", "a1"), item(1, 1, "2.", "b"),
		item(2, 1, "1.", "c"),
	}}
	out, _ := Write(d)
	content := string(wtest.Open(t, out).Files["content.xml"])
	if strings.Count(content, `<text:list text:style-name="L2">`) != 2 {
		t.Errorf("two model lists must be two ODF lists so numbering restarts:\n%s", content)
	}
	if !strings.Contains(content, `a1</text:p></text:list-item></text:list></text:list-item><text:list-item><text:p text:style-name="LP">b`) {
		t.Errorf("the nested list is not closed inside its parent item before the next item:\n%s", content)
	}
}

func TestODTNothingWrittenIsNilAndAFlatPictureKeepsAHeight(t *testing.T) {
	if out, _ := Write(docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Figure}}}); out != nil {
		t.Error("blocks that render nothing must not become a blank 200")
	}
	d := docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Figure, Image: &docmodel.Image{Ext: "png", Data: []byte{1}, Width: 100000, Height: 1}}}}
	out, _ := Write(d)
	if strings.Contains(string(wtest.Open(t, out).Files["content.xml"]), `svg:height="0.000in"`) {
		t.Error("a zero-height frame")
	}
}
