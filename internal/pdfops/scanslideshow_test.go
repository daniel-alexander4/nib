package pdfops

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// slideshowPDF holds an alternate presentation (ISO 32000-1 §13.5): a slideshow in the catalog's
// /Names /AlternatePresentations tree, whose /Resources name a script stream. Nothing else in the file is active.
//
// spec writes /Resources as Table 273 has it, a name tree (a dictionary). pdfcpu's validator wants an ARRAY there
// (`validate/nameTree.go:467`, v0.13.0: "really an array of (string,indRef) pairs") and refuses the dictionary, so
// the two shapes reach different doors.
func slideshowPDF(spec bool) []byte {
	resources := "[(main.js) 11 0 R]"
	if spec {
		resources = "<< /Names [(main.js) 11 0 R] >>"
	}
	return testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /Names << /AlternatePresentations << /Names [(show) 10 0 R] >> >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << >> >>",
		4:  "<< /Length 0 >>\nstream\n\nendstream",
		10: "<< /Type /SlideShow /Subtype /Embedded /Resources " + resources + " /StartResource (main.js) >>",
		11: "<< /Length 17 >>\nstream\nSLIDESHOW-SCRIPT;\nendstream",
	})
}

// TestASlideshowIsSeenAndStripped — `/pending 864`. Scan read /Names for JavaScript, EmbeddedFiles and Renditions
// and not for AlternatePresentations, so a slideshow — which carries its own scripts — scanned clean, StripActive
// left it, and StripActive's own re-scan passed it.
func TestASlideshowIsSeenAndStripped(t *testing.T) {
	const want = "slideshow/medium: Slideshow (alternate presentation; can contain its own scripts)"
	reported := func(pdf []byte) bool {
		for _, f := range must(t, pdf).Findings {
			if f.Kind+"/"+f.Severity+": "+f.Detail == want {
				return true
			}
		}
		return false
	}
	// Scan reads without the validator, so it reaches BOTH shapes; it is the reader the window's report and
	// every removal's verification use.
	for name, pdf := range map[string][]byte{"as the spec writes it": slideshowPDF(true), "as pdfcpu's validator accepts it": slideshowPDF(false)} {
		if !reported(pdf) {
			t.Errorf("%s: the scan did not report %q; findings: %+v", name, want, must(t, pdf).Findings)
		}
	}

	// The shape the validator accepts is the one a rewrite can reach, and StripActive takes it out.
	out, err := StripActive(slideshowPDF(false))
	if err != nil {
		t.Fatalf("StripActive: %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("the result does not validate: %v", err)
	}
	if reported(out) {
		t.Error("the slideshow is still reported after StripActive")
	}
	if _, _, kinds := writtenFacts(t, out); kinds["SlideShow"] != 0 {
		t.Errorf("a /SlideShow dictionary is still in the file: %v", kinds)
	}
	if strings.Contains(string(out), "SLIDESHOW-SCRIPT") {
		t.Error("the slideshow's script is still in the file")
	}

	// The spec's shape is refused by every validating read, so no rewrite reaches it: StripActive refuses the
	// document, by name, and never returns one it calls stripped.
	spec := slideshowPDF(true)
	const refusal = "slideShowDict entry=Resources invalid type types.Dict"
	for name, read := range map[string]func() error{
		"pdfread.Validated": func() error { _, err := pdfread.Validated(spec, model.NewDefaultConfiguration()); return err },
		"pdfread.ReadOptimized": func() error {
			_, err := pdfread.ReadOptimized(spec, model.NewDefaultConfiguration())
			return err
		},
		"Validate":    func() error { return Validate(spec) },
		"StripActive": func() error { _, err := StripActive(spec); return err },
		// Remove files and media leaves a slideshow alone, but it is a rewrite and is refused like any other.
		"RemoveFilesAndMedia": func() error { _, err := RemoveFilesAndMedia(spec); return err },
	} {
		if err := read(); err == nil || !strings.Contains(err.Error(), refusal) {
			t.Errorf("%s on the spec's shape: error %v, want the validator's refusal %q", name, err, refusal)
		}
	}
	if _, err := api.ReadContext(bytes.NewReader(spec), model.NewDefaultConfiguration()); err != nil {
		t.Errorf("the unvalidated read Scan uses refused the spec's shape: %v", err)
	}

	// Remove files and media is the gentle tier — it leaves active code — so the slideshow stays and its own
	// verification does not call that a failure.
	kept, err := RemoveFilesAndMedia(slideshowPDF(false))
	if err != nil {
		t.Fatalf("RemoveFilesAndMedia: %v", err)
	}
	if !reported(kept) {
		t.Error("RemoveFilesAndMedia took out the slideshow, which is StripActive's to remove")
	}
}
