package cli

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
)

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestStampPutsAMarkOnTheNamedPagesOnly — /pending 907. `nib stamp` reaches pdfops.StampPages: the output
// carries a stamp layer the input did not, a page the document lacks is refused with no output, and exactly one kind is required. Which pages take the mark is pdfops' test.
func TestStampPutsAMarkOnTheNamedPagesOnly(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf", "one", "two", "three")
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 40, 20))); err != nil {
		t.Fatal(err)
	}
	logo := filepath.Join(dir, "logo.png")
	if err := os.WriteFile(logo, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "out.pdf")
	if code := cmdStamp([]string{in, "-o", out, "--image", logo, "--pages", "2"}); code != 0 {
		t.Fatalf("stamp --image exit = %d, want 0", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if has, _ := pdfops.HasStampLayer(b); !has {
		t.Fatal("the output carries no stamp layer; --image stamped nothing")
	}
	if has, _ := pdfops.HasStampLayer(mustRead(t, in)); has {
		t.Fatal("setup: the input already carried a stamp layer, so the check above proves nothing")
	}

	for name, args := range map[string][]string{
		"none given": {in, "-o", filepath.Join(dir, "a.pdf")},
		"two given":  {in, "-o", filepath.Join(dir, "b.pdf"), "--text", "x", "--watermark", "y"},
		"bad page":   {in, "-o", filepath.Join(dir, "c.pdf"), "--text", "x", "--pages", "9"},
	} {
		if code := cmdStamp(args); code == 0 {
			t.Errorf("%s: exit 0, want a refusal", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "c.pdf")); err == nil {
		t.Error("a refused stamp left an output file")
	}
}
