package pdfops

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

func pictureFixture(t *testing.T, name string) ExportPage {
	t.Helper()
	for _, f := range testpdf.ExportPictures() {
		if f.Name == name {
			pages, err := ExportPages(f.PDF, nil)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			return pages[0]
		}
	}
	t.Fatalf("no fixture %q", name)
	return ExportPage{}
}

func TestExportPicturesCarryOrNameTheirCause(t *testing.T) {
	cases := []struct {
		name    string
		figures int
		cause   string
	}{
		{"picture-plain", 1, ""},
		{"picture-twice", 2, ""},
		{"picture-in-form", 1, ""},
		{"picture-turned", 1, PlaceholderTurned},
		{"picture-mirrored", 1, PlaceholderTurned},
		{"picture-softmask", 1, PlaceholderSoftMask},
		{"picture-stencil", 1, PlaceholderStencil},
		{"picture-inline", 1, PlaceholderInline},
		{"picture-jpeg", 1, ""},
		{"picture-cmyk", 1, PlaceholderColour},
		{"picture-masked", 1, PlaceholderMasked},
		{"picture-ccitt", 1, PlaceholderFilter},
		{"picture-skewed", 1, PlaceholderTurned},
		{"picture-narrow", 0, ""},
		{"picture-small", 0, ""},
		{"picture-artifact", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := pictureFixture(t, c.name)
			if len(p.Figures) != c.figures {
				t.Fatalf("figures = %d, want %d (%+v)", len(p.Figures), c.figures, p.Figures)
			}
			for _, f := range p.Figures {
				if f.Placeholder != c.cause {
					t.Errorf("placeholder = %q, want %q", f.Placeholder, c.cause)
				}
				if (c.cause == "") != (len(f.Data) > 0) {
					t.Errorf("data %d bytes with placeholder %q", len(f.Data), f.Placeholder)
				}
				if c.cause == "" && f.Ext != "png" && f.Ext != "jpg" {
					t.Errorf("ext = %q", f.Ext)
				}
				if c.cause == "" {
					im, _, err := image.Decode(bytes.NewReader(f.Data))
					if err != nil || im.Bounds().Dx() != f.Width || im.Bounds().Dy() != f.Height {
						t.Errorf("carried bytes do not decode to %dx%d: %v", f.Width, f.Height, err)
					}
					if c.name == "picture-jpeg" && f.Ext != "jpg" {
						t.Errorf("a stored JPEG is carried as %q", f.Ext)
					}
				}
				if f.Rect[2] <= f.Rect[0] || f.Rect[3] <= f.Rect[1] {
					t.Errorf("rect %v is empty", f.Rect)
				}
			}
		})
	}
	if p := pictureFixture(t, "picture-small"); p.SkippedImages["small"] != 1 {
		t.Errorf("small not counted: %v", p.SkippedImages)
	}
	if p := pictureFixture(t, "picture-artifact"); p.SkippedImages["artifact"] != 1 {
		t.Errorf("artifact not counted: %v", p.SkippedImages)
	}
}

func TestExportPictureDrawnTwiceSharesItsBytes(t *testing.T) {
	p := pictureFixture(t, "picture-twice")
	if len(p.Figures) != 2 || p.Figures[0].ObjNr != p.Figures[1].ObjNr || &p.Figures[0].Data[0] != &p.Figures[1].Data[0] {
		t.Fatalf("two figures should name one object and one buffer: %+v", p.Figures)
	}
	if p.Figures[0].Rect == p.Figures[1].Rect {
		t.Fatal("the two placements have one rect")
	}
}

