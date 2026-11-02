package cli

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
)

// `nib import` is the second door to the native producers (P01.S03a): the same function, so the same pages.
func TestNibImportConvertsAGIFAndACBZWithNoOutsideTool(t *testing.T) {
	dir := t.TempDir()
	var g bytes.Buffer
	if err := gif.EncodeAll(&g, &gif.GIF{
		Image: []*image.Paletted{image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.White, color.Black}), image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.White, color.Black})},
		Delay: []int{0, 0},
	}); err != nil {
		t.Fatal(err)
	}
	gifPath := filepath.Join(dir, "a.gif")
	os.WriteFile(gifPath, g.Bytes(), 0o600)

	var p bytes.Buffer
	png.Encode(&p, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	var z bytes.Buffer
	zw := zip.NewWriter(&z)
	for _, n := range []string{"p1.png", "p2.png", "p3.png"} {
		w, _ := zw.Create(n)
		w.Write(p.Bytes())
	}
	zw.Close()
	cbzPath := filepath.Join(dir, "b.cbz")
	os.WriteFile(cbzPath, z.Bytes(), 0o600)

	for path, want := range map[string]int{gifPath: 2, cbzPath: 3} {
		out, stderr, code := fillChild(t, "", "", "import", path, "-o", "-")
		if code != 0 {
			t.Fatalf("import %s exited %d: %s", filepath.Base(path), code, stderr)
		}
		if n, err := pdfops.PageCount(out); err != nil || n != want {
			t.Errorf("import %s: %d pages (%v), want %d", filepath.Base(path), n, err, want)
		}
	}
	// A refusal exits non-zero and names its cause on stderr.
	bad := filepath.Join(dir, "c.cbz")
	os.WriteFile(bad, []byte("PK\x03\x04 not an archive"), 0o600)
	if _, stderr, code := fillChild(t, "", "", "import", bad, "-o", filepath.Join(dir, "x.pdf")); code == 0 || stderr == "" {
		t.Errorf("a damaged CBZ: code %d stderr %q", code, stderr)
	}
}
