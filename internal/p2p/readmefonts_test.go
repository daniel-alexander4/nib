package p2p

import (
	"bytes"
	"nib/mdpdf"
	"strings"
	"testing"

	"nib/internal/pdfops"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// nonEmbedded names the fonts pdf draws with and does not carry. It is the local twin of
// pdfops.nonEmbeddedFonts, which is unexported; both read the produced document rather than a list.
func nonEmbedded(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	xt := ctx.XRefTable
	var missing []string
	seen := map[string]bool{}
	for objNr, e := range xt.Table {
		if e == nil {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok {
			continue
		}
		if ty := d.NameEntry("Type"); ty == nil || *ty != "Font" {
			continue
		}
		if emb, err := font.Embedded(xt, d, objNr); err != nil || emb {
			continue
		}
		name := "(unnamed)"
		if n := d.NameEntry("BaseFont"); n != nil {
			name = *n
		}
		if !seen[name] {
			seen[name] = true
			missing = append(missing, name)
		}
	}
	return missing
}

// TestNibsOwnPagesEmbedTheirFonts — `PLAN-accessibility.md` P04.S05.
//
// The readme and the signature pages are prose nib WROTE, stapled into a document somebody signs.
// They were set in Helvetica, so every co-signed document carried non-embedded fonts on exactly the
// pages nib is responsible for.
func TestNibsOwnPagesEmbedTheirFonts(t *testing.T) {
	readme, err := RenderReadme()
	if err != nil {
		t.Fatalf("RenderReadme: %v", err)
	}
	if missing := nonEmbedded(t, readme); len(missing) > 0 {
		t.Errorf("the trust-explainer readme draws with fonts it does not embed: %s",
			strings.Join(missing, ", "))
	}
	page, err := renderPage([]any{map[string]any{
		"value": "Signature page", "pos": []any{62.0, 700.0},
		"font": map[string]any{"name": "$body"},
	}}, []mdpdf.Role{{Kind: mdpdf.RoleHeading, Level: 1}})
	if err != nil {
		t.Fatalf("renderPage: %v", err)
	}
	if missing := nonEmbedded(t, page); len(missing) > 0 {
		t.Errorf("a signature page draws with fonts it does not embed: %s", strings.Join(missing, ", "))
	}

	// And embedding brings its own clause: pdfcpu writes a `/CIDSet` over the USED glyphs, which
	// PDF/UA 7.21.4.2 t2 forbids. `CreateFromJSON` is the third door P04.S02's rule had to reach,
	// and it was found by measuring this page rather than by reading the call graph.
	for _, c := range []struct {
		name string
		pdf  []byte
	}{{"the readme", readme}, {"a signature page", page}} {
		if n := cidSets(t, c.pdf); n > 0 {
			t.Errorf("%s carries %d /CIDSet(s) — see pdfops.dropCIDSets", c.name, n)
		}
	}
}

// cidSets counts FontDescriptors carrying a /CIDSet.
func cidSets(t *testing.T, pdf []byte) int {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	n := 0
	for _, e := range ctx.XRefTable.Table {
		if e == nil || e.Object == nil {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok {
			continue
		}
		if ty, _ := d["Type"].(types.Name); ty != "FontDescriptor" {
			continue
		}
		if _, ok := d["CIDSet"]; ok {
			n++
		}
	}
	return n
}

// TestAppendingNibsPagesLeavesTheDocumentsOwnFontsAlone.
//
// `Append` keeps the FIRST document's catalog, but page objects and their resources come across —
// so this asks the question the catalog rule does not: does stapling nib's pages on change what the
// USER's pages are drawn with? A document that acquires a font it never used is a document nib
// edited without being asked.
func TestAppendingNibsPagesLeavesTheDocumentsOwnFontsAlone(t *testing.T) {
	user, err := pdfops.CreateFromJSON([]byte(`{"pages":{"1":{"content":{"text":[` +
		`{"value":"the user's own page","anchor":"TopLeft","position":[72,720],` +
		`"font":{"name":"Times-Roman","size":12}}]}}}}`))
	if err != nil {
		t.Fatalf("build the user document: %v", err)
	}
	before := nonEmbedded(t, user)
	if len(before) == 0 {
		t.Fatal("setup: the user document embeds all its fonts, so this cannot tell a preserved " +
			"core font from one nib removed")
	}

	out, err := AppendReadme(user)
	if err != nil {
		t.Fatalf("AppendReadme: %v", err)
	}
	after := nonEmbedded(t, out)
	for _, b := range before {
		found := false
		for _, a := range after {
			if a == b {
				found = true
			}
		}
		if !found {
			t.Errorf("the user's own font %q is gone from the composed document — appending nib's "+
				"pages must not touch the pages that were already there", b)
		}
	}
	// And nib's own pages must not have brought Helvetica back with them.
	for _, a := range after {
		was := false
		for _, b := range before {
			if a == b {
				was = true
			}
		}
		if !was {
			t.Errorf("appending nib's pages added a NON-EMBEDDED font %q that the user's document "+
				"did not have — nib's own pages are supposed to carry theirs", a)
		}
	}
}
