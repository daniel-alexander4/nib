package pdfops

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// subsetNames is every tagged `/FontName` in pdf's descriptors, sorted as found.
func subsetNames(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	var out []string
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
		if n, _ := d["FontName"].(types.Name); strings.Contains(string(n), "+") {
			out = append(out, string(n))
		}
	}
	return out
}

// TestEveryDoorThatEmbedsAFaceIsAFunctionOfItsInputs — /pending 757. Each door that embeds a face nib
// supplied is run TWICE on the same input, and the two outputs must have one ContentDigest.
//
// pdfcpu draws a subset tag from the clock (`subFontPrefix`), `ContentDigest` reads the font
// dictionaries, and so on base every one of these doors produced a different digest on every call over
// byte-identical font programs and content streams — and the ceremony readme, a page with no inputs at
// all, could not be reproduced from them. The census reads the OUTPUT per door, as the CIDSet census
// does, so a new door not routed through `ownFacesOf` goes red here rather than passing a list.
func TestEveryDoorThatEmbedsAFaceIsAFunctionOfItsInputs(t *testing.T) {
	body, _, embedded := AuthoredTextFaces()
	if !embedded {
		t.Skip("SKIP (not a pass): the embedded faces are unavailable here, so no door embeds a subset")
	}
	for _, c := range []struct {
		name string
		make func() ([]byte, error)
	}{
		{"authored Markdown", func() ([]byte, error) { return ConvertDocToPDF([]byte(p4Markdown), ".md") }},
		{"a CreateFromJSON spec naming an embedded face", func() ([]byte, error) {
			return CreateFromJSON([]byte(`{"pages":{"1":{"content":{"text":[{"value":"a page",` +
				`"anchor":"TopLeft","position":[72,720],"font":{"name":"` + body + `","size":12}}]}}}}`))
		}},
		{"OCR text layer", func() ([]byte, error) {
			return StampTextLayer(threePagePDF(t),
				[]Word{{Page: 1, Rect: [4]float64{20, 40, 70, 50}, Text: "hello world"}}, "eng")
		}},
		{"stamped fields", func() ([]byte, error) {
			o, _, err := StampFields(threePagePDF(t),
				[]Field{{Page: 1, Rect: [4]float64{50, 400, 300, 420}, Text: "Replaced text", Font: "Times-Roman", Size: 12}})
			return o, err
		}},
		{"watermark", func() ([]byte, error) { return StampWatermark(threePagePDF(t), "DRAFT", WatermarkStyle{}) }},
		{"page numbers", func() ([]byte, error) { return StampPageNumbers(threePagePDF(t), PageNumberStyle{}) }},
	} {
		var digests, tags [2]string
		for i := range digests {
			out, err := c.make()
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			names := subsetNames(t, out)
			if len(names) == 0 {
				t.Errorf("%s: the output carries no subset font, so \"deterministic\" is vacuous — this door "+
					"has stopped embedding a subset of a face", c.name)
			}
			tags[i] = strings.Join(names, ",")
			if digests[i], err = ContentDigest(out); err != nil {
				t.Fatalf("%s: digest: %v", c.name, err)
			}
		}
		if digests[0] != digests[1] {
			t.Errorf("%s: two runs on the same input have different ContentDigests (%.12s… and %.12s…); "+
				"subset tags %q and %q.\n\tpdfcpu seeds its subset tag from the clock, and ContentDigest reads "+
				"the font dictionaries, so the door must derive the tag from the subset — see retagSubsetsOf.",
				c.name, digests[0], digests[1], tags[0], tags[1])
		}
	}
}

// TestTwoSubsetsOfOneFaceKeepDistinctTags is the other half of the rule: a reader may take two fonts
// with one tag for one font, so the derived tag must separate two DIFFERENT subsets of one face — a
// constant tag would pass the determinism census above and fail this.
func TestTwoSubsetsOfOneFaceKeepDistinctTags(t *testing.T) {
	body, _, embedded := AuthoredTextFaces()
	if !embedded {
		t.Skip("SKIP (not a pass): the embedded faces are unavailable here")
	}
	page := func(text string) string {
		out, err := CreateFromJSON([]byte(`{"pages":{"1":{"content":{"text":[{"value":"` + text + `",` +
			`"anchor":"TopLeft","position":[72,720],"font":{"name":"` + body + `","size":12}}]}}}}`))
		if err != nil {
			t.Fatal(err)
		}
		names := subsetNames(t, out)
		if len(names) != 1 {
			t.Fatalf("want one subset font, got %q", names)
		}
		if _, ok := subsetFace(names[0]); !ok {
			t.Fatalf("%q is not a six-uppercase-letter subset tag and a face", names[0])
		}
		return names[0]
	}
	a, b := page("abc"), page("xyz")
	if a == b {
		t.Errorf("two different subsets of %s carry one tag, %q — the tag must be derived from the program", body, a)
	}
}
