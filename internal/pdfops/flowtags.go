package pdfops

import (
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
)

// A tagged paragraph keeps its structure when it changes page — `PLAN-text-reflow.md` P07.S07.
//
// Every multi-page real-producer document is tagged (14 of 14), so a carry that refused marked content reached none of
// them. A marked-content sequence now goes to the other page WHOLE, with the element that owns it:
//
//   - the source page loses the sequence's `BDC` and `EMC` with its shows (`deleteRuns`), and its `/ParentTree` slot is
//     cleared — nothing on that page claims the element any more;
//   - the target page draws the runs inside a sequence of the same tag and property list, under an MCID free there — above
//     both its row and every MCID its own stream draws, so neither a short row nor a stale one can hand out one in use;
//   - the element's kid naming the old content is REPLACED IN PLACE by an MCR naming the target page — never an appended
//     integer (which means "on the element's `/Pg`", wrong when the element keeps content on the source page) and never a
//     re-pointed `/Pg` (which would move the element's other content with it). Reading order is the tree's, and the kid
//     keeps its place in it.
//
// A sequence that also draws text that STAYS is split, not refused: the source keeps the sequence, its slot and its kid
// for what stays, and what leaves is a new sequence on the target page and a new MCR of the same element, right after the
// kid it came from — so the element reads its text in order across the two pages. Measured: 90 of the 124 real-producer
// carries still refused after the whole-sequence cut were this shape, 26 more a clipping path inside the sequence, which
// paints nothing and is now allowed.
//
// What cannot be carried is refused `tagged-across-pages` (law 3; ADR-031 — nothing claims tagging it has not): a sequence
// that paints anything, or opens one inside it; one nested in another MCID's; one drawing a form or left open; an MCID no
// element claims; and a property list with an indirect value.

// tagMove is one marked-content sequence a carry takes to another page.
type tagMove struct {
	from int               // its MCID on the source page
	elem types.IndirectRef // the element whose kid it is
	// part says the sequence also draws text that stays: the source keeps it, its slot and its kid, and the carried text
	// becomes a second kid of the same element, after the first.
	part bool
	// opener is the sequence's `BDC` with its MCID cut out: tag and property list up to the number, and after it.
	pre, post string
	to        int // its MCID on the target page, once the target is known
}

// tagCarry is the structure a carry takes with it from src: its moves in the order the runs first name them.
type tagCarry struct {
	src    pdfread.Page
	srcKey int
	moves  []*tagMove
	byMCID map[int]*tagMove
}

// nonDrawing are the operators a carried sequence may hold besides its shows: text and graphics state, and paths that are
// built or clipped by and never painted — the source keeps running them once the sequence's brackets are gone (ISO
// 32000-1 Table 51, Table 105, Table 74). A clip that would cut a leaving run has refused it already (`carryRefusal`).
var nonDrawing = map[string]bool{
	"BT": true, "ET": true, "Tf": true, "Tc": true, "Tw": true, "Tz": true, "TL": true, "Ts": true, "Tr": true,
	"Td": true, "TD": true, "Tm": true, "T*": true, "q": true, "Q": true, "cm": true, "w": true, "J": true, "j": true,
	"M": true, "d": true, "ri": true, "i": true, "gs": true, "g": true, "G": true, "rg": true, "RG": true, "k": true,
	"K": true, "cs": true, "CS": true, "sc": true, "SC": true, "scn": true, "SCN": true,
	// Path construction and clipping paint nothing (§8.5.2, §8.5.4); `n` ends a path unpainted.
	"m": true, "l": true, "c": true, "v": true, "y": true, "h": true, "re": true, "W": true, "W*": true, "n": true,
}

