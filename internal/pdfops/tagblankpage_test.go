package pdfops

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/mdpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// `/pending 504`: a Markdown page that draws no text. mdpdf kept blank pages valid with a lone-space text
// run no role described, so the page drew one text operator against zero runs, `tagOnePage` refused the
// mismatch, and `ConvertDocToPDF` fell back to the untagged render — which also cost the ADR-033 label.
// Reached by any code block whose blank lines spill onto a page of their own.

// markdownWithATextlessPage holds the two shapes: the blank page last, and blank pages in the middle.
var markdownWithATextlessPage = map[string]string{
	"trailing":   "# Title\n\n```\n" + strings.Repeat("x\n", 48) + strings.Repeat("\n", 60) + "```\n",
	"in between": "# Title\n\n```\n" + strings.Repeat("x\n", 48) + strings.Repeat("\n", 140) + "x\n```\n\nAfter.\n",
}

// textlessPages returns the 1-based pages mdpdf laid out with no runs.
func textlessPages(t *testing.T, md string) []int {
	t.Helper()
	_, st, err := mdpdf.ConvertStructured([]byte(md), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []int
	for i, p := range st.Pages {
		if len(p) == 0 {
			out = append(out, i+1)
		}
	}
	return out
}

func TestAMarkdownPageThatDrawsNoTextIsStillTagged(t *testing.T) {
	for name, md := range markdownWithATextlessPage {
		t.Run(name, func(t *testing.T) {
			blank := textlessPages(t, md)
			// STIMULUS: the layout really has a page with no runs, or this is any other conversion.
			if len(blank) == 0 {
				t.Fatal("setup: no page without text — the fixture no longer reaches the case")
			}
			out, err := tagMarkdown([]byte(md), authoringFaces(), markdownFallbackFonts())
			if err != nil {
				t.Fatalf("a document with a text-less page (page %v) could not be tagged: %v", blank, err)
			}
			if kinds := elementKinds(t, out); kinds["H1"] != 1 {
				t.Errorf("the tagged tree lost the heading: %v", kinds)
			}
			// What the blank page draws is still content, and untagged content fails 7.1 t3: it is an artifact.
			ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range blank {
				d, _, _, err := ctx.PageDict(p, false)
				if err != nil {
					t.Fatal(err)
				}
				src, err := ctx.PageContent(d, p)
				if err != nil {
					t.Fatal(err)
				}
				if n := len(textOperatorSpans(src)); n != 0 {
					t.Errorf("page %d has no runs and draws %d text operator(s)", p, n)
				}
				if !bytes.Contains(src, []byte("/Artifact BMC")) {
					t.Errorf("page %d draws its placeholder outside any artifact: %q", p, src)
				}
			}
		})
	}
}

// TestALabelledConversionWithATextlessPageConforms — the reported path end to end: the conversion is
// tagged, earns the label, and veraPDF agrees.
func TestALabelledConversionWithATextlessPageConforms(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the conformance the label claims is UNMEASURED here")
	}
	dir := t.TempDir()
	var files []string
	for name, md := range markdownWithATextlessPage {
		labelled, err := LabelUA(labelReady(t, md), true)
		if err != nil {
			t.Fatalf("%s: LabelUA refused a conversion with a text-less page: %v", name, err)
		}
		p := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".pdf")
		if err := os.WriteFile(p, labelled, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	cl := ua1FailedClauses(t, vp, files)
	for _, p := range files {
		b := filepath.Base(p)
		if cl[b] == nil {
			t.Errorf("veraPDF could not validate %s", b)
		} else if len(cl[b]) > 0 {
			t.Errorf("%s: the labelled conversion fails PDF/UA-1 on %v", b, sortedClauses(cl[b]))
		}
	}
}
