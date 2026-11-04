package pptxexport

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"nib/internal/docwrite/wtest"
	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

type deck struct {
	sldSz        [2]string
	slides       []string
	texts        [][]string // per slide, the a:t values in order
	boxes, pics  int
	bold, italic int
	sizes        []string
	off          [][2]string
	rels         map[string]string
}

func para(t string, bold bool, r pdfops.MapRect, size float64) pdfops.ExportParagraph {
	return pdfops.ExportParagraph{Spans: []pdfops.ExportSpan{{Text: t, Bold: bold}}, Rect: r, Size: size}
}

func read(t *testing.T, data []byte) (wtest.Package, deck) {
	t.Helper()
	pkg := wtest.Open(t, data)
	pkg.WellFormed(t)
	var d deck
	for i := 1; ; i++ {
		b, ok := pkg.Files["ppt/slides/slide"+itoa(i)+".xml"]
		if !ok {
			break
		}
		var words []string
		var inT bool
		dec := xml.NewDecoder(bytes.NewReader(b))
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			switch e := tok.(type) {
			case xml.StartElement:
				switch e.Name.Local {
				case "t":
					inT = true
				case "sp":
					d.boxes++
				case "pic":
					d.pics++
				case "rPr":
					for _, a := range e.Attr {
						switch {
						case a.Name.Local == "b" && a.Value == "1":
							d.bold++
						case a.Name.Local == "i" && a.Value == "1":
							d.italic++
						case a.Name.Local == "sz":
							d.sizes = append(d.sizes, a.Value)
						}
					}
				}
			case xml.CharData:
				if inT {
					words = append(words, string(e))
				}
			case xml.EndElement:
				if e.Name.Local == "t" {
					inT = false
				}
			}
		}
		d.texts = append(d.texts, words)
	}
	return pkg, d
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestOneSlidePerPageWithPositionedBoxes(t *testing.T) {
	doc := pdfops.ExportDoc{Pages: []pdfops.ExportPage{
		{Page: 1, Width: 612, Height: 792, Paragraphs: []pdfops.ExportParagraph{
			para("Title </a:t><script>", true, pdfops.MapRect{0.1, 0.1, 0.6, 0.15}, 20),
			para("second", false, pdfops.MapRect{0.1, 0.5, 0.6, 0.55}, 10),
		}},
		{Page: 2, Width: 612, Height: 792},
		{Page: 3, Width: 612, Height: 792, Paragraphs: []pdfops.ExportParagraph{para("third", false, pdfops.MapRect{0, 0, 1, 1}, 12)}},
	}}
	out, c, err := Write(doc, `a "title" & <b>`)
	if err != nil {
		t.Fatal(err)
	}
	pkg, d := read(t, out)
	if c.Slides != 3 || len(d.texts) != 3 {
		t.Fatalf("slides = %d / %d, want 3: one per page, a blank page included", c.Slides, len(d.texts))
	}
	if d.texts[0][0] != "Title </a:t><script>" || d.texts[0][1] != "second" || len(d.texts[1]) != 0 || d.texts[2][0] != "third" {
		t.Errorf("text by slide = %q", d.texts)
	}
	if c.TextBoxes != 3 || d.bold != 1 {
		t.Errorf("boxes = %d, bold runs = %d", c.TextBoxes, d.bold)
	}
	if d.sizes[0] != "2000" || d.sizes[1] != "1000" {
		t.Errorf("sizes = %v, want 2000 and 1000 (points x 100)", d.sizes)
	}
	s := string(pkg.Files["ppt/slides/slide1.xml"])
	if !strings.Contains(s, `<a:off x="777240" y="1005840"/>`) || !strings.Contains(s, `<a:ext cx="`) {
		t.Errorf("the first box is not at 0.1 of the page: %s", s)
	}
	if strings.Contains(s, "<script") {
		t.Error("text became markup")
	}
	if !strings.Contains(string(pkg.Files["ppt/presentation.xml"]), `<p:sldSz cx="7772400" cy="10058400"/>`) {
		t.Errorf("slide size: %s", pkg.Files["ppt/presentation.xml"])
	}
	if !strings.Contains(string(pkg.Files["docProps/core.xml"]), "&lt;b&gt;") {
		t.Error("title not escaped")
	}
}

