package pdfops

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// Rule 6 of ContentDigest (ADR-143, /pending 691 R5-5 and R5-6): one row per kind of page resource rules 1 to 5
// did not follow, each a pair of documents that differ in THAT resource and in nothing else.
//
// Every row is two-sided, as rule 5's rows were against rule 4:
//
//   - rule 5 must digest the pair ALIKE. That is rule 5's measured reading, and the legacy arm must keep it — a
//     row whose rule-5 answer moved is a rule-5 record that now reads as a changed document. It is also what
//     keeps the row honest: a difference rule 5 already saw would pass rule 6's half for a reason that has
//     nothing to do with rule 6.
//   - rule 6 must tell the pair APART, because a reader draws them differently.

// allKindsObjects is a one-page document whose content uses one resource of each kind rule 6 newly follows. The
// page content (object 4) is the same bytes in every variant: only the object a row names is replaced.
//
// With `inherited` the whole /Resources dictionary sits on the /Pages node and the page has none — the shape
// many producers write. It does NOT separate "the effective resources" from "the page's own": the digest's
// optimized read copies an inherited dictionary down onto the page (measured: 6 kinds on the page after
// `ReadOptimized`, 0 after `Validated`), so a rule reading either sees the same thing here. The variant holds
// that every row still moves when the resources arrive that way.
func allKindsObjects(inherited bool) map[int]string {
	const content = "/GS0 gs BT /F1 24 Tf 20 150 Td (rent is 1000) Tj ET\n" +
		"/GS1 gs 0 0 50 50 re f\n" +
		"/Pattern cs /P0 scn 60 0 50 50 re f\n" +
		"/CS0 cs 1 sc 120 0 50 50 re f\n" +
		"/OC /MC0 BDC 0 60 50 50 re f EMC\n" +
		"/Sh0 sh\n"
	const res = "/Resources << /Font << /F1 5 0 R >> /ExtGState << /GS0 6 0 R /GS1 7 0 R >> " +
		"/Pattern << /P0 9 0 R >> /ColorSpace << /CS0 10 0 R >> /Shading << /Sh0 11 0 R >> " +
		"/Properties << /MC0 12 0 R >> >>"
	stream := func(dict, body string) string {
		return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(body), body)
	}
	pages, page := "", " "+res
	if inherited {
		pages, page = " "+res, ""
	}
	return map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [13 0 R] /D << /Order [13 0 R] >> >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1" + pages + " >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R" + page + " >>",
		4: stream("", content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		6: "<< /Type /ExtGState /ca 1 >>",
		7: "<< /Type /ExtGState /SMask << /Type /Mask /S /Luminosity /G 8 0 R >> >>",
		8: stream("/Type /XObject /Subtype /Form /BBox [0 0 200 200] /Group << /S /Transparency /CS /DeviceGray >>",
			"1 g 0 0 200 200 re f"),
		9: stream("/Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 1 1] /XStep 1 /YStep 1 "+
			"/Resources << >>", "1 0 0 rg 0 0 1 1 re f"),
		10: "[/Indexed /DeviceRGB 1 <000000FFFFFF>]",
		11: "<< /ShadingType 2 /ColorSpace /DeviceGray /Coords [0 0 200 0] " +
			"/Function << /FunctionType 2 /Domain [0 1] /C0 [0] /C1 [1] /N 1 >> >>",
		12: "<< /Type /OCMD /OCGs 13 0 R /P /AnyOn >>",
		13: "<< /Type /OCG /Name (terms) >>",
	}
}

// ruleSixRows are the five kinds the grill named and the one found while building them. `obj` is the one object
// replaced; `with` rewrites its body.
var ruleSixRows = []struct {
	name, draws string
	obj         int
	with        func(body string) string
}{
	{"ExtGState /ca", "the text is painted fully transparent, and no stream is involved at all", 6,
		swap("/ca 1", "/ca 0")},
	{"ExtGState /SMask /G", "the soft mask's form goes from all-showing to all-hiding", 8,
		swap("1 g 0 0 200 200 re f", "0 g 0 0 200 200 re f")},
	{"page Pattern", "the tile that fills a region changes colour", 9,
		swap("1 0 0 rg 0 0 1 1 re f", "0 0 1 rg 0 0 1 1 re f")},
	{"page ColorSpace palette", "the Indexed palette's entry 1 goes from white to black", 10,
		swap("<000000FFFFFF>", "<000000000000>")},
	{"page Shading", "the gradient's end colour goes from white to black", 11,
		swap("/C1 [1]", "/C1 [0]")},
	{"page Properties", "the optional-content membership the marked content names turns it off", 12,
		swap("/P /AnyOn", "/P /AllOff")},
}

func swap(old, new string) func(string) string {
	return func(body string) string {
		if n := bytes.Count([]byte(body), []byte(old)); n != 1 {
			panic(fmt.Sprintf("setup: %q occurs %d times in the object body, want exactly once", old, n))
		}
		return string(bytes.Replace([]byte(body), []byte(old), []byte(new), 1))
	}
}

