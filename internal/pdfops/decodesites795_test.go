package pdfops

import (
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 795`, the five decodes the door guard pinned without judging. Four read a stream the DOCUMENT sized and
// inflated whatever it declared; each now reads through `formWalkBudget.formContent`. The fifth (stamprotation) is
// exempt at its site. Each case is one such stream inflating to `decodeBomb`, read by the function that held the
// uncapped decode: it must refuse as it refuses an unreadable stream, having allocated under `mostToRefuse`.
func TestADocumentSizedStreamIsInflatedOnlyWithinABudget(t *testing.T) {
	bomb := func(extra string) string { return optStream(flatedSpaces(decodeBomb), extra+" /Filter /FlateDecode") }
	doc := func(catalogExtra, pageRes string, objs map[int]string) *model.Context {
		objs[1] = "<< /Type /Catalog /Pages 2 0 R " + catalogExtra + " >>"
		objs[2] = "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"
		objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << " + pageRes + " >> /Contents 4 0 R >>"
		objs[4] = optStream("0 0 1 1 re f", "")
		if strings.Contains(pageRes, "/XObject") {
			objs[4] = optStream("/Fm1 Do", "")
		}
		return optimizedCtx(t, assembleFixture(objs))
	}
	objAt := func(ctx *model.Context, nr int) types.Dict {
		d, err := ctx.DereferenceDict(*types.NewIndirectRef(nr, 0))
		if err != nil || d == nil {
			t.Fatalf("setup: object %d is not a dictionary (%v)", nr, err)
		}
		return d
	}
	const font = "<< /Type /Font /Subtype /Type0 /BaseFont /ABCDEF+Face /Encoding /Identity-H /ToUnicode 9 0 R >>"
	cases := []struct {
		name string
		read func() func() bool // reads the document, and returns the call under test: whether it refused the stream
	}{
		{"the n-up note carry's form (annotcarry placementMatrix)", func() func() bool {
			ctx := doc("", "/XObject << /Fm1 9 0 R >>", map[int]string{9: bomb("/Type /XObject /Subtype /Form /BBox [0 0 10 10]")})
			return func() bool {
				_, err := placementMatrix(ctx, sheetPlacement{sheet: 1, name: "Fm1", cm: identityMatrix}, pageAt(ctx, nil, 1),
					[]byte("x"), newFormWalkBudget(1))
				return err != nil && strings.Contains(err.Error(), "does not decode")
			}
		}},
		{"the ToUnicode of a font named like nib's face (stampwrite fontNotNibs)", func() func() bool {
			ctx := doc("", "/Font << /F1 8 0 R >>", map[int]string{8: font, 9: bomb("")})
			return func() bool {
				return fontNotNibs(ctx, "Face", objAt(ctx, 8)) == "its ToUnicode map does not decode"
			}
		}},
		{"the ToUnicode of a font named like nib's face (stampwrite repairToUnicodeOf)", func() func() bool {
			ctx := doc("", "/Font << /F1 8 0 R >>", map[int]string{8: font, 9: bomb("")})
			return func() bool {
				repairToUnicodeOf(ctx, []string{"Face"})
				return true // it reports nothing: a map it cannot read is left as it was
			}
		}},
		{"the catalog packet read for its title (structcarry documentTitle)", func() func() bool {
			ctx := doc("/Metadata 9 0 R", "", map[int]string{9: bomb("/Type /Metadata /Subtype /XML")})
			return func() bool {
				return documentTitle(ctx, objAt(ctx, 1)) == ""
			}
		}},
		{"a subset's font program (subsettag fontProgramBytes)", func() func() bool {
			ctx := doc("", "", map[int]string{
				8: "<< /Type /FontDescriptor /FontName /ABCDEF+Face /FontFile2 9 0 R >>", 9: bomb("")})
			return func() bool {
				return fontProgramBytes(ctx, objAt(ctx, 8)) == nil
			}
		}},
	}
	for _, c := range cases {
		var refused bool
		call := c.read() // outside the measurement: reading the fixture is not what is under test
		alloc := allocated(func() { refused = call() })
		if !refused {
			t.Errorf("%s: a stream inflating to %d MiB was not refused as an unreadable one is", c.name, decodeBomb>>20)
		}
		if alloc > mostToRefuse {
			t.Errorf("%s: allocated %d MiB over a stream that inflates to %d MiB, want under %d MiB — it decodes "+
				"through formWalkBudget.formContent", c.name, alloc>>20, decodeBomb>>20, mostToRefuse>>20)
		}
	}

	// Control: the same readers still read an ordinary stream, so a refusal above is the budget's and not the fixture's.
	ctx := doc("/Metadata 9 0 R", "", map[int]string{9: optStream(
		`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title><rdf:Alt><rdf:li xml:lang="x-default">Kept`+
			`</rdf:li></rdf:Alt></dc:title></rdf:Description></rdf:RDF></x:xmpmeta>`, "/Type /Metadata /Subtype /XML")})
	if got := documentTitle(ctx, objAt(ctx, 1)); got != "Kept" {
		t.Errorf("control: an ordinary packet's title read %q, want Kept", got)
	}
	ctx = doc("", "", map[int]string{8: "<< /Type /FontDescriptor /FontName /ABCDEF+Face /FontFile2 9 0 R >>",
		9: optStream("program", "")})
	if got := fontProgramBytes(ctx, objAt(ctx, 8)); string(got) != "program" {
		t.Errorf("control: an ordinary font program read %q, want it whole", got)
	}
}
