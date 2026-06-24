package pdfops

import (
	"bytes"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"nib/internal/testpdf"
)

// TestConvertPDFAGhostscript: the Ghostscript path converts a document the pure-Go
// PreparePDFA refuses — testpdf.Text uses the standard-14 Courier (not embedded) —
// into a PDF/A candidate that embeds the font, carries an sRGB OutputIntent and the
// pdfaid identifier, and validates. Skips when Ghostscript isn't installed (the
// poppler-oracle pattern), so the suite stays green on a gs-less machine.
func TestConvertPDFAGhostscript(t *testing.T) {
	if !GhostscriptAvailable() {
		t.Skip("Ghostscript not installed; skipping general PDF/A conversion")
	}
	src, err := testpdf.Text("Hello — non-embedded Courier")
	if err != nil {
		t.Fatal(err)
	}
	// Precondition: the pure-Go path refuses this (font not embedded).
	if _, blockers, _ := PreparePDFA(src); len(blockers) == 0 {
		t.Fatal("expected pure-Go PreparePDFA to refuse a non-embedded-font doc")
	}

	out, err := ConvertPDFAGhostscript(src)
	if err != nil {
		t.Fatalf("ConvertPDFAGhostscript: %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("Ghostscript PDF/A output invalid: %v", err)
	}
	if !bytes.Contains(out, []byte("pdfaid:part")) {
		t.Error("output missing the pdfaid identifier")
	}

	// Font now embedded, and an OutputIntent is present (gs needs the def.ps for it).
	info, err := api.PDFInfo(bytes.NewReader(out), "", nil, true, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	embedded := false
	for _, f := range info.Fonts {
		if f.Embedded {
			embedded = true
		}
	}
	if !embedded {
		t.Error("Ghostscript should have embedded the font")
	}
	ctx, err := api.ReadContext(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, _ := ctx.XRefTable.Catalog()
	if oi, _ := ctx.XRefTable.DereferenceArray(root["OutputIntents"]); len(oi) != 1 {
		t.Errorf("OutputIntents = %v, want one", oi)
	}
}
