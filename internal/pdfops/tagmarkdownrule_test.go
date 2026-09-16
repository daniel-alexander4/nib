package pdfops

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// pageContentsOf returns every page's decoded content stream.
func pageContentsOf(t *testing.T, pdf []byte) [][]byte {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	var out [][]byte
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, derr := ctx.PageDict(p, false)
		if derr != nil {
			t.Fatal(derr)
		}
		src, cerr := ctx.PageContent(d, p)
		if cerr != nil {
			t.Fatal(cerr)
		}
		out = append(out, src)
	}
	return out
}

// TestAThematicBreakIsDeclaredAnArtifact — the one mdpdf construct a labelled conversion failed PDF/UA over
// (7.1 t3), for `/pending 486`.
func TestAThematicBreakIsDeclaredAnArtifact(t *testing.T) {
	out, err := tagMarkdown([]byte("# Title\n\nBefore.\n\n---\n\nAfter.\n"), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	var textless, artifacts int
	for _, src := range pageContentsOf(t, out) {
		for _, g := range drawingGroups(src) {
			if !g.showsText {
				textless++
			}
		}
		artifacts += strings.Count(string(src), "/Artifact BMC")
	}
	// Stimulus: the rule really is a text-less group, or "every one is an artifact" is vacuous.
	if textless == 0 {
		t.Fatal("setup: the tagged page has no drawing group without text — the rule is not drawn as one, so this asserts nothing")
	}
	if artifacts != textless {
		t.Errorf("%d drawing group(s) show no text and %d are bracketed /Artifact — content that is neither tagged nor an artifact fails PDF/UA 7.1 t3", textless, artifacts)
	}

	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (half checked): the bracket is asserted; veraPDF is absent, so 7.1 t3 itself is UNMEASURED here")
	}
	p := filepath.Join(t.TempDir(), "rule.pdf")
	if err := os.WriteFile(p, out, 0o600); err != nil {
		t.Fatal(err)
	}
	if cl := ua1FailedClauses(t, vp, []string{p}); cl["rule.pdf"] == nil || cl["rule.pdf"]["7.1 t3"] {
		t.Errorf("veraPDF still fails 7.1 t3 on a tagged conversion with a thematic break: %v", sortedClauses(cl["rule.pdf"]))
	}
}
