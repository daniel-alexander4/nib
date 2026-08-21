package pdfops

import (
	"testing"

	"nib/internal/testpdf"
)

// This test lives in pdfops rather than in internal/server, where it was first written,
// because pdfcpu caches its config directory in a package-level variable on FIRST use, and
// internal/server's startServerWith sets HOME to a per-test t.TempDir (helpers_test.go:114).
// Whichever server test touches pdfcpu's font registry first therefore pins every later one
// to a directory that has since been deleted, and the font load fails with ENOENT. Nothing
// in internal/server rewrites that; the hazard is worth knowing about before adding another
// font-touching test there.
// TestOCRSkipsUnrepresentableWordsRatherThanFailingTheLayer pins the ONE producer that is
// deliberately not routed through wroteStampTextError, so that choice is a tested property
// rather than a comment. Changing it to an error is a behaviour change for every scan that
// contains a stray %, and this is where that shows up.
func TestOCRSkipsUnrepresentableWordsRatherThanFailingTheLayer(t *testing.T) {
	base, err0 := testpdf.Text("page one")
	if err0 != nil {
		t.Fatal(err0)
	}
	out, err := StampTextLayer(base, []Word{
		{Text: "100%% done", Page: 1, Rect: [4]float64{10, 10, 60, 20}},
		{Text: "readable", Page: 1, Rect: [4]float64{10, 40, 60, 50}},
	}, "eng")
	if err != nil {
		t.Fatalf("the layer failed over one unrepresentable word: %v\n"+
			"a scan the user cannot retype would lose its whole text layer to a stray %%", err)
	}
	if len(out) == 0 {
		t.Fatal("no output")
	}
	// And the stimulus: the same word DOES fail the primitive the user can retype into.
	if _, werr := StampWatermark(base, "100%% done", WatermarkStyle{}); werr == nil {
		t.Fatal("StampWatermark accepted the same text, so the contrast this test draws is imaginary")
	}
}
