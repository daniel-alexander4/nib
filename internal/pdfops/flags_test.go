package pdfops

import (
	"bytes"
	"testing"

	"nib/internal/testpdf"
)

// TestFlagsRoundTrip covers the whole transit cycle: embed the placeholders,
// confirm they read back identically, confirm they survive a watermark bake
// (the recipient fills + bakes before the strip runs), then confirm the strip
// removes them so the finished document doesn't re-trigger the signing banner.
func TestFlagsRoundTrip(t *testing.T) {
	base, err := testpdf.Text("flag me")
	if err != nil {
		t.Fatal(err)
	}
	flags := []byte(`[{"page":1,"frac":[0.1,0.2,0.32,0.25],"type":"sign"},{"page":1,"frac":[0.5,0.6,0.63,0.635],"type":"date"}]`)

	withFlags, err := SetFlags(base, flags)
	if err != nil {
		t.Fatalf("SetFlags: %v", err)
	}
	if got, err := FlagsJSON(withFlags); err != nil || !bytes.Equal(got, flags) {
		t.Fatalf("FlagsJSON after embed = %q, %v; want %q", got, err, flags)
	}

	// A bake (image stamp) must NOT drop the property — that's why the strip has
	// to be explicit.
	baked, err := StampImages(withFlags, []Stamp{{Page: 1, Rect: [4]float64{100, 100, 200, 140}, PNG: pngBytes(t, 40, 20)}})
	if err != nil {
		t.Fatalf("StampImages: %v", err)
	}
	if got, err := FlagsJSON(baked); err != nil || !bytes.Equal(got, flags) {
		t.Fatalf("flags did not survive bake: got %q, %v", got, err)
	}

	cleared, err := ClearFlags(baked)
	if err != nil {
		t.Fatalf("ClearFlags: %v", err)
	}
	if got, err := FlagsJSON(cleared); err != nil || got != nil {
		t.Fatalf("FlagsJSON after clear = %q, %v; want nil", got, err)
	}
}

// TestFlagsAbsent: a plain PDF reads as no flags, and clearing one is a no-op
// rather than an error.
func TestFlagsAbsent(t *testing.T) {
	base, err := testpdf.Text("no flags here")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := FlagsJSON(base); err != nil || got != nil {
		t.Fatalf("FlagsJSON on plain PDF = %q, %v; want nil", got, err)
	}
	if _, err := ClearFlags(base); err != nil {
		t.Fatalf("ClearFlags on plain PDF: %v", err)
	}
}
