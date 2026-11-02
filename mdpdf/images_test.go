package mdpdf

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeImages(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	im := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			im.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var p, j bytes.Buffer
	png.Encode(&p, im)
	jpeg.Encode(&j, im, nil)
	os.WriteFile(filepath.Join(dir, "logo.png"), p.Bytes(), 0o600)
	os.WriteFile(filepath.Join(dir, "photo.jpg"), j.Bytes(), 0o600)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a picture"), 0o600)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "deep.png"), p.Bytes(), 0o600)
	return dir
}

// TestAnImageIsEmbeddedScaledAndNamedForTheTagger — I3. A local picture is placed after its paragraph, never wider than the
// page, with its alternative text kept for a Figure; without ImageDir nothing changes.
func TestAnImageIsEmbeddedScaledAndNamedForTheTagger(t *testing.T) {
	dir := writeImages(t)
	md := "Before.\n\n![The firm's logo](logo.png)\n\nAfter, with ![](photo.jpg) a decorative one.\n"
	pdf, l, err := convert([]byte(md), nil, nil, Options{ImageDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.figs[0]) != 2 {
		t.Fatalf("%d figures on page 1, want 2", len(l.figs[0]))
	}
	f := l.figs[0][0]
	if f.alt != "The firm's logo" || f.w > l.g.contentW()+0.01 || f.w < 100 || abs(f.w/f.h-2) > 0.01 {
		t.Errorf("figure %+v: want the alt text, a 2:1 shape and a width within the content (%v)", f, l.g.contentW())
	}
	if l.figs[0][1].alt != "" {
		t.Errorf("an image with no alt text should be decorative, got %q", l.figs[0][1].alt)
	}
	if f.before >= l.figs[0][1].before {
		t.Errorf("reading order: the first figure is after %d runs and the second after %d", f.before, l.figs[0][1].before)
	}
	if !bytes.Contains(pdf, []byte("/Image")) && !bytes.Contains(pdf, []byte("Im0")) {
		t.Error("no image object in the output")
	}
	_, st, _ := ConvertStructuredWithOptions([]byte(md), nil, nil, Options{ImageDir: dir})
	if len(st.Figures) != 1 || len(st.Figures[0]) != 2 || st.Figures[0][0].Alt != "The firm's logo" {
		t.Errorf("structure figures %+v", st.Figures)
	}
	// Without ImageDir the picture is skipped, as it always was.
	_, l2, err := convert([]byte(md), nil, nil, Options{})
	if err != nil || len(l2.figs[0]) != 0 {
		t.Errorf("no ImageDir: %d figures, %v", len(l2.figs[0]), err)
	}
	// A big picture is scaled down, never up.
	big := image.NewRGBA(image.Rect(0, 0, 3000, 2000))
	var b bytes.Buffer
	png.Encode(&b, big)
	os.WriteFile(filepath.Join(dir, "big.png"), b.Bytes(), 0o600)
	_, l3, err := convert([]byte("![big](big.png)\n"), nil, nil, Options{ImageDir: dir})
	if err != nil || l3.figs[0][0].w > l3.g.contentW()+0.01 || l3.figs[0][0].h > (l3.g.top()-l3.g.my) {
		t.Errorf("big image %+v %v", l3.figs[0], err)
	}
	_, l4, _ := convert([]byte("![small](logo.png)\n"), nil, nil, Options{ImageDir: dir})
	if got := l4.figs[0][0].w; abs(got-150) > 0.01 {
		t.Errorf("a 200px image at 96 dpi is 150pt wide, got %v", got)
	}
}

// TestAnImageSourceOutsideTheFolderIsRefusedByName — the path door: nothing a template names is read unless it is a
// relative file under the image folder.
func TestAnImageSourceOutsideTheFolderIsRefusedByName(t *testing.T) {
	dir := writeImages(t)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.png"), []byte("x"), 0o600)
	os.Symlink(outside, filepath.Join(dir, "link"))
	os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(dir, "evil.png"))
	for name, c := range map[string]struct{ src, why string }{
		"url":       {"https://example.com/a.png", "link"},
		"scheme":    {"data:image/png;base64,AAAA", "link"},
		"file":      {"file:///etc/passwd", "link"},
		"protocol":  {"//host/a.png", "link"},
		"absolute":  {filepath.Join(outside, "secret.png"), "absolute"},
		"dotdot":    {"../" + filepath.Base(outside) + "/secret.png", "climbs"},
		"dirlink":   {"link/secret.png", "outside the image folder"},
		"filelink":  {"evil.png", "outside the image folder"},
		"missing":   {"nope.png", "no such file"},
		"notimage":  {"notes.txt", "not a PNG or JPEG"},
		"directory": {"sub", "not a regular file"},
	} {
		_, _, err := convertOpts(t, "![x]("+c.src+")\n", Options{ImageDir: dir})
		var refused *ImageRefusedError
		if !errors.As(err, &refused) || !strings.Contains(err.Error(), c.why) {
			t.Errorf("%s: %q gave %v, want a refusal containing %q", name, c.src, err, c.why)
		}
	}
	// A file in a sub-folder is fine.
	if _, _, err := convertOpts(t, "![deep](sub/deep.png)\n", Options{ImageDir: dir}); err != nil {
		t.Errorf("sub/deep.png refused: %v", err)
	}
	// Too many images in one document.
	var b strings.Builder
	for i := 0; i <= maxImages; i++ {
		b.WriteString("![a](logo.png)\n\n")
	}
	if _, _, err := convertOpts(t, b.String(), Options{ImageDir: dir}); !errors.Is(err, ErrTooManyImages) {
		t.Errorf("%d images: %v, want ErrTooManyImages", maxImages+1, err)
	}
}

func TestAnImageAltTextTakesAPlaceholder(t *testing.T) {
	dir := writeImages(t)
	_, l, err := convert([]byte("![Logo of {{firm}}](logo.png)\n"), nil, nil, Options{ImageDir: dir, Vars: map[string]string{"firm": "Acme"}})
	if err != nil || l.figs[0][0].alt != "Logo of Acme" {
		t.Errorf("alt %q, %v", l.figs[0][0].alt, err)
	}
}