func TestExportStyledSpansAreTheParagraphsText(t *testing.T) {
	for _, f := range testpdf.ExportSet() {
		pages, err := ExportPages(f.PDF, nil)
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		layoutText := map[int][]string{}
		_ = layoutText
		for _, p := range pages {
			if p.UnstyledLines != 0 {
				t.Errorf("%s: %d lines fell back to a plain span", f.Name, p.UnstyledLines)
			}
		}
	}
	for _, f := range testpdf.ExportSet() {
		if f.Name != "styled-linked" {
			continue
		}
		pages, _ := ExportPages(f.PDF, nil)
		paras := pages[0].Paragraphs
		plain, _ := Paragraphs(f.PDF, 1)
		if len(paras) != len(plain) {
			t.Fatalf("paragraphs %d vs %d", len(paras), len(plain))
		}
		for i := range paras {
			if paras[i].Text() != plain[i].Text {
				t.Errorf("paragraph %d: spans read %q, Paragraphs read %q", i, paras[i].Text(), plain[i].Text)
			}
		}
		if s := paras[0].Spans; len(s) == 0 || !s[0].Bold {
			t.Errorf("title should open bold: %+v", s)
		}
		var linked string
		for _, p := range paras {
			for _, s := range p.Spans {
				if s.Link != "" {
					linked += s.Text
					if s.Link != "https://example.org/x" {
						t.Errorf("link = %q", s.Link)
					}
				}
			}
		}
		if !strings.Contains(linked, "link") {
			t.Errorf("no span carries the link: %+v", paras)
		}
	}
}

func TestExportFigureIsPlacedAndOrdered(t *testing.T) {
	p := pictureFixture(t, "picture-plain")
	f := p.Figures[0]
	// Drawn 72..132 across and 600..660 up a 612x792 page: fractions of the displayed page, top-left origin.
	want := MapRect{72.0 / 612, (792.0 - 660) / 792, 132.0 / 612, (792.0 - 600) / 792}
	for i := range want {
		if d := f.Rect[i] - want[i]; d > 1e-6 || d < -1e-6 {
			t.Fatalf("rect = %v, want %v", f.Rect, want)
		}
	}
	if f.Before != 1 || len(p.Paragraphs) != 1 {
		t.Errorf("the picture is under the one line of text: before = %d of %d paragraphs", f.Before, len(p.Paragraphs))
	}
}

// A stencil makes pdfcpu refuse the page's extraction. The export does not fail for it: the stencil is named from its
// dictionary, and the page's other picture, which pdfcpu would not hand over, is `not-extracted` (declared).
func TestExportAStencilDoesNotSinkItsPage(t *testing.T) {
	p := pictureFixture(t, "picture-stencil-beside-plain")
	if len(p.Figures) != 2 {
		t.Fatalf("figures = %+v", p.Figures)
	}
	got := map[string]int{}
	for _, f := range p.Figures {
		got[f.Placeholder]++
	}
	if got[PlaceholderStencil] != 1 || got[PlaceholderNotRead]+got[""] != 1 {
		t.Errorf("causes = %v", got)
	}
	if n := PlaceholderCounts([]ExportPage{p})[PlaceholderStencil]; n != 1 {
		t.Errorf("PlaceholderCounts = %d", n)
	}
}

func TestExportCapsRefuseByName(t *testing.T) {
	defer func(a, b int64, c int) { exportImageBytes, exportImageTotal, exportImagePixels = a, b, c }(exportImageBytes, exportImageTotal, exportImagePixels)
	cases := []struct {
		name  string
		set   func()
		cause string
	}{
		{"bytes", func() { exportImageBytes = 4 }, PlaceholderTooBig},
		{"pixels", func() { exportImagePixels = 3 }, PlaceholderTooBig},
		{"total", func() { exportImageTotal = 4 }, PlaceholderBudget},
	}
	for _, c := range cases {
		exportImageBytes, exportImageTotal, exportImagePixels = MaxExportImageBytes, MaxExportImageTotal, maxImagePixels
		c.set()
		p := pictureFixture(t, "picture-plain")
		if p.Figures[0].Placeholder != c.cause || p.Figures[0].Data != nil {
			t.Errorf("%s: placeholder %q, data %d bytes", c.name, p.Figures[0].Placeholder, len(p.Figures[0].Data))
		}
	}
}

