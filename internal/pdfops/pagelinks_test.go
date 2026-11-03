package pdfops

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"encoding/json"
	"nib/internal/pdfread"
	"reflect"
	"strings"
	"testing"
)

// PLAN-import-export P02.S03a — font style and links, the document model's first two missing inputs.

func linkPage(annots ...string) []byte {
	var refs []string
	for i := range annots {
		refs = append(refs, itoa(6+i)+" 0 R")
	}
	return pageWith("BT /F1 10 Tf 100 700 Td (see here) Tj ET", "", "", "/Annots ["+strings.Join(refs, " ")+"]", helvetica, annots...)
}

func uriLink(rect, uri string) string {
	return "<< /Type /Annot /Subtype /Link /F 4 /Rect [" + rect + "] /A << /S /URI /URI (" + uri + ") >> >>"
}

func TestOnlyHttpHttpsAndMailtoLinksAreCarriedAndTheRestCounted(t *testing.T) {
	pdf := linkPage(
		uriLink("100 698 140 710", "https://example.org/a"),
		uriLink("100 598 140 610", "javascript:alert(1)"),
		uriLink("100 498 140 510", "file:///etc/passwd"),
		uriLink("100 398 140 410", "mailto:a@b.example"),
		"<< /Type /Annot /Subtype /Link /F 4 /Rect [100 298 140 310] /Dest [3 0 R /Fit] >>",
		"<< /Type /Annot /Subtype /Link /F 4 /Rect [100 198 140 210] /A << /S /GoToR /F (other.pdf) /D [0 /Fit] >> >>",
		"<< /Type /Annot /Subtype /Link /F 4 /Rect [100 98 140 110] /A << /S /Launch /F (calc.exe) >> >>",
		"<< /Type /Annot /Subtype /Link /F 32 /Rect [10 50 40 60] /A << /S /URI /URI (https://noview.example) >> >>",
		"<< /Type /Annot /Subtype /Link /F 4 /Rect [100 148 140 160] /A << /S /GoTo /D [3 0 R /Fit] >> >>",
		"<< /Type /Annot /Subtype /Link /F 2 /Rect [10 10 40 20] /A << /S /URI /URI (https://hidden.example) >> >>",
		"<< /Type /Annot /Subtype /Link /Rect [10 30 40 40] /A << /S /URI /URI (https://noflags.example) >> >>", // no /F: shown, a link
	)
	got, err := PageLinks(pdf, 1)
	if err != nil {
		t.Fatal(err)
	}
	var uris []string
	for _, l := range got.Links {
		uris = append(uris, l.URI)
	}
	if want := []string{"https://example.org/a", "mailto:a@b.example", "https://noflags.example"}; !reflect.DeepEqual(uris, want) {
		t.Fatalf("carried %v, want %v", uris, want)
	}
	want := map[string]int{DroppedScheme: 2, DroppedInternal: 2, DroppedRemote: 2}
	if !reflect.DeepEqual(got.Dropped, want) {
		t.Fatalf("dropped %v, want %v", got.Dropped, want)
	}
}

func TestALinkIsBoxedAsTheMapBoxesAndTakenByTheRunsCentre(t *testing.T) {
	pdf := linkPage(uriLink("100 698 140 710", "https://example.org/a"))
	l, err := PageLinks(pdf, 1)
	if err != nil || len(l.Links) != 1 {
		t.Fatalf("links = %+v, err %v", l, err)
	}
	m, err := MapPage(pdf, 1)
	if err != nil || len(m.Text) != 1 {
		t.Fatalf("map = %+v, err %v", m, err)
	}
	r := m.Text[0].Rect
	if _, ok := l.At((r[0]+r[2])/2-0.001, (r[1]+r[3])/2); !ok {
		t.Errorf("the run's centre %v is not inside the link %v", r, l.Links[0].Rect)
	}
	if _, ok := l.At(0.9, 0.9); ok {
		t.Error("a point far from the link found one")
	}
	if !mapNear(l.Links[0].Rect[0], 100.0/612) || !mapNear(l.Links[0].Rect[1], 1-710.0/792) {
		t.Errorf("link box = %v, want left 100/612 top 1-710/792 in the displayed page's fractions", l.Links[0].Rect)
	}
}

func TestAWrappedLinkIsOneBoxPerQuadAndARotatedPageIsConvertedOnce(t *testing.T) {
	quad := "<< /Type /Annot /Subtype /Link /F 4 /Rect [100 680 300 710] /QuadPoints [100 698 300 698 100 710 300 710  100 680 200 680 100 692 200 692] /A << /S /URI /URI (https://example.org/w) >> >>"
	got, _ := PageLinks(linkPage(quad), 1)
	if len(got.Links) != 2 {
		t.Fatalf("a link over two lines gave %d boxes, want 2 (the union would hold the gap)", len(got.Links))
	}
	rot := pageWith("BT /F1 10 Tf 100 700 Td (x) Tj ET", "", "", "/Rotate 90 /Annots [6 0 R]", helvetica, uriLink("100 698 140 710", "https://example.org/r"))
	r, _ := PageLinks(rot, 1)
	m, _ := MapPage(rot, 1)
	if len(r.Links) != 1 || len(m.Text) != 1 {
		t.Fatalf("rotated: links %v, text %v", r.Links, m.Text)
	}
	c := m.Text[0].Rect
	if _, ok := r.At((c[0]+c[2])/2, (c[1]+c[3])/2); !ok {
		t.Errorf("on a page turned 90° the run's centre %v misses its link %v: conversion is not the map's", c, r.Links[0].Rect)
	}
}

