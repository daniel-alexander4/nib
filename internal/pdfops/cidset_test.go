package pdfops

import (
	"bytes"
	"fmt"
	"testing"

	"nib/mdpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// cidSetCount reports how many embedded font descriptors in pdf carry a /CIDSet, and how many
// carry an embedded font program at all. The second number is the stimulus floor: a document with
// no embedded fonts trivially has no CIDSets, and the two results must never be confused.
func cidSetCount(t *testing.T, pdf []byte) (withCIDSet, embedded int) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	xt := ctx.XRefTable
	for _, e := range xt.Table {
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
		if _, ok := d["FontFile2"]; ok {
			embedded++
		}
		if _, ok := d["CIDSet"]; ok {
			withCIDSet++
		}
	}
	return withCIDSet, embedded
}

// TestNothingNibEmbedsAFontIntoCarriesACIDSet — P04.S02, over both doors that embed a face nib
// supplied.
//
// The assertion is read out of the OUTPUT rather than from a list of call sites, for the reason
// P04.S01's guard gives: a list passes when a third door is added and not routed.
func TestNothingNibEmbedsAFontIntoCarriesACIDSet(t *testing.T) {
	for _, c := range []struct {
		name string
		make func() ([]byte, error)
	}{
		{"authored Markdown", func() ([]byte, error) {
			return ConvertDocToPDF([]byte(p4Markdown), ".md")
		}},
		{"a CreateFromJSON spec naming an embedded face", func() ([]byte, error) {
			body, _, embedded := AuthoredTextFaces()
			if !embedded {
				t.Skip("SKIP (not a pass): the embedded faces are unavailable here, so this door " +
					"is drawing in core fonts and has no CIDSet to carry")
			}
			return CreateFromJSON([]byte(`{"pages":{"1":{"content":{"text":[{"value":"a page",` +
				`"anchor":"TopLeft","position":[72,720],"font":{"name":"` + body + `","size":12}}]}}}}`))
		}},
		{"a tagged Markdown document", func() ([]byte, error) {
			return tagMarkdown([]byte(p4Markdown), authoringFaces(), markdownFallbackFonts())
		}},
		{"OCR text layer", func() ([]byte, error) {
			return StampTextLayer(threePagePDF(t),
				[]Word{{Page: 1, Rect: [4]float64{20, 40, 70, 50}, Text: "hello world"}}, "eng")
		}},
		// The stamps, `PLAN-ua-coverage.md` P01.S03 — on a document that embeds no face of its own, so a
		// stamp's font is a NEW descriptor. On the census document the watermark reused the Markdown's
		// Roboto, whose CIDSet was already gone, and passed without routing through the door.
		{"stamped fields", func() ([]byte, error) {
			o, _, err := StampFields(threePagePDF(t),
				[]Field{{Page: 1, Rect: [4]float64{50, 400, 300, 420}, Text: "Replaced text", Font: "Times-Roman", Size: 12}})
			return o, err
		}},
		{"watermark", func() ([]byte, error) { return StampWatermark(threePagePDF(t), "DRAFT", WatermarkStyle{}) }},
		{"page numbers", func() ([]byte, error) { return StampPageNumbers(threePagePDF(t), PageNumberStyle{}) }},
	} {
		out, err := c.make()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		withCIDSet, embedded := cidSetCount(t, out)
		if embedded == 0 {
			t.Errorf("%s: the output embeds NO font program, so \"no CIDSet\" is vacuous — this "+
				"door has stopped embedding a face and P04.S01's rule is broken too", c.name)
			continue
		}
		if withCIDSet > 0 {
			t.Errorf("%s: %d of %d embedded font descriptor(s) carry a /CIDSet.\n\tPDF/UA "+
				"7.21.4.2 t2 requires one to identify every CID in the font PROGRAM, and pdfcpu "+
				"computes it over the USED glyphs (its own comment, font/fontDict.go:252). The "+
				"clause is conditional on the stream being there, so the door removes it rather "+
				"than restating what the program already says — see dropCIDSets.",
				c.name, withCIDSet, embedded)
		}
	}
}

