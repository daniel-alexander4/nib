package pdfops

import (
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// Rule 5 of ContentDigest (ADR-080): what it closes, one axis per row, each asserted under BOTH rules.
//
// Each row is a pair of documents that a reader shows differently (or, for /pending 720, identically) and the
// assertion is two-sided: rule 5 must answer as the reader does, and rule 4 must still answer as it was MEASURED
// to — because rule 4 is the legacy arm every v4 record is checked by, and a row whose rule-4 answer moved is a
// v4 record that now reads as tampering. The rule-4 half is also what keeps each row honest: a mutation rule 4
// already saw would pass rule 5's half for a reason that has nothing to do with this change.

// digestsUnder returns the document's digest under rule 4 and rule 5.
func digestsUnder(t *testing.T, pdf []byte) (v4, v5 string) {
	t.Helper()
	v4, err := ContentDigestAt(pdf, legacyContentDigestVersion)
	if err != nil {
		t.Fatalf("rule 4: %v", err)
	}
	v5, err = ContentDigestAt(pdf, ContentDigestVersion)
	if err != nil {
		t.Fatalf("rule 5: %v", err)
	}
	return v4, v5
}

func mustMutate(t *testing.T, pdf []byte, fn func(*model.Context) error) []byte {
	t.Helper()
	out, err := writeMutated(pdf, fn)
	if err != nil {
		t.Fatalf("mutating: %v", err)
	}
	return out
}

func pagesNode(ctx *model.Context) (types.Dict, error) {
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		return nil, err
	}
	return ctx.XRefTable.DereferenceDict(root["Pages"])
}

// TestRuleFiveSeesWhatTheReaderSees — /pending 578 (inherited page attributes) and /pending 616 (the catalog's
// behaviour and page /UserUnit, /AA): each mutation changes what a reader shows or does, and rule 4 did not see it.
func TestRuleFiveSeesWhatTheReaderSees(t *testing.T) {
	three, err := testpdf.Text("clause one", "clause two", "clause three")
	if err != nil {
		t.Fatal(err)
	}
	// The base hoists every page's /MediaBox and /Rotate to the /Pages node, so the inherited rows below change
	// a value the pages INHERIT — the shape 578 names — rather than one written on the page.
	base := mustMutate(t, three, func(ctx *model.Context) error {
		pn, err := pagesNode(ctx)
		if err != nil {
			return err
		}
		pn["MediaBox"] = types.NewNumberArray(0, 0, 612, 792)
		pn["Rotate"] = types.Integer(0)
		for p := 1; p <= ctx.PageCount; p++ {
			d, _, _, err := ctx.PageDict(p, false)
			if err != nil {
				return err
			}
			delete(d, "MediaBox")
			delete(d, "Rotate")
			delete(d, "CropBox")
		}
		return nil
	})
	b4, b5 := digestsUnder(t, base)

	jsAction := func(js string) types.Dict {
		return types.Dict{"S": types.Name("JavaScript"), "JS": types.StringLiteral(js)}
	}
	onCatalog := func(fn func(root types.Dict, ctx *model.Context) error) func(*model.Context) error {
		return func(ctx *model.Context) error {
			root, err := ctx.XRefTable.Catalog()
			if err != nil {
				return err
			}
			return fn(root, ctx)
		}
	}
	onPage1 := func(fn func(d types.Dict)) func(*model.Context) error {
		return func(ctx *model.Context) error {
			d, _, _, err := ctx.PageDict(1, false)
			if err != nil {
				return err
			}
			fn(d)
			return nil
		}
	}
	for _, c := range []struct {
		name, item, why string
		mut             func(*model.Context) error
	}{
		{"/Rotate on the /Pages node", "578", "turns every page beneath it",
			func(ctx *model.Context) error {
				pn, err := pagesNode(ctx)
				if err == nil {
					pn["Rotate"] = types.Integer(90)
				}
				return err
			}},
		{"/MediaBox on the /Pages node", "578", "resizes every page beneath it",
			func(ctx *model.Context) error {
				pn, err := pagesNode(ctx)
				if err == nil {
					pn["MediaBox"] = types.NewNumberArray(0, 0, 200, 200)
				}
				return err
			}},
		{"/CropBox on the /Pages node", "578", "excises what every page beneath it shows",
			func(ctx *model.Context) error {
				pn, err := pagesNode(ctx)
				if err == nil {
					pn["CropBox"] = types.NewNumberArray(0, 0, 50, 50)
				}
				return err
			}},
		{"catalog /OCProperties", "616", "hides optional content (/D /OFF) without touching a page",
			onCatalog(func(root types.Dict, ctx *model.Context) error {
				ocg, err := ctx.IndRefForNewObject(types.Dict{"Type": types.Name("OCG"), "Name": types.StringLiteral("terms")})
				if err != nil {
					return err
				}
				root["OCProperties"] = types.Dict{"OCGs": types.Array{*ocg},
					"D": types.Dict{"OFF": types.Array{*ocg}}}
				return nil
			})},
		{"catalog /OpenAction", "616", "runs on opening",
			onCatalog(func(root types.Dict, _ *model.Context) error {
				root["OpenAction"] = jsAction("app.alert('opened')")
				return nil
			})},
		{"catalog /AA", "616", "runs before closing",
			onCatalog(func(root types.Dict, _ *model.Context) error {
				root["AA"] = types.Dict{"WC": jsAction("app.alert('closing')")}
				return nil
			})},
		{"/Names /JavaScript", "616", "adds document-level script",
			onCatalog(func(root types.Dict, ctx *model.Context) error {
				names, _ := ctx.XRefTable.DereferenceDict(root["Names"])
				if names == nil {
					names = types.Dict{}
					root["Names"] = names
				}
				names["JavaScript"] = types.Dict{"Names": types.Array{types.StringLiteral("a"), jsAction("app.alert(1)")}}
				return nil
			})},
		{"page /UserUnit", "616", "scales every unit on the page",
			onPage1(func(d types.Dict) { d["UserUnit"] = types.Float(4) })},
		{"page /AA", "616", "runs when the page opens",
			onPage1(func(d types.Dict) { d["AA"] = types.Dict{"O": jsAction("app.alert('page')")} })},
	} {
		t.Run(c.name, func(t *testing.T) {
			a4, a5 := digestsUnder(t, mustMutate(t, base, c.mut))
			if a5 == b5 {
				t.Errorf("rule %d is unchanged after %s — it %s (/pending %s)", ContentDigestVersion, c.name, c.why, c.item)
			}
			if a4 != b4 {
				t.Errorf("rule 4 MOVED after %s, and it was measured not to — either the row mutates something rule 4 "+
					"already covered (so it proves nothing about rule 5), or the legacy arm is no longer rule 4", c.name)
			}
		})
	}
}