func TestAControlCharacterOrALongURIIsNotCarried(t *testing.T) {
	long := "https://example.org/" + strings.Repeat("a", maxLinkURI)
	got, _ := PageLinks(linkPage(uriLink("100 698 140 710", long), uriLink("100 598 140 610", "https://a b.example")), 1)
	if len(got.Links) != 0 || got.Dropped[DroppedMalformed] != 2 {
		t.Fatalf("links %v dropped %v, want none carried and two malformed", got.Links, got.Dropped)
	}
}

func styleFont(base, desc string) map[string]string {
	w := strings.TrimSpace(strings.Repeat("500 ", 95))
	return map[string]string{"F1": "<< /Type /Font /Subtype /TrueType /BaseFont /" + base + " /FirstChar 32 /LastChar 126 /Widths [" + w +
		"] /Encoding /WinAnsiEncoding " + desc + " >>"}
}

func styleOf(t *testing.T, fonts map[string]string, extra ...string) MapText {
	t.Helper()
	m, err := MapPage(pageWith("BT /F1 10 Tf 100 700 Td (word) Tj ET", "", "", "", fonts, extra...), 1)
	if err != nil || len(m.Text) != 1 {
		t.Fatalf("map = %+v, err %v", m, err)
	}
	return m.Text[0]
}

func TestFontStyleComesFromTheDescriptorThenTheName(t *testing.T) {
	desc := func(extra string) string { return "/FontDescriptor 6 0 R" }
	fd := func(body string) string { return "<< /Type /FontDescriptor /FontName /X " + body + " >>" }
	cases := []struct {
		name         string
		font         string
		fd           string
		bold, italic bool
	}{
		{"weight 700", "Foo", "/Flags 32 /FontWeight 700", true, false},
		{"weight 400 beats a Bold name", "Foo-Bold", "/Flags 32 /FontWeight 400", false, false},
		{"force bold flag", "Foo", "/Flags 262176", true, false},
		{"italic flag", "Foo", "/Flags 96", false, true},
		{"italic angle", "Foo", "/Flags 32 /ItalicAngle -12", false, true},
		{"a weight the descriptor gave does not silence the name's italic", "Foo-Italic", "/Flags 32 /FontWeight 400", false, true},
		{"name only", "ABCDEF+Arial-BoldItalicMT", "", true, true},
		{"oblique", "Helvetica-Oblique", "", false, true},
		{"semibold is not bold", "Foo-Semibold", "", false, false},
		{"regular", "Times-Roman", "", false, false},
	}
	for _, c := range cases {
		fonts := styleFont(c.font, "")
		extra := []string(nil)
		if c.fd != "" {
			fonts = styleFont(c.font, desc(""))
			extra = []string{fd(c.fd)}
		}
		got := styleOf(t, fonts, extra...)
		if got.Bold != c.bold || got.Italic != c.italic {
			t.Errorf("%s: bold %v italic %v, want %v %v", c.name, got.Bold, got.Italic, c.bold, c.italic)
		}
	}
}

func TestAType0FontReadsItsDescendantsDescriptor(t *testing.T) {
	fonts := map[string]string{"F1": "<< /Type /Font /Subtype /Type0 /BaseFont /Foo /Encoding /Identity-H /DescendantFonts [6 0 R] >>"}
	pdf := pageWith("BT /F1 10 Tf 100 700 Td <0041> Tj ET", "", "", "", fonts,
		"<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Foo /FontDescriptor 7 0 R /DW 600 /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> >>",
		"<< /Type /FontDescriptor /FontName /Foo /Flags 32 /FontBBox [0 0 1000 1000] /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontWeight 800 /ItalicAngle -10 >>")
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, nil, 1)
	res := derefDict(ctx.XRefTable, pg.Dict["Resources"])
	f := derefDict(ctx.XRefTable, derefDict(ctx.XRefTable, res["Font"])["F1"])
	if bold, italic := fontStyle(ctx.XRefTable, f); !bold || !italic {
		t.Fatalf("bold %v italic %v, want both from the descendant's descriptor", bold, italic)
	}
}

func TestFakeBoldRenderModeTwoIsBold(t *testing.T) {
	m, _ := MapPage(pageWith("BT /F1 10 Tf 2 Tr 100 700 Td (word) Tj ET", "", "", "", helvetica), 1)
	if len(m.Text) != 1 || !m.Text[0].Bold {
		t.Fatalf("text = %+v, want bold", m.Text)
	}
}

// TestTheMapTextJSONNames pins the wire names of every MapText field: the fields the document model adds are
// omitempty, so a plain page's payload is byte-identical to the one before them (W13).
func TestTheMapTextJSONNames(t *testing.T) {
	rt := reflect.TypeOf(MapText{})
	var got []string
	for i := 0; i < rt.NumField(); i++ {
		got = append(got, rt.Field(i).Tag.Get("json"))
	}
	want := []string{"rect", "text", "cuts,omitempty", "chars,omitempty", "size", "hidden,omitempty", "ink,omitempty", "short,omitempty",
		"faint,omitempty", "reversed,omitempty", "bold,omitempty", "italic,omitempty"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MapText json names = %v, want %v", got, want)
	}
	m, _ := MapPage(pageWith("BT /F1 10 Tf 100 700 Td (word) Tj ET", "", "", "", helvetica), 1)
	b, _ := json.Marshal(m.Text[0])
	if strings.Contains(string(b), "bold") || strings.Contains(string(b), "italic") {
		t.Errorf("a regular run's JSON carries style: %s", b)
	}
}
