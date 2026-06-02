// Package nib is the module root. Its only job is to embed the web UI assets
// (the single-page front-end plus the vendored pdf.js engine) so the whole app
// ships as one self-contained binary.
package nib

import (
	"embed"
	"io/fs"
)

//go:embed all:web
var webFS embed.FS

// WebFS returns the UI asset tree rooted at the web/ directory, so "/" maps to
// index.html.
func WebFS() fs.FS {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err) // embed guarantees web/ exists; a failure here is a build bug
	}
	return sub
}
