package nib

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheVendoredViewerIsUpstreamsButForItsOnePatch holds ADR-167's maintenance rule, the one ADR-066 and
// ADR-142 hold for nib's two modified Go modules (librarypatch_test.go). `web/vendor/pdfjs/` is the npm
// package `pdfjs-dist` at the version its VERSION file names, and `pdf_viewer.mjs` differs from the
// published file in two lines (/pending 887: a page redrawn while its text layer renders lost the links
// pdf.js makes from its text). There is no module cache to compare a vendored web file against, so the
// published files are pinned here by SHA-256:
//
//   - VERSION is the pinned version: a bump leaves the patch applied to a viewer it was not written
//     against — or silently gone — and this fails until the patch is re-applied (or dropped, where
//     upstream has fixed it) and the pins moved;
//   - the three files nib does not touch hash as published;
//   - the viewer carries both patched lines, and with each put back as upstream wrote it the file hashes
//     as published — so the two lines are the whole difference, and a third edit is one the notice does
//     not declare.
//
// The hashes are of `pdfjs-dist@<version>`'s `build/pdf.min.mjs`, `build/pdf.worker.min.mjs`,
// `web/pdf_viewer.css` and `web/pdf_viewer.mjs` (`npm pack pdfjs-dist@<version>`).
func TestTheVendoredViewerIsUpstreamsButForItsOnePatch(t *testing.T) {
	const (
		dir     = "web/vendor/pdfjs"
		version = "6.1.200"
	)
	published := map[string]string{
		"pdf.min.mjs":        "4ba2f15599b03fde8755ad91349920c21dadd3e8fd6b6460a7663d46d4cf21b5",
		"pdf.worker.min.mjs": "2ab9e09667296dab1a618868b3ce6e6c23d5b8f48120ae7c5b34e7e335ed01fa",
		"pdf_viewer.css":     "a38845ce94bd5ac25121c5b0b00a12516f75acfcc9d864e0ecd41ff99e9b437f",
		"pdf_viewer.mjs":     "b1899e04b342991e480c113f82066f842d558e045762476e4e845be6aa0dca0d",
	}
	// Each patched line, and the line upstream has in its place.
	patch := [][2]string{
		{
			"        return false; // Nib (ADR-167): this drawing's text was never rendered.\n      }\n      console.error(\"#renderTextLayer:\", ex);",
			"        return;\n      }\n      console.error(\"#renderTextLayer:\", ex);",
		},
		{
			"      if ((await textLayerPromise) === false || annotationLayer !== this.annotationLayer || textLayer !== this.textLayer) {\n        return; // Nib (ADR-167): no text links are looked for in a text layer that was not rendered.\n",
			"      await textLayerPromise;\n      if (annotationLayer !== this.annotationLayer || textLayer !== this.textLayer) {\n        return;\n",
		},
	}
	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

	got, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if v := strings.TrimSpace(string(got)); v != version {
		t.Fatalf("%s/VERSION is %s and this guard pins %s — re-apply the patch NOTICE.md describes to the new viewer (or drop it, if upstream now refuses to look for links in an aborted text layer: test/ui/autolink.test.mjs says), then move the version and the four hashes here", dir, v, version)
	}
	if _, err := os.Stat(filepath.Join(dir, "NOTICE.md")); err != nil {
		t.Errorf("%s/NOTICE.md: %v — the modified copy must say what was modified", dir, err)
	}
	for _, name := range []string{"pdf.min.mjs", "pdf.worker.min.mjs", "pdf_viewer.css"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if sum(b) != published[name] {
			t.Errorf("%s/%s is not pdfjs-dist@%s's file — NOTICE.md says nib leaves it as published", dir, name, version)
		}
	}
	viewer, err := os.ReadFile(filepath.Join(dir, "pdf_viewer.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	restored := viewer
	for i, p := range patch {
		if n := bytes.Count(restored, []byte(p[0])); n != 1 {
			t.Fatalf("%s/pdf_viewer.mjs carries patched line %d %d times, want once — the copy has been reverted to upstream, or the patch has changed without this guard", dir, i+1, n)
		}
		restored = bytes.Replace(restored, []byte(p[0]), []byte(p[1]), 1)
	}
	if sum(restored) != published["pdf_viewer.mjs"] {
		t.Errorf("%s/pdf_viewer.mjs differs from pdfjs-dist@%s's by more than the two lines NOTICE.md declares", dir, version)
	}
}