// planTagCarry plans taking the structure of runs — leaving page pg, whose content is src, resources res and marked-content
// sequences seqs — to another page: the moves, and the source edits that take each sequence's brackets away. Runs with no
// MCID carry nothing. It returns nil when no run is tagged, or the cause.
func planTagCarry(ctx *model.Context, pg pdfread.Page, src []byte, res types.Dict, seqs []markedSeq, runs []textRun) (*tagCarry, []runMove, string) {
	leavingEnds := map[int]bool{}
	var mcids []int
	seen := map[int]bool{}
	for _, r := range runs {
		leavingEnds[r.span.end] = true
		if r.mcid >= 0 && !seen[r.mcid] {
			seen[r.mcid] = true
			mcids = append(mcids, r.mcid)
		}
	}
	if len(mcids) == 0 {
		return nil, nil, ""
	}
	xt := ctx.XRefTable
	key, ok, _ := structParentsOf(xt, pg.Dict)
	cat, err := xt.Catalog()
	if !ok || err != nil {
		return nil, nil, causeTaggedAcross
	}
	root := derefDict(xt, cat["StructTreeRoot"])
	if root == nil {
		return nil, nil, causeTaggedAcross
	}
	row, _, found := rowFor(ctx, &structTree{root: root}, key)
	if !found || pg.Ref == nil {
		return nil, nil, causeTaggedAcross
	}
	tc := &tagCarry{src: pg, srcKey: key, byMCID: map[int]*tagMove{}}
	var edits []runMove
	for _, m := range mcids {
		seq, part, ok := carriedSequence(src, seqs, m, leavingEnds)
		if !ok || m >= len(row) {
			return nil, nil, causeTaggedAcross
		}
		elem, isRef := row[m].(types.IndirectRef)
		if !isRef {
			return nil, nil, causeTaggedAcross
		}
		if _, _, i := mcidKid(ctx, elem, *pg.Ref, m); i < 0 {
			return nil, nil, causeTaggedAcross // the tree says the element owns it and the element does not say so
		}
		pre, post, ok := openerAround(xt, src[seq.opener.start:seq.opener.end], res)
		if !ok {
			return nil, nil, causeTaggedAcross
		}
		mv := &tagMove{from: m, elem: elem, part: part, pre: pre, post: post}
		tc.moves = append(tc.moves, mv)
		tc.byMCID[m] = mv
		if !part {
			edits = append(edits, runMove{span: seq.opener, with: []byte(" ")}, runMove{span: seq.close, with: []byte(" ")})
		}
	}
	return tc, edits, ""
}

// carriedSequence is the one sequence of the page's own stream carrying MCID m, when it paints nothing but text — no
// sequence opened inside it, none enclosing it that carries an MCID of its own, no form drawn in it, and closed in the
// stream it opened in — and whether it is only PART of what leaves: it shows text that stays too.
func carriedSequence(src []byte, seqs []markedSeq, m int, leavingEnds map[int]bool) (markedSeq, bool, bool) {
	var seq markedSeq
	n := 0
	for _, s := range seqs {
		if s.mcid == m && !s.inForm {
			seq, n = s, n+1
		}
	}
	if n != 1 || seq.drawsForm || seq.close == (opSpan{}) || seq.close.start < seq.opener.end {
		return markedSeq{}, false, false
	}
	for _, s := range seqs {
		if !s.inForm && s.opener.start < seq.opener.start && (s.close == (opSpan{}) || s.close.start > seq.opener.start) {
			return markedSeq{}, false, false // nested in another MCID's sequence, which owns this text too
		}
	}
	body := src[seq.opener.end:seq.close.start]
	part := false
	for _, tk := range contentstream.Tokenize(body) {
		if tk.Kind == contentstream.InlineImage {
			return markedSeq{}, false, false
		}
		if tk.Kind != contentstream.Operator {
			continue
		}
		op := string(tk.Bytes(body))
		switch {
		case op == "Tj" || op == "TJ" || op == "'" || op == "\"":
			if !leavingEnds[seq.opener.end+tk.End] {
				part = true // text that stays: the sequence is split across the two pages
			}
		case !nonDrawing[op]:
			return markedSeq{}, false, false
		}
	}
	return seq, part, true
}

