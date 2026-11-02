package pdfops

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/mdpdf"
)

func figureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	im := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			im.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, im)
	os.WriteFile(filepath.Join(dir, "logo.png"), b.Bytes(), 0o600)
	return dir
}

// TestAMarkdownImageIsTaggedAsAFigureWithItsDescription — I3. A picture with alt text is a Figure carrying /Alt; one with
// none is an artifact, never a Figure with no description (ADR-122); the text around it is still tagged and in order.
func TestAMarkdownImageIsTaggedAsAFigureWithItsDescription(t *testing.T) {
	dir := figureDir(t)
	md := "# Report\n\nIntro text.\n\n![The firm's logo](logo.png)\n\nMiddle text.\n\n![](logo.png)\n\nEnd.\n"
	out, err := ConvertDocToPDFWith(context.Background(), []byte(md), ".md", mdpdf.Options{ImageDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.ReadForInspection(out)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := readStructTree(ctx, livePageObjects(ctx))
	if err != nil {
		t.Fatalf("no structure tree: %v", err)
	}
	var figures []string
	var order []string
	for _, e := range tree.elems {
		switch e.kind {
		case "Figure":
			alt := textEntry(ctx.XRefTable, e.dict, "Alt")
			figures = append(figures, alt)
			order = append(order, "Figure")
		case "H1", "P":
			order = append(order, e.kind)
		}
	}
	if len(figures) != 1 || figures[0] != "The firm's logo" {
		t.Errorf("figures %q, want exactly one, described \"The firm's logo\" (the alt-less one is an artifact)", figures)
	}
	// Reading order: heading, intro, FIGURE, middle, end.
	if got := strings.Join(order, " "); got != "H1 P Figure P P" {
		t.Errorf("element order %q, want H1 P Figure P P", got)
	}
	// The document still reads, and has two pictures (one tagged, one artifact).
	files, err := ExtractImageFiles(out)
	if err != nil || len(files) < 1 {
		t.Errorf("pictures in the output: %d, %v", len(files), err)
	}
	// Without ImageDir the same document converts with no picture, as before.
	plain, err := ConvertDocToPDF(context.Background(), []byte(md), ".md")
	if err != nil {
		t.Fatal(err)
	}
	if files, _ := ExtractImageFiles(plain); len(files) != 0 {
		t.Errorf("a conversion without ImageDir embedded %d picture(s)", len(files))
	}
	// Options on a LibreOffice document, including ImageDir, are refused.
	if _, err := ConvertDocToPDFWith(context.Background(), []byte("x"), ".docx", mdpdf.Options{ImageDir: dir}); err == nil {
		t.Error("ImageDir on a .docx was accepted")
	}
}

// TestAMarkdownImageDocumentConformsToPDFUA — the claim the label makes, measured for a document with a Figure and an
// artifact image on every build that has veraPDF.
func TestAMarkdownImageDocumentConformsToPDFUA(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the conformance of a picture is UNMEASURED here")
	}
	dir := figureDir(t)
	pdf, err := ConvertDocToPDFWith(context.Background(), []byte("# Title\n\nSome text.\n\n![A described picture](logo.png)\n\n![](logo.png)\n\nMore text.\n"), ".md", mdpdf.Options{ImageDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if pdf, err = TitleFromName(pdf, "notes.md"); err != nil {
		t.Fatal(err)
	}
	if pdf, err = SetLang(pdf, "en"); err != nil {
		t.Fatal(err)
	}
	labelled, err := LabelUA(pdf, true)
	if err != nil {
		t.Fatalf("LabelUA refused: %v", err)
	}
	p := filepath.Join(t.TempDir(), "figures.pdf")
	os.WriteFile(p, labelled, 0o600)
	cl := ua1FailedClauses(t, vp, []string{p})
	if cl["figures.pdf"] == nil {
		t.Fatal("veraPDF could not validate the document")
	}
	if len(cl["figures.pdf"]) > 0 {
		t.Errorf("a Markdown document with pictures fails PDF/UA-1 on %v", sortedClauses(cl["figures.pdf"]))
	}
}
