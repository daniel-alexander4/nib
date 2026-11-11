package pdfops

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
)

// The page objects' edit door (PLAN-image-edit P01.S02, D2-D5, L1-L5).
//
// `EditPageObjects` is the ONE function that changes a page's content for an object; `internal/objedit` is the only
// caller outside this package, and it holds the two refusals this package cannot (a signed document, a ceremony
// record: `pdfops` imports neither `sign` nor `ceremony`). `TestEveryPageObjectEditRoutesThroughTheDoor` holds both.
//
// A removal cuts the painting operator and nothing around it (D3): `q`, `cm` and `Q` stay. A picture a structure
// element owns goes with its element, in the same rewrite (D4's pin), or is refused by name. The page's resources
// are then pruned to what its drawing still names (`pruneKeptResources`, redaction's prune), so the picture's bytes
// leave the file unless another page or form still names them (`SharedWith`); the writer frees what it no longer
// reaches, an image's `/SMask` with it. The written output is read back and must be the page minus exactly what
// was removed, or the edit is refused (`rewrite-differs`).

// ObjectEditDelete removes a picture (P01); ObjectEditMove places it in a new box (P02, D8).
const (
	ObjectEditDelete = "delete"
	ObjectEditMove   = "move"
)

// maxObjectEdits bounds one request's edits: a person selecting pictures one page at a time is nowhere near it,
// and every edit is a re-read of its page.
const maxObjectEdits = 64

// ObjectEdit names one picture by the address a read handed out (D2) and what to do to it.
type ObjectEdit struct {
	Kind    string `json:"kind"`
	Page    int    `json:"page"`
	Ordinal int    `json:"ordinal"`
	Digest  string `json:"digest"`
	// Box is a move's target, as fractions of the displayed page (the listing's space, ADR-088); nil for a removal.
	Box *MapRect `json:"box,omitempty"`
}

// The refusal causes besides `objectEditability`'s (`WhyFormDrawn` and the rest), each a stable string.
const (
	// CauseSigned and CauseCeremony are `internal/objedit`'s: the document carries a signature, or a ceremony record.
	CauseSigned   = "signed"
	CauseCeremony = "ceremony"
	// WhyTaggedShared: the picture's element, or its marked-content sequence, holds other content too.
	WhyTaggedShared = "tagged-shared"
	// WhyTaggedLast: the picture's element is the last of the structure tree (ADR-031: a claim over an empty tree).
	WhyTaggedLast = "tagged-last"
	// CauseObjectChanged: the address no longer names the operator it named (D2) — 409, the window reads again.
	CauseObjectChanged = "object-changed"
	// CauseRewriteDiffers: the written page is not the page minus exactly what was removed.
	CauseRewriteDiffers = "rewrite-differs"
	// CauseNotRewritable: the document does not survive a rewrite through this path.
	CauseNotRewritable = "not-rewritable"
	// CauseBadRequest: no edit, too many, an unknown kind or one picture named twice.
	CauseBadRequest = "bad-request"
)

// objectCauseWords is every cause an edit of a page object can be refused with, in the words the CLI prints and the
// window shows (plan-review pin: wording). `TestEveryObjectCauseHasItsWords` pins the count and that every cause
// the code names is here.
var objectCauseWords = map[string]string{
	CauseSigned:         "This document is signed. Changing it would break the signature.",
	CauseCeremony:       "This document is part of a signing ceremony; edit before you start one.",
	WhyFormDrawn:        "This picture is part of a repeated page element, such as a letterhead, so editing it would change other pages.",
	WhyTagged:           "This picture is tagged in a way Nib cannot take apart safely, so it is left as it is.",
	WhyTaggedShared:     "This picture's tag also holds other content, so removing it would leave that tag describing something that is gone.",
	WhyTaggedLast:       "This picture's tag is the only one in the document; to start again, remove all tags first.",
	WhyNotIsolated:      "This picture is drawn without the bracket that keeps its settings apart from the rest of the page, so removing it could change how the rest is drawn.",
	WhyUnreadableBox:    "Nib cannot work out where this picture is drawn.",
	WhyOffPage:          "This picture is drawn outside the page, where it cannot be seen.",
	CauseObjectChanged:  "The page changed since you selected this. Select the picture again.",
	CauseRewriteDiffers: "Nib checked the page after changing the picture and it was not the page it expected, so nothing was changed.",
	CauseNotRewritable:  "Nib cannot write this document back without changing it, so it cannot change the picture.",
	CauseBadRequest:     "The request to change a picture could not be read.",
	WhyNotAlone:         "This picture is placed together with other drawing, so moving it on its own could move or change that too.",
	WhyNotUpright:       "This picture is turned, mirrored or slanted on the page; Nib moves only pictures that sit straight.",
	WhyPartlyOffPage:    "Part of this picture is outside the page, so Nib cannot show all of it to move.",
	WhyClipped:          "This picture is drawn under a clip that hides part of the page, so moving it could hide some of it; Nib leaves it where it is.",
}