// openerAround cuts a `BDC` opener's MCID out: what is written before the number and after it, with the tag, so the
// sequence can be opened again under another. A property list named in the resources' `/Properties` is written inline,
// and refused when a value in it is indirect — an inline dictionary cannot hold a reference.
func openerAround(xt *model.XRefTable, opener []byte, res types.Dict) (pre, post string, ok bool) {
	toks := contentstream.Tokenize(opener)
	var ops []contentstream.Token
	for _, t := range toks {
		if t.Kind != contentstream.Whitespace {
			ops = append(ops, t)
		}
	}
	if len(ops) < 3 || ops[0].Kind != contentstream.Operand || string(ops[len(ops)-1].Bytes(opener)) != "BDC" {
		return "", "", false
	}
	tag := string(ops[0].Bytes(opener))
	if ops[1].Kind == contentstream.DictOpen {
		for i := 1; i+1 < len(ops); i++ {
			if string(ops[i].Bytes(opener)) == "/MCID" && ops[i+1].Kind == contentstream.Operand {
				body := opener[ops[1].Start:ops[len(ops)-1].Start]
				at, end := ops[i+1].Start-ops[1].Start, ops[i+1].End-ops[1].Start
				return tag + " " + string(body[:at]), string(body[end:]), true
			}
		}
		return "", "", false
	}
	name := strings.TrimPrefix(string(ops[1].Bytes(opener)), "/")
	props := derefDict(xt, derefDict(xt, res["Properties"])[name])
	if props == nil {
		return "", "", false
	}
	d := types.Dict{}
	for k, v := range props {
		if k == "MCID" {
			continue
		}
		if !directValue(v) {
			return "", "", false
		}
		d[k] = v
	}
	s := d.PDFString()
	return tag + " " + strings.TrimSuffix(s, ">>") + "/MCID ", ">>", true
}

// directValue says whether o holds no indirect reference at any depth.
func directValue(o types.Object) bool {
	switch v := o.(type) {
	case types.IndirectRef:
		return false
	case types.Array:
		for _, e := range v {
			if !directValue(e) {
				return false
			}
		}
	case types.Dict:
		for _, e := range v {
			if !directValue(e) {
				return false
			}
		}
	}
	return true
}

// mcidKid finds the kid of elem naming MCID m of page pg's own stream: an integer on the page the element's content is
// on (its `/Pg`, inherited from the nearest ancestor naming one), or an MCR naming m on pg with no `/Stm`. It returns the
// element's kids, where they are written back, and the kid's index.
func mcidKid(ctx *model.Context, elem types.IndirectRef, pg types.IndirectRef, m int) (types.Array, func(types.Array), int) {
	xt := ctx.XRefTable
	d := derefDict(xt, elem)
	if d == nil {
		return nil, nil, -1
	}
	kids, set := kidsArray(ctx, d)
	onPage := func(o types.Object) bool { return sameObject(o, pg) }
	inherited := func() bool {
		o := types.Object(elem)
		// ADR-009, a named exemption: `structTree.readKid` answers the same question top-down during a whole-tree walk this
		// carry does not make; this walks `/P` upward from one element. The two agree wherever `/P` is the element's parent
		// in `/K`, which `checkStructConsistency` holds every writer of nib's to.
		for depth := 0; depth < maxStructDepth; depth++ {
			e := derefDict(xt, o)
			if e == nil {
				return false
			}
			if p, has := e["Pg"]; has {
				return onPage(p)
			}
			o = e["P"]
		}
		return false
	}
	for i, k := range kids {
		if n, ok := k.(types.Integer); ok {
			if n.Value() == m && inherited() {
				return kids, set, i
			}
			continue
		}
		mcr := derefDict(xt, k)
		if mcr == nil {
			continue
		}
		if t := mcr.NameEntry("Type"); t == nil || *t != "MCR" {
			continue
		}
		if n, ok := pdfNumber(xt, mcr["MCID"]); !ok || int(n) != m {
			continue
		}
		if _, stm := mcr["Stm"]; stm {
			continue
		}
		if p, has := mcr["Pg"]; has && onPage(p) || !has && inherited() {
			return kids, set, i
		}
	}
	return nil, nil, -1
}

