package pdfops

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
)

// Tagging a region of a page, and reading what on a page no tag owns — ADR-125 (`/pending 855` item 2).
//
// # What a region is for
//
// Content the proposer missed, content a reviewer ignored (the commit wrote it as an artifact), and content a
// producer left untagged could not be brought into the structure tree at all. And a DRAWN graphic — vector
// paths — is never proposed as a Figure (ADR-122, `/pending 860`): this is where a person makes one. So a
// region takes text, images AND painted paths.
//
// # What it takes
//
// Every piece of the page whose box CENTRE is inside the rectangle — or, for a region given as the pieces the
// reader listed, whose box lies INSIDE one of them — and that no element owns: a text run under no
// MCID, an image the page's own stream draws, a painted path — each either bare or inside an `/Artifact`
// sequence. What an element already owns is left alone and is not an error. The pieces become ONE new element,
// which owns them in content-stream order, placed among its parent's elements as a created tag is
// (`placeAmongElements`, ADR-124).
//
// # How it is written
//
// A bare piece is bracketed where the commit writer brackets one: a run at its own show operator, a drawing
// through `drawingBrackets.around` (ADR-122). A piece inside an `/Artifact` sequence is never bracketed — a
// marked sequence inside an artifact one is content the page calls decoration and the tree calls content.
// Instead, when EVERY piece of the sequence is taken, its opener is rewritten to the marked opener: the
// artifact edit's inverse (`structartifact.go`), which touches the opener and nothing else. Brackets are all
// that changes in the page: with them taken out again the stream is byte for byte what it was.
//
// # Declared limits
//
//   - **A run is the unit.** One show operator cannot be split, so a region through the middle of a line takes
//     the runs whose centres it holds.
//   - **Part of an artifact sequence is refused**, with a sentence saying to widen the region.
//   - **Content a form XObject draws is refused**: its operator is in the form's stream, which may be drawn more
//     than once (`errCommitInForm`'s reason).
//   - **A shading (`sh`) is not taken.** It paints its clip, which the reader does not track, so it has no box.
//   - **No reading order is guessed.** The person places the new tag.
//   - **A piece is judged by the part of it the page shows**: its box is cut to the displayed page before its
//     centre is taken, and a piece wholly off the page is neither listed nor taken.

// maxRegionRects bounds how many rectangles one region edit may name: each is compared with every piece of
// the page, and a request chooses how many it sends.
const maxRegionRects = 4096

// regionMinSide is the least a piece's box measures, as a fraction of the page, once placed on it: a run
// measured from no widths and a hairline have no extent one way, and a box with none has no inside to tick.
const regionMinSide = 1e-4

// pieceRect is box — a piece of the page, in user space — as the part of the displayed page it covers, and
// false for one that is not on the page at all. It is the ONE placing of a piece for the region edit and for
// the reader of untagged content, so what one lists the other takes.
func pieceRect(sp displaySpace, box [4]float64) (MapRect, bool) {
	for _, v := range box {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return MapRect{}, false
		}
	}
	r := sp.rect(box)
	if r[2] < 0 || r[0] > 1 || r[3] < 0 || r[1] > 1 || sp.w <= 0 || sp.h <= 0 {
		return MapRect{}, false
	}
	r = MapRect{math.Max(r[0], 0), math.Max(r[1], 0), math.Min(r[2], 1), math.Min(r[3], 1)}
	for _, ax := range [2]int{0, 1} {
		if r[ax+2]-r[ax] < regionMinSide {
			lo := math.Max(0, math.Min(1-regionMinSide, (r[ax]+r[ax+2]-regionMinSide)/2))
			r[ax], r[ax+2] = lo, lo+regionMinSide
		}
	}
	return r, true
}

// within says whether a region of rects takes piece. A drawn rectangle (`Rect`) takes what is CENTRED in it. A
// named piece (`Pieces`, whole) takes what lies INSIDE it: a listed piece's rectangle is the box of everything
// grouped into it, so its own members are inside by construction, and a ground painted under the page — whose
// centre may well fall inside a ticked drawing — is not.
func within(rects [][4]float64, piece MapRect, whole bool) bool {
	const eps = 1e-9
	x, y := (piece[0]+piece[2])/2, (piece[1]+piece[3])/2
	for _, r := range rects {
		if whole {
			// regionMinSide of slack: a piece with no extent one way is widened about its own middle (`pieceRect`).
			if piece[0] >= r[0]-regionMinSide && piece[2] <= r[2]+regionMinSide && piece[1] >= r[1]-regionMinSide && piece[3] <= r[3]+regionMinSide {
				return true
			}
			continue
		}
		if x >= r[0]-eps && x <= r[2]+eps && y >= r[1]-eps && y <= r[3]+eps {
			return true
		}
	}
	return false
}

