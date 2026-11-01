package cli

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
)

// `nib shrink` — /pending 908. Level and target both reach the shrinker, the numbers are said on stderr, a target
// that cannot be met is said, and a size is read in the units people write.
func TestShrinkMakesThePicturesSmallerAndSaysSo(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	img := image.NewRGBA(image.Rect(0, 0, 1800, 1200))
	for y := 0; y < 1200; y++ {
		for x := 0; x < 1800; x++ {
			img.Set(x, y, color.RGBA{uint8(x*255/1800 + r.Intn(30)), uint8(y*255/1200 + r.Intn(30)), uint8(90 + r.Intn(60)), 255})
		}
	}
	var jb bytes.Buffer
	if err := jpeg.Encode(&jb, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	doc, err := pdfops.ImagesToPDF([]pdfops.RasterPage{{Image: jb.Bytes(), W: 450, H: 300}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, doc, 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "out.pdf")
	var code int
	said := captureStderr(t, func() { code = cmdShrink([]string{in, "-o", out, "--level", "3"}) })
	if code != 0 {
		t.Fatalf("shrink exit = %d: %s", code, said)
	}
	got := readPDF(t, out)
	if len(got)*2 > len(doc) {
		t.Errorf("--level 3: %d bytes of %d, want under half", len(got), len(doc))
	}
	if !strings.Contains(said, "1 of 1 shrunk") {
		t.Errorf("stderr %q does not say the pictures were shrunk", said)
	}

	// --target: met, then not met.
	said = captureStderr(t, func() { code = cmdShrink([]string{in, "-o", out, "--target", "2MB"}) })
	if code != 0 || strings.Contains(said, "could not be met") {
		t.Errorf("a target the document meets: exit %d, stderr %q", code, said)
	}
	said = captureStderr(t, func() { code = cmdShrink([]string{in, "-o", out, "--target", "1KB"}) })
	if code != 0 || !strings.Contains(said, "could not be met") {
		t.Errorf("an impossible target: exit %d, stderr %q, want it said", code, said)
	}

	for _, bad := range []string{"big", "0", "-3MB", "MB"} {
		if c := cmdShrink([]string{in, "-o", out, "--target", bad}); c == 0 {
			t.Errorf("--target %q was accepted", bad)
		}
	}
	if c := cmdShrink([]string{in, "-o", out, "--level", "99"}); c == 0 {
		t.Error("--level 99 was accepted")
	}
}

func TestParseSizeReadsTheUnitsPeopleWrite(t *testing.T) {
	for in, want := range map[string]int{"1048576": 1 << 20, "500KB": 500 << 10, "1.5mb": 3 << 19, "2 MB": 2 << 20, "100B": 100} {
		if got, err := parseSize(in); err != nil || got != want {
			t.Errorf("parseSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
}