// ObjectCauseWords is the sentence a refusal with cause says, or "" for a cause there is none for.
func ObjectCauseWords(cause string) string { return objectCauseWords[cause] }

// objectCauses lists every refusal cause, sorted.
func objectCauses() []string {
	out := make([]string, 0, len(objectCauseWords))
	for c := range objectCauseWords {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// ObjectRefusal is an edit of a page object refused with a cause. Its text is the cause's sentence, then what
// was refused; Page and Ordinal name the picture (0 and -1 when the refusal is the document's or the request's).
type ObjectRefusal struct {
	Cause   string
	Page    int
	Ordinal int
	Detail  string
	err     error
}

// Error is the cause's sentence, then the picture and whatever caused the refusal.
func (r *ObjectRefusal) Error() string {
	s := objectCauseWords[r.Cause]
	if r.Page > 0 && r.Ordinal >= 0 {
		s += fmt.Sprintf(" (page %d, picture %d)", r.Page, r.Ordinal+1)
	}
	if r.Detail != "" {
		s += " " + r.Detail
	}
	if r.err != nil {
		s += ": " + r.err.Error()
	}
	return s
}

// Unwrap is what caused the refusal, when something did.
func (r *ObjectRefusal) Unwrap() error { return r.err }

// RefuseObject is a refusal with cause; err, when not nil, is what caused it.
func RefuseObject(cause string, page, ordinal int, err error) *ObjectRefusal {
	return &ObjectRefusal{Cause: cause, Page: page, Ordinal: ordinal, err: err}
}

// badObjectRequest is a request the door cannot read as edits, with what is wrong with it.
func badObjectRequest(detail string) *ObjectRefusal {
	return &ObjectRefusal{Cause: CauseBadRequest, Ordinal: -1, Detail: detail}
}

// RemovedObject is what removing one picture did.
type RemovedObject struct {
	Page    int    `json:"page"`
	Ordinal int    `json:"ordinal"`
	Kind    string `json:"kind"`
	// ElementRemoved says the structure element that owned it went with it; Alt is that element's description.
	ElementRemoved bool   `json:"elementRemoved"`
	Alt            string `json:"alt"`
	// LinksOver counts the page's link annotations whose rectangle lies inside the picture's box: kept, and said.
	LinksOver int `json:"linksOver"`
	// SharedWith lists the pages that still name the image, so its bytes stay in the file. Empty for an inline image.
	SharedWith []int `json:"sharedWith"`
	// StaysInFile says the image's bytes are still in the file: a page in SharedWith draws it, or the form's default
	// resources (`/AcroForm /DR`) name it, which are the form's and are not pruned for a page's edit.
	StaysInFile bool `json:"staysInFile"`
}

// ObjectEditResult is what an edit did, one entry per removed picture in the order asked.
type ObjectEditResult struct {
	Removed []RemovedObject `json:"removed"`
	// Moved is one entry per moved picture, in the order asked.
	Moved []MovedObject `json:"moved"`
}

// MovedObject is what a move did: the picture's address before it, the box it was placed in as the written page
// reads it, and the links over where it was, which stay there (a link is the page's, not the picture's).
type MovedObject struct {
	Page      int     `json:"page"`
	Ordinal   int     `json:"ordinal"`
	Box       MapRect `json:"box"`
	LinksOver int     `json:"linksOver"`
}

// EditPageObjects applies edits to pdf as ONE rewrite and answers what it did. Every refusal is an
// `*ObjectRefusal`; nothing is written unless every edit applies.
func EditPageObjects(pdf []byte, edits []ObjectEdit) ([]byte, ObjectEditResult, error) {
	pages, err := groupObjectEdits(edits)
	if err != nil {
		return nil, ObjectEditResult{}, err
	}
	var plans []pageRemoval
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		var ferr error
		plans, ferr = removeObjects(ctx, pages)
		return ferr
	})
	if err != nil {
		var ref *ObjectRefusal
		if errors.As(err, &ref) {
			return nil, ObjectEditResult{}, err
		}
		return nil, ObjectEditResult{}, RefuseObject(CauseNotRewritable, 0, -1, err)
	}
	if err := verifyWritten(out, plans); err != nil {
		return nil, ObjectEditResult{}, err
	}
	res := ObjectEditResult{Removed: []RemovedObject{}, Moved: []MovedObject{}}
	byAddr := map[[2]int]RemovedObject{}
	movedAt := map[[2]int]MovedObject{}
	for _, p := range plans {
		for _, r := range p.removed {
			byAddr[[2]int{r.Page, r.Ordinal}] = r
		}
		for _, m := range p.movedAnswers {
			movedAt[[2]int{m.Page, m.Ordinal}] = m
		}
	}
	for _, e := range edits {
		if e.Kind == ObjectEditMove {
			res.Moved = append(res.Moved, movedAt[[2]int{e.Page, e.Ordinal}])
		} else {
			res.Removed = append(res.Removed, byAddr[[2]int{e.Page, e.Ordinal}])
		}
	}
	return out, res, nil
}