// validRegion says whether r is a rectangle on the displayed page: left < right and top < bottom, all in 0..1.
func validRegion(r [4]float64) bool {
	for _, v := range r {
		if math.IsNaN(v) || v < 0 || v > 1 {
			return false
		}
	}
	return r[0] < r[2] && r[1] < r[3]
}

// regionElement writes one new element of type ed.value that owns every piece of page ed.page in the region
// no element owns, and places it as ed.index-th element kid of ed.parent (ADR-125).
func regionElement(ctx *model.Context, tree *structTree, ed structEdit) error {
	if ed.elem != 0 {
		return fmt.Errorf("%w: a region makes a new tag, which has no number until it is written, and this edit names element %d — leave the element out", ErrTagsReview, ed.elem)
	}
	if !standardStructTypes[ed.value] {
		return fmt.Errorf("%w: %q is not a standard structure type", ErrTagsReview, ed.value)
	}
	if tree.walked == nil {
		tree.walked = pdfread.Pages(ctx)
	}
	if ed.page < 1 || ed.page > len(tree.walked) {
		return fmt.Errorf("%w: page %d is not a page of this document, which has %d", ErrTagsReview, ed.page, len(tree.walked))
	}
	rects, whole := ed.pieces, true
	if len(rects) == 0 {
		rects, whole = [][4]float64{ed.rect}, false
	}
	if len(rects) > maxRegionRects {
		return fmt.Errorf("%w: a region names at most %d pieces of a page, and this one names %d — tag them in more than one step", ErrTagsReview, maxRegionRects, len(rects))
	}
	for _, r := range rects {
		if !validRegion(r) {
			return fmt.Errorf("%w: a region is a rectangle on the page — left, top, right and bottom as fractions of the page as it is shown, each from 0 to 1, with some width and some height — and %v is not one", ErrTagsReview, r)
		}
	}
	// ADR-122's law, for a Figure made here as for one proposed: none is written without what it shows.
	if standardRole(tree, ed.value) == figureRole && strings.TrimSpace(ed.alt) == "" {
		return fmt.Errorf("%w: a figure is written only with a description of what it shows — describe the region, or give it another type", ErrTagsReview)
	}
	holder := tree.root
	var parent *types.IndirectRef
	if ed.parent != 0 && ed.parent != rootParent {
		to := tree.byObj[ed.parent]
		if to == nil {
			return staleEdit{ed.parent}
		}
		ref := objectRef(ctx, to.objNr)
		holder, parent = to.dict, &ref
	}

	pg := tree.walkedPage(ctx, ed.page)
	// ADR-009 exemption (grouping_test.go): this reads the page's pieces to bracket them and groups nothing.
	pr, err := readPageDrawings(ctx, pg)
	if err != nil {
		return err
	}
	sp := newDisplaySpace(pg)

	// unit is one sequence the element will own: a bare piece to bracket, or a whole artifact sequence.
	type unit struct {
		span opSpan
		art  int // the artifact sequence rewritten, plus one; 0 for a bare piece bracketed at span
	}
	var units []unit
	taken := map[int]int{} // artifact sequence (plus one) -> its pieces in the region
	inForm := false
	take := func(box [4]float64, owned, form bool, art int, span func() opSpan) {
		if owned {
			return
		}
		if r, ok := pieceRect(sp, box); !ok || !within(rects, r, whole) {
			return
		}
		switch {
		case form:
			inForm = true
		case art > 0:
			taken[art]++
		default:
			units = append(units, unit{span: span()})
		}
	}
	for _, r := range pr.runs {
		r := r
		take(runBox(r), r.mcid >= 0, r.inForm, r.art, func() opSpan { return r.span })
	}
	var brackets *drawingBrackets
	for _, im := range pr.images {
		im := im
		take(im.box, im.marked, im.inForm, im.art, func() opSpan {
			if brackets == nil {
				src, _ := pdfread.PageContent(ctx, pg.Dict, ed.page)
				brackets = &drawingBrackets{src: src}
			}
			return brackets.around(im.span)
		})
	}
	for _, p := range pr.paths {
		p := p
		take(p.box, p.marked, p.inForm, p.art, func() opSpan { return p.span })
	}
	if inForm {
		return fmt.Errorf("%w: content in that region of page %d is drawn inside a form XObject, which cannot be marked without describing the form instead of what it draws — leave it out of the region", ErrTagsReview, ed.page)
	}
	for art, n := range taken {
		a := pr.artifacts[art-1]
		switch {
		case a.inForm:
			return fmt.Errorf("%w: content in that region of page %d is drawn inside a form XObject, which cannot be marked without describing the form instead of what it draws — leave it out of the region", ErrTagsReview, ed.page)
		case a.nested:
			return fmt.Errorf("%w: decoration in that region of page %d is marked inside other tagged content, or holds some, so it cannot become a tag of its own without one owner's content sitting inside another's", ErrTagsReview, ed.page)
		case a.close == (opSpan{}):
			return fmt.Errorf("%w: decoration in that region of page %d is marked by a sequence the page never closes, so where it ends cannot be read", ErrTagsReview, ed.page)
		case a.unboxed > 0:
			return fmt.Errorf("%w: the region of page %d takes content the page marks as one piece of decoration together with a shading or a form XObject, which cannot be tagged from the page", ErrTagsReview, ed.page)
		case n < a.pieces:
			return fmt.Errorf("%w: the region cuts through content page %d marks as one piece of decoration (%d of its %d pieces are in the region) — widen the region to take all of it", ErrTagsReview, ed.page, n, a.pieces)
		}
		units = append(units, unit{span: a.opener, art: art})
	}
	if len(units) == 0 {
		return fmt.Errorf("%w: nothing untagged is in that region of page %d", ErrTagsReview, ed.page)
	}
	sort.Slice(units, func(i, j int) bool { return units[i].span.start < units[j].span.start })

	d, _, err := tree.page(ctx, ed.page)
	if err != nil {
		return err
	}
	src, err := pdfread.PageContent(ctx, d, ed.page)
	if err != nil {
		return err
	}
	edit := contentstream.NewEdit(src)
	var elem *types.IndirectRef
	for i, u := range units {
		var mcid int
		if i == 0 {
			mcid, elem, err = addMarkedElementUnder(ctx, tree, ed.page, ed.value, parent)
		} else {
			mcid, err = addMCIDTo(ctx, tree, ed.page, *elem)
		}
		if err != nil {
			return err
		}
		opener := fmt.Sprintf("/%s <</MCID %d>> BDC", ed.value, mcid)
		if u.art > 0 {
			edit.Replace(u.span.start, u.span.end, []byte(opener))
			continue
		}
		edit.InsertBefore(u.span.start, []byte(opener+"\n"))
		edit.InsertBefore(u.span.end, []byte("\nEMC"))
	}
	edited, err := edit.Apply()
	if err != nil {
		return err
	}
	if err := setPageContent(ctx, d, edited); err != nil {
		return err
	}

	// `addMarkedElementUnder` lists the element last; it is placed where the edit says, by the one rule.
	kids, set := kidsArray(ctx, holder)
	for i := len(kids) - 1; i >= 0; i-- {
		if ir, ok := kids[i].(types.IndirectRef); ok && ir == *elem {
			set(append(append(types.Array{}, kids[:i]...), kids[i+1:]...))
			break
		}
	}
	placeAmongElements(ctx, holder, *elem, ed.index)

	if ed.alt != "" {
		ed2, derr := ctx.DereferenceDict(*elem)
		if derr != nil || ed2 == nil {
			return fmt.Errorf("pdfops: the element just written does not resolve: %v", derr)
		}
		esc, eerr := types.EscapedUTF16String(ed.alt)
		if eerr != nil {
			return eerr
		}
		ed2["Alt"] = types.StringLiteral(*esc)
	}
	return nil
}