// TestRuleFiveReadsADividedPageAsItsReaderDoes — /pending 718: `[… (A) Tj, ET …]` shows the text and the single
// stream `… (A) TjET …` does not (`TjET` is one unknown operator). Rule 4 hashed pdfcpu's bare join, under which the
// two are the same bytes; rule 5 hashes `pdfread.PageContent`'s.
func TestRuleFiveReadsADividedPageAsItsReaderDoes(t *testing.T) {
	divided, _, err := testpdf.SplitContents("a divided page", testpdf.JoinRegular)
	if err != nil {
		t.Fatal(err)
	}
	fused := mustMutate(t, divided, func(ctx *model.Context) error {
		d, _, _, err := ctx.PageDict(1, false)
		if err != nil {
			return err
		}
		bare, err := pdfread.PageContentAsPdfcpu(ctx, d, 1)
		if err != nil {
			return err
		}
		sd, err := ctx.NewStreamDictForBuf(bare)
		if err != nil {
			return err
		}
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		d["Contents"] = *ref
		return nil
	})
	d4, d5 := digestsUnder(t, divided)
	f4, f5 := digestsUnder(t, fused)
	if d4 != f4 {
		t.Errorf("setup: rule 4 tells the divided page from its fused re-join, so this pair does not reproduce 718")
	}
	if d5 == f5 {
		t.Errorf("rule %d digests the divided page and its fused re-join alike (%s), though one shows its text and "+
			"the other does not (/pending 718)", ContentDigestVersion, d5)
	}
}

// TestRuleFiveDoesNotHashAReachedPagesEncoding — /pending 720 on a generated document, so a fresh clone without the
// real-producer corpus `TestANoOpWalkKeepsTheDigest` reads still asserts it: an annotation whose /P names its page,
// and page 1's content re-stored UNCOMPRESSED — the same decoded bytes, a different /Filter and /Length.
func TestRuleFiveDoesNotHashAReachedPagesEncoding(t *testing.T) {
	two, err := testpdf.Text("clause one", "clause two")
	if err != nil {
		t.Fatal(err)
	}
	noted := mustMutate(t, two, func(ctx *model.Context) error {
		ref, err := ctx.PageDictIndRef(1)
		if err != nil {
			return err
		}
		d, _, _, err := ctx.PageDict(1, false)
		if err != nil {
			return err
		}
		d["Annots"] = types.Array{types.Dict{"Type": types.Name("Annot"), "Subtype": types.Name("Text"),
			"Rect": types.NewNumberArray(10, 10, 30, 30), "Contents": types.StringLiteral("see clause two"), "P": *ref}}
		return nil
	})
	var filterBefore types.Object
	reencoded := mustMutate(t, noted, func(ctx *model.Context) error {
		d, _, _, err := ctx.PageDict(1, false)
		if err != nil {
			return err
		}
		content, err := pdfread.PageContent(ctx, d, 1)
		if err != nil {
			return err
		}
		if old, _, _ := ctx.XRefTable.DereferenceStreamDict(d["Contents"]); old != nil {
			filterBefore = old.Dict["Filter"]
		}
		sd, err := ctx.NewStreamDictForBuf(content)
		if err != nil {
			return err
		}
		sd.FilterPipeline = nil
		delete(sd.Dict, "Filter")
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		d["Contents"] = *ref
		return nil
	})
	if filterBefore == nil {
		t.Fatal("setup: page 1's content had no /Filter, so storing it uncompressed changes no encoding")
	}
	n4, n5 := digestsUnder(t, noted)
	r4, r5 := digestsUnder(t, reencoded)
	if n4 == r4 {
		t.Errorf("setup: rule 4 held over the re-encode, so this pair does not reproduce 720")
	}
	if n5 != r5 {
		t.Errorf("rule %d moved %s → %s over a re-encode that changed no content, because an annotation names the "+
			"page (/pending 720)", ContentDigestVersion, n5, r5)
	}
}
