package pdfops

import (
	"errors"
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

// nestHeadingLevels renumbers the document's headings so each level nests under the one before —
// `/pending 487`.
//
// PDF/UA-1 7.4.2 t1, as veraPDF judges it on its own corpus: the first numbered heading is H1, and a
// descent never skips a level, while a heading may return to any shallower level. Markdown lets an author
// write `#` then `###`, and mapping the source level straight through wrote H1 then H3 — a tree that fails
// the clause whatever the author meant, and nib's own report could not see it until the checker learned the
// rule in the same change. The relative hierarchy is kept: a heading is one level deeper than the nearest
// earlier heading whose SOURCE level is shallower, and H1 when there is none.
//
// Document-wide, because a section runs across pages; a heading block that straddles a page break is
// renumbered identically on both sides, since it pops back to the same parent.
func nestHeadingLevels(pages [][]mdpdf.Role) {
	type open struct{ src, out int }
	var stack []open
	for p := range pages {
		roles := pages[p]
		for i := 0; i < len(roles); {
			j := i
			for j < len(roles) && roles[j].Block == roles[i].Block {
				j++
			}
			if roles[i].Kind == mdpdf.RoleHeading {
				src := roles[i].Level
				for len(stack) > 0 && stack[len(stack)-1].src >= src {
					stack = stack[:len(stack)-1]
				}
				out := 1
				if len(stack) > 0 {
					out = stack[len(stack)-1].out + 1
				}
				stack = append(stack, open{src, out})
				for k := i; k < j; k++ {
					roles[k].Level = out
				}
			}
			i = j
		}
	}
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
	nestHeadingLevels(st.Pages)
	return tagFromRoles(pdf, st.Pages, "tagMarkdown")
}

// TagAuthoredPages tags pages nib drew itself from text it composed — the co-sign readme, the ceremony
// page and the signature pages (`PLAN-ua-coverage.md` P02.S09) — with one role per text run, in draw
// order, per page. The roles are EXACT: the caller built every line and knows what it is, so the tree
// records `Exact` and a graft onto an `Exact` host keeps that tier (ADR-048).
//
// It fails rather than falling back, for `tagMarkdown`'s reason: its whole product is the tagged page.
// A role count that disagrees with the page's text runs is refused by `tagOnePage`, so a layout change
// that adds or drops a run cannot tag the wrong line silently.
//
// Call it AFTER the page's content-language declaration: `declareContentLang` skips a page that is
// already marked, so tagging first would drop the `/Lang` with no error.
func TagAuthoredPages(pdf []byte, pages [][]mdpdf.Role) ([]byte, error) {
	// **Only for a page nib has just drawn, which has no structure.** `tagOnePage` brackets every text
	// run it is given and does not ask whether a run is already marked, so on a document that has a tree
	// it would nest a second MCID inside the first and describe the same words twice.
	if inspectTags(pdf).tree {
		return nil, errAuthoredAlreadyTagged
	}
	return tagFromRoles(pdf, pages, "TagAuthoredPages")
}

// errAuthoredAlreadyTagged is TagAuthoredPages refusing a document that already has a structure tree.
var errAuthoredAlreadyTagged = errors.New("pdfops: TagAuthoredPages tags pages nib has just drawn, and this document already has a structure tree")

// tagFromRoles brackets every page's text runs by the roles given and claims the tree at the exact tier:
// the one tail `tagMarkdown` and `TagAuthoredPages` share (ADR-009).
//
// One role list per page, exactly: a list with no page is a line the caller composed that the document
// does not hold, and a page with no list would be left undescribed under a tree that claims the
// document — so a count mismatch is refused rather than tagging the pages the two happen to share.
func tagFromRoles(pdf []byte, pages [][]mdpdf.Role, door string) ([]byte, error) {
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		if len(pages) != ctx.PageCount {
			return fmt.Errorf("pdfops: %s was given roles for %d pages of a %d-page document", door, len(pages), ctx.PageCount)
		}
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
		for p := 1; p <= ctx.PageCount; p++ {
			if err := tagOnePage(ctx, tree, p, pages[p-1]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Both halves and the tier, through the one door (ADR-009). D4's tier here is exact: the
	// structure came from something that KNOWS it — `mdpdf`'s own AST, or the lines nib composed.
	claimed, ok, err := claimTagging(nil, out, sourceExact)
	if err != nil {
		return nil, err
	}
	// This door fails rather than falling back, because its whole product IS the tagged document —
	// an untagged `mdpdf` render is what `ConvertWithFaces` is for, and a caller that asked for the
	// tagged one should not silently get the other.
	if !ok {
		return nil, orphanedClaimError(door, out)
	}
	return claimed, nil
}

// tagOnePage brackets each run of one page and builds the elements that describe them.
//
// The runs arrive in draw order with a block ordinal; consecutive runs sharing an ordinal are one
// element with several MCIDs, and a change of ordinal starts a new one. List items additionally
// group: a run of `Lbl`/`LBody` blocks at one depth becomes one `L` holding an `LI` per pair.
func tagOnePage(ctx *model.Context, tree *structTree, pageNr int, roles []mdpdf.Role) error {
	d, _, err := tree.page(ctx, pageNr)
	if err != nil {
		return err
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
	// No early return for a page with no text (`/pending 504`): what such a page draws — a rule, or
	// mdpdf's blank-page placeholder — still has to be marked an artifact below.
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

	// **What draws and says nothing is an artifact** — a thematic break's rule is a filled box with no
	// text. Left unmarked it is content neither tagged nor an artifact, and veraPDF fails 7.1 t3 over it
	// (measured on a labelled conversion: the one mdpdf construct that failed). Same edit, same walk.
	for _, g := range drawingGroups(src) {
		if !g.showsText {
			edit.InsertBefore(g.start, []byte("/Artifact BMC\n"))
			edit.InsertBefore(g.end, []byte("\nEMC"))
		}
	}

	edited, eerr := edit.Apply()
	if eerr != nil {
		return eerr
	}
	return setPageContent(ctx, d, edited)
}