// UntaggedPiece is one piece of a page that no structure element owns (ADR-125).
type UntaggedPiece struct {
	Page int `json:"page"`
	// Kind is "text" (a paragraph, grouped as the proposer groups), "image", "drawing" (painted paths whose
	// boxes touch, or one path that is neither of the next two), "rule" (a thin line that touches no other
	// path) or "box" (one plain rectangle that touches none, or only holds others).
	Kind string `json:"kind"`
	// Text is a text piece's words, and "" for every other kind.
	Text string `json:"text"`
	// Rect is where the piece is, as fractions of the displayed page — the page map's space (ADR-088), and what
	// a region edit's Pieces takes.
	Rect MapRect `json:"rect"`
	// Decoration says the page marks the piece as an artifact; false is content the page says nothing about.
	Decoration bool `json:"decoration"`
	// InForm says a form XObject draws the piece, which a region cannot take.
	InForm bool `json:"inForm"`
}

// UntaggedContent is what the pages of a document draw that no structure element owns.
type UntaggedContent struct {
	// Pages is how many pages the document has.
	Pages  int             `json:"pages"`
	Pieces []UntaggedPiece `json:"pieces"`
}

// clusterBudget is how many pairs of paths one page's drawings may be compared over. Paths are joined into
// drawings where their boxes touch, which is the square of what a page draws where everything overlaps; past
// the budget the paths left are each listed alone, which costs a grouping and hides nothing.
const clusterBudget = 4_000_000

