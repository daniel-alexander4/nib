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

// TestTheAnswerFactsReadALabelAndATaggingClaim — ADR-147: the two facts `ReadAnswerFacts` added to the read
// the server already makes for every document. The label is asked through the drop's own door, so it is true
// of exactly the documents the next change would take a label from; a signed copy says "kept".
func TestTheAnswerFactsReadALabelAndATaggingClaim(t *testing.T) {
	plain, err := testpdf.Text("no label, no tags")
	if err != nil {
		t.Fatal(err)
	}
	if plain, err = SetTitle(plain, "No label, no tags"); err != nil {
		t.Fatal(err)
	}
	if f, err := ReadAnswerFacts(plain); err != nil || f.Labelled || f.Tagged {
		t.Fatalf("a plain document reads labelled=%v tagged=%v (err %v)", f.Labelled, f.Tagged, err)
	}
	if note := SigningLabelNote(plain, false); note != "" {
		t.Errorf("a document with no label is given a note about one: %q", note)
	}
	labelled, err := testpdf.WithUAIdentification(plain)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := ReadAnswerFacts(labelled); err != nil || !f.Labelled {
		t.Fatalf("a document carrying pdfuaid:part reads labelled=%v (err %v)", f.Labelled, err)
	}
	pdfa, err := testpdf.WithPDFAIdentification(plain)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := ReadAnswerFacts(pdfa); err != nil || !f.Labelled {
		t.Fatalf("a document carrying pdfaid:part reads labelled=%v (err %v) — the drop takes that label too", f.Labelled, err)
	}
	if got := SigningLabelNote(labelled, false); got != LabelRemovedBySigning {
		t.Errorf("unsigned and labelled: %q", got)
	}
	if got := SigningLabelNote(labelled, true); got != LabelKeptBySigning {
		t.Errorf("signed and labelled: %q", got)
	}
	// The asking must not have changed what is asked: the same bytes still carry the label.
	if f, _ := ReadAnswerFacts(labelled); !f.Labelled {
		t.Error("asking whether a document is labelled took the label from the caller's bytes")
	}
	tagged, err := taggedTwoPageFixture(), error(nil)
	if err != nil {
		t.Fatal(err)
	}
	f, err := ReadAnswerFacts(tagged)
	if err != nil || !f.Tagged {
		t.Fatalf("a document that claims tagging reads tagged=%v (err %v)", f.Tagged, err)
	}
	if f.Tagged != Inspect(tagged).Tagged {
		t.Error("the answer's reading of the tagging claim and the commit door's disagree")
	}
}

// TestInspectNamesEveryTerminalField — `/pending 582`: the commit door compares form fields by fully
// qualified name. A node that only groups other fields is not a field; its named kids are.
func TestInspectNamesEveryTerminalField(t *testing.T) {
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if got := Inspect(form).FieldNames; len(got) != 2 || got[0] != "fullName" || got[1] != "agree" {
		t.Fatalf("the form fixture's fields read %q, want fullName and agree", got)
	}
	doubled, err := Append(form, form)
	if err != nil {
		t.Fatal(err)
	}
	got := Inspect(doubled).FieldNames
	want := []string{"fullName", "agree", "2.fullName", "2.agree"}
	if len(got) != len(want) {
		t.Fatalf("a form appended to itself reads %q, want %q — the incoming fields are under one parent, and the parent is not a field", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("a form appended to itself reads %q, want %q", got, want)
		}
	}
}
