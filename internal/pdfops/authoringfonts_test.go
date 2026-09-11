package pdfops

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"nib/mdpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// P04.S01 — the faces nib authors text in are embedded in what it produces.
//
// PDF/UA rule 7.21.4.1: *the font programs for all fonts used for rendering within a conforming
// file shall be embedded within that file.* A Base-14 core font cannot satisfy it — it is a name
// the reader supplies, not a program in the document — so `mdpdf`'s five core faces were the whole
// of why nib's own Markdown output failed it.

const p4Markdown = "# A heading\n\nBody text with **bold**, *italic* and ***both***.\n\n" +
	"```\ncode block line\n```\n\n- a list item\n- another\n"

// TestAuthoredMarkdownEmbedsEveryFontItDrawsWith reads the ANSWER out of the output rather than
// checking a list of face names.
//
// **That is the whole design of the assertion.** A guard that compares `authoringFaces()` against
// a written-down list of five names passes when a sixth face is added to `mdpdf` and not supplied
// — which is exactly the failure that would reintroduce the defect. `nonEmbeddedFonts` walks the
// produced document's own font dictionaries, so a face nobody remembered shows up as the thing it
// is: a font this document draws with and does not carry.
func TestAuthoredMarkdownEmbedsEveryFontItDrawsWith(t *testing.T) {
	pdf, err := ConvertDocToPDF([]byte(p4Markdown), ".md")
	if err != nil {
		t.Fatalf("ConvertDocToPDF: %v", err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	if missing := nonEmbeddedFonts(ctx.XRefTable); len(missing) > 0 {
		t.Errorf("authored Markdown draws with fonts it does not embed: %s\n\tPDF/UA 7.21.4.1. "+
			"A Base-14 core face cannot be embedded, so a face reaching the output means "+
			"`authoringFaces()` did not supply it and `mdpdf` fell back to the core set for it.",
			strings.Join(missing, ", "))
	}
}

// TestTheCoreFacesAreStillTheFallbackAndStillFail is the stimulus floor, and it asserts the
// OPPOSITE of the test above on the same input.
//
// Without it, `TestAuthoredMarkdownEmbedsEveryFontItDrawsWith` passes on a build where
// `nonEmbeddedFonts` has stopped reporting anything at all — which is a plausible failure, since
// it depends on `font.Embedded` and walks a dict layout pdfcpu owns. A rule verified only in the
// direction where it holds is not verified.
func TestTheCoreFacesAreStillTheFallbackAndStillFail(t *testing.T) {
	pdf, err := mdpdf.ConvertWithFonts([]byte(p4Markdown), markdownFallbackFonts())
	if err != nil {
		t.Fatalf("ConvertWithFonts: %v", err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	missing := nonEmbeddedFonts(ctx.XRefTable)
	if len(missing) == 0 {
		t.Fatal("the Base-14 path reports no non-embedded fonts, so the instrument cannot see " +
			"the condition the test above asserts is absent — either mdpdf's core path is gone " +
			"or nonEmbeddedFonts has stopped reading font dictionaries")
	}
	for _, m := range missing {
		if !strings.Contains(m, "Helvetica") && !strings.Contains(m, "Courier") {
			t.Errorf("the core path reports %q as non-embedded, which is not a Base-14 face — "+
				"the fallback pool is leaking into a path that should be core-only", m)
		}
	}
}

// TestEveryAuthoringFaceIsNamedAsPdfcpuNamesIt.
//
// pdfcpu registers a face under the PostScript name inside the TTF and **ignores the name it was
// given**, so a wrong constant installs a face nothing can reference. `mdpdf.installFallbacks`
// refuses that mismatch rather than installing under a dead name — which is how
// `LiberationMono-Regular` was found to be `LiberationMono` while building this slice, in the one
// second it took to run the conversion.
//
// This drives the refusal on the real set, so the constants cannot drift from the files.
func TestEveryAuthoringFaceIsNamedAsPdfcpuNamesIt(t *testing.T) {
	faces := authoringFaces()
	if faces == nil {
		t.Fatal("authoringFaces() is nil: the embedded TTFs are unreadable, which cannot happen " +
			"in a built binary and means the //go:embed list and the map have diverged")
	}
	// ConvertWithFaces installs every base face and returns the mismatch error verbatim.
	if _, err := mdpdf.ConvertWithFaces([]byte("x\n"), faces, nil); err != nil {
		t.Fatalf("the authoring faces do not install: %v", err)
	}
}

// TestEveryVendoredAuthoringFontIsEmbeddedInTheBinary — the map and the `//go:embed` list are two
// places naming the same files, and a file in one and not the other fails only at run time.
func TestEveryVendoredAuthoringFontIsEmbeddedInTheBinary(t *testing.T) {
	if len(authoringFontFiles) < 5 {
		t.Fatalf("authoringFontFiles has %d entries; mdpdf draws with five base faces",
			len(authoringFontFiles))
	}
	for name, path := range authoringFontFiles {
		bb, err := ocrFontFS.ReadFile(path)
		if err != nil {
			t.Errorf("%s (%s) is in authoringFontFiles and not in the //go:embed list: %v",
				name, path, err)
			continue
		}
		if len(bb) < 10000 {
			t.Errorf("%s is only %d bytes — that is not a TTF", name, len(bb))
		}
	}
}

// TestADegradedConversionSaysSo — `PLAN-accessibility.md` P04.S03's "and saying so".
//
// A document set in core fonts looks entirely correct and fails PDF/UA 7.21.4.1. If the degrade is
// silent, the only way anyone learns of it is by validating a document — which is the situation the
// whole phase exists to end.
func TestADegradedConversionSaysSo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("SKIP (not a pass): running as root ignores the directory mode this depends on")
	}
	model.NewDefaultConfiguration()
	orig := font.UserFontDir
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { font.UserFontDir = orig; os.Chmod(dir, 0o700) })
	font.UserFontDir = dir

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	out, err := ConvertDocToPDF([]byte(p4Markdown), ".md")
	if err != nil {
		t.Fatalf("the conversion failed instead of degrading: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("the degrade produced nothing")
	}
	if !strings.Contains(buf.String(), "Base-14 core fonts") {
		t.Errorf("the degrade was silent. Logged:\n%s", buf.String())
	}
	// And the degrade is real, not just announced: the document must actually name core fonts.
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatalf("re-read: %v", rerr)
	}
	if missing := nonEmbeddedFonts(ctx.XRefTable); len(missing) == 0 {
		t.Error("the log says the document degraded and its fonts are all embedded — one of the " +
			"two is lying, and a message nobody can check is worse than none")
	}
}