// groupObjectEdits checks the request and groups it by page, pages ascending.
func groupObjectEdits(edits []ObjectEdit) ([][]ObjectEdit, error) {
	switch {
	case len(edits) == 0:
		return nil, badObjectRequest("It names no picture.")
	case len(edits) > maxObjectEdits:
		return nil, badObjectRequest(fmt.Sprintf("It names %d pictures, and one request removes at most %d.", len(edits), maxObjectEdits))
	}
	seen := map[[2]int]bool{}
	byPage := map[int][]ObjectEdit{}
	for _, e := range edits {
		switch e.Kind {
		case ObjectEditDelete:
			if e.Box != nil {
				return nil, badObjectRequest("A removal takes no box.")
			}
		case ObjectEditMove:
			if e.Box == nil {
				return nil, badObjectRequest("A move names the box to place the picture in.")
			}
			if why := moveBoxProblem(*e.Box); why != "" {
				return nil, badObjectRequest(why)
			}
		default:
			return nil, badObjectRequest(fmt.Sprintf("%q is not an edit Nib makes to a picture.", e.Kind))
		}
		if e.Page < 1 || e.Ordinal < 0 {
			return nil, badObjectRequest(fmt.Sprintf("Page %d, picture %d is not an address: pages and pictures count from 1.", e.Page, e.Ordinal+1))
		}
		k := [2]int{e.Page, e.Ordinal}
		if seen[k] {
			return nil, badObjectRequest(fmt.Sprintf("It names picture %d of page %d twice.", e.Ordinal+1, e.Page))
		}
		seen[k] = true
		byPage[e.Page] = append(byPage[e.Page], e)
	}
	nrs := make([]int, 0, len(byPage))
	for nr := range byPage {
		nrs = append(nrs, nr)
	}
	sort.Ints(nrs)
	out := make([][]ObjectEdit, len(nrs))
	for i, nr := range nrs {
		out[i] = byPage[nr]
	}
	return out, nil
}

// pageRemoval is one page's edits as applied: what was removed, and what the written page must read as.
type pageRemoval struct {
	page    int
	removed []RemovedObject
	// want is the page's pictures' digests, in order, minus exactly the removed; text its runs' text, in order.
	want []string
	text []string
	// objNrs is each removed picture's image XObject object number, 0 for an inline image.
	objNrs []int
	// moved holds, by index into want, what a moved picture must read as: its box, drawn straight, by the same
	// operator bytes; its want entry is "" because a move rewrites the `cm` its operator digest covers. movedAnswers
	// is what the moves answer.
	moved        map[int]movedWant
	movedAnswers []MovedObject
}

// objectTagging is what the structure tree says about one marked picture: the verdict (`objectEditability`'s
// tagged cause, "" when it may go), and when it may, the element that owns it (nil when none does) and the
// marked-content sequence around it (nil when it is left in place).
type objectTagging struct {
	why   string
	owner *structElem
	seq   *markedSeq
	// alt is the owner's `/Alt`, read while the tree is in hand.
	alt string
}

// taggingOf answers objectTagging for every marked picture pr lists, by image index; src is the page's content. tree nil with treeErr nil
// means the document has no structure tree; with treeErr set, a tree nib could not read — every marked picture is
// then `tagged`.
func taggingOf(ctx *model.Context, tree *structTree, treeErr error, pgObj int, pr pageRuns, src []byte) map[int]objectTagging {
	out := map[int]objectTagging{}
	for i, im := range pr.images {
		if !im.marked || im.inForm {
			continue
		}
		out[i] = tagVerdict(ctx, tree, treeErr, pgObj, pr, i, src)
	}
	return out
}

// tagVerdict is taggingOf's answer for picture i.
func tagVerdict(ctx *model.Context, tree *structTree, treeErr error, pgObj int, pr pageRuns, i int, src []byte) objectTagging {
	im := pr.images[i]
	seq := sequenceAround(pr.sequences, im)
	if seq == nil || treeErr != nil {
		return objectTagging{why: WhyTagged}
	}
	if seq.drawsForm || seq.inForm || sequencePaintsMore(src, *seq, im.span) {
		return objectTagging{why: WhyTaggedShared}
	}
	if tree == nil {
		return objectTagging{seq: seq}
	}
	var owners []*structElem
	for _, e := range tree.elems {
		for _, k := range e.kids {
			if (k.kind == kidMCID || k.kind == kidMCR) && k.mcid == im.mcid && k.pgObj == pgObj && k.stm == 0 {
				owners = append(owners, e)
				break
			}
		}
	}
	switch {
	case len(owners) == 0:
		return objectTagging{seq: seq}
	case len(owners) > 1:
		return objectTagging{why: WhyTagged}
	}
	e := owners[0]
	if e.objNr == 0 || (e.parent != nil && e.parent.objNr == 0) {
		return objectTagging{why: WhyTagged}
	}
	if entries, _ := kidsArray(ctx, e.dict); len(entries) != 1 {
		return objectTagging{why: WhyTaggedShared}
	}
	if e.parent == nil && !otherTopLevelElement(ctx, tree, e) {
		return objectTagging{why: WhyTaggedLast}
	}
	return objectTagging{owner: e, seq: seq, alt: elementAlt(ctx, e)}
}

