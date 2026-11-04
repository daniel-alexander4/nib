package cli

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// TestMergeTakesAnImageAsThePageTheOpenerMakesOfIt — /pending 909. `nib merge` reads a PNG through
// `pdfops.ImageToDocument`, the door the window's opener uses, so a 192×96 picture is a 144×72 pt
// page (96 dpi) in the position it was named, beside a PDF page of the producer's own size. A
// non-image, non-PDF input is still refused.
func TestMergeTakesAnImageAsThePageTheOpenerMakesOfIt(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 192, 96))); err != nil {
		t.Fatal(err)
	}
	pic := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(pic, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := writePDF(t, dir, "a.pdf", "x")
	out := filepath.Join(dir, "out.pdf")

	if code := cmdMerge([]string{pic, doc, "-o", out}); code != 0 {
		t.Fatalf("merge exit = %d, want 0", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	dims, err := api.PageDims(context.Background(), bytes.NewReader(b), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if len(dims) != 2 || math.Round(dims[0].Width) != 144 || math.Round(dims[0].Height) != 72 {
		t.Fatalf("pages = %+v, want the picture first at 144×72 then the PDF's page", dims)
	}

	// ADR-185: merge decides through pdfops.Import as the window's Combine does, so a text or Markdown file is
	// converted to the page the opener would make of it (it used to be refused here while Combine took it) ...
	for _, name := range []string{"notes.txt", "readme.md"} {
		src := filepath.Join(dir, name)
		if err := os.WriteFile(src, []byte("plain words on a page"), 0o644); err != nil {
			t.Fatal(err)
		}
		merged := filepath.Join(dir, "with-"+name+".pdf")
		if code := cmdMerge([]string{doc, src, "-o", merged}); code != 0 {
			t.Fatalf("merge of a PDF and %s exit = %d, want 0", name, code)
		}
		mb, err := os.ReadFile(merged)
		if err != nil {
			t.Fatal(err)
		}
		if n, err := pdfops.PageCount(mb); err != nil || n != 2 {
			t.Errorf("%s merged into %d pages (%v), want 2: the PDF's and the converted file's", name, n, err)
		}
		pages, err := pdfops.DocumentText(mb)
		if err != nil || len(pages) != 2 || !strings.Contains(strings.Join(strings.Fields(runsText(pages[1])), ""), "plainwordsonapage") {
			t.Errorf("%s was merged but its words are not on page 2 (%v)", name, err)
		}
	}
	// ... and a file Nib has no route for is still refused, by Combine, as in the window.
	junk := filepath.Join(dir, "mystery.xyz")
	if err := os.WriteFile(junk, []byte("not a document"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdMerge([]string{pic, junk, "-o", filepath.Join(dir, "no.pdf")}); code == 0 {
		t.Error("a file Nib has no route for was merged; the opener refuses it and merge must too")
	}
}

func runsText(p pdfops.PageText) string {
	var b strings.Builder
	for _, r := range p.Runs {
		b.WriteString(r.Text + " ")
	}
	return b.String()
}

// A PDF is merged as it came and never offered to Import, whose size cap is for conversions: a PDF over MaxImportBytes
// merged before ADR-185 and must still (the review of /pending 940).
func TestMergeInputPassesAPDFOfAnySizeUntouched(t *testing.T) {
	b := make([]byte, pdfops.MaxImportBytes+1)
	copy(b, "%PDF-1.7\n")
	got, err := mergeInput(context.Background(), b, "big.pdf")
	if err != nil {
		t.Fatalf("a large PDF was refused: %v", err)
	}
	if len(got) != len(b) || &got[0] != &b[0] {
		t.Error("a PDF was not handed on as it came")
	}
}

// Anything else is the opener's page, and a file Nib does not recognise is left for Combine to name.
func TestMergeInputConvertsWhatIsNotAPDFAndLeavesWhatIsNotADocument(t *testing.T) {
	got, err := mergeInput(context.Background(), []byte("# Title\n\nbody\n"), "note.md")
	if err != nil || !pdfops.LooksLikePDF(got) {
		t.Fatalf("Markdown was not converted: %v", err)
	}
	junk := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	got, err = mergeInput(context.Background(), junk, "blob.bin")
	if err != nil || string(got) != string(junk) {
		t.Errorf("an unrecognised file was not left as it came: %v", err)
	}
}