// TestPdfcpuStillWritesTheCIDSetTheDoorRemoves is the stimulus floor for the test above, and it is
// the half that rots.
//
// If a later pdfcpu stops writing `/CIDSet`, or starts writing a correct one, the door becomes a
// no-op and the guard above passes for a reason that has nothing to do with nib. Then this goes
// red, and whoever reads it can retire the door instead of carrying it forever.
func TestPdfcpuStillWritesTheCIDSetTheDoorRemoves(t *testing.T) {
	raw, err := mdpdfRaw(t)
	if err != nil {
		t.Fatal(err)
	}
	withCIDSet, embedded := cidSetCount(t, raw)
	if embedded == 0 {
		t.Fatal("the raw conversion embeds no font program at all")
	}
	if withCIDSet == 0 {
		t.Skipf("SKIP (a finding, not a pass): pdfcpu no longer writes a /CIDSet for its %d "+
			"embedded font(s), so dropCIDSets is now a no-op and can be retired — check "+
			"PDF/UA 7.21.4.2 against a fresh output before removing it", embedded)
	}
	t.Logf("pdfcpu wrote %d /CIDSet(s) over %d embedded font(s); the door removes them", withCIDSet, embedded)
}

// TestDroppingCIDSetsKeepsPDFAConformance.
//
// `/CIDSet` is required for subset fonts by **PDF/A-1 only** — PDF/A-2 dropped it — and nib targets
// PDF/A-2b. That is a claim about a standard, which is exactly the kind this repo does not take on
// trust: the reference validator says so here, on a document that went through the door.
func TestDroppingCIDSetsKeepsPDFAConformance(t *testing.T) {
	pdf, err := ConvertDocToPDF([]byte(p4Markdown), ".md")
	if err != nil {
		t.Fatalf("ConvertDocToPDF: %v", err)
	}
	if withCIDSet, _ := cidSetCount(t, pdf); withCIDSet != 0 {
		t.Fatalf("setup: %d /CIDSet(s) survived the door, so this proves nothing about removing them", withCIDSet)
	}
	out, blockers, err := PreparePDFA(pdf)
	if err != nil {
		t.Fatalf("PreparePDFA: %v", err)
	}
	if len(blockers) > 0 {
		t.Fatalf("PreparePDFA refused a document nib authored: %v", blockers)
	}
	requireVeraPDFCompliant(t, out, "2b")
}

// mdpdfRaw is the Markdown conversion WITHOUT the door, so a test can see what pdfcpu produced.
func mdpdfRaw(t *testing.T) ([]byte, error) {
	t.Helper()
	out, err := mdpdf.ConvertWithFaces([]byte(p4Markdown), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		return nil, fmt.Errorf("raw convert: %w", err)
	}
	return out, nil
}