// sequenceAround is the innermost marked-content sequence with the picture's MCID that encloses its operator in
// the same stream, or nil.
func sequenceAround(seqs []markedSeq, im drawnImage) *markedSeq {
	var best *markedSeq
	for j := range seqs {
		s := &seqs[j]
		if s.mcid != im.mcid || s.stm != im.stm || s.opener.end > im.span.start {
			continue
		}
		if s.close != (opSpan{}) && s.close.start < im.span.end {
			continue
		}
		if best == nil || s.opener.start > best.opener.start {
			best = s
		}
	}
	return best
}

// paintingOperators are the operators that put marks on a page: text shows, path paints, a shading, an XObject or an
// inline image. `n` ends a path without painting and is not one.
var paintingOperators = map[string]bool{"Tj": true, "TJ": true, "'": true, "\"": true, "f": true, "F": true, "f*": true,
	"B": true, "B*": true, "b": true, "b*": true, "S": true, "s": true, "sh": true, "Do": true, "BI": true}

// sequencePaintsMore says the bytes of seq in src paint something besides the picture whose operator is at im: read
// from the tokens, not from the reader's lists, because the lists keep no span for a shading (`sh` is only counted)
// and a sequence that also draws anything is not the picture's alone (code-review 2026-11-09).
func sequencePaintsMore(src []byte, seq markedSeq, im opSpan) bool {
	end := len(src)
	if seq.close != (opSpan{}) {
		end = seq.close.start
	}
	if seq.opener.end > end || end > len(src) {
		return true
	}
	for _, tok := range contentstream.Tokenize(src[seq.opener.end:end]) {
		at := seq.opener.end + tok.Start
		if at >= im.start && at < im.end {
			continue
		}
		if tok.Kind == contentstream.Operator && paintingOperators[string(tok.Bytes(src[seq.opener.end:end]))] {
			return true
		}
	}
	return false
}

// otherTopLevelElement says the root holds an element besides e.
func otherTopLevelElement(ctx *model.Context, tree *structTree, e *structElem) bool {
	rootKids, _ := kidsArray(ctx, tree.root)
	for _, en := range rootKids {
		if ir, ok := en.(types.IndirectRef); ok && ir.ObjectNumber.Value() == e.objNr {
			continue
		}
		if isElementEntry(ctx, en) {
			return true
		}
	}
	return false
}

// topLevelLeft counts the elements the root keeps once every owner in owners that sits at the top is deleted — the
// last-element refusal for several pictures on one page, which each verdict alone cannot see (each counts the others).
func topLevelLeft(ctx *model.Context, tree *structTree, owners []objectTagging) int {
	going := map[int]bool{}
	for _, t := range owners {
		if t.owner.parent == nil {
			going[t.owner.objNr] = true
		}
	}
	if len(going) == 0 {
		return 1
	}
	rootKids, _ := kidsArray(ctx, tree.root)
	n := 0
	for _, en := range rootKids {
		if ir, ok := en.(types.IndirectRef); ok && going[ir.ObjectNumber.Value()] {
			continue
		}
		if isElementEntry(ctx, en) {
			n++
		}
	}
	return n
}

// readTreeForEdit reads the structure tree, nil and no error when the document has none.
func readTreeForEdit(ctx *model.Context, live map[int]bool) (*structTree, error) {
	tree, err := readStructTree(ctx, live)
	if errors.Is(err, errNoStructTree) {
		return nil, nil
	}
	return tree, err
}

