package pdfops

import (
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
)

// Flow across pages — `PLAN-text-reflow.md` P07.S06.
//
// One rule, applied page after page: a block of a column's paragraphs moves down by p; what no longer fits above the
// page's floor leaves, whole, for the top of the next page, where it lands on that page's first paragraph's baseline and
// pushes that page's block down by its own baseline span plus its column's paragraph step — and so on, to the last page,
// where what still does not fit refuses `page-full`. The edited page is the rule's first application.

// flowStep is one page's share of a flow: the edits to its content, what it moves within itself, and what it hands on.
type flowStep struct {
	pg  pdfread.Page
	src []byte
	// moves are the edits to src: kept paragraphs moved, leaving ones deleted.
	moves []runMove
	// keep is the zone moved within the page, and by how much; leave is the zone of what went to the next page, and how
	// far down it was drawn there. Either may be nil.
	keep, leave *anchorShift
	// content is the page's new content, for every page but the first (whose edit the rewrite composes itself).
	content []byte
}

// carriedBlock is what a page hands to the next: runs read from src (under resources res), drawn dy lower.
type carriedBlock struct {
	src  []byte
	res  types.Dict
	runs []textRun
	dy   float64
	tags *tagCarry // the structure the runs take with them (P07.S07), or nil
}

// flowReach is the full width of a page from its floor up to top: anything anchored there moves, or refuses.
func flowReach(floor, top float64) [4]float64 {
	return [4]float64{math.Inf(-1), floor, math.Inf(1), top}
}

// pushDown plans moving block — paragraph indices of layout on pg, top to bottom, one column — down by p, above floor;
// top is the top of what moves on this page (the edited paragraph's bottom on the first page, the first paragraph's top on
// the others), step the column's paragraph step, and in what arrives from the page before, if anything. It returns the
// steps from this page on, or the refusal and the text of the paragraph that caused it.
//
// xs is the zone's horizontal extent: the page's width on a page of one column, the column's on several (`anchorZone`).
func pushDown(ctx *model.Context, pg pdfread.Page, layout pageLayout, src []byte, block []int, p, floor, top, step float64, xs [2]float64, in *carriedBlock) ([]flowStep, string, string, error) {
	keepN := len(block)
	for keepN > 0 && layout.paragraphs[block[keepN-1]].bottom()-p < floor-measureSlack {
		keepN--
	}
	keep, leave := block[:keepN], block[keepN:]
	st := flowStep{pg: pg, src: src}
	for _, q := range keep {
		moves, cause := moveRuns(src, paragraphRunsWithBlanks(layout, q), p)
		if cause != "" {
			return nil, cause, layout.paragraphs[q].text(), nil
		}
		st.moves = append(st.moves, moves...)
	}
	var leaving []textRun
	for _, q := range leave {
		for _, r := range paragraphRunsWithBlanks(layout, q) {
			if c := carryRefusal(r); c != "" {
				return nil, c, layout.paragraphs[q].text(), nil
			}
			leaving = append(leaving, r)
		}
	}
	var tags *tagCarry
	if len(leaving) > 0 {
		dels, cause := deleteRuns(src, leaving)
		if cause != "" {
			return nil, cause, layout.paragraphs[leave[0]].text(), nil
		}
		var res types.Dict
		if pg.Attrs != nil {
			res = pg.Attrs.Resources
		}
		t, brackets, cause := planTagCarry(ctx, pg, src, res, layout.sequences, leaving)
		if cause != "" {
			return nil, cause, layout.paragraphs[leave[0]].text(), nil
		}
		tags = t
		st.moves = append(st.moves, dels...)
		st.moves = append(st.moves, brackets...)
	}
	// Everything anchored at the height that moves is inside what stays and moves, or inside what leaves; anything else —
	// straddling, in the free room, beside the column on a page of several — refuses.
	keepBottom := top
	if len(keep) > 0 {
		keepBottom = layout.paragraphs[keep[len(keep)-1]].bottom()
		st.keep = &anchorShift{zone: [4]float64{xs[0], keepBottom, xs[1], top}, dy: p}
	}
	var leaveZone [4]float64
	if len(leave) > 0 {
		leaveZone = [4]float64{xs[0], layout.paragraphs[leave[len(leave)-1]].bottom(), xs[1], layout.paragraphs[leave[0]].top()}
	}
	for _, a := range pageAnchors(ctx, pg, true) {
		if !touches(a.box, flowReach(math.Min(floor, keepBottom), top)) {
			continue
		}
		if st.keep != nil && covers(st.keep.zone, a.box) || len(leave) > 0 && covers(leaveZone, a.box) {
			continue
		}
		return nil, causeAnchored, "", nil
	}
	if len(leave) > 0 {
		next, nl, cause, err := nextPageFor(ctx, pg)
		if err != nil || cause != "" {
			return nil, cause, "", err
		}
		first, last := layout.paragraphs[leave[0]], layout.paragraphs[leave[len(leave)-1]]
		q0 := nl.paragraphs[0]
		dy := first.lines[0].y - q0.lines[0].y
		nr := regionOf(nl, 0, visibleBoxOf(next))
		if last.bottom()-dy < nr.floor-measureSlack {
			return nil, causePageFull, "", nil // the block is taller than the next page's body
		}
		if len(nr.marks) > 0 || drawnOver(nl, 0) {
			return nil, causeAnchored, "", nil
		}
		nsrc, err := pdfread.PageContent(ctx, next.Dict, next.Nr)
		if err != nil {
			return nil, "", "", err
		}
		var res types.Dict
		if pg.Attrs != nil {
			res = pg.Attrs.Resources
		}
		span := first.lines[0].y - last.lines[len(last.lines)-1].y
		rest, cause, below, err := pushDown(ctx, next, nl, nsrc, append([]int{0}, nr.paragraphs...), span+step, nr.floor, q0.top(), step,
			[2]float64{math.Inf(-1), math.Inf(1)}, &carriedBlock{src: src, res: res, runs: leaving, dy: dy, tags: tags})
		if err != nil || cause != "" {
			return nil, cause, below, err
		}
		st.leave = &anchorShift{zone: leaveZone, dy: dy}
		return finishStep(ctx, st, layout.sequences, in, rest)
	}
	return finishStep(ctx, st, layout.sequences, in, nil)
}