// pageOneContent is page 1's content as the digest reads it.
func pageOneContent(t *testing.T, pdf []byte) []byte {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	d, _, _, err := ctx.PageDict(context.Background(), 1, false)
	if err != nil || d == nil {
		t.Fatalf("page 1: %v", err)
	}
	c, err := pdfread.PageContent(ctx, d, 1)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRuleSixFollowsEveryKindOfResourceAPageBinds(t *testing.T) {
	// Rule 5 is retired (D1, P02.S08): its blind half cannot be asserted because nothing computes it. These rows keep
	// the SEEING half, under the rule this build computes, which is rule 6's axes read by pdfcpu v0.16.x.
	if ContentDigestVersion != 7 {
		t.Fatalf("these rows pin the every-kind axes under rule 7; this build computes rule %d, so the rows need "+
			"re-reading against the rule they name", ContentDigestVersion)
	}
	for _, inherited := range []bool{false, true} {
		where := "on the page"
		if inherited {
			where = "inherited from the /Pages node"
		}
		base := allKindsObjects(inherited)
		basePDF := testpdf.Assemble(base)
		before := digestsUnder(t, basePDF)
		content := pageOneContent(t, basePDF)
		if len(content) == 0 {
			t.Fatal("setup: the fixture's page draws nothing")
		}
		for _, row := range ruleSixRows {
			t.Run(row.name+", "+where, func(t *testing.T) {
				objs := map[int]string{}
				for n, body := range base {
					objs[n] = body
				}
				objs[row.obj] = row.with(base[row.obj])
				changed := testpdf.Assemble(objs)
				// The stimulus: the page's content is the same bytes, so whatever moves a digest is the resource.
				if !bytes.Equal(pageOneContent(t, changed), content) {
					t.Fatal("setup: the page content moved, so this pair differs in more than the resource")
				}
				after := digestsUnder(t, changed)
				if after[0] == before[0] {
					t.Errorf("rule %d is unchanged over a change to %s (%s) — %s, with the content stream "+
						"byte-identical", ContentDigestVersion, row.name, where, row.draws)
				}
			})
		}
	}
}

// TestRuleSixTakesItsKindsFromTheDictionary — rule 6 has no list of kinds, so what it follows is what the read
// hands it. Two facts about that, pinned:
//
//   - an entry that is not a dictionary (/ProcSet is an array) is hashed as the object it is;
//   - a key outside the eight ISO 32000-1 Table 33 names never ARRIVES: pdfcpu's validator deletes it from the
//     resource dictionary (`validateResourceDict`), and the digest hashes the validator's reading (as it does a
//     font the validator removed, ADR-129). No content operator can name such a key, so no reader draws by it.
//     It is pinned so that a pdfcpu that stops deleting it shows up here as a moved digest, not in the field.
func TestRuleSixTakesItsKindsFromTheDictionary(t *testing.T) {
	variant := func(old, new string) []byte {
		objs := allKindsObjects(false)
		objs[3] = swap(old, new)(objs[3])
		return testpdf.Assemble(objs)
	}
	const at = "/Properties << /MC0 12 0 R >>"
	for _, c := range []struct {
		name, a, b string
		seen       bool
	}{
		{"a kind that is not a dictionary", at + " /ProcSet [/PDF /Text]", at + " /ProcSet [/PDF]", true},
		{"a key the validator deletes", at, at + " /NibUnknownKind << /A 6 0 R >>", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, b := digestsUnder(t, variant(at, c.a)), digestsUnder(t, variant(at, c.b))
			if (a[0] != b[0]) != c.seen {
				t.Errorf("rule %d tells the pair apart: %v, want %v", ContentDigestVersion, a[0] != b[0], c.seen)
			}
		})
	}
}

// TestRuleSixHoldsAcrossARewrite — the property the digest exists for (a byte hash is not stable across pdfcpu's
// writer), asserted over the kinds rule 6 newly follows: the all-kinds document, written out by pdfcpu with every
// object renumbered and re-encoded, digests as it did. The stimulus is asserted first.
func TestRuleSixHoldsAcrossARewrite(t *testing.T) {
	for _, inherited := range []bool{false, true} {
		src := testpdf.Assemble(allKindsObjects(inherited))
		for name, rewrite := range map[string]func([]byte) ([]byte, error){
			"a no-op write": func(b []byte) ([]byte, error) {
				return writeMutated(b, func(*model.Context) error { return nil })
			},
			"Optimize": Optimize,
		} {
			out, err := rewrite(src)
			if err != nil {
				t.Fatalf("%s (inherited %v): %v", name, inherited, err)
			}
			if bytes.Equal(out, src) {
				t.Fatalf("setup: %s left the bytes alone, so nothing was rewritten", name)
			}
			before, after := digestsUnder(t, src), digestsUnder(t, out)
			for i, rule := range readableRules {
				if before[i] != after[i] {
					t.Errorf("%s (inherited %v) moved rule %d: %s → %s", name, inherited, rule,
						short16(before[i]), short16(after[i]))
				}
			}
		}
	}
}
