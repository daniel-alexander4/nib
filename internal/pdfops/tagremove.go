package pdfops

import (
	"fmt"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
)

// Removing a structure tree — ADR-120.
//
// A document that arrives tagged badly enough is quicker to tag again than to correct, and the commit writer
// refuses a document that has a tree, or content marked with ids, for good reason: it does not decide to
// replace a producer's structure. This is that decision, taken by a person, as its own operation. Nothing
// calls it on the way to something else.
//
// # What goes, all of it or none
//
//   - **Every marked-content sequence that carries an MCID loses its opener and its `EMC`** — in a page's
//     content and in every form XObject a page draws. What was between them is the page's own bytes,
//     untouched. An id with no tree to say what it means is the state the commit writer refuses
//     (`errCommitMarked`), so leaving one behind would leave a document that can be neither read as tagged
//     nor tagged again.
//   - **`/StructTreeRoot` and `/MarkInfo`** leave the catalog; **`/StructParents` and `/StructParent`** leave
//     every page, annotation and form XObject (`parentTreeOwners`' population): each names a row of a tree
//     that is gone.
//
// # What stays
//
// **An `/Artifact` sequence stays.** It carries no id and names no tree; it says "not content", which is as
// true after as before, and a later commit keeps what it says.
//
// The document's `/Lang`, title and metadata stay. A PDF/UA identification does not survive any change
// (ADR-032), this one included.

// RemoveStructure takes pdf's structure tree and every marked-content id away, leaving the document untagged.
// A document with no tree is ErrTagsStale: the person was shown one.
func RemoveStructure(pdf []byte) ([]byte, error) {
	return writeMutated(pdf, func(ctx *model.Context) error {
		cat, err := ctx.XRefTable.Catalog()
		if err != nil {
			return err
		}
		if _, tagged := cat["StructTreeRoot"]; !tagged {
			return staleEdit{}
		}
		walked := pdfread.Pages(ctx)
		budget := newFormWalkBudget(len(walked)) // one for the document (`readPageRuns`)
		// Each form's sequences once, however often and on however many pages the form is drawn.
		inForms := map[int]map[opSpan]opSpan{}
		for _, pg := range walked {
			// ADR-009 exemption (grouping_test.go): this reads the page's marked-content sequences and groups nothing.
			pr, perr := readPageRuns(ctx, pg, budget)
			if perr != nil {
				return perr
			}
			var own []markedSeq
			for _, s := range pr.sequences {
				switch {
				case s.stm > 0:
					if inForms[s.stm] == nil {
						inForms[s.stm] = map[opSpan]opSpan{}
					}
					inForms[s.stm][s.opener] = s.close
				case s.inForm:
					return fmt.Errorf("pdfops: page %d marks content with /MCID %d inside a form XObject that is not an object of its own, so the mark cannot be found again to remove", pg.Nr, s.mcid)
				default:
					own = append(own, s)
				}
			}
			if len(own) == 0 {
				continue
			}
			src, cerr := pdfread.PageContent(ctx, pg.Dict, pg.Nr)
			if cerr != nil {
				return cerr
			}
			edit := contentstream.NewEdit(src)
			for _, s := range own {
				unbracket(edit, s.opener, s.close)
			}
			edited, aerr := edit.Apply()
			if aerr != nil {
				return aerr
			}
			if err := setPageContent(ctx, pg.Dict, edited); err != nil {
				return err
			}
		}
		forms := make([]int, 0, len(inForms))
		for nr := range inForms {
			forms = append(forms, nr)
		}
		sort.Ints(forms)
		for _, nr := range forms {
			en, found := ctx.XRefTable.Table[nr]
			if !found || en == nil {
				return fmt.Errorf("pdfops: form XObject %d is not in the xref table", nr)
			}
			sd, isStream := en.Object.(types.StreamDict)
			if !isStream {
				return fmt.Errorf("pdfops: form XObject %d is not a stream", nr)
			}
			// The bytes the walk read, through its one door (`formContent`): the spans are offsets into exactly these.
			gen := 0
			if en.Generation != nil {
				gen = *en.Generation
			}
			body := budget.formContent(&sd, *types.NewIndirectRef(nr, gen))
			if body == nil {
				return fmt.Errorf("pdfops: form XObject %d's content could not be read again, so its marks cannot be removed", nr)
			}
			edit := contentstream.NewEdit(body)
			for opener, closer := range inForms[nr] {
				unbracket(edit, opener, closer)
			}
			edited, aerr := edit.Apply()
			if aerr != nil {
				return aerr
			}
			sd.Content = edited
			if err := sd.Encode(); err != nil {
				return err
			}
			en.Object = sd
		}

		delete(cat, "StructTreeRoot")
		delete(cat, "MarkInfo")
		seenForms := map[int]bool{}
		for _, rec := range scanPages(ctx) {
			delete(rec.dict, "StructParents")
			annots, _ := ctx.DereferenceArray(rec.dict["Annots"])
			for _, a := range annots {
				if ad, aerr := ctx.DereferenceDict(a); aerr == nil && ad != nil {
					delete(ad, "StructParent")
				}
			}
			if rec.res != nil {
				eachFormXObject(ctx, rec.res, seenForms, 0, func(_ int, sd *types.StreamDict) {
					delete(sd.Dict, "StructParents")
					delete(sd.Dict, "StructParent")
				})
			}
		}
		return nil
	})
}

// unbracket takes a marked-content sequence's opener and its `EMC` out of a stream, leaving a space where each
// was so the tokens either side stay apart. A sequence the stream ended before closing has no `EMC` to take.
func unbracket(edit *contentstream.Edit, opener, closer opSpan) {
	edit.Replace(opener.start, opener.end, []byte(" "))
	if closer != (opSpan{}) {
		edit.Replace(closer.start, closer.end, []byte(" "))
	}
}
