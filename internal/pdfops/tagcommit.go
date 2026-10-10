package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
)

// The commit writer — `PLAN-accessibility.md` P08.S06a.
//
// A reviewed proposal (S05, edited by a person in S06c) becomes a structure tree: each element's runs
// bracketed at their own show operators with an MCID, the elements built through the typed tree
// writers, and the claim made through `claimTagging` with D4's `Inferred` tier. This is the ONLY place
// inferred structure is written, and it runs only when asked (law 3, D5).
//
// # What it refuses, and why each is a refusal and not a best effort
//
//   - **A document that already has a tree.** A second tree over the first is two answers to what the
//     document says; replacing the producer's is a decision nobody asked this function to make.
//   - **A page whose runs already sit under MCIDs.** Marked content with ids and no tree to say what
//     they mean — a stripped copy is exactly this. Nesting new MCIDs inside them gives content two
//     owners.
//   - **A run drawn inside a form XObject** — proposed or not, unless an artifact covers it. Its show
//     operator is in the form's stream; bracketing the page stream would describe the `Do`, the form
//     may be drawn more than once, and leaving it unmarked would put it under the claim this writer
//     makes (`/pending 495`).
//   - **A proposal that no longer matches the page.** Runs are matched by the span of their show
//     operator AND their text; a document changed since it was proposed fails here instead of tagging
//     the wrong bytes.
//
// # Tables (ADR-121)
//
// A proposed table is written as `Table` → `TR` → `TH`/`TD`: the Table and each TR as grouping elements, a
// cell as the element its text is bracketed under, and a cell with no text as a grouping element, so every
// row has as many cells as the grid has columns. A `TH` gets its `/Scope` through `withTableAttribute`, the
// one writer of a Table attribute (ADR-119). The rules themselves are painted paths and become artifacts with
// every other uncovered drawing.
//
// # Figures (ADR-122)
//
// A kept Figure brackets its image's own operator — `Do` with its operand, or an inline image together with
// the `q … cm` and the `Q` it is drawn between (`drawingBrackets`) — under
// an MCID, and carries the reviewer's description as `/Alt`, written as the structure editor writes one. The
// operator must be one a fresh read of the page finds uncovered (`uncoveredDrawingSpans`), or the proposal is
// stale; and a figure that is kept is not ALSO declared an artifact. An ignored one is, with every other
// drawing nothing covers.
//
// # What is marked as an artifact
//
// Every text run on a committed page that no element covers — a whitespace run the grouping dropped,
// or an element the reviewer chose to ignore — is bracketed `/Artifact`, and so is every painted path
// (`/pending 495`), every picture, shading and inline image (`/pending 514`). Left unmarked it is
// content that is neither tagged nor an artifact, which is ua1 7.1 t3 by name.
//
// **A FORM XObject no element covers is still not bracketed**, and that is the one left: its content
// is another stream, which may be drawn more than once, so a bracket here would describe the `Do`.
// `claimTagging` counts what such a form draws instead, so a page carrying one is refused rather than
// claimed over — the same answer `errCommitInForm` gives for text.

var (
	errCommitTagged = errors.New("pdfops: this document already has a structure tree; a proposal is not written over it — remove its tags first (Review Structure Tree, Remove all tags), then propose again")
	errCommitMarked = errors.New("pdfops: this page already carries marked content with ids and no tree to say what they mean, so new structure cannot be written into it")
	errCommitInForm = errors.New("pdfops: text on this page is drawn inside a form XObject, which a commit cannot mark without describing the form instead of its text")
	errCommitStale  = errors.New("pdfops: the proposal does not match the document any more — propose again")
)

