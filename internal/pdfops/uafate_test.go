package pdfops

import (
	"bytes"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/testpdf"
)

// labelledFixture is the tag-fate census's tagged fixture, titled, carrying `pdfuaid:part 1` — written by
// the shared test helper, not through writeMutated, which now removes exactly this.
func labelledFixture(t *testing.T) []byte {
	t.Helper()
	return labelled(t, taggedFixture())
}

// labelled is base titled and carrying `pdfuaid:part 1`.
func labelled(t *testing.T, base []byte) []byte {
	t.Helper()
	titled, err := SetTitle(base, "Census")
	if err != nil {
		t.Fatal(err)
	}
	out, err := testpdf.WithUAIdentification(titled)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return out
}

// TestAFailedCorrectionStillDropsTheClaim — `/pending 503`. `StampImages` and the OCR text layer fall back
// to pdfcpu's own output when their correction fails, and that output carries the identification through.
func TestAFailedCorrectionStillDropsTheClaim(t *testing.T) {
	src := labelledFixture(t)
	if !claimsUA(t, catalogPacket(t, src)) {
		t.Fatal("setup: the labelled fixture does not claim PDF/UA")
	}
	out := rewriteOrDropClaim(src, func(*model.Context) error { return errors.New("the correction failed") })
	if bytes.Equal(out, src) {
		t.Fatal("a failed correction returned the labelled bytes unchanged, claim and all")
	}
	if claimsUA(t, catalogPacket(t, out)) {
		t.Error("a failed correction kept the document's PDF/UA identification")
	}
}

// TestNoOperationCarriesAnIdentificationItDidNotVerify — `/pending 492`, over the tag-fate census's whole
// driven population, so an operation added tomorrow is asked the moment it gets a census row.
//
// The claim is the element or attribute, never the namespace string: a PDF/A extension-schema block names
// the namespace as text while claiming nothing (measured on veraPDF's corpus).
func TestNoOperationCarriesAnIdentificationItDidNotVerify(t *testing.T) {
	// Both identifications at once (ADR-083): a PDF/UA file is often a PDF/A one too, and the two are
	// dropped by one door, so one drive asks both questions.
	src, err := testpdf.WithPDFAIdentification(labelledFixture(t))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if p := catalogPacket(t, src); !claimsUA(t, p) || !testpdf.PacketClaimsPDFA(p) {
		t.Fatal("setup: the labelled fixture does not claim both PDF/UA and PDF/A, so every row below would pass on a build that keeps a claim")
	}
	two, err := testpdf.WithPDFAIdentification(labelled(t, taggedTwoPageFixture()))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if p := catalogPacket(t, two); !claimsUA(t, p) || !testpdf.PacketClaimsPDFA(p) {
		t.Fatal("setup: the labelled two-page fixture does not claim both PDF/UA and PDF/A")
	}
	driven := 0
	var kept, keptA, unasked []string
	names := make([]string, 0, len(tagFates))
	for n := range tagFates {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		f := tagFates[name]
		drive := f.uaDrive
		if drive == nil {
			drive = f.drive
		}
		if drive == nil {
			// A row that returns no document has nothing to carry a claim; any other row is asked here,
			// or says why it is not. A silent `continue` here is how /pending 641 hid two operations.
			if f.verdict != "untouched" && strings.TrimSpace(f.uaWhy) == "" {
				unasked = append(unasked, name)
			}
			continue
		}
		// The writing doors, by name, each for its own claim only: `LabelUA` writes PDF/UA's on nib's own
		// Markdown conversion, with veraPDF's measurement behind it (ADR-033); `PreparePDFA` and
		// `ConvertPDFAGhostscript` write PDF/A's, as a candidate the user is told to verify (ADR-083).
		// Every other row must drop both.
		exemptUA := name == "LabelUA"
		exemptA := name == "PreparePDFA" || name == "ConvertPDFAGhostscript"
		in := src
		if f.twoPages {
			in = two
		}
		out, err := drive(in)
		if err != nil {
			t.Logf("%s: not exercised on this fixture (%v)", name, err)
			continue
		}
		driven++
		// An operation with nothing to do returns the document's OWN bytes (`ClearFlags` on an unflagged
		// file, `DeclareAuthoredProseLang` with nothing to bracket). Nothing changed, so the claim stands.
		if bytes.Equal(out, in) {
			continue
		}
		p := catalogPacket(t, out)
		if !exemptUA && claimsUA(t, p) {
			kept = append(kept, name)
		}
		if !exemptA && testpdf.PacketClaimsPDFA(p) {
			keptA = append(keptA, name)
		}
	}
	if len(unasked) > 0 {
		t.Errorf("%d operation(s) change a document and are asked by neither drive nor uaDrive, with no uaWhy: %s",
			len(unasked), strings.Join(unasked, ", "))
	}
	if len(keptA) > 0 {
		t.Errorf("%d operation(s) change a PDF/A-labelled document and keep its PDF/A identification: %s.\n"+
			"nib verifies none of PDF/A's rules, so any change drops the claim (ADR-083) — through the same door as PDF/UA's.",
			len(keptA), strings.Join(keptA, ", "))
	}
	if driven < 20 {
		t.Errorf("only %d operation(s) were driven; the census is reporting coverage it barely has", driven)
	}
	if len(kept) > 0 {
		t.Errorf("%d operation(s) change a labelled document and keep its PDF/UA identification: %s.\n"+
			"nib cannot verify an edit kept conformance (its checker covers only part of the 106 rules), so any change drops the claim — "+
			"route the operation through writeMutated or rewriteWithConf, or drop it after its own write "+
			"(withoutUAClaim).", len(kept), strings.Join(kept, ", "))
	}
}
