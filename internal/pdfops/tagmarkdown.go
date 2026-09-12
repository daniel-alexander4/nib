package pdfops

import (
	"fmt"

	"nib/internal/contentstream"
	"nib/mdpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Tagging a Markdown document from its own AST — `PLAN-accessibility.md` P06.S02, D4's
// observed-exact tier.
//
// P05.S04's emitter wrapped each page in one `/Div`, which was the only honest claim available to
// something that knew nothing about the content. `mdpdf` knows exactly: P06.S01 carries the heading
// level, the list depth and the block boundary out of the goldmark AST. This is the same mechanism
// pointed at that.
//
// # The correspondence this rests on, and where it is checked
//
// `mdpdf`'s `spec()` emits one pdfcpu text entry per laid-out run, in order, and pdfcpu writes them
// as text-drawing operators in that order — so **the Nth run is the Nth text operator on the page**.
// Measured before this was built: a document of nine runs produced nine operators.
//
// It is checked on every run rather than trusted: `tagMarkdown` refuses when the counts disagree.
// A silent mismatch would attach every element to the wrong content from the point of divergence,
// which is a document that is confidently and invisibly wrong — the failure mode ADR-031 exists for.

// structTypeFor maps a Markdown role to the PDF structure type that describes it.
//
// # Why a code block is `/P` and not `/Code`
//
// `/Code` is an INLINE structure type in ISO 32000-1 — a span of code inside a paragraph, not a
// block of it. Using it for a block would be a type error a reader can act on. A richer mapping (a
// `/P` holding `/Code` spans, or a custom type role-mapped like LibreOffice's `Preformatted Text`)
// belongs with the structure editor in P09; `/P` says *a block of text*, which is true.
func structTypeFor(r mdpdf.Role) string {
	switch r.Kind {
	case mdpdf.RoleHeading:
		lvl := r.Level
		if lvl < 1 {
			lvl = 1
		}
		if lvl > 6 {
			lvl = 6
		}
		return fmt.Sprintf("H%d", lvl)
	case mdpdf.RoleListItem:
		return "LBody"
	case mdpdf.RoleMarker:
		return "Lbl"
	case mdpdf.RoleQuote:
		return "P"
	case mdpdf.RoleCode:
		return "P"
	}
	return "P"
}

// tagMarkdown converts Markdown and tags the result from its own structure.
func tagMarkdown(md []byte, base *mdpdf.Faces, fallbacks []mdpdf.Font) ([]byte, error) {
	pdf, st, err := mdpdf.ConvertStructured(md, base, fallbacks)
	if err != nil {
		return nil, err
	}
	// **The FOURTH door that embeds a font of nib's**, and it was missed until veraPDF said so:
	// this reaches `mdpdf` directly rather than through `ConvertDocToPDF`, so P04.S02's `/CIDSet`
	// tail did not run and the tagged document failed ua1 7.21.4.2 t2 — a clause the untagged one
	// passes. `TestNothingNibEmbedsAFontIntoCarriesACIDSet` now drives this door too, which is what
	// its own comment said it was for: *a list of call sites passes when a third door is added and
	// not routed*.
	pdf = embeddedFontsAreHonest(pdf)
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		live := map[int]bool{}
		for p := 1; p <= ctx.PageCount; p++ {
			if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
				live[ir.ObjectNumber.Value()] = true
			}
		}
		tree, terr := ensureStructTree(ctx, live)
		if terr != nil {
			return terr
		}
		for p := 1; p <= ctx.PageCount && p <= len(st.Pages); p++ {
			if err := tagOnePage(ctx, tree, p, st.Pages[p-1]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Both halves and the tier, through the one door (ADR-009). D4's tier here is exact: the
	// structure came from `mdpdf`'s own AST.
	claimed, ok, err := claimTagging(out, sourceExact)
	if err != nil {
		return nil, err
	}
	// This door fails rather than falling back, because its whole product IS the tagged document —
	// an untagged `mdpdf` render is what `ConvertWithFaces` is for, and a caller that asked for the
	// tagged one should not silently get the other.
	if !ok {
		return nil, orphanedClaimError("tagMarkdown", out)
	}
	return claimed, nil
}

// tagOnePage brackets each run of one page and builds the elements that describe them.
//
// The runs arrive in draw order with a block ordinal; consecutive runs sharing an ordinal are one
// element with several MCIDs, and a change of ordinal starts a new one. List items additionally
// group: a run of `Lbl`/`LBody` blocks at one depth becomes one `L` holding an `LI` per pair.
func tagOnePage(ctx *model.Context, tree *structTree, pageNr int, roles []mdpdf.Role) error {
	d, _, _, err := ctx.PageDict(pageNr, false)
	if err != nil || d == nil {
		return fmt.Errorf("pdfops: page %d does not resolve: %w", pageNr, err)
	}
	src, cerr := ctx.PageContent(d, pageNr)
	if cerr != nil {
		return cerr
	}
	spans := textOperatorSpans(src)

	// **The correspondence, checked.** A mismatch attaches every element from the point of
	// divergence to the wrong content, and the document looks entirely correct.
	if len(spans) != len(roles) {
		return fmt.Errorf("pdfops: page %d draws %d text operator(s) and the structure describes "+
			"%d run(s) — refusing to tag content by position when the two do not correspond",
			pageNr, len(spans), len(roles))
	}
	if len(spans) == 0 {
		return nil
	}

	edit := contentstream.NewEdit(src)
	var listRef, itemRef *types.IndirectRef
	listLevel := 0

	for i := 0; i < len(roles); {
		// Every run of this block.
		j := i
		for j < len(roles) && roles[j].Block == roles[i].Block {
			j++
		}
		role := roles[i]

		// Lists: an `L` wraps consecutive items at one depth, and each `Lbl`+`LBody` pair is an
		// `LI`. A depth change or a non-list block closes the list.
		inList := role.Kind == mdpdf.RoleMarker || role.Kind == mdpdf.RoleListItem
		if !inList || (listRef != nil && listLevel != role.Level) {
			listRef, itemRef, listLevel = nil, nil, 0
		}
		var parent *types.IndirectRef
		if inList {
			if listRef == nil {
				lr, lerr := addGroupingElement(ctx, tree, "L", nil)
				if lerr != nil {
					return lerr
				}
				listRef, listLevel = lr, role.Level
			}
			if role.Kind == mdpdf.RoleMarker || itemRef == nil {
				ir, ierr := addGroupingElement(ctx, tree, "LI", listRef)
				if ierr != nil {
					return ierr
				}
				itemRef = ir
			}
			parent = itemRef
		}

		mcid, elemRef, aerr := addMarkedElementUnder(ctx, tree, pageNr, structTypeFor(role), parent)
		if aerr != nil {
			return aerr
		}
		// **The element owns every run of its block.** A paragraph that wraps to four lines is four
		// runs and ONE element with four MCIDs — which is the distinction P06.S01's `Block` ordinal
		// exists to make, and getting it wrong turns every wrapped paragraph into one element per
		// line.
		for k := i; k < j; k++ {
			id := mcid
			if k > i {
				extra, eerr := addMCIDTo(ctx, tree, pageNr, *elemRef)
				if eerr != nil {
					return eerr
				}
				id = extra
			}
			edit.InsertBefore(spans[k].start, []byte(fmt.Sprintf("/%s <</MCID %d>> BDC\n",
				structTypeFor(role), id)))
			edit.InsertBefore(spans[k].end, []byte("\nEMC"))
		}
		i = j
	}

	edited, eerr := edit.Apply()
	if eerr != nil {
		return eerr
	}
	return setPageContent(ctx, d, edited)
}
