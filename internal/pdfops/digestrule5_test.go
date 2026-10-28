package pdfops

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// What rule 5 of ContentDigest closed (ADR-080), one axis per row, asserted under EVERY rule this build computes —
// rule 5, the legacy arm, and the current one, which keeps all of it.
//
// Each row is a pair of documents that a reader shows differently (or, for /pending 720, identically), and each
// readable rule must answer as the reader does.
//
// **These rows were two-sided until rule 4 was retired (ADR-143)**: rule 4 had to answer as it was MEASURED to —
// blind to each row — which was also what kept a row honest, since a mutation rule 4 already saw would pass for
// a reason that had nothing to do with rule 5. That half was measured green for as long as rule 4 was computed
// and cannot be asserted now that nothing computes it. Rule 6's own rows (digestrule6_test.go) are two-sided
// against rule 5 in the same way.

// readableRules is every rule this build computes, oldest first.
var readableRules = []int{ContentDigestVersion}

// digestsUnder returns the document's digest under each of readableRules, in that order.
func digestsUnder(t *testing.T, pdf []byte) []string {
	t.Helper()
	out := make([]string, 0, len(readableRules))
	for _, rule := range readableRules {
		d, err := ContentDigestAt(pdf, rule)
		if err != nil {
			t.Fatalf("rule %d: %v", rule, err)
		}
		out = append(out, d)
	}
	return out
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
// behaviour and page /UserUnit, /AA): each mutation changes what a reader shows or does.
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
			d, _, _, err := ctx.PageDict(context.Background(), p, false)
			if err != nil {
				return err
			}
			delete(d, "MediaBox")
			delete(d, "Rotate")
			delete(d, "CropBox")
		}
		return nil
	})
	before := digestsUnder(t, base)

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
			d, _, _, err := ctx.PageDict(context.Background(), 1, false)
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
			after := digestsUnder(t, mustMutate(t, base, c.mut))
			for i, rule := range readableRules {
				if after[i] == before[i] {
					t.Errorf("rule %d is unchanged after %s — it %s (/pending %s)", rule, c.name, c.why, c.item)
				}
			}
		})
	}
}

// TestRuleFiveReadsADividedPageAsItsReaderDoes — /pending 718: `[… (A) Tj, ET …]` shows the text and the single
// stream `… (A) TjET …` does not (`TjET` is one unknown operator). Rule 4 hashed pdfcpu's bare join, under which the
// two are the same bytes; rules 5 and 6 hash `pdfread.PageContent`'s. The fused document is built from pdfcpu's own
// join (`ctx.PageContent`), which is the reading that fuses.
func TestRuleFiveReadsADividedPageAsItsReaderDoes(t *testing.T) {
	divided, _, err := testpdf.SplitContents("a divided page", testpdf.JoinRegular)
	if err != nil {
		t.Fatal(err)
	}
	fused := mustMutate(t, divided, func(ctx *model.Context) error {
		d, _, _, err := ctx.PageDict(context.Background(), 1, false)
		if err != nil {
			return err
		}
		bare, err := ctx.PageContent(d, 1)
		if err != nil {
			return err
		}
		door, err := pdfread.PageContent(ctx, d, 1)
		if err != nil {
			return err
		}
		if bytes.Equal(bare, door) {
			return fmt.Errorf("setup: pdfcpu's join and the door's are the same bytes, so nothing is fused")
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
	d, f := digestsUnder(t, divided), digestsUnder(t, fused)
	for i, rule := range readableRules {
		if d[i] == f[i] {
			t.Errorf("rule %d digests the divided page and its fused re-join alike (%s), though one shows its text "+
				"and the other does not (/pending 718)", rule, d[i])
		}
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
		ref, err := ctx.PageDictIndRef(context.Background(), 1)
		if err != nil {
			return err
		}
		d, _, _, err := ctx.PageDict(context.Background(), 1, false)
		if err != nil {
			return err
		}
		d["Annots"] = types.Array{types.Dict{"Type": types.Name("Annot"), "Subtype": types.Name("Text"),
			"Rect": types.NewNumberArray(10, 10, 30, 30), "Contents": types.StringLiteral("see clause two"), "P": *ref}}
		return nil
	})
	var filterBefore types.Object
	reencoded := mustMutate(t, noted, func(ctx *model.Context) error {
		d, _, _, err := ctx.PageDict(context.Background(), 1, false)
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
	n, r := digestsUnder(t, noted), digestsUnder(t, reencoded)
	for i, rule := range readableRules {
		if n[i] != r[i] {
			t.Errorf("rule %d moved %s → %s over a re-encode that changed no content, because an annotation names "+
				"the page (/pending 720)", rule, n[i], r[i])
		}
	}
}
