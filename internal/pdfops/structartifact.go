package pdfops

import (
	"fmt"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
)

// The artifact edit — `PLAN-accessibility.md` P09.S03.
//
// A reviewer decides an element is not content: a running header a producer tagged as a paragraph, a
// decorative rule tagged as a figure. Its marked-content sequences — and its descendants' — become
// `/Artifact` sequences, the ParentTree slots that named them are emptied, and the element leaves its
// parent. It is the one structure edit that rewrites page content.
//
// # What changes in the stream, and what does not
//
// Only each sequence's OPENER: `/P <</MCID 3>> BDC` becomes `/Artifact BMC`. Its `EMC` and everything
// between are the page's own bytes, untouched — the same law P08's commit writer holds when it
// brackets. The sequences are found by the run reader's own reading of `BDC` (`markedSeq`), so a figure
// with no text is found by the same rule a paragraph is.
//
// # What it refuses, each because the rewrite would describe something else
//
//   - **Content drawn through a form XObject**, either a sequence inside a form or one around a `Do`:
//     the opener is in one stream and the content in another, which may be drawn more than once.
//   - **A sequence that encloses content another element owns**: making the outer one an artifact
//     would declare the inner element's content not content while the tree still describes it.
//   - **An element that describes an annotation or form field** (an `OBJR` kid): that is not page
//     content, and an artifact sequence cannot hold it.
//   - **An MCID the page does not draw**: there is nothing to rewrite, and removing the element anyway
//     would hide a tree that already disagreed with its page.
//   - **The last element under the root**: a document claiming tagging over an empty tree is the claim
//     ADR-031 forbids.

// artifactElement declares e's content, and its descendants', an artifact, and takes e out of the tree.
func artifactElement(ctx *model.Context, tree *structTree, e *structElem) error {
	if e.parent == nil {
		others := 0
		kids, _ := kidsArray(ctx, tree.root)
		for _, en := range kids {
			if ir, ok := en.(types.IndirectRef); ok && ir.ObjectNumber.Value() == e.objNr {
				continue
			}
			if isElementEntry(ctx, en) {
				others++
			}
		}
		if others == 0 {
			return fmt.Errorf("%w: element %d is the last element of the structure tree, and a document that claims tagging over an empty tree is the claim ADR-031 forbids", ErrTagsReview, e.objNr)
		}
	}

	pageNr := map[int]int{}
	for pg := 1; pg <= ctx.PageCount; pg++ {
		if ir, err := ctx.PageDictIndRef(pg); err == nil && ir != nil {
			pageNr[ir.ObjectNumber.Value()] = pg
		}
	}
	owned := map[int]map[int]bool{} // page -> MCIDs the subtree owns there
	var collect func(x *structElem) error
	collect = func(x *structElem) error {
		for _, k := range x.kids {
			switch k.kind {
			case kidElement:
				if k.elem != nil {
					if err := collect(k.elem); err != nil {
						return err
					}
				}
			case kidOBJR:
				return fmt.Errorf("%w: element %d describes an annotation or form field, which is not page content an artifact can hold", ErrTagsReview, x.objNr)
			case kidMCID, kidMCR:
				if d, derr := ctx.DereferenceDict(k.raw); derr == nil && d != nil {
					if _, inStream := d["Stm"]; inStream {
						return fmt.Errorf("%w (element %d, /MCID %d)", errCommitInForm, x.objNr, k.mcid)
					}
				}
				pg := pageNr[k.pgObj]
				if pg == 0 {
					return fmt.Errorf("pdfops: element %d owns /MCID %d on no page of this document, so its content cannot be found", x.objNr, k.mcid)
				}
				if owned[pg] == nil {
					owned[pg] = map[int]bool{}
				}
				owned[pg][k.mcid] = true
			}
		}
		return nil
	}
	if err := collect(e); err != nil {
		return err
	}

	pages := make([]int, 0, len(owned))
	for pg := range owned {
		pages = append(pages, pg)
	}
	sort.Ints(pages)
	for _, pg := range pages {
		mine := owned[pg]
		// ADR-009 exemption (grouping_test.go): this reads the page's marked-content sequences and groups nothing.
		pr, err := readPageRuns(ctx, pg)
		if err != nil {
			return err
		}
		d, _, derr := tree.page(ctx, pg)
		if derr != nil {
			return derr
		}
		src, cerr := ctx.PageContent(d, pg)
		if cerr != nil {
			return cerr
		}
		edit := contentstream.NewEdit(src)
		found := map[int]bool{}
		for _, s := range pr.sequences {
			if !mine[s.mcid] {
				continue
			}
			if s.inForm || s.drawsForm {
				return fmt.Errorf("%w (page %d, /MCID %d)", errCommitInForm, pg, s.mcid)
			}
			for _, other := range pr.sequences {
				if other.inForm || mine[other.mcid] {
					continue
				}
				if other.opener.start > s.opener.start && (s.close == (opSpan{}) || other.opener.start < s.close.start) {
					return fmt.Errorf("%w: element %d's content on page %d encloses content another element owns (/MCID %d)", ErrTagsReview, e.objNr, pg, other.mcid)
				}
			}
			edit.Replace(s.opener.start, s.opener.end, []byte("/Artifact BMC"))
			found[s.mcid] = true
		}
		for mcid := range mine {
			if !found[mcid] {
				return fmt.Errorf("pdfops: element %d claims /MCID %d on page %d, and the page draws no sequence with it", e.objNr, mcid, pg)
			}
		}
		edited, aerr := edit.Apply()
		if aerr != nil {
			return aerr
		}
		if err := setPageContent(ctx, d, edited); err != nil {
			return err
		}
		if sp, ok := d["StructParents"].(types.Integer); ok {
			for mcid := range mine {
				if err := clearParentTreeSlot(ctx, tree, sp.Value(), mcid); err != nil {
					return err
				}
			}
		}
	}
	_, err := removeFromParent(ctx, tree, e)
	return err
}