// ReadUntagged reads what page of pdf draws that no structure element owns — every page, for page 0 — each
// piece placed on the displayed page. It writes nothing. Text comes first, then pictures, then drawings, then
// rules and plain boxes, so what a reader most likely wants is not under a table's ruling.
func ReadUntagged(pdf []byte, page int) (UntaggedContent, error) {
	ctx, err := inspectionRead(pdf)
	if err != nil {
		return UntaggedContent{}, err
	}
	walked := pdfread.Pages(ctx)
	out := UntaggedContent{Pages: len(walked), Pieces: []UntaggedPiece{}}
	first, last := 1, len(walked)
	if page != 0 {
		if err := pageInDocument(page, len(walked)); err != nil {
			return UntaggedContent{}, err
		}
		first, last = page, page
	}
	budget := newFormWalkBudget(last - first + 1) // one for the pages read (`readPageRuns`)
	for nr := first; nr <= last; nr++ {
		pg := pageAt(ctx, walked, nr)
		// ADR-009 exemption (grouping_test.go): the runs go to the grouping door (`groupUnowned`); nothing is grouped here.
		pr, perr := readPageDrawings(ctx, pg, budget)
		if perr != nil {
			return UntaggedContent{}, perr
		}
		out.Pieces = append(out.Pieces, untaggedPieces(nr, newDisplaySpace(pg), groupUnowned(pr.runs), pr.images, pr.paths)...)
	}
	return out, nil
}

// untaggedPieces is one page's unowned pieces, listed: its text as the grouping door grouped it, the images it
// draws and its painted paths.
func untaggedPieces(nr int, sp displaySpace, text []unownedText, images []drawnImage, painted []drawnPath) []UntaggedPiece {
	var out []UntaggedPiece
	add := func(kind, text string, box [4]float64, decoration, form bool) {
		if r, ok := pieceRect(sp, box); ok {
			out = append(out, UntaggedPiece{Page: nr, Kind: kind, Text: text, Rect: r, Decoration: decoration, InForm: form})
		}
	}
	for _, t := range text {
		add("text", t.paragraph.text(), paragraphBox(t.paragraph), t.decoration, t.inForm)
	}
	for _, im := range images {
		if !im.marked {
			add("image", "", im.box, im.artifact, im.inForm)
		}
	}

	// Painted paths. Paths whose boxes touch are one drawing — among paths of the same standing (bare or
	// decoration, the page's or a form's), and never THROUGH a plain rectangle that holds the other whole: a
	// page's ground or a panel's fill touches everything drawn on it, and would make one drawing of the page.
	// A path that joins nothing is listed for what it is: a thin `rule`, a plain `box`, or a drawing of one path.
	var paths []drawnPath
	for _, p := range painted {
		if !p.marked {
			paths = append(paths, p)
		}
	}
	thin := func(p drawnPath) bool {
		return !p.curved && math.Min(p.box[2]-p.box[0], p.box[3]-p.box[1]) <= ruleMax
	}
	plainBox := func(p drawnPath) bool { return !p.curved && p.rects == 1 && p.ops == 1 && !thin(p) }
	sort.SliceStable(paths, func(i, j int) bool { return paths[i].box[0] < paths[j].box[0] })
	group := make([]int, len(paths))
	for i := range group {
		group[i] = i
	}
	find := func(i int) int {
		for group[i] != i {
			group[i] = group[group[i]]
			i = group[i]
		}
		return i
	}
	spent := 0
	for i := range paths {
		for j := i + 1; j < len(paths) && paths[j].box[0] <= paths[i].box[2] && spent < clusterBudget; j++ {
			spent++
			a, b := paths[i], paths[j]
			if a.artifact != b.artifact || a.inForm != b.inForm || !touches(a.box, b.box) {
				continue
			}
			if (plainBox(a) && covers(a.box, b.box)) || (plainBox(b) && covers(b.box, a.box)) {
				continue
			}
			group[find(j)] = find(i)
		}
	}
	type drawing struct {
		box   [4]float64
		first drawnPath
		n     int
	}
	var drawings []*drawing
	at := map[int]*drawing{}
	for i, p := range paths {
		g := find(i)
		d := at[g]
		if d == nil {
			d = &drawing{box: p.box, first: p}
			at[g] = d
			drawings = append(drawings, d)
		}
		d.box = unionBox(d.box, p.box)
		d.n++
	}
	// Drawings first, then what stands alone as a rule or a plain box, each in the order the page draws them.
	for _, kind := range [3]string{"drawing", "rule", "box"} {
		for _, d := range drawings {
			is := "drawing"
			switch {
			case d.n == 1 && thin(d.first):
				is = "rule"
			case d.n == 1 && plainBox(d.first):
				is = "box"
			}
			if is == kind {
				add(kind, "", d.box, d.first.artifact, d.first.inForm)
			}
		}
	}
	return out
}
