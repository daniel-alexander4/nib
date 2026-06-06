// Package nib is the module root. Its only job is to embed the web UI assets
// (the single-page front-end plus the vendored pdf.js engine) and the licence
// texts, so the whole app ships as one self-contained binary.
package nib

import (
	"embed"
	"io/fs"
)

//go:embed all:web
var webFS embed.FS

// legalFS carries the GPLv3 licence and the third-party attribution file so the
// About dialog can show the same text that ships in the .deb/release — they're
// served straight from these embedded copies, so the in-app view can't drift.
//
//go:embed LICENSE THIRD-PARTY-NOTICES.md
var legalFS embed.FS

// WebFS returns the UI asset tree rooted at the web/ directory, so "/" maps to
// index.html.
func WebFS() fs.FS {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err) // embed guarantees web/ exists; a failure here is a build bug
	}
	return sub
}

// LegalFS returns LICENSE and THIRD-PARTY-NOTICES.md at its root, for the About
// dialog's in-app licence/notices view.
func LegalFS() fs.FS { return legalFS }
