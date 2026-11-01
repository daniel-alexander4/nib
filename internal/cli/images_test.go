package cli

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

func patterned() *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			im.Set(x, y, color.RGBA{uint8(x * 6), uint8(y * 8), uint8(x*y) ^ 0x5a, 255})
		}
	}
	return im
}

// pdfOfImage builds a one-page PDF from an image file through `nib merge`, the way the handoff states it.
func pdfOfImage(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	src, out := filepath.Join(dir, name), filepath.Join(dir, name+".pdf")
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}
	// merge wants two inputs: the picture first, then a page of text, so the picture is page 1.
	doc := writePDF(t, dir, name+".txt.pdf", "after the picture")
	if _, code := captureStdout(t, func() int { _, c := Run([]string{"merge", src, doc, "-o", out}, "t"); return c }); code != 0 {
		t.Fatalf("merge exited %d", code)
	}
	return out
}

func listImages(t *testing.T, args ...string) ([]map[string]any, int) {
	t.Helper()
	out, code := captureStdout(t, func() int { _, c := Run(args, "t"); return c })
	var rows []map[string]any
	if code == 0 {
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("not a JSON array: %v\n%s", err, out)
		}
	}
	return rows, code
}

// TestImagesGivesBackThePictureAPdfWasBuiltFrom — RelaxTax N5, which OCRs a scan whose every page is one
// embedded picture. A PNG comes back with the same pixels; a JPEG comes back as the same bytes.
func TestImagesGivesBackThePictureAPdfWasBuiltFrom(t *testing.T) {
	dir := t.TempDir()
	want := patterned()
	var pngBytes, jpgBytes bytes.Buffer
	if err := png.Encode(&pngBytes, want); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpgBytes, want, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}

	t.Run("png", func(t *testing.T) {
		pdf := pdfOfImage(t, dir, "scan.png", pngBytes.Bytes())
		out := filepath.Join(t.TempDir(), "pics")
		rows, code := listImages(t, "images", pdf, "--out-dir", out, "--json")
		if code != 0 || len(rows) != 1 {
			t.Fatalf("exit %d, %d rows, want 1", code, len(rows))
		}
		r := rows[0]
		pinKeys(t, "an images row", r, []string{"page", "index", "width", "height", "filter", "bytes", "file"}, nil)
		if r["page"] != 1.0 || r["index"] != 1.0 || r["width"] != 40.0 || r["height"] != 30.0 || r["filter"] != "FlateDecode" {
			t.Errorf("row = %v, want page 1 index 1, 40x30 and the stream's own FlateDecode", r)
		}
		b, err := os.ReadFile(filepath.Join(out, r["file"].(string)))
		if err != nil {
			t.Fatal(err)
		}
		if r["bytes"] != float64(len(b)) {
			t.Errorf("bytes = %v, file holds %d", r["bytes"], len(b))
		}
		got, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("the picture is not a PNG: %v (file %s)", err, r["file"])
		}
		for y := 0; y < 30; y++ {
			for x := 0; x < 40; x++ {
				gr, gg, gb, _ := got.At(x, y).RGBA()
				wr, wg, wb, _ := want.At(x, y).RGBA()
				if gr != wr || gg != wg || gb != wb {
					t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got.At(x, y), want.At(x, y))
				}
			}
		}
		if info, _ := os.Stat(filepath.Join(out, r["file"].(string))); info.Mode().Perm() != 0o600 {
			t.Errorf("the picture is mode %v, want 0600", info.Mode().Perm())
		}
	})
	t.Run("jpeg is passed through", func(t *testing.T) {
		pdf := pdfOfImage(t, dir, "scan.jpg", jpgBytes.Bytes())
		out := filepath.Join(t.TempDir(), "pics")
		rows, code := listImages(t, "images", pdf, "--out-dir", out, "--json")
		if code != 0 || len(rows) != 1 {
			t.Fatalf("exit %d, %d rows", code, len(rows))
		}
		b, _ := os.ReadFile(filepath.Join(out, rows[0]["file"].(string)))
		if rows[0]["filter"] != "DCTDecode" {
			t.Errorf("filter = %v, want DCTDecode", rows[0]["filter"])
		}
		if !strings.HasSuffix(rows[0]["file"].(string), ".jpg") || !bytes.Equal(b, jpgBytes.Bytes()) {
			t.Errorf("the JPEG came back as %s, %d bytes (source %d): not the same stream", rows[0]["file"], len(b), jpgBytes.Len())
		}
	})
	t.Run("a document with none", func(t *testing.T) {
		text, _ := testpdf.Text("no pictures here")
		p := filepath.Join(dir, "text.pdf")
		os.WriteFile(p, text, 0o600)
		out := filepath.Join(t.TempDir(), "never")
		rows, code := listImages(t, "images", p, "--out-dir", out, "--json")
		if code != 0 || rows == nil || len(rows) != 0 {
			t.Errorf("exit %d rows %v, want an empty array", code, rows)
		}
		if _, err := os.Stat(out); err == nil {
			t.Error("the folder was created for a document with no pictures")
		}
	})
	t.Run("stdin", func(t *testing.T) {
		pdf := pdfOfImage(t, dir, "again.png", pngBytes.Bytes())
		f, _ := os.Open(pdf)
		defer f.Close()
		old := os.Stdin
		os.Stdin = f
		defer func() { os.Stdin = old }()
		if rows, code := listImages(t, "images", "-", "--out-dir", t.TempDir(), "--json"); code != 0 || len(rows) != 1 {
			t.Errorf("stdin: exit %d, %d rows", code, len(rows))
		}
	})
	t.Run("never overwrites, and says nothing was written", func(t *testing.T) {
		pdf := pdfOfImage(t, dir, "third.png", pngBytes.Bytes())
		out := t.TempDir()
		if _, code := listImages(t, "images", pdf, "--out-dir", out, "--json"); code != 0 {
			t.Fatal("first extraction failed")
		}
		stderr := captureStderr(t, func() {
			if _, code := listImages(t, "images", pdf, "--out-dir", out, "--json"); code != 1 {
				t.Errorf("a rerun over the same folder exited %d, want 1", code)
			}
		})
		if !strings.Contains(stderr, "already exists") {
			t.Errorf("the refusal said %q", stderr)
		}
	})
	t.Run("--out-dir is required", func(t *testing.T) {
		if _, code := listImages(t, "images", "x.pdf"); code != 1 {
			t.Errorf("exit %d, want 1", code)
		}
	})
}
