package pdfops

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"nib/internal/testpdf"
)

// TestPreparePDFA converts an image-only PDF (no fonts → nothing to embed) and
// confirms the output carries the sRGB OutputIntent and the pdfaid XMP, that the
// XMP stream is unfiltered (a PDF/A requirement), and that it still validates as a
// structurally sound PDF.
func TestPreparePDFA(t *testing.T) {
	src, err := ImagesToPDF([]RasterPage{rasterPage(t, 400, 600)})
	if err != nil {
		t.Fatal(err)
	}
	out, blockers, err := PreparePDFA(src)
	if err != nil {
		t.Fatalf("PreparePDFA: %v", err)
	}
	if len(blockers) != 0 {
		t.Fatalf("image-only PDF should convert, got blockers: %v", blockers)
	}

	// Structural sanity: still a valid PDF.
	if err := Validate(out); err != nil {
		t.Fatalf("converted PDF is invalid: %v", err)
	}

	// OutputIntents present in the catalog.
	ctx, err := api.ReadContext(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, _ := ctx.XRefTable.Catalog()
	if oi, _ := ctx.XRefTable.DereferenceArray(root["OutputIntents"]); len(oi) != 1 {
		t.Fatalf("OutputIntents = %v, want one entry", oi)
	}

	// XMP metadata stream present, unfiltered, carrying the PDF/A identifier.
	md, _, err := ctx.XRefTable.DereferenceStreamDict(root["Metadata"])
	if err != nil || md == nil {
		t.Fatalf("catalog /Metadata missing: %v", err)
	}
	if md.Dict["Filter"] != nil {
		t.Errorf("XMP metadata stream must be unfiltered, got Filter %v", md.Dict["Filter"])
	}
	if err := md.Decode(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pdfaid:part", "<pdfaid:part>2</pdfaid:part>", "pdfaid:conformance"} {
		if !bytes.Contains(md.Content, []byte(want)) {
			t.Errorf("XMP missing %q:\n%s", want, md.Content)
		}
	}

	// The embedded ICC profile is the real sRGB profile.
	if !bytes.Contains(out, []byte("GTS_PDFA1")) {
		t.Error("output missing GTS_PDFA1 OutputIntent subtype")
	}
}

// TestPreparePDFARefusesNonEmbeddedFonts: a document that references a font
// without embedding it (testpdf.Text uses the standard-14 Courier) cannot be made
// conformant, so PreparePDFA refuses it with a font blocker and no output.
func TestPreparePDFARefusesNonEmbeddedFonts(t *testing.T) {
	src, err := testpdf.Text("not embedded")
	if err != nil {
		t.Fatal(err)
	}
	out, blockers, err := PreparePDFA(src)
	if err != nil {
		t.Fatalf("PreparePDFA: %v", err)
	}
	if out != nil {
		t.Error("expected no output when fonts cannot be embedded")
	}
	if len(blockers) == 0 || !strings.Contains(strings.ToLower(blockers[0]), "embed") {
		t.Fatalf("expected a non-embedded-font blocker, got %v", blockers)
	}
}

// TestPreparePDFARefusesEncrypted: an encrypted document is refused with a clear
// blocker rather than a raw engine error.
func TestPreparePDFARefusesEncrypted(t *testing.T) {
	src, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt(src, "secret")
	if err != nil {
		t.Fatal(err)
	}
	out, blockers, err := PreparePDFA(enc)
	if err != nil {
		t.Fatalf("PreparePDFA on encrypted input should not hard-error, got %v", err)
	}
	if out != nil {
		t.Error("expected no output for an encrypted document")
	}
	if len(blockers) == 0 || !strings.Contains(strings.ToLower(blockers[0]), "password") {
		t.Fatalf("expected a password blocker, got %v", blockers)
	}
}