// commitProposal writes a reviewed proposal into pdf, and also commits alsoPages — pages whose every
// proposed element the reviewer ignored.
//
// **A page with nothing kept is still a committed page** (found at the P01 runpending sweep's tier 3,
// a regression of `/pending 495`): its text has to be bracketed as an artifact like any other uncovered
// run, or it is content neither tagged nor an artifact under the claim this writer makes, and
// `claimTagging` refuses the whole commit — the keyboard review that ignored page two's paragraph could
// no longer commit at all.
func commitProposal(pdf []byte, elements []proposedElement, alsoPages ...int) ([]byte, error) {
	if len(elements) == 0 {
		return nil, errors.New("pdfops: an empty proposal describes nothing, so there is nothing to commit")
	}
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		if _, terr := readStructTree(ctx, nil); terr == nil {
			return errCommitTagged
		} else if !errors.Is(terr, errNoStructTree) {
			return terr
		}

		// Each committed page, read once: its runs indexed by where their show operator starts.
		type committedPage struct {
			dict   types.Dict
			src    []byte
			edit   *contentstream.Edit
			runs   map[int]textRun
			marked map[int]bool
			// images is the page's `/XObject` names that resolve to an image, which is what tells
			// `uncoveredDrawingSpans` an uncovered `Do` is a picture and not a form.
			images map[string]bool
			res    types.Dict
			// drawings is every drawing on the page that nothing covers, by where its operator ENDS — which is
			// one place for the walker's reading of an image and for this one — and figured those a Figure took.
			drawings map[int]opSpan
			figured  map[int]bool
			// brackets is where a sequence about one of them opens and closes (`drawingBrackets`).
			brackets drawingBrackets
		}
		pages := map[int]*committedPage{}
		var order []int
		pageNrs := make([]int, 0, len(elements)+len(alsoPages))
		for _, el := range elements {
			pageNrs = append(pageNrs, el.page)
		}
		pageNrs = append(pageNrs, alsoPages...)
		walked := pdfread.Pages(ctx)              // one walk for the whole read, not one per page (/pending 756)
		budget := newFormWalkBudget(len(pageNrs)) // one for the pages read (`readPageRuns`)
		for _, pageNr := range pageNrs {
			el := proposedElement{page: pageNr}
			if _, seen := pages[el.page]; seen {
				continue
			}
			pr, perr := readPageRuns(ctx, pageAt(ctx, walked, el.page), budget)
			if perr != nil {
				return perr
			}
			byStart := map[int]textRun{}
			for _, r := range pr.runs {
				if r.mcid >= 0 {
					return fmt.Errorf("%w (page %d)", errCommitMarked, el.page)
				}
				if !r.inForm {
					byStart[r.span.start] = r
					continue
				}
				// **Text drawn inside a form that no artifact covers is refused whether or not it was
				// proposed** (`/pending 495`). It cannot be bracketed here and it cannot be artifacted
				// without hiding real text, so committing the page would claim tagging over it — a claim
				// `claimTagging` now refuses, which reached the caller as a generic refusal instead of
				// this one.
				if !r.artifact {
					return fmt.Errorf("%w (page %d, %q)", errCommitInForm, el.page, r.text)
				}
			}
			pages[el.page] = &committedPage{runs: byStart, marked: map[int]bool{}}
			order = append(order, el.page)
		}

		live := livePageObjects(ctx)
		tree, terr := ensureStructTree(ctx, live)
		if terr != nil {
			return terr
		}
		for _, pg := range order {
			d, _, derr := tree.page(ctx, pg)
			if derr != nil {
				return derr
			}
			src, cerr := pdfread.PageContent(ctx, d, pg)
			if cerr != nil {
				return cerr
			}
			// Inherited resources, as `PageDict` resolves them: a page that inherits its `/XObject` from an
			// ancestor draws the same pictures, and reading only `d["Resources"]` would see none of them.
			var res types.Dict
			if wp := tree.walkedPage(ctx, pg); wp.Err == nil && wp.Attrs != nil {
				res = wp.Attrs.Resources
			}
			pages[pg].dict = d
			pages[pg].src = src
			pages[pg].edit = contentstream.NewEdit(src)
			pages[pg].images = imageXObjectNames(ctx, res)
			pages[pg].res = res
			pages[pg].drawings, pages[pg].figured = map[int]opSpan{}, map[int]bool{}
			pages[pg].brackets = drawingBrackets{src: src}
			drawings, _ := uncoveredDrawingSpans(ctx.XRefTable, res, src, pages[pg].images)
			for _, sp := range drawings {
				pages[pg].drawings[sp.end] = sp
			}
		}

		// mark brackets runs as one element of structType under parent, and answers the element — nil for no runs.
		mark := func(page int, runs []textRun, structType string, parent *types.IndirectRef) (*types.IndirectRef, error) {
			if len(runs) == 0 {
				return nil, nil
			}
			cp := pages[page]
			var elem *types.IndirectRef
			for i, r := range runs {
				if r.inForm {
					return nil, fmt.Errorf("%w (page %d, %q)", errCommitInForm, page, r.text)
				}
				fresh, ok := cp.runs[r.span.start]
				if !ok || fresh.text != r.text || fresh.span != r.span || cp.marked[r.span.start] {
					return nil, fmt.Errorf("%w (page %d, %q)", errCommitStale, page, r.text)
				}
				var id int
				if i == 0 {
					mcid, ref, aerr := addMarkedElementUnder(ctx, tree, page, structType, parent)
					if aerr != nil {
						return nil, aerr
					}
					id, elem = mcid, ref
				} else {
					extra, aerr := addMCIDTo(ctx, tree, page, *elem)
					if aerr != nil {
						return nil, aerr
					}
					id = extra
				}
				cp.marked[r.span.start] = true
				cp.edit.InsertBefore(r.span.start, []byte(fmt.Sprintf("/%s <</MCID %d>> BDC\n", structType, id)))
				cp.edit.InsertBefore(r.span.end, []byte("\nEMC"))
			}
			return elem, nil
		}

		lists := map[int]*types.IndirectRef{}
		// table and row are the Table and the TR the elements being written sit under (ADR-121). A proposal
		// lists a table's elements consecutively — Table, then each TR and its cells — and a review may not
		// part them (`CommitTags`), so the sequence is the nesting.
		var table, row *types.IndirectRef
		for _, el := range elements {
			if _, ok := pages[el.page]; !ok {
				return fmt.Errorf("%w (page %d)", errCommitStale, el.page)
			}
			runs := elementRuns(el)
			if tableRoles[el.role] {
				if err := func() (err error) {
					switch {
					case el.role == "Table":
						table, err = addGroupingElement(ctx, tree, "Table", nil)
						row = nil
						return err
					case el.role == "TR" && table != nil:
						row, err = addGroupingElement(ctx, tree, "TR", table)
						return err
					case el.role == "TR" || row == nil:
						return fmt.Errorf("%w: a %s outside a table's rows", errCommitStale, el.role)
					}
					// A cell with nothing in it is still a cell: the row keeps its length.
					var cell *types.IndirectRef
					if len(runs) == 0 {
						cell, err = addGroupingElement(ctx, tree, el.role, row)
					} else {
						cell, err = mark(el.page, runs, el.role, row)
					}
					if err != nil || el.role != "TH" {
						return err
					}
					d, derr := ctx.DereferenceDict(*cell)
					if derr != nil || d == nil {
						return fmt.Errorf("pdfops: the header cell just written does not resolve: %v", derr)
					}
					d["A"] = withTableAttribute(ctx, d["A"], "Scope", types.Name(headerScope(el)))
					return nil
				}(); err != nil {
					return err
				}
				continue
			}
			table, row = nil, nil
			if el.role == figureRole {
				// The image's operator, where a fresh read of this page finds a drawing nothing covers: the
				// proposal was made from these bytes, and one that names anything else is stale (ADR-122).
				cp, sp := pages[el.page], el.figure
				fresh, ok := cp.drawings[sp.end]
				if !ok || fresh.start > sp.start || !drawsAnImage(cp.src[fresh.start:sp.end]) {
					return fmt.Errorf("%w (page %d, a figure)", errCommitStale, el.page)
				}
				// The writer's own half of the law, whoever calls it: a Figure with nothing to say fails the clause
				// it exists for (ua1 7.3 t1), so none is written.
				if strings.TrimSpace(el.alt) == "" {
					return fmt.Errorf("pdfops: a figure is written only with a description of what it shows (page %d)", el.page)
				}
				mcid, ref, aerr := addMarkedElementUnder(ctx, tree, el.page, figureRole, nil)
				if aerr != nil {
					return aerr
				}
				d, derr := ctx.DereferenceDict(*ref)
				if derr != nil || d == nil {
					return fmt.Errorf("pdfops: the figure just written does not resolve: %v", derr)
				}
				esc, eerr := types.EscapedUTF16String(el.alt)
				if eerr != nil {
					return eerr
				}
				d["Alt"] = types.StringLiteral(*esc)
				cp.figured[sp.end] = true
				sp = cp.brackets.around(sp)
				cp.edit.InsertBefore(sp.start, []byte(fmt.Sprintf("/%s <</MCID %d>> BDC\n", figureRole, mcid)))
				cp.edit.InsertBefore(sp.end, []byte("\nEMC"))
				continue
			}
			if el.role != "LI" {
				if _, err := mark(el.page, runs, el.role, nil); err != nil {
					return err
				}
				continue
			}
			list, ok := lists[el.list]
			if !ok {
				l, lerr := addGroupingElement(ctx, tree, "L", nil)
				if lerr != nil {
					return lerr
				}
				list, lists[el.list] = l, l
			}
			item, ierr := addGroupingElement(ctx, tree, "LI", list)
			if ierr != nil {
				return ierr
			}
			// The label is its own element only where the document drew it as its own run.
			body := runs
			if len(runs) > 1 && strings.TrimSpace(runs[0].text) == el.marker {
				if _, err := mark(el.page, runs[:1], "Lbl", item); err != nil {
					return err
				}
				body = runs[1:]
			}
			if _, err := mark(el.page, body, "LBody", item); err != nil {
				return err
			}
		}

		// Whatever text no element covers is declared an artifact, so nothing on a committed page is
		// left neither tagged nor an artifact.
		for _, pg := range order {
			cp := pages[pg]
			starts := make([]int, 0, len(cp.runs))
			for s := range cp.runs {
				starts = append(starts, s)
			}
			sort.Ints(starts)
			for _, s := range starts {
				if cp.marked[s] {
					continue
				}
				r := cp.runs[s]
				cp.edit.InsertBefore(r.span.start, []byte("/Artifact BMC\n"))
				cp.edit.InsertBefore(r.span.end, []byte("\nEMC"))
			}
			// A painted path is covered by no element either — the proposer offers none for a rule, a
			// border or a box — and until `/pending 495` it stayed neither tagged nor an artifact, so a
			// committed page with one underline failed 7.1 t3 under the claim this writer makes. Since
			// `/pending 514` the same is true of a picture, a shading and an inline image: 495 left
			// those out and 514 measured that leaving them out fixes nothing (`uncoveredDrawingSpans`).
			// A picture a Figure took is content now, and is not declared decoration as well (ADR-122).
			ends := make([]int, 0, len(cp.drawings))
			for e := range cp.drawings {
				if !cp.figured[e] {
					ends = append(ends, e)
				}
			}
			sort.Ints(ends)
			for _, e := range ends {
				sp := cp.brackets.around(cp.drawings[e])
				cp.edit.InsertBefore(sp.start, []byte("/Artifact BMC\n"))
				cp.edit.InsertBefore(sp.end, []byte("\nEMC"))
			}
			edited, aerr := cp.edit.Apply()
			if aerr != nil {
				return aerr
			}
			if err := setPageContent(ctx, cp.dict, edited); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	claimed, ok, err := claimTagging(pdf, out, sourceInferred)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, orphanedClaimError("commitProposal", out)
	}
	return claimed, nil
}

// drawsAnImage says whether op — one operator and its operands — is what draws an image: `Do`, or an inline
// image, which ends at its `EI`.
func drawsAnImage(op []byte) bool {
	return bytes.HasSuffix(op, []byte("Do")) || bytes.HasSuffix(op, []byte("EI"))
}

// elementRuns is an element's runs, line by line, in the order they were drawn on each line.
func elementRuns(el proposedElement) []textRun {
	var out []textRun
	for _, ln := range el.lines {
		out = append(out, ln.runs...)
	}
	return out
}