func TestEveryPartIsReferencedAndTyped(t *testing.T) {
	doc := pdfops.ExportDoc{Pages: []pdfops.ExportPage{{Page: 1, Width: 100, Height: 100, Figures: []pdfops.ExportFigure{
		{Rect: pdfops.MapRect{0.1, 0.1, 0.5, 0.5}, Ext: "png", Data: []byte{0x89, 'P', 'N', 'G'}},
		{Rect: pdfops.MapRect{0.5, 0.5, 0.9, 0.9}, Placeholder: pdfops.PlaceholderMasked},
	}}}}
	out, c, err := Write(doc, "t")
	if err != nil {
		t.Fatal(err)
	}
	pkg, d := read(t, out)
	if c.Pictures != 1 || c.Placeholders != 1 || d.pics != 1 {
		t.Errorf("pictures = %d, placeholders = %d, p:pic = %d", c.Pictures, c.Placeholders, d.pics)
	}
	ct := string(pkg.Files["[Content_Types].xml"])
	for name := range pkg.Files {
		if strings.HasSuffix(name, ".rels") || name == "[Content_Types].xml" {
			continue
		}
		ext := filepath.Ext(name)[1:]
		if !strings.Contains(ct, `PartName="/`+name+`"`) && !strings.Contains(ct, `Extension="`+ext+`"`) {
			t.Errorf("%s has no content type", name)
		}
	}
	if !strings.Contains(string(pkg.Files["ppt/slides/_rels/slide1.xml.rels"]), `Target="../media/image1.png"`) {
		t.Errorf("rels: %s", pkg.Files["ppt/slides/_rels/slide1.xml.rels"])
	}
	for _, bad := range []string{"http://", "https://"} {
		for n, b := range pkg.Files {
			if strings.Contains(string(b), bad) && !strings.Contains(string(b), "schemas.") && !strings.Contains(string(b), "purl.org") {
				t.Errorf("%s holds an external reference %q", n, bad)
			}
		}
	}
}

func TestLinksAreDroppedAndCounted(t *testing.T) {
	p := pdfops.ExportParagraph{Spans: []pdfops.ExportSpan{{Text: "see ", Link: ""}, {Text: "here", Link: "https://example.org"}}, Rect: pdfops.MapRect{0, 0, 1, 1}, Size: 10}
	out, c, _ := Write(pdfops.ExportDoc{Pages: []pdfops.ExportPage{{Page: 1, Width: 100, Height: 100, Paragraphs: []pdfops.ExportParagraph{p}}}}, "t")
	if c.DroppedLinks != 1 || strings.Contains(string(wtest.Open(t, out).Files["ppt/slides/slide1.xml"]), "example.org") {
		t.Errorf("dropped links = %d", c.DroppedLinks)
	}
}

func TestTooManyPagesRefused(t *testing.T) {
	_, _, err := Write(pdfops.ExportDoc{Pages: make([]pdfops.ExportPage, MaxSlides+1)}, "t")
	if err != ErrTooManySlides {
		t.Errorf("err = %v", err)
	}
}

func TestHugePageIsScaledToPowerPointsCeiling(t *testing.T) {
	out, _, _ := Write(pdfops.ExportDoc{Pages: []pdfops.ExportPage{{Page: 1, Width: 14400, Height: 7200}}}, "t")
	if !strings.Contains(string(wtest.Open(t, out).Files["ppt/presentation.xml"]), `cx="51206400" cy="25603200"`) {
		t.Error("not clamped")
	}
}

