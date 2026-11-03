package pdfops

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"testing"

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
