package pdfops

import (
	"bytes"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestAnUnparsableClaimOverADanglingReferenceDoesNotRefuseTheEdit — `/pending 739`. A packet that names the
// schema and is not well-formed loses the whole `/Metadata` key; pdfcpu's `DeleteDictEntry` refused when the
// stream's dictionary held a reference with no xref entry, and the refusal failed the operation itself.
func TestAnUnparsableClaimOverADanglingReferenceDoesNotRefuseTheEdit(t *testing.T) {
	ctx, err := pdfread.ReadOptimized(labelledFixture(t), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	ir, ok := root["Metadata"].(types.IndirectRef)
	if !ok {
		t.Fatal("setup: the labelled fixture's /Metadata is not indirect")
	}
	sd, _, err := ctx.XRefTable.DereferenceStreamDict(ir)
	if err != nil || sd == nil {
		t.Fatalf("setup: %v", err)
	}
	sd.Content = []byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/" xmlns:pdfuaid="` + pdfuaidNS + `"><pdfuaid:part>1</x:xmpmeta>`)
	if err := sd.Encode(); err != nil {
		t.Fatal(err)
	}
	sd.Dict.Insert("Foo", *types.NewIndirectRef(9999, 0)) // no xref entry: the dangling reference
	entry, _ := ctx.XRefTable.FindTableEntryForIndRef(&ir)
	entry.Object = *sd
	var b bytes.Buffer
	if err := api.WriteContext(ctx, &b); err != nil {
		t.Fatal(err)
	}
	src := b.Bytes()
	if !claimsUA(t, catalogPacket(t, src)) {
		t.Fatal("setup: the broken packet does not read as a claim, so the delete branch is not reached")
	}

	out, err := Rotate(src, nil, 90)
	if err != nil {
		t.Fatalf("rotating a document whose unparsable claim sits over a dangling reference refused: %v", err)
	}
	if p := catalogPacket(t, out); p != "" {
		t.Errorf("the unparsable claim survived the edit: %q", p)
	}
	saved, err := DropUAIdentificationUnlessSigned(src, false)
	if err != nil {
		t.Fatalf("the save/sign door refused the same document: %v", err)
	}
	if catalogPacket(t, saved) != "" {
		t.Error("the save/sign door kept the unparsable claim")
	}
}

// TestAChangeDropsAPDFAIdentification — ADR-083. nib's own PDF/A candidate keeps its claim as written, and
// the first change nib makes to it drops the claim, keeping the rest of the packet.
func TestAChangeDropsAPDFAIdentification(t *testing.T) {
	img, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	titled, err := SetTitle(img, "Archive")
	if err != nil {
		t.Fatal(err)
	}
	labelled, err := testpdf.WithUAIdentification(titled)
	if err != nil {
		t.Fatal(err)
	}
	a, blockers, err := PreparePDFA(labelled)
	if err != nil || len(blockers) > 0 {
		t.Fatalf("setup: PreparePDFA refused an image document: %v %v", blockers, err)
	}
	p := catalogPacket(t, a)
	if !testpdf.PacketClaimsPDFA(p) {
		t.Fatal("setup: PreparePDFA's output does not claim PDF/A — the writing door lost its own claim")
	}
	if claimsUA(t, p) {
		t.Error("PreparePDFA carried the input's PDF/UA identification through")
	}
	rotated, err := Rotate(a, nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	p = catalogPacket(t, rotated)
	if testpdf.PacketClaimsPDFA(p) {
		t.Errorf("rotating a PDF/A candidate kept its PDF/A identification:\n%s", p)
	}
	if !bytes.Contains([]byte(p), []byte("Archive")) {
		t.Errorf("dropping the claim took the title with it:\n%s", p)
	}
}

// TestRedactingPageOneKeepsTheLanguage — `/pending 642`. The merge keeps the first segment's catalog, and with
// page 1 redacted that segment is a raster with none; page 2's redaction kept the language all along.
func TestRedactingPageOneKeepsTheLanguage(t *testing.T) {
	two, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200), rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	two, err = SetLang(two, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range []int{1, 2} {
		out, err := RedactPages(two, map[int]RasterPage{page: rasterPage(t, 200, 200)})
		if err != nil {
			t.Fatal(err)
		}
		ctx, err := inspectionRead(out)
		if err != nil {
			t.Fatal(err)
		}
		root, err := ctx.XRefTable.Catalog()
		if err != nil {
			t.Fatal(err)
		}
		if got := readLang(ctx.XRefTable, root["Lang"]); got != "de-DE" {
			t.Errorf("redacting page %d: /Lang = %q, want de-DE (7.2 t34)", page, got)
		}
		if ctx.PageCount != 2 {
			t.Errorf("redacting page %d: %d pages, want 2", page, ctx.PageCount)
		}
	}
}
