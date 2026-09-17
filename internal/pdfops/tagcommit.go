package pdfops

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
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
	errCommitTagged = errors.New("pdfops: this document already has a structure tree; a proposal is not written over it")
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
		}
		pages := map[int]*committedPage{}
		var order []int
		pageNrs := make([]int, 0, len(elements)+len(alsoPages))
		for _, el := range elements {
			pageNrs = append(pageNrs, el.page)
		}
		pageNrs = append(pageNrs, alsoPages...)
		for _, pageNr := range pageNrs {
			el := proposedElement{page: pageNr}
			if _, seen := pages[el.page]; seen {
				continue
			}
			pr, perr := readPageRuns(ctx, el.page)
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

		live := map[int]bool{}
		for pg := 1; pg <= ctx.PageCount; pg++ {
			if ir, e := ctx.PageDictIndRef(pg); e == nil && ir != nil {
				live[ir.ObjectNumber.Value()] = true
			}
		}
		tree, terr := ensureStructTree(ctx, live)
		if terr != nil {
			return terr
		}
		for _, pg := range order {
			d, _, derr := tree.page(ctx, pg)
			if derr != nil {
				return derr
			}
			src, cerr := ctx.PageContent(d, pg)
			if cerr != nil {
				return cerr
			}
			// Inherited resources, through `PageDict`: a page that inherits its `/XObject` from an
			// ancestor draws the same pictures, and reading only `d["Resources"]` would see none of them.
			var res types.Dict
			if _, _, attrs, aerr := ctx.PageDict(pg, false); aerr == nil && attrs != nil {
				res = attrs.Resources
			}
			pages[pg].dict = d
			pages[pg].src = src
			pages[pg].edit = contentstream.NewEdit(src)
			pages[pg].images = imageXObjectNames(ctx, res)
		}

		// mark brackets runs as one element of structType under parent.
		mark := func(page int, runs []textRun, structType string, parent *types.IndirectRef) error {
			if len(runs) == 0 {
				return nil
			}
			cp := pages[page]
			var elem *types.IndirectRef
			for i, r := range runs {
				if r.inForm {
					return fmt.Errorf("%w (page %d, %q)", errCommitInForm, page, r.text)
				}
				fresh, ok := cp.runs[r.span.start]
				if !ok || fresh.text != r.text || fresh.span != r.span || cp.marked[r.span.start] {
					return fmt.Errorf("%w (page %d, %q)", errCommitStale, page, r.text)
				}
				var id int
				if i == 0 {
					mcid, ref, aerr := addMarkedElementUnder(ctx, tree, page, structType, parent)
					if aerr != nil {
						return aerr
					}
					id, elem = mcid, ref
				} else {
					extra, aerr := addMCIDTo(ctx, tree, page, *elem)
					if aerr != nil {
						return aerr
					}
					id = extra
				}
				cp.marked[r.span.start] = true
				cp.edit.InsertBefore(r.span.start, []byte(fmt.Sprintf("/%s <</MCID %d>> BDC\n", structType, id)))
				cp.edit.InsertBefore(r.span.end, []byte("\nEMC"))
			}
			return nil
		}

		lists := map[int]*types.IndirectRef{}
		for _, el := range elements {
			if _, ok := pages[el.page]; !ok {
				return fmt.Errorf("%w (page %d)", errCommitStale, el.page)
			}
			runs := elementRuns(el)
			if el.role != "LI" {
				if err := mark(el.page, runs, el.role, nil); err != nil {
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
				if err := mark(el.page, runs[:1], "Lbl", item); err != nil {
					return err
				}
				body = runs[1:]
			}
			if err := mark(el.page, body, "LBody", item); err != nil {
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
			drawings, _ := uncoveredDrawingSpans(cp.src, cp.images)
			for _, sp := range drawings {
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

// elementRuns is an element's runs, line by line, in the order they were drawn on each line.
func elementRuns(el proposedElement) []textRun {
	var out []textRun
	for _, ln := range el.lines {
		out = append(out, ln.runs...)
	}
	return out
}