// removeObjects applies the grouped edits to ctx: the content, the structure, the page's `/Thumb` and resources.
func removeObjects(ctx *model.Context, pages [][]ObjectEdit) ([]pageRemoval, error) {
	live := livePageObjects(ctx)
	tree, treeErr := readTreeForEdit(ctx, live)
	already := map[string]bool{}
	if tree != nil {
		for _, d := range checkStructConsistency(ctx, tree) {
			already[d.key] = true
		}
	}
	structureTouched := false
	walked := pdfread.Pages(ctx)
	plans := make([]pageRemoval, 0, len(pages))
	for _, edits := range pages {
		nr := edits[0].Page
		if nr > len(walked) {
			return nil, &ObjectRefusal{Cause: CauseBadRequest, Ordinal: -1, Detail: fmt.Sprintf("It names page %d, and the document has %d.", nr, len(walked))}
		}
		if structureTouched {
			if tree, treeErr = readTreeForEdit(ctx, live); treeErr != nil {
				return nil, treeErr
			}
		}
		plan, touched, err := removeOnPage(ctx, tree, treeErr, pageAt(ctx, walked, nr), nr, edits)
		if err != nil {
			return nil, err
		}
		structureTouched = structureTouched || touched
		plans = append(plans, plan)
	}
	if structureTouched {
		after, err := readTreeForEdit(ctx, live)
		if err != nil {
			return nil, err
		}
		if after != nil {
			for _, d := range checkStructConsistency(ctx, after) {
				if !already[d.key] {
					return nil, &ObjectRefusal{Cause: WhyTagged, Ordinal: -1, Detail: "Removing it would leave the tags contradicting themselves: " + d.what}
				}
			}
		}
	}
	// A page that names a removed image through a shared or inherited `/Resources` without drawing it would keep
	// its bytes in the file: each such page's resources are pruned to what it draws, as the edited page's were
	// (and as every read's optimize pass already does where it can), and only then is a page that still names
	// the image one that draws it.
	pruned := map[int]bool{}
	var extra []pageRemoval
	for _, p := range plans {
		pruned[p.page] = true
	}
	for i := range plans {
		for j := range plans[i].removed {
			objNr := plans[i].removedObj(j)
			if objNr == 0 {
				continue
			}
			for _, nr := range pagesNamingXObject(ctx, walked, objNr) {
				if pruned[nr] {
					continue
				}
				pruned[nr] = true
				pg := pageAt(ctx, walked, nr)
				kept, err := pageAsItStands(ctx, pg, nr)
				if err != nil {
					return nil, err
				}
				if err := pruneOnePage(ctx, pg, nr); err != nil {
					return nil, err
				}
				// A page whose resources were pruned and whose content was not touched must read as it did:
				// the after-check holds it too (code-review 2026-11-09).
				extra = append(extra, kept)
			}
			r := &plans[i].removed[j]
			r.SharedWith = pagesNamingXObject(ctx, walked, objNr)
			if len(r.SharedWith) == 0 {
				dropFromPageTreeNodes(ctx, objNr)
			}
			r.StaysInFile = len(r.SharedWith) > 0 || formDefaultsName(ctx, objNr)
		}
	}
	return append(plans, extra...), nil
}

// formDefaultsName says the form's default resources (`/AcroForm /DR`) name the object objNr — measured on the corpus
// (2026-11-09): eight removed pictures were held only there. They are the form's, used to build field appearances, so
// a page's edit leaves them and says the picture stays in the file.
func formDefaultsName(ctx *model.Context, objNr int) bool {
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		return false
	}
	af, err := ctx.DereferenceDict(cat["AcroForm"])
	if err != nil || af == nil {
		return false
	}
	return resourcesName(ctx, af["DR"], objNr, map[int]bool{}, 0)
}

// dropFromPageTreeNodes takes objNr out of the `/Resources` of every intermediate page-tree node that names it, once
// no page names it. A page with its own `/Resources` does not inherit its parent's, and a page that inherited was
// given its own when it was pruned (`pruneOnePage`), so no page reads what is taken here — yet the writer still
// reaches the node, and the image with it (measured on the corpus, code-review 2026-11-09: 27 pictures held only by
// a `/Pages` node's resources). The node's dictionaries are copied before the entry goes, so a dictionary shared with
// anything else is never changed under it.
func dropFromPageTreeNodes(ctx *model.Context, objNr int) {
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		return
	}
	seen := map[int]bool{}
	var walk func(o types.Object, depth int)
	walk = func(o types.Object, depth int) {
		if depth > 64 {
			return
		}
		if ir, ok := o.(types.IndirectRef); ok {
			if seen[ir.ObjectNumber.Value()] {
				return
			}
			seen[ir.ObjectNumber.Value()] = true
		}
		node, err := ctx.DereferenceDict(o)
		if err != nil || node == nil {
			return
		}
		if t, _ := node["Type"].(types.Name); t != "Pages" {
			return
		}
		if res, err := ctx.DereferenceDict(node["Resources"]); err == nil && res != nil {
			if xo, err := ctx.DereferenceDict(res["XObject"]); err == nil && xo != nil {
				kept := types.Dict{}
				dropped := false
				for k, v := range xo {
					if ir, ok := v.(types.IndirectRef); ok && ir.ObjectNumber.Value() == objNr {
						dropped = true
						continue
					}
					kept[k] = v
				}
				if dropped {
					fresh := types.Dict{}
					for k, v := range res {
						fresh[k] = v
					}
					fresh["XObject"] = kept
					node["Resources"] = fresh
				}
			}
		}
		kids, _ := ctx.DereferenceArray(node["Kids"])
		for _, k := range kids {
			walk(k, depth+1)
		}
	}
	walk(cat["Pages"], 0)
}