// finishStep builds the page's content when it is not the first — its own edits, then what arrives from the page before
// drawn after them, its structure landing among seqs, the sequences the page's own stream draws — and puts it before the
// steps that follow.
func finishStep(ctx *model.Context, st flowStep, seqs []markedSeq, in *carriedBlock, rest []flowStep) ([]flowStep, string, string, error) {
	if in != nil {
		e := contentstream.NewEdit(st.src)
		for _, m := range st.moves {
			e.Replace(m.span.start, m.span.end, m.with)
		}
		edited, err := e.Apply()
		if err != nil {
			return nil, "", "", err
		}
		content, cause, err := setRunsOn(ctx, in.src, in.res, in.runs, st.pg, edited, 0, in.dy, in.tags, seqs)
		if err != nil || cause != "" {
			return nil, cause, "", err
		}
		st.content = content
	}
	return append([]flowStep{st}, rest...), "", "", nil
}

// nextPageFor is the page after pg with its layout — or the reason a flow cannot land there: there is none, it is not a
// single column, or it draws no paragraph to land on.
func nextPageFor(ctx *model.Context, pg pdfread.Page) (pdfread.Page, pageLayout, string, error) {
	if pg.Nr >= ctx.PageCount {
		return pdfread.Page{}, pageLayout{}, causePageFull, nil
	}
	next := pageAt(ctx, nil, pg.Nr+1)
	nl, err := readPageGlyphLayout(ctx, next)
	if err != nil {
		return pdfread.Page{}, pageLayout{}, "", err
	}
	if nl.columns != 1 || len(nl.paragraphs) == 0 {
		return pdfread.Page{}, pageLayout{}, causePageFull, nil
	}
	return next, nl, "", nil
}

// drawnOver says whether a mark — a drawing, or text no paragraph holds — lies over paragraph pi of l, which a push moves
// out from under it.
func drawnOver(l pageLayout, pi int) bool {
	own := paragraphBox(l.paragraphs[pi])
	for _, o := range obstaclesOf(l, map[int]bool{pi: true}) {
		if o.kind != markText && meets(o.box, own) && !covers(o.box, own) {
			return true
		}
	}
	return false
}

// applyFlow writes a flow's pages after the first, then moves what is anchored, page by page: each page's own anchors move
// before those carried onto it arrive, so an arriving anchor — already at its final place — is never moved twice.
func applyFlow(ctx *model.Context, flow []flowStep) error {
	for _, st := range flow[min(1, len(flow)):] {
		if err := setPageContent(ctx, st.pg.Dict, st.content); err != nil {
			return err
		}
	}
	for k := range flow {
		st := flow[k]
		if st.keep != nil {
			if err := shiftAnchors(ctx, st.pg, st.keep.zone, st.keep.dy); err != nil {
				return err
			}
		}
		if k > 0 && flow[k-1].leave != nil {
			if err := carryAnchors(ctx, flow[k-1].pg, st.pg, flow[k-1].leave.zone, flow[k-1].leave.dy); err != nil {
				return err
			}
		}
	}
	return nil
}
