package nib

import (
	"io/fs"
	"testing"
)

// The single-binary promise depends on every UI asset being embedded. This
// guards against a missing vendored pdf.js file or a renamed front-end file
// silently shipping a broken binary.
func TestWebAssetsEmbedded(t *testing.T) {
	want := []string{
		"index.html",
		"app.js",
		"style.css",
		"vendor/pdfjs/pdf.min.mjs",
		"vendor/pdfjs/pdf.worker.min.mjs",
		"vendor/pdfjs/pdf_viewer.mjs",
		"vendor/pdfjs/pdf_viewer.css",
	}
	web := WebFS()
	for _, name := range want {
		info, err := fs.Stat(web, name)
		if err != nil {
			t.Errorf("asset %q not embedded: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("asset %q is empty", name)
		}
	}
}
