package pdfops

import (
	"testing"

	"nib/internal/testpdf"
)

// TestTheFormDoorsKeepTheCatalogMetadata — `PLAN-ua-coverage.md` P01.S04, PDF/UA 7.1 t8. Placing form
// fields is not a reason to lose the document's metadata, and the packet kept must not keep a PDF/UA
// identification nib did not verify (ADR-032).
func TestTheFormDoorsKeepTheCatalogMetadata(t *testing.T) {
	base, err := LabelUA(labelReady(t, censusMarkdown()), true) // the census's own base
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	packet := catalogPacket(t, base)
	// Stimulus before response: the input has a titled packet, and it carries the identification to drop.
	if packet == "" || !packetHasTitle(packet) {
		t.Fatal("setup: the base document has no titled catalog /Metadata, so keeping it is vacuous")
	}
	if !testpdf.PacketClaimsUA(packet) {
		t.Fatal("setup: the base packet carries no PDF/UA identification, so dropping one is vacuous")
	}
	fields := []FormField{{Page: 1, Rect: [4]float64{10, 10, 200, 40}, Kind: "text", Name: "f1", Label: "Your full name"}}
	tagged, _, err := AuthorTaggedForm(base, fields)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := AuthorForm(base, fields)
	if err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string][]byte{"AuthorTaggedForm": tagged, "AuthorForm": plain} {
		kept := catalogPacket(t, out)
		if kept == "" {
			t.Errorf("%s dropped the catalog /Metadata (PDF/UA 7.1 t8)", name)
			continue
		}
		if !packetHasTitle(kept) {
			t.Errorf("%s kept a /Metadata packet without the document's title", name)
		}
		if testpdf.PacketClaimsUA(kept) {
			t.Errorf("%s kept the PDF/UA identification through a change (ADR-032)", name)
		}
	}
}
