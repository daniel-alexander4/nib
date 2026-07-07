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

// The About dialog shows the AGPLv3 licence and third-party attributions by
// serving these embedded files; if either drops out of the embed the in-app
// view (and our licence-compliance claim) silently breaks.
func TestLegalFilesEmbedded(t *testing.T) {
	legal := LegalFS()
	for _, name := range []string{"LICENSE", "THIRD-PARTY-NOTICES.md"} {
		info, err := fs.Stat(legal, name)
		if err != nil {
			t.Errorf("legal file %q not embedded: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("legal file %q is empty", name)
		}
	}
}