// TestTheCIDSetTailIsSkippedForACoreFontSpec — the cost half of P04's phase close.
//
// `CreateFromJSON` is called once per signature page, and the tail is a parse-and-rewrite that more
// than doubles it (3.8 ms → 8.7 ms, measured). A spec naming only Base-14 faces cannot produce a
// `/CIDSet`, so the tail is asked of the spec rather than run unconditionally.
//
// **The assertion is over-inclusiveness, which is the direction that is safe.** A spec that merely
// MENTIONS a user font anywhere must take the tail even if no page draws with it; the reverse — a
// spec that draws with one and is skipped — is the failure that ships the clause violation.
func TestTheCIDSetTailIsSkippedForACoreFontSpec(t *testing.T) {
	body, _, embedded := AuthoredTextFaces()
	if !embedded {
		t.Skip("SKIP (not a pass): no embedded face is installed here, so a spec cannot name one")
	}
	for _, c := range []struct {
		name string
		spec string
		want bool
	}{
		{"only Base-14 faces", `{"fonts":{"b":{"name":"Helvetica","size":11}},"pages":{}}`, false},
		{"a user font under fonts", `{"fonts":{"b":{"name":"` + body + `","size":11}},"pages":{}}`, true},
		{"a user font named anywhere at all", `{"pages":{"1":{"content":{"text":[` +
			`{"value":"` + body + `","font":{"name":"Helvetica"}}]}}}}`, true},
		{"unparseable", `{not json`, true},
	} {
		if got := specNamesAUserFont([]byte(c.spec)); got != c.want {
			t.Errorf("%s: specNamesAUserFont = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestAStampDropsOnlyTheCIDSetsOfTheFacesItDrew — `dropCIDSetsOf` with faces named leaves every other
// descriptor's /CIDSet alone: a stamp drew those faces, and the user's own fonts are their file's.
func TestAStampDropsOnlyTheCIDSetsOfTheFacesItDrew(t *testing.T) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(threePagePDF(t)), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	add := func(fontName string) types.Dict {
		set, serr := ctx.XRefTable.IndRefForNewObject(types.Dict{})
		if serr != nil {
			t.Fatal(serr)
		}
		d := types.Dict{"Type": types.Name("FontDescriptor"), "FontName": types.Name(fontName), "CIDSet": *set}
		if _, err := ctx.XRefTable.IndRefForNewObject(d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	drawn, users := add("ABCDEF+LiberationSans"), add("GHIJKL+MinionPro-Regular")
	dropCIDSetsOf(ctx, []string{"LiberationSans"})
	if _, still := drawn["CIDSet"]; still {
		t.Error("the stamp's own face kept its /CIDSet")
	}
	if _, kept := users["CIDSet"]; !kept {
		t.Error("a stamp removed the /CIDSet of a font it did not draw")
	}
}

// descriptorCIDSets reports, for each FontDescriptor whose FontName ends in "+"+face, whether it carries a
// /CIDSet — so a test can tell a door's own face from the document's.
func descriptorCIDSets(t *testing.T, pdf []byte, face string) (found, withCIDSet int) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	for _, e := range ctx.XRefTable.Table {
		if e == nil {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok {
			continue
		}
		if ty, _ := d["Type"].(types.Name); ty != "FontDescriptor" {
			continue
		}
		if name, _ := d["FontName"].(types.Name); len(name) > 7 && string(name[6:]) == "+"+face {
			found++
			if _, has := d["CIDSet"]; has {
				withCIDSet++
			}
		}
	}
	return found, withCIDSet
}

// TestTheOCRLayerLeavesTheDocumentsOwnCIDSetsAlone — the P01 phase-close review: the stamps dropped only
// the /CIDSet of the faces they drew, and the OCR door dropped every one in the user's scan.
func TestTheOCRLayerLeavesTheDocumentsOwnCIDSetsAlone(t *testing.T) {
	host, err := mdpdfRaw(t)
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus first: the host's own font carries a /CIDSet for the door to leave alone.
	if found, with := descriptorCIDSets(t, host, "Roboto-Regular"); found == 0 || with == 0 {
		t.Fatalf("setup: the host has %d Roboto-Regular descriptor(s), %d with a /CIDSet", found, with)
	}
	out, err := StampTextLayer(host, []Word{{Page: 1, Rect: [4]float64{20, 40, 70, 50}, Text: "สวัสดี"}}, "tha")
	if err != nil {
		t.Fatal(err)
	}
	if found, with := descriptorCIDSets(t, out, "NotoSansThai-Regular"); found == 0 || with != 0 {
		t.Errorf("the OCR face: %d descriptor(s), %d still with a /CIDSet", found, with)
	}
	if _, with := descriptorCIDSets(t, out, "Roboto-Regular"); with == 0 {
		t.Error("the OCR layer removed the /CIDSet of a font it did not draw")
	}
}