// pageAsItStands is a page's pictures and text as the after-check must find them when nothing on it is removed.
func pageAsItStands(ctx *model.Context, pg pdfread.Page, nr int) (pageRemoval, error) {
	pr, err := readPageDrawings(ctx, pg)
	if err != nil {
		return pageRemoval{}, err
	}
	keep := pageRemoval{page: nr}
	for _, o := range pageObjectsOf(nr, newDisplaySpace(pg), pr, nil).Objects {
		keep.want = append(keep.want, o.operator)
	}
	for _, r := range pr.runs {
		keep.text = append(keep.text, r.text)
	}
	return keep, nil
}

// pruneOnePage gives page nr a direct `/Resources` of its own, holding only what its drawing names.
func pruneOnePage(ctx *model.Context, pg pdfread.Page, nr int) error {
	if pg.Dict == nil {
		return nil
	}
	if _, own := pg.Dict["Resources"]; !own && pg.Attrs != nil && pg.Attrs.Resources != nil {
		pg.Dict["Resources"] = pg.Attrs.Resources
	}
	return pruneKeptResources(ctx.XRefTable, []types.Dict{pg.Dict}, []int{nr}, keepUnreadable)
}

// removedObj is the image XObject's object number removal j took off the page, 0 for an inline image.
func (p pageRemoval) removedObj(j int) int { return p.objNrs[j] }

// removeOnPage applies one page's edits to ctx and answers its plan, and whether the structure tree changed.
func removeOnPage(ctx *model.Context, tree *structTree, treeErr error, pg pdfread.Page, nr int, edits []ObjectEdit) (pageRemoval, bool, error) {
	if pg.Err != nil || pg.Dict == nil || pg.Ref == nil {
		return pageRemoval{}, false, RefuseObject(CauseNotRewritable, nr, -1, fmt.Errorf("page %d does not resolve: %v", nr, pg.Err))
	}
	pr, err := readPageDrawings(ctx, pg)
	if err != nil {
		return pageRemoval{}, false, err
	}
	pgObj := pg.Ref.ObjectNumber.Value()
	src, err := pdfread.PageContent(ctx, pg.Dict, nr)
	if err != nil {
		return pageRemoval{}, false, err
	}
	tagging := taggingOf(ctx, tree, treeErr, pgObj, pr, src)
	objs := pageObjectsOf(nr, newDisplaySpace(pg), pr, tagging)
	edit := contentstream.NewEdit(src)
	plan := pageRemoval{page: nr}
	gone := map[int]bool{}
	var owners []objectTagging
	// unowned are the MCIDs of removed marked pictures no element lists: a stale `/ParentTree` slot for one is cleared.
	var unowned []int
	sp := newDisplaySpace(pg)
	moved := map[int][4]float64{}
	for _, e := range edits {
		o, ok := ObjectAt(objs, e.Ordinal, e.Digest)
		if !ok {
			return pageRemoval{}, false, RefuseObject(CauseObjectChanged, nr, e.Ordinal, nil)
		}
		if e.Kind == ObjectEditMove {
			im := pr.images[e.Ordinal]
			if !o.Movable {
				return pageRemoval{}, false, RefuseObject(o.MoveWhy, nr, e.Ordinal, nil)
			}
			target := sp.unrect(*e.Box)
			cm, ok := placementFor(target, im.outer)
			if !ok {
				return pageRemoval{}, false, RefuseObject(WhyUnreadableBox, nr, e.Ordinal, nil)
			}
			edit.Replace(im.place.start, im.place.end, cm)
			moved[e.Ordinal] = target
			plan.movedAnswers = append(plan.movedAnswers, MovedObject{Page: nr, Ordinal: e.Ordinal,
				LinksOver: linksInside(ctx, pg.Dict, im.box)})
			continue
		}
		if !o.Editable {
			return pageRemoval{}, false, RefuseObject(o.Why, nr, e.Ordinal, nil)
		}
		gone[e.Ordinal] = true
		edit.Replace(o.span.start, o.span.end, nil)
		r := RemovedObject{Page: nr, Ordinal: e.Ordinal, Kind: o.Kind, SharedWith: []int{},
			LinksOver: linksInside(ctx, pg.Dict, pr.images[e.Ordinal].box)}
		if t, marked := tagging[e.Ordinal]; marked {
			if t.seq != nil {
				unbracket(edit, t.seq.opener, t.seq.close)
			}
			if t.owner != nil {
				r.ElementRemoved = true
				r.Alt = elementAlt(ctx, t.owner)
				owners = append(owners, t)
			} else if t.seq != nil {
				unowned = append(unowned, t.seq.mcid)
			}
		}
		plan.removed = append(plan.removed, r)
		plan.objNrs = append(plan.objNrs, o.objNr)
	}
	edited, err := edit.Apply()
	if err != nil {
		return pageRemoval{}, false, err
	}
	if err := setPageContent(ctx, pg.Dict, edited); err != nil {
		return pageRemoval{}, false, err
	}
	key := pageRowKey(ctx, pgObj)
	if tree != nil && topLevelLeft(ctx, tree, owners) == 0 {
		return pageRemoval{}, false, RefuseObject(WhyTaggedLast, nr, -1, nil)
	}
	for _, mcid := range unowned {
		if tree != nil && key >= 0 {
			if err := clearParentTreeSlot(ctx, tree, key, mcid); err != nil {
				return pageRemoval{}, false, err
			}
		}
	}
	for _, t := range owners {
		delete(t.owner.dict, "K")
		t.owner.kids = nil
		if err := clearParentTreeSlot(ctx, tree, key, t.seq.mcid); err != nil {
			return pageRemoval{}, false, err
		}
		if err := deleteElement(ctx, tree, t.owner); err != nil {
			return pageRemoval{}, false, &ObjectRefusal{Cause: WhyTagged, Page: nr, Ordinal: -1, err: err}
		}
	}
	delete(pg.Dict, "Thumb")
	if len(gone) > 0 { // a move names no new resource and drops none, so only a removal prunes
		if err := pruneOnePage(ctx, pg, nr); err != nil {
			return pageRemoval{}, false, err
		}
	}
	for i, o := range objs.Objects {
		if gone[i] {
			continue
		}
		if b, ok := moved[i]; ok {
			if plan.moved == nil {
				plan.moved = map[int]movedWant{}
			}
			plan.moved[len(plan.want)] = movedWant{box: b, painter: o.painter}
			plan.want = append(plan.want, "")
			continue
		}
		plan.want = append(plan.want, o.operator)
	}
	for _, r := range pr.runs {
		plan.text = append(plan.text, r.text)
	}
	return plan, len(owners) > 0, nil
}

