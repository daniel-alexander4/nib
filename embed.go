// Package nib is the module root. Its only job is to embed the web UI assets
// (the single-page front-end plus the vendored pdf.js engine) and the licence
// texts, so the whole app ships as one self-contained binary.
package nib

import (
	"embed"
	"io/fs"
	"mime"
)

// contentTypes is what these assets are, decided here rather than asked of the
// machine at runtime. net/http serves a file by looking its extension up in the
// mime package (net/http/fs.go), and that package seeds itself from the OS: the
// Windows registry, or /etc/mime.types and friends on Unix. So a stray per-machine
// entry decides the Content-Type of files Nib ships inside its own binary.
//
// That is not hypothetical. Go carries a workaround for the well-known Windows
// registry entry mapping ".js" to text/plain (mime/type_windows.go, Go issue
// #32350) — but it covers ".js" alone, and only for that exact value. Nothing
// protects ".mjs", and the UI is an ES module that imports five of them. A machine
// whose registry misfiles ".mjs" makes Chromium refuse the modules under strict
// MIME checking, so app.js never runs: the window renders from HTML and CSS,
// looking entirely normal, with no setup dialog and no button that responds.
//
// The asset tree is embedded and fixed at build time, so its types are known at
// build time and have no business being a runtime question. embed_test.go walks
// the tree and fails if anything here ships an extension this table doesn't name.
var contentTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".mjs":  "text/javascript; charset=utf-8",
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".gz":   "application/gzip", // the vendored tesseract <lang>.traineddata.gz
	".md":   "text/markdown; charset=utf-8",
}

// registerContentTypes pins the table above, overriding whatever the OS told the
// mime package. mime.AddExtensionType runs the OS-seeded initialization first and
// then applies the override, so calling it at any point before serving is enough.
//
// It is a named function purely so a test can plant a hostile mapping, call this,
// and watch the override win — an assertion on the ambient table would pass on a
// healthy machine whether or not this code existed.
func registerContentTypes() {
	for ext, typ := range contentTypes {
		// The only error is a malformed type or a missing dot, both of which are
		// this file's own typos and are caught by embed_test.go.
		_ = mime.AddExtensionType(ext, typ)
	}
}

// Registered from init rather than from a caller: the failure is silent — a
// wrong Content-Type looks like a working binary until a user's machine happens
// to disagree — so it must not be possible to forget the call.
func init() { registerContentTypes() }

//go:embed all:web
var webFS embed.FS

// legalFS carries the AGPLv3 licence and the third-party attribution file so the
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