// targetRefusal is why page dst cannot take a carried sequence's structure, or "": its `/StructParents` is not a key, or
// names a single reference rather than a row, or `parentTreeDict` would refuse to place its row — asked through the same
// non-mutating predicate the writer asks (`parentTreePlace`), so every refusal of the writer's is one here (P07
// phase-close review: this restated the nested-range refusal and missed an empty `/Kids`, a tree too deep and a kid that
// does not resolve). A page with no key gets one above every key (`allocParentTreeKey`), which is asked about too. Asked
// before `land` writes anything, so the carry refuses by name rather than failing half-written — and it writes nothing
// itself: not a key, not a `/ParentTree`.
func targetRefusal(ctx *model.Context, dst pdfread.Page) string {
	xt := ctx.XRefTable
	cat, err := xt.Catalog()
	if err != nil {
		return causeTaggedAcross
	}
	root := derefDict(xt, cat["StructTreeRoot"])
	if root == nil {
		return causeTaggedAcross
	}
	tree := &structTree{root: root}
	key, ok, written := structParentsOf(xt, dst.Dict)
	switch {
	case written && !ok:
		return causeTaggedAcross
	case !written:
		key = parentTreeKeyFloor(ctx, root) // what `allocParentTreeKey` hands out, read without caching it
	}
	if _, single, _ := parentTreeKey(ctx, tree, key); single {
		return causeTaggedAcross
	}
	if _, _, err := parentTreePlace(ctx, tree, key); err != nil {
		return causeTaggedAcross
	}
	return ""
}

// land gives each move an MCID on page dst — whose own stream draws the sequences dstSeqs — and writes the tree: the
// target row gains the element at it, and the element's kid becomes an MCR naming dst while the source row forgets it —
// or, for part of a sequence, the MCR joins the element after the kid that stays.
func (tc *tagCarry) land(ctx *model.Context, dst pdfread.Page, dstSeqs []markedSeq) error {
	if dst.Ref == nil || tc.src.Ref == nil {
		return fmt.Errorf("pdfops: a page a paragraph's structure moves between has no reference")
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		return err
	}
	root := derefDict(ctx.XRefTable, cat["StructTreeRoot"])
	if root == nil {
		return fmt.Errorf("pdfops: the structure tree went away under a carry")
	}
	tree := &structTree{root: root}
	key, next, err := pageRow(ctx, tree, dst.Dict, dst.Nr)
	if err != nil {
		return err
	}
	for _, s := range dstSeqs {
		if !s.inForm && s.mcid >= next {
			next = s.mcid + 1
		}
	}
	for _, mv := range tc.moves {
		kids, set, i := mcidKid(ctx, mv.elem, *tc.src.Ref, mv.from)
		if i < 0 {
			return fmt.Errorf("pdfops: element %d no longer names /MCID %d on page %d", mv.elem.ObjectNumber.Value(), mv.from, tc.src.Nr)
		}
		mv.to, next = next, next+1
		mcr := types.Dict{"Type": types.Name("MCR"), "Pg": *dst.Ref, "MCID": types.Integer(mv.to)}
		if mv.part {
			kids = append(kids[:i+1], append(types.Array{mcr}, kids[i+1:]...)...)
			set(kids)
		} else {
			kids[i] = mcr
			set(kids)
			followsItsContent(ctx, mv.elem, kids, *tc.src.Ref, *dst.Ref)
			if err := clearParentTreeSlot(ctx, tree, tc.srcKey, mv.from); err != nil {
				return err
			}
		}
		if err := setParentTreeSlot(ctx, tree, key, mv.to, mv.elem); err != nil {
			return err
		}
	}
	return nil
}

// followsItsContent re-points elem's own `/Pg` from src to dst when nothing is left on src for it to describe and nothing
// reads it: every kid names its own page, and none names src. The structure reader places an element on its `/Pg`
// (`readStructureView`), so an element whose content all went to dst would otherwise be shown on src with nothing there
// to outline. A kid that inherits the `/Pg` — an integer MCID, an MCR or OBJR without one, an element without one — keeps
// it where it is: re-pointing would move that kid's content with it.
func followsItsContent(ctx *model.Context, elem types.IndirectRef, kids types.Array, src, dst types.IndirectRef) {
	xt := ctx.XRefTable
	d := derefDict(xt, elem)
	if d == nil || !sameObject(d["Pg"], src) {
		return
	}
	for _, k := range kids {
		kd := derefDict(xt, k)
		if kd == nil {
			return // an integer kid: its content is on the element's /Pg
		}
		pg, has := kd["Pg"]
		if !has || sameObject(pg, src) {
			return
		}
	}
	d["Pg"] = dst
}

// opener is the move's `BDC`, under its target MCID.
func (mv *tagMove) opener() string { return fmt.Sprintf("%s%d%s BDC", mv.pre, mv.to, mv.post) }
