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
	titled, err := SetTitle(taggedFixture(), "Census")
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
	src := labelledFixture(t)
	if !claimsUA(t, catalogPacket(t, src)) {
		t.Fatal("setup: the labelled fixture does not claim PDF/UA, so every row below would pass on a build that keeps the claim")
	}
	driven := 0
	var kept []string
	names := make([]string, 0, len(tagFates))
	for n := range tagFates {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		f := tagFates[name]
		if f.drive == nil {
			continue
		}
		// The one exemption, by name: `LabelUA` is the door that WRITES the identification, on nib's own
		// Markdown conversion, with veraPDF's measurement behind it (ADR-033). Every other row must drop it.
		if name == "LabelUA" {
			continue
		}
		out, err := f.drive(src)
		if err != nil {
			t.Logf("%s: not exercised on this fixture (%v)", name, err)
			continue
		}
		driven++
		// An operation with nothing to do returns the document's OWN bytes (`ClearFlags` on an unflagged
		// file, `DeclareAuthoredProseLang` with nothing to bracket). Nothing changed, so the claim stands.
		if bytes.Equal(out, src) {
			continue
		}
		if claimsUA(t, catalogPacket(t, out)) {
			kept = append(kept, name)
		}
	}
	if driven < 20 {
		t.Errorf("only %d operation(s) were driven; the census is reporting coverage it barely has", driven)
	}
	if len(kept) > 0 {
		t.Errorf("%d operation(s) change a labelled document and keep its PDF/UA identification: %s.\n"+
			"nib cannot verify an edit kept conformance (22 of 106 rules), so any change drops the claim — "+
			"route the operation through writeMutated or rewriteWithConf, or drop it after its own write "+
			"(withoutUAClaim).", len(kept), strings.Join(kept, ", "))
	}
}
