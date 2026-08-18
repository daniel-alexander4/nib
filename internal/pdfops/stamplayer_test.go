package pdfops_test

import (
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// The premise this whole file rests on is that pdfcpu tags its own stamps, and that is a
// fact about pdfcpu rather than about Nib — so it is asserted rather than assumed. If a
// pdfcpu bump ever renames the optional-content group, HasStampLayer goes quiet (always
// false) instead of wrong, and this is the test that says so out loud.
func TestHasStampLayerSeesPdfcpuOwnStamp(t *testing.T) {
	base, err := testpdf.Text("nothing stamped here")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}

	// The control, and it is the half that makes the other half mean anything: a detector
	// that answered true for everything would pass the stamped case on its own.
	if got, err := pdfops.HasStampLayer(base); err != nil || got {
		t.Fatalf("an unstamped document reports a stamp layer (got %v, err %v) — the detector answers true for anything", got, err)
	}

	for _, tc := range []struct {
		name  string
		stamp func([]byte) ([]byte, error)
	}{
		{"page numbers", func(b []byte) ([]byte, error) {
			return pdfops.StampPageNumbers(b, pdfops.PageNumberStyle{Position: "bc"})
		}},
		{"watermark", func(b []byte) ([]byte, error) {
			return pdfops.StampWatermark(b, "DRAFT", pdfops.WatermarkStyle{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.stamp(base)
			if err != nil {
				t.Fatalf("stamping: %v", err)
			}
			got, err := pdfops.HasStampLayer(out)
			if err != nil {
				t.Fatalf("HasStampLayer: %v", err)
			}
			if !got {
				t.Errorf("a document stamped with %s reports NO stamp layer. pdfcpu puts every watermark it writes into an OCG named \"Watermark\" or \"Background\" — if that stopped being true, this detector is silently answering false for every document and the double-stamp warning built on it never fires", tc.name)
			}
		})
	}
}

// The limitation, asserted rather than left in a comment. The warning built on this can
// only say "already stamped", never which kind, and a future reader who assumes otherwise
// should find out here rather than from a user reporting a wrong message.
func TestHasStampLayerCannotTellTheTwoKindsApart(t *testing.T) {
	base, err := testpdf.Text("one kind or the other")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	numbered, err := pdfops.StampPageNumbers(base, pdfops.PageNumberStyle{Position: "bc"})
	if err != nil {
		t.Fatalf("page numbers: %v", err)
	}
	marked, err := pdfops.StampWatermark(base, "DRAFT", pdfops.WatermarkStyle{})
	if err != nil {
		t.Fatalf("watermark: %v", err)
	}
	a, err1 := pdfops.HasStampLayer(numbered)
	b, err2 := pdfops.HasStampLayer(marked)
	if err1 != nil || err2 != nil {
		t.Fatalf("HasStampLayer: %v / %v", err1, err2)
	}
	if a != b {
		t.Fatalf("page numbers report %v and a watermark reports %v — they are distinguishable after all, and the API should say which kind instead of returning one bit", a, b)
	}
}