func TestExportOneObjectOnTwoPagesIsReadOnce(t *testing.T) {
	defer func(b int64) { exportImageTotal = b }(exportImageTotal)
	one, err := ExportPages(testpdf.ExportPictureTwoPages(), nil)
	if err != nil || len(one[0].Figures[0].Data) == 0 {
		t.Fatalf("baseline: %v", err)
	}
	exportImageTotal = int64(len(one[0].Figures[0].Data)) + 1 // the picture fits once; a second charge for it would not
	pages, err := ExportPages(testpdf.ExportPictureTwoPages(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		if len(p.Figures) != 1 || p.Figures[0].Placeholder != "" {
			t.Fatalf("page %d: %+v", p.Page, p.Figures)
		}
	}
}

// The page-level facts the document model reads at S03c; read here so each is a named, exercised observable.
func TestExportPageCarriesItsFacts(t *testing.T) {
	scanned, err := ExportPages(testpdf.ExportSet()[2].PDF, nil)
	if err != nil || !scanned[0].NoText || scanned[0].Unsupported != "" || len(scanned[0].Tables) != 0 {
		t.Fatalf("scanned page: %+v %v", scanned, err)
	}
	for _, f := range testpdf.ExportSet() {
		if f.Name == "styled-linked" {
			p, _ := ExportPages(f.PDF, nil)
			if p[0].NoText || p[0].DroppedLinks["internal"] != 0 {
				t.Errorf("styled-linked: %+v", p[0])
			}
		}
	}
}

// A page whose pictures include one pdfcpu cannot render keeps the others: the pass skips that picture and counts it,
// and the file is read once (/pending 941 #3).
func TestExportAPictureThatCannotBeRenderedDoesNotCostItsNeighbours(t *testing.T) {
	var pdf []byte
	for _, f := range testpdf.ExportPictures() {
		if f.Name == "picture-stencil-beside-plain" {
			pdf = f.PDF
		}
	}
	p := pictureFixture(t, "picture-stencil-beside-plain")
	carried := 0
	for _, f := range p.Figures {
		if len(f.Data) > 0 {
			carried++
		}
	}
	if carried != 1 {
		t.Fatalf("the plain neighbour of an unrenderable stencil is carried: %d figures with data (%+v)", carried, p.Figures)
	}
	seen := 0
	skipped, err := pdfread.ExtractImagesEach(context.Background(), pdf, nil, 1<<40, func(model.Image) error { seen++; return nil })
	if err != nil || seen+skipped != 2 {
		t.Fatalf("seen %d skipped %d err %v, want the two images accounted for", seen, skipped, err)
	}
}

// The pixel cap is applied from the dictionary, before pdfcpu decodes: a picture over it is never handed to the
// renderer, even when it shares a page with one that is carried (/pending 941 #4).
func TestExportPixelCapStopsBeforeTheDecode(t *testing.T) {
	var pdf []byte
	for _, f := range testpdf.ExportPictures() {
		if f.Name == "picture-twice" {
			pdf = f.PDF
		}
	}
	seen := 0
	skipped, err := pdfread.ExtractImagesEach(context.Background(), pdf, nil, 3, func(model.Image) error { seen++; return nil })
	if err != nil || seen != 0 || skipped != 1 {
		t.Fatalf("2x2 image under a cap of 3 px: seen %d skipped %d err %v, want 0 seen, 1 skipped", seen, skipped, err)
	}
	seen = 0
	if _, err := pdfread.ExtractImagesEach(context.Background(), pdf, nil, 4, func(model.Image) error { seen++; return nil }); err != nil || seen == 0 {
		t.Fatalf("2x2 image at a cap of 4 px (the control): seen %d err %v", seen, err)
	}
}

func TestImageCauseReadsWhatTheDictionaryHides(t *testing.T) {
	icc := func(n int) types.Object { return types.StreamDict{Dict: types.Dict{"N": types.Integer(n)}} }
	xt := &model.XRefTable{Table: map[int]*model.XRefTableEntry{
		1: {Object: types.Name("JBIG2Decode")},
		2: {Object: icc(4)},
		3: {Object: icc(3)},
		4: {Object: types.Name("FlateDecode")},
		5: {Object: types.Array{types.Name("DeviceCMYK")}},
		6: {Object: types.Array{types.Name("ICCBased"), *types.NewIndirectRef(2, 0)}},
	}}
	ref := func(n int) types.Object { return *types.NewIndirectRef(n, 0) }
	base := func(extra types.Dict) types.Dict {
		d := types.Dict{"Width": types.Integer(2), "Height": types.Integer(2), "ColorSpace": types.Name("DeviceRGB"), "BitsPerComponent": types.Integer(8)}
		for k, v := range extra {
			d[k] = v
		}
		return d
	}
	for name, c := range map[string]struct {
		d    types.Dict
		want string
	}{
		"control":                  {base(nil), ""},
		"default decode":           {base(types.Dict{"Decode": types.Array{types.Integer(0), types.Integer(1), types.Integer(0), types.Integer(1), types.Integer(0), types.Integer(1)}}), ""},
		"inverted decode":          {base(types.Dict{"Decode": types.Array{types.Integer(1), types.Integer(0), types.Integer(1), types.Integer(0), types.Integer(1), types.Integer(0)}}), PlaceholderColour},
		"indirect filter":          {base(types.Dict{"Filter": ref(1)}), PlaceholderFilter},
		"indirect filter element":  {base(types.Dict{"Filter": types.Array{ref(4), ref(1)}}), PlaceholderFilter},
		"icc 4":                    {base(types.Dict{"ColorSpace": types.Array{types.Name("ICCBased"), ref(2)}}), PlaceholderColour},
		"icc 3":                    {base(types.Dict{"ColorSpace": types.Array{types.Name("ICCBased"), ref(3)}}), ""},
		"indexed over cmyk":        {base(types.Dict{"ColorSpace": types.Array{types.Name("Indexed"), types.Name("DeviceCMYK"), types.Integer(1), types.StringLiteral("")}}), PlaceholderColour},
		"indexed over icc 4":       {base(types.Dict{"ColorSpace": types.Array{types.Name("Indexed"), ref(6), types.Integer(1), types.StringLiteral("")}}), PlaceholderColour},
		"indexed over rgb":         {base(types.Dict{"ColorSpace": types.Array{types.Name("Indexed"), types.Name("DeviceRGB"), types.Integer(1), types.StringLiteral("")}}), ""},
		"indexed default decode":   {base(types.Dict{"ColorSpace": types.Array{types.Name("Indexed"), types.Name("DeviceRGB"), types.Integer(1), types.StringLiteral("")}, "Decode": types.Array{types.Integer(0), types.Integer(255)}}), ""},
		"indexed reversed decode":  {base(types.Dict{"ColorSpace": types.Array{types.Name("Indexed"), types.Name("DeviceRGB"), types.Integer(1), types.StringLiteral("")}, "Decode": types.Array{types.Integer(255), types.Integer(0)}}), PlaceholderColour},
		"decode narrowed":          {base(types.Dict{"Decode": types.Array{types.Integer(0), types.Float(0.5), types.Integer(0), types.Float(0.5), types.Integer(0), types.Float(0.5)}}), PlaceholderColour},
		"decode shifted":           {base(types.Dict{"Decode": types.Array{types.Float(0.5), types.Integer(1), types.Float(0.5), types.Integer(1), types.Float(0.5), types.Integer(1)}}), PlaceholderColour},
		"decode of odd length":     {base(types.Dict{"Decode": types.Array{types.Integer(0), types.Integer(1), types.Integer(0)}}), PlaceholderColour},
		"indexed decode wrong top": {base(types.Dict{"ColorSpace": types.Array{types.Name("Indexed"), types.Name("DeviceRGB"), types.Integer(1), types.StringLiteral("")}, "Decode": types.Array{types.Integer(0), types.Integer(100)}}), PlaceholderColour},
		"over the pixel cap":       {base(types.Dict{"Width": types.Integer(20000), "Height": types.Integer(20000)}), PlaceholderTooBig},
		"indirect size over cap":   {base(types.Dict{"Width": types.Integer(1 << 20), "Height": types.Integer(1 << 20)}), PlaceholderTooBig},
		"cmyk device":              {base(types.Dict{"ColorSpace": ref(5)}), PlaceholderColour},
	} {
		if got := imageCause(xt, c.d); got != c.want {
			t.Errorf("%s: imageCause = %q, want %q", name, got, c.want)
		}
	}
}

func spacedRun(text string, x float64) textRun {
	r := textRun{text: text, decoded: true, x: x, y: 100, size: 10}
	for _, c := range text {
		r.glyphs = append(r.glyphs, runGlyph{text: string(c), decoded: true, advance: 10})
	}
	r.width = 10 * float64(len(r.glyphs))
	return r
}

// A link that covers part of a run links that part (/pending 941 #8). A word is in a link when the link holds half its
// glyphs; a run whose glyphs cannot be laid along it is judged by its own centre.
func TestALinkOverPartOfARunLinksThatPart(t *testing.T) {
	sp := displaySpace{box: [4]float64{0, 0, 200, 200}, w: 200, h: 200}
	const uri = "https://x.example/"
	type span struct {
		text string
		link bool
	}
	cases := []struct {
		name  string
		x0    float64 // the link's left and right edge, in points (glyph i spans 10i..10i+10)
		x1    float64
		mod   func(*textRun)
		spans []span
	}{
		{"exactly a word", 40, 80, nil, []span{{"see ", false}, {"docs", true}, {" now", false}}},
		{"half a word links the word", 40, 60, nil, []span{{"see ", false}, {"docs", true}, {" now", false}}},
		{"under half a word links nothing", 40, 49, nil, []span{{"see docs now", false}}},
		{"two words of one link take the space between", 40, 120, nil, []span{{"see ", false}, {"docs now", true}}},
		{"turned: the run's own centre", 40, 80, func(r *textRun) { r.rotated = true }, []span{{"see docs now", true}}},
		{"vertical: the run's own centre", 40, 80, func(r *textRun) { r.vertical = true }, []span{{"see docs now", true}}},
		{"reversed: the run's own centre", 40, 80, func(r *textRun) { r.reversed = true }, []span{{"see docs now", true}}},
		{"glyphs that are not the span's text: the run's own centre", 40, 80, func(r *textRun) { r.glyphs = r.glyphs[:len(r.glyphs)-1] }, []span{{"see docs now", true}}},
	}
	for _, c := range cases {
		links := PageLinkSet{Links: []PageLink{{Rect: MapRect{c.x0 / 200, 0, c.x1 / 200, 1}, URI: uri}}}
		r := spacedRun("see docs now", 0)
		if c.mod != nil {
			c.mod(&r)
		}
		got := lineSpans(sp, links, textLine{runs: []textRun{r}, starts: []int{0}, text: r.text})
		var have []span
		for _, g := range got {
			have = append(have, span{g.Text, g.Link == uri})
		}
		if len(have) != len(c.spans) {
			t.Errorf("%s: spans %+v, want %+v", c.name, have, c.spans)
			continue
		}
		for i := range have {
			if have[i] != c.spans[i] {
				t.Errorf("%s: spans %+v, want %+v", c.name, have, c.spans)
				break
			}
		}
	}
}

// A run drawn filled-and-stroked (Tr 2) is bold in the export as it is in the page map (/pending 941 #10).
func TestAFauxBoldRunIsBoldInTheExport(t *testing.T) {
	r := textRun{text: "heavy", x: 0, y: 100, size: 10, width: 50}
	r.state.tr = 2
	ln := textLine{runs: []textRun{r}, starts: []int{0}, text: r.text}
	got := lineSpans(displaySpace{box: [4]float64{0, 0, 200, 200}, w: 200, h: 200}, PageLinkSet{}, ln)
	if len(got) != 1 || !got[0].Bold {
		t.Fatalf("spans = %+v, want one bold span", got)
	}
	if !r.drawsBold() {
		t.Fatal("drawsBold")
	}
	r.state.tr = 0
	if r.drawsBold() {
		t.Fatal("a plain run is not bold")
	}
	r.bold = true
	if !r.drawsBold() {
		t.Fatal("a bold font is bold at Tr 0")
	}
}

// A cancelled pass stops between pictures and says so, instead of finishing the page and leaving the caller to find the
// context's error afterwards (/pending 946 I10). The control: the same document, not cancelled, hands its picture over.
func TestAnImagePassObservesItsContextBetweenPictures(t *testing.T) {
	var pdf []byte
	for _, f := range testpdf.ExportPictures() {
		if f.Name == "picture-twice" {
			pdf = f.PDF
		}
	}
	seen := 0
	if _, err := pdfread.ExtractImagesEach(context.Background(), pdf, nil, 1<<40, func(model.Image) error { seen++; return nil }); err != nil || seen == 0 {
		t.Fatalf("control: seen %d err %v", seen, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	seen = 0
	if _, err := pdfread.ExtractImagesEach(ctx, pdf, nil, 1<<40, func(model.Image) error { seen++; return nil }); !errors.Is(err, context.Canceled) || seen != 0 {
		t.Fatalf("a cancelled pass: seen %d err %v, want 0 pictures and context.Canceled", seen, err)
	}
}