// elementAlt is the element's `/Alt`, or "".
func elementAlt(ctx *model.Context, e *structElem) string {
	if v, ok := e.dict["Alt"]; ok {
		if s, err := ctx.DereferenceStringOrHexLiteral(v, model.V10, nil); err == nil {
			return s
		}
	}
	return ""
}

// linksInside counts the page's link annotations whose rectangle lies inside box (user space, half a point of
// slack): what a person may have meant to go with the picture, kept and said.
func linksInside(ctx *model.Context, page types.Dict, box [4]float64) int {
	x0, y0, x1, y1 := box[0], box[1], box[2], box[3]
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	annots, err := ctx.DereferenceArray(page["Annots"])
	if err != nil {
		return 0
	}
	n := 0
	const slack = 0.5
	for _, a := range annots {
		d, err := ctx.DereferenceDict(a)
		if err != nil || d == nil {
			continue
		}
		if st, ok := d["Subtype"].(types.Name); !ok || st.Value() != "Link" {
			continue
		}
		llx, lly, urx, ury, ok := rectOf(ctx, d["Rect"])
		if ok && llx >= x0-slack && lly >= y0-slack && urx <= x1+slack && ury <= y1+slack {
			n++
		}
	}
	return n
}

// pagesNamingXObject lists the pages whose resources — their own, inherited, or a form's they name — still name
// the object objNr, so its bytes stay in the file.
func pagesNamingXObject(ctx *model.Context, walked []pdfread.Page, objNr int) []int {
	out := []int{}
	for _, pg := range walked {
		if pg.Dict == nil {
			continue
		}
		res := pg.Dict["Resources"]
		if res == nil && pg.Attrs != nil && pg.Attrs.Resources != nil {
			res = pg.Attrs.Resources
		}
		if resourcesName(ctx, res, objNr, map[int]bool{}, 0) {
			out = append(out, pg.Nr)
		}
	}
	return out
}

