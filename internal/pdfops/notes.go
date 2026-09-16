package pdfops

import (
	"errors"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// noteIconSize is the side of the sticky-note icon box in PDF points. pdf.js (and
// most viewers) draw the note icon at a small fixed size, so this only sets where
// the icon sits, not how big it renders.
const noteIconSize = 20.0

// Note is a comment to drop on a page: its text and the page-space point (PDF
// points, bottom-left origin) where the note icon's top-left corner sits.
type Note struct {
	Page int     `json:"page"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Text string  `json:"text"`
}

// AddNotes adds each note as a native /Text (sticky-note) annotation: a clickable
// icon whose popup shows the comment. pdf.js draws the icon from /Name and
// synthesizes the popup from /Contents, so no appearance stream or explicit
// /Popup is needed. Empty input is a no-op.
func AddNotes(pdf []byte, notes []Note) ([]byte, error) {
	if len(notes) == 0 {
		return pdf, nil
	}
	m := map[int][]model.AnnotationRenderer{}
	for _, nt := range notes {
		rect := types.NewRectangle(nt.X, nt.Y-noteIconSize, nt.X+noteIconSize, nt.Y)
		ann := model.NewTextAnnotation(*rect, 0, nt.Text, "", "", 0, nil, "", nil, nil, "", "", 0, 0, 0, false, "Note")
		m[nt.Page] = append(m[nt.Page], &ann)
	}
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.ADDANNOTATIONS
	noted, err := rewriteWithConf(pdf, conf, func(ctx *model.Context) error {
		ok, err := pdfcpu.AddAnnotationsMap(ctx, m, false)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("pdfcpu: AddAnnotationsMap: No annotations added") // api.AddAnnotationsMap's own refusal
		}
		// PDF/UA 7.18.3: a page with an annotation orders its tabs by structure. Set whether or not the
		// document is tagged — the clause does not ask, and form authoring already does the same.
		setStructureTabOrder(ctx)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return describeNotes(noted), nil
}

// describeNotes nests each note of a TAGGED document in an `/Annot` element — PDF/UA 7.18.1,
// `PLAN-ua-coverage.md` P01.S02 — and returns the document unchanged when it cannot.
//
// **Only in a document that already has a tree**: a note must not invent a structure tree over pages
// nothing else describes, which is ADR-031 law 1's claim over nothing.
//
// **It never costs the caller the notes**, the rule `AuthorTaggedForm` keeps for fields. A tree nib's
// parser refuses (`ensureStructTree`) used to fail the whole bake — a 500 that aborted the user's save —
// so describing is its own pass, and any failure returns the notes undescribed rather than a document
// with half a tree written. A second rewrite, paid only by tagged documents.
//
// **Pages in page order**, because each element is appended to the root's kids as it is described:
// iterating the notes' page map put the elements, and so the reading order and the bytes, in a
// different order on every run.
func describeNotes(pdf []byte) []byte {
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		if _, tagged := root["StructTreeRoot"]; !tagged {
			return errNoStructTree
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
			if _, derr := describeAnnotationsOnPage(ctx, tree, p, "Text", "Annot"); derr != nil {
				return derr
			}
		}
		return nil
	})
	if err != nil {
		return pdf
	}
	return out
}