// The fixtures are real PDFs: the visibility rule is the reader's, so a covered run reaches no slide (R1).
func TestFromRealPagesCoveredTextIsNotOnASlide(t *testing.T) {
	var pdf []byte
	for _, f := range testpdf.ExportSet() {
		if f.Name == "covered" {
			pdf = f.PDF
		}
	}
	doc, err := pdfops.ExportRead(pdf, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := Write(doc, "t")
	if err != nil {
		t.Fatal(err)
	}
	_, d := read(t, out)
	all := strings.Join(d.texts[0], " ")
	if !strings.Contains(all, "visible heading") || strings.Contains(all, "secret") {
		t.Errorf("slide text = %q", all)
	}
}

func TestFromRealPagesPicturesArePlaced(t *testing.T) {
	var pdf []byte
	for _, f := range testpdf.ExportPictures() {
		if f.Name == "picture-jpeg" {
			pdf = f.PDF
		}
	}
	doc, _ := pdfops.ExportRead(pdf, nil)
	out, c, _ := Write(doc, "t")
	pkg, d := read(t, out)
	if c.Pictures != 1 || d.pics != 1 || len(pkg.Files["ppt/media/image1.jpg"]) == 0 {
		t.Errorf("pictures = %d, p:pic = %d, media = %v", c.Pictures, d.pics, pkg.Order)
	}
}

// LibreOffice Impress reads it back: one page per slide and the words in order (D22). Skipped, by name, without soffice.
func TestImpressReadsItBack(t *testing.T) {
	bin, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("soffice is not installed: the Impress confirmatory run is NOT RUN (the XML oracle ran)")
	}
	pdf, err := testpdf.Text("first page words", "second page words")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pdfops.ExportRead(pdf, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := Write(doc, "t")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "deck.pptx")
	if err := os.WriteFile(in, out, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-env:UserInstallation=file://"+filepath.Join(dir, "profile"), "--headless", "--convert-to", "pdf", "--outdir", filepath.Join(dir, "out"), in)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "LANG=C.UTF-8", "TMPDIR=" + dir}
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("soffice: %v\n%s", err, b)
	}
	back, err := os.ReadFile(filepath.Join(dir, "out", "deck.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfops.DocumentText(back)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("Impress made %d pages, want 2", len(pages))
	}
	for i, want := range []string{"first page words", "second page words"} {
		var got []string
		for _, r := range pages[i].Runs {
			got = append(got, r.Text)
		}
		if j := strings.Join(strings.Fields(strings.Join(got, " ")), " "); j != want {
			t.Errorf("page %d reads %q, want %q", i+1, j, want)
		}
	}
}

func TestTableCellsItalicBlanksAndBoxesStayOnTheSlide(t *testing.T) {
	cell := func(p pdfops.ExportParagraph) pdfops.ExportCell {
		return pdfops.ExportCell{Paragraphs: []pdfops.ExportParagraph{p}}
	}
	it := pdfops.ExportParagraph{Spans: []pdfops.ExportSpan{{Text: "slanted", Italic: true}}, Rect: pdfops.MapRect{0.9, 0.1, 1, 0.2}, Size: 10}
	doc := pdfops.ExportDoc{Pages: []pdfops.ExportPage{
		{Page: 1, Width: 100, Height: 100,
			Tables:     []pdfops.ExportTable{{Rows: [][]pdfops.ExportCell{{cell(para("c11", false, pdfops.MapRect{0.1, 0.3, 0.3, 0.4}, 10)), cell(it)}}}},
			Paragraphs: []pdfops.ExportParagraph{para("   ", false, pdfops.MapRect{0.1, 0.5, 0.3, 0.6}, 10)}},
		// a page of another shape is fitted inside the deck's slide, not stretched over it
		{Page: 2, Width: 200, Height: 100, Paragraphs: []pdfops.ExportParagraph{para("wide", false, pdfops.MapRect{0, 0, 1, 1}, 10)}},
	}}
	out, c, err := Write(doc, "t")
	if err != nil {
		t.Fatal(err)
	}
	pkg, d := read(t, out)
	if c.TextBoxes != 3 || d.italic != 1 || len(d.texts[0]) != 2 || d.texts[0][0] != "c11" {
		t.Errorf("boxes = %d, italic = %d, texts = %q: a table cell is a box, a blank paragraph is none", c.TextBoxes, d.italic, d.texts)
	}
	// slide 100x100 pt = 1270000 EMU square; the right-edge box must end inside it, and the wide page sits letterboxed
	type xf struct{ x, y, cx, cy int }
	var xfs [][]xf
	for i := 1; i <= 2; i++ {
		var row []xf
		dec := xml.NewDecoder(bytes.NewReader(pkg.Files["ppt/slides/slide"+strconv.Itoa(i)+".xml"]))
		var cur xf
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			if e, ok := tok.(xml.StartElement); ok {
				for _, a := range e.Attr {
					v, _ := strconv.Atoi(a.Value)
					switch {
					case e.Name.Local == "off" && a.Name.Local == "x":
						cur.x = v
					case e.Name.Local == "off" && a.Name.Local == "y":
						cur.y = v
					case e.Name.Local == "ext" && a.Name.Local == "cx":
						cur.cx = v
					case e.Name.Local == "ext" && a.Name.Local == "cy":
						cur.cy = v
						row = append(row, cur)
					}
				}
			}
		}
		xfs = append(xfs, row[1:]) // the group's own zero transform comes first
	}
	for _, b := range xfs[0] {
		if b.x+b.cx > 1270000 {
			t.Errorf("a box runs past the slide: %+v", b)
		}
	}
	if w := xfs[1][0]; w.cx != 1270000 || w.cy != 635000 || w.y != 317500 {
		t.Errorf("a 2:1 page on a square deck should fill the width and sit centred: %+v", w)
	}
}