// resourcesName says the resources dictionary res names objNr as an XObject, itself or through a form it names.
func resourcesName(ctx *model.Context, res types.Object, objNr int, seen map[int]bool, depth int) bool {
	if depth > maxFormDepth {
		return false
	}
	rd, err := ctx.DereferenceDict(res)
	if err != nil || rd == nil {
		return false
	}
	xo, err := ctx.DereferenceDict(rd["XObject"])
	if err != nil || xo == nil {
		return false
	}
	for _, v := range xo {
		ir, ok := v.(types.IndirectRef)
		if !ok {
			continue
		}
		n := ir.ObjectNumber.Value()
		if n == objNr {
			return true
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		sd, _, err := ctx.DereferenceStreamDict(ir)
		if err != nil || sd == nil {
			continue
		}
		if st, ok := sd.Dict["Subtype"].(types.Name); ok && st.Value() == "Form" {
			if resourcesName(ctx, sd.Dict["Resources"], objNr, seen, depth+1) {
				return true
			}
		}
	}
	return false
}

// verifyWritten reads out back and holds every edited page to its plan (D2's pin).
func verifyWritten(out []byte, plans []pageRemoval) error {
	ctx, err := inspectionRead(out)
	if err != nil {
		return RefuseObject(CauseNotRewritable, 0, -1, err)
	}
	walked := pdfread.Pages(ctx)
	for _, p := range plans {
		if p.page > len(walked) {
			return RefuseObject(CauseRewriteDiffers, p.page, -1, fmt.Errorf("page %d is not in the written document", p.page))
		}
		pg := pageAt(ctx, walked, p.page)
		pr, err := readPageDrawings(ctx, pg)
		if err != nil {
			return RefuseObject(CauseRewriteDiffers, p.page, -1, err)
		}
		got := pageObjectsOf(p.page, newDisplaySpace(pg), pr, nil)
		var text []string
		for _, r := range pr.runs {
			text = append(text, r.text)
		}
		if err := rewriteAgrees(p, got.Objects, text); err != nil {
			return err
		}
		for k := range p.movedAnswers {
			m := &p.movedAnswers[k]
			if at := movedIndex(p, m.Ordinal); at >= 0 && at < len(got.Objects) {
				m.Box = got.Objects[at].Box
			}
		}
	}
	return nil
}

// rewriteAgrees holds a written page's pictures and text to what removing exactly p.removed leaves.
func rewriteAgrees(p pageRemoval, got []PageObject, text []string) error {
	if len(got) != len(p.want) {
		return RefuseObject(CauseRewriteDiffers, p.page, -1, fmt.Errorf("page %d draws %d pictures, and %d were expected", p.page, len(got), len(p.want)))
	}
	for i, o := range got {
		if m, moved := p.moved[i]; moved {
			// Where it was asked, drawn straight (a turned or mirrored matrix can fill the same box), and by the
			// same operator bytes (a different picture can sit in the same box).
			if !boxesAgree(o.userBox, m.box) || !o.plain || o.painter != m.painter {
				return RefuseObject(CauseRewriteDiffers, p.page, -1, fmt.Errorf("page %d's picture %d is not the picture it moved, where it was moved to", p.page, i+1))
			}
			continue
		}
		if o.operator != p.want[i] {
			return RefuseObject(CauseRewriteDiffers, p.page, -1, fmt.Errorf("page %d's picture %d is not the one that was there", p.page, i+1))
		}
	}
	if len(text) != len(p.text) {
		return RefuseObject(CauseRewriteDiffers, p.page, -1, fmt.Errorf("page %d has %d runs of text, and %d were expected", p.page, len(text), len(p.text)))
	}
	for i := range text {
		if text[i] != p.text[i] {
			return RefuseObject(CauseRewriteDiffers, p.page, -1, fmt.Errorf("page %d's text changed", p.page))
		}
	}
	return nil
}

// movedWant is what the after-check holds a moved picture to.
type movedWant struct {
	box     [4]float64
	painter [32]byte
}

// moveTolerance is how far, in user-space units (points unless the page sets `/UserUnit`), a moved picture's edges may
// read from the box asked: the `cm` is written
// exactly (`num`), so only the round trip through an inverse matrix and the reader's parse are left.
const moveTolerance = 1e-3

// boxesAgree says two user-space boxes have every edge within moveTolerance.
func boxesAgree(a, b [4]float64) bool {
	for i := range a {
		if math.Abs(a[i]-b[i]) > moveTolerance {
			return false
		}
	}
	return true
}

// movedIndex is where the picture at ordinal reads on the written page: its ordinal less the removals before it.
func movedIndex(p pageRemoval, ordinal int) int {
	before := 0
	for _, r := range p.removed {
		if r.Ordinal < ordinal {
			before++
		}
	}
	return ordinal - before
}

// moveBoxProblem is why box cannot be a move's target, or "": four numbers, inside the displayed page, with room.
func moveBoxProblem(b MapRect) string {
	for _, v := range b {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "The box is not four numbers."
		}
	}
	if b[0] < 0 || b[1] < 0 || b[2] > 1 || b[3] > 1 {
		return "The box reaches outside the page."
	}
	if b[2]-b[0] < regionMinSide || b[3]-b[1] < regionMinSide {
		return "The box has no width or no height."
	}
	return ""
}

// placementFor is the `cm` that places the unit square at target (user space) when outer is the matrix in force
// before it: the picture's new matrix [w 0 0 h x y] followed by outer undone, so cm × outer is that matrix (PDF's
// row-vector order, `runMatrix.mul`). Written by `num`, the content stream's one number writer: exact, no exponent.
func placementFor(target [4]float64, outer runMatrix) ([]byte, bool) {
	inv, ok := outer.inverse()
	if !ok {
		return nil, false
	}
	m := runMatrix{target[2] - target[0], 0, 0, target[3] - target[1], target[0], target[1]}.mul(inv)
	var b strings.Builder
	for _, v := range m {
		if !finite(v) {
			return nil, false
		}
		b.WriteString(num(v))
		b.WriteByte(' ')
	}
	b.WriteString("cm")
	return []byte(b.String()), true
}
