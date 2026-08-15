package nib

import (
	"io/fs"
	"mime"
	"path"
	"strings"
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

// Every extension in the embedded tree must have a content type declared here,
// because anything left out falls through to the machine's own MIME table — the
// exact route by which a Windows registry entry for ".mjs" stopped the whole UI
// from executing. Vendoring a new kind of asset should fail this test, not ship a
// binary whose behaviour depends on the user's registry.
//
// Extensionless files are allowed through: net/http sniffs them, and nothing in
// the tree executes without an extension.
func TestEveryEmbeddedExtensionHasAContentType(t *testing.T) {
	seen := map[string][]string{}
	for _, tree := range []struct {
		name string
		fsys fs.FS
	}{{"web", WebFS()}, {"legal", LegalFS()}} {
		err := fs.WalkDir(tree.fsys, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if ext := strings.ToLower(path.Ext(p)); ext != "" {
				seen[ext] = append(seen[ext], tree.name+"/"+p)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", tree.name, err)
		}
	}
	if len(seen) == 0 {
		t.Fatal("walked the embedded trees and found no files — the walk itself is broken")
	}
	for ext, files := range seen {
		if _, ok := contentTypes[ext]; !ok {
			t.Errorf("extension %q is embedded (e.g. %s) but has no entry in contentTypes; "+
				"without one its Content-Type comes from the user's machine", ext, files[0])
		}
	}
}

// The override has to beat a hostile mapping, which is the whole point — so the
// test plants one first. Asserting the ambient table instead would pass on any
// healthy machine with or without registerContentTypes, and this project has been
// bitten by exactly that shape of green.
func TestRegisterContentTypesOverridesTheMachine(t *testing.T) {
	for _, ext := range []string{".mjs", ".js"} {
		if err := mime.AddExtensionType(ext, "text/plain"); err != nil {
			t.Fatalf("planting a hostile type for %s: %v", ext, err)
		}
		if got := mime.TypeByExtension(ext); !strings.HasPrefix(got, "text/plain") {
			t.Fatalf("could not plant a hostile type for %s (got %q) — the test proves nothing", ext, got)
		}
	}

	registerContentTypes()

	for _, ext := range []string{".mjs", ".js"} {
		got := mime.TypeByExtension(ext)
		if !strings.HasPrefix(got, "text/javascript") {
			t.Errorf("TypeByExtension(%q) = %q after registration, want text/javascript; "+
				"Chromium refuses a module served as anything else", ext, got)
		}
	}
}
