package pdfops

import (
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	"nib/internal/contentstream"
	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Replacing a text layer Nib added — ADR-101, /pending 851 part 4.
//
// # What a layer is, read from what the stamp writes and not from what it was meant to write
//
// Per word, appended to the page's content:
//
//	/Artifact <</Subtype /Watermark /Type /Pagination >>BDC q a b c d e f cm /GS0 gs /Fm0 Do Q EMC
//
// with `/P <</MCID n>> BDC` in the marker's place once `TagOCRLayer` has described it. `/Fm0` is a form of the
// word's own — `q BT /F1 12 Tf ET … cm BT … 3 Tr (…) Tj ET Q`, inside `/ReversedChars BMC … EMC` for a word set in
// reverse (ADR-097) — whose font is one of the faces Nib stamps a layer in, and which belongs to the optional-content
// group every pdfcpu stamp shares. `/GS0` is an entry in the page's `/ExtGState`. Over a tagged layer there is a `P`
// per paragraph holding the words' MCIDs, a `Sect` per block, and the page's row of the parent tree.
//
// # What counts as Nib's own, and why it is this narrow
//
// Taking out the wrong thing destroys a user's content, so a word is Nib's only when ALL of it is that shape: the
// marker (pdfcpu's watermark marker, by `watermarkArtifactSpans`, or an MCID), the placement and the draw
// (`placementAfter`, `formDrawnAfter` — the readers the fit pairs words by), and a form that sets one invisible
// string in an OCR face and paints nothing. A visible watermark fails the last test; another program's layer fails
// the first. And a PAGE is Nib's only when taking those words out leaves no invisible text on it at all, read by
// the rule that says a page has a layer (`hiddenTextOn`): a page that holds another program's words as well is left
// whole, because a page half replaced is a page whose words are found twice.

// TextLayerKind is whose text layer a page carries, which is whether it can be replaced.
type TextLayerKind string

const (
	// LayerOwn is a layer Nib stamped, and nothing else invisible: it can be taken out and stamped again.
	LayerOwn TextLayerKind = "own"
	// LayerOther is invisible text Nib did not stamp, or not only. It is left as it is.
	LayerOther TextLayerKind = "not-nibs-layer"
	// LayerStructure is Nib's layer under structure that cannot be taken out with it — an element that owns a word
	// and is not where the tree's own reading says it is. It is left as it is: content gone from under an element
	// that still names it is the one outcome worse than an old layer.
	LayerStructure TextLayerKind = "structure"
)

// hiddenTextOn is whether a page sets invisible text, as the page map reads it (`MapText.Hidden`): the one rule for
// "this page has a text layer" (ADR-094), asked before a layer is added and after one is taken out.
func hiddenTextOn(ctx *model.Context, pg pdfread.Page) (bool, error) {
	hidden, _, err := textOn(ctx, pg)
	return hidden, err
}

// textOn is the one read behind both answers about a page's text: hidden is "it carries invisible text" (a text
// layer), and unread is "it sets no glyph at all and yet paints something" — a scan nothing has read, or text drawn
// as outlines, which this reader cannot tell from one. A page that paints nothing is blank and is neither.
func textOn(ctx *model.Context, pg pdfread.Page) (hidden, unread bool, err error) {
	pr, err := readPageShapes(ctx, pg)
	if err != nil {
		return false, false, err
	}
	sp := newDisplaySpace(pg)
	for _, r := range pr.runs {
		if t, ok := mapText(sp, r); ok && t.Hidden {
			return true, false, nil
		}
	}
	return false, pr.noText && len(pr.marks)+len(pr.shapes) > 0, nil
}

// ocrFaceName is the face of a font Nib stamps an OCR layer in, from its /BaseFont with any subset tag taken off.
// The name is checked against the table and never handed on unchecked: a document chooses its own font names.
func ocrFaceName(baseFont string) (string, bool) {
	name := baseFont
	if i := strings.IndexByte(name, '+'); i >= 0 {
		name = name[i+1:]
	}
	if _, known := ocrFontFiles[name]; !known && name != ocrFont {
		return "", false
	}
	return name, true
}

// ownWord is one OCR word as Nib stamped it: the bytes of the page's content that draw it, the resources those
// bytes name, and the marked-content id a tagged layer gave it (-1 for the stamp's own artifact marker).
type ownWord struct {
	start, end int
	form       string
	gs         []string
	mcid       int
}

// ownOCRWords finds the words of Nib's own text layer in a page's content. See the file header for the shape; a
// sequence that departs from it anywhere is not one.
func ownOCRWords(ctx *model.Context, res types.Dict, src []byte, budget *formWalkBudget) []ownWord {
	toks := contentstream.Tokenize(src)
	watermark := map[int]bool{}
	for _, a := range watermarkArtifactSpans(src) {
		watermark[a.start] = true
	}
	// prev and next step over whitespace; -1 and len(toks) are the ends.
	prev := func(i int) int {
		for i--; i >= 0 && toks[i].Kind == contentstream.Whitespace; i-- {
		}
		return i
	}
	next := func(i int) int {
		for i++; i < len(toks) && toks[i].Kind == contentstream.Whitespace; i++ {
		}
		return i
	}
	op := func(i int, s string) bool {
		return i >= 0 && i < len(toks) && toks[i].Kind == contentstream.Operator && string(toks[i].Bytes(src)) == s
	}
	name := func(i int) (string, bool) {
		if i < 0 || i >= len(toks) || toks[i].Kind != contentstream.Operand {
			return "", false
		}
		s := string(toks[i].Bytes(src))
		return strings.TrimPrefix(s, "/"), len(s) > 1 && s[0] == '/'
	}
	forms := map[string]bool{} // a form's answer, asked once a page
	var out []ownWord
	for b := range toks {
		if !op(b, "BDC") {
			continue
		}
		// Backwards: the property list, flat as both writers write it, and the tag before it.
		closeAt := prev(b)
		if closeAt < 0 || toks[closeAt].Kind != contentstream.DictClose {
			continue
		}
		open, mcid := prev(closeAt), -1
		for ; open >= 0 && toks[open].Kind != contentstream.DictOpen; open = prev(open) {
			if toks[open].Kind != contentstream.Operand {
				open = -1
				break
			}
			if k, isName := name(prev(open)); isName && k == "MCID" {
				if n, err := strconv.Atoi(string(toks[open].Bytes(src))); err == nil && n >= 0 {
					mcid = n
				}
			}
		}
		tagAt := prev(open)
		tag, isTag := name(tagAt)
		if open < 0 || !isTag {
			continue
		}
		if tag == "Artifact" {
			if !watermark[toks[tagAt].Start] {
				continue
			}
			mcid = -1
		} else if mcid < 0 {
			continue
		}
		// Forwards: `q a b c d e f cm`, the graphics state, the one form, `Q EMC` — and nothing between.
		_, after, placed := placementAfter(src, toks[b+1:])
		if !placed {
			continue
		}
		form, drawn := formDrawnAfter(src, toks[b+1+after:])
		if !drawn {
			continue
		}
		w := ownWord{start: toks[tagAt].Start, form: form, mcid: mcid}
		do := next(b + after)
		for ; do < len(toks) && !op(do, "Do"); do = next(do) {
			if op(do, "gs") {
				if g, ok := name(prev(do)); ok {
					w.gs = append(w.gs, g)
				}
			}
		}
		q := next(do)
		emc := next(q)
		if !op(q, "Q") || !op(emc, "EMC") {
			continue
		}
		w.end = toks[emc].End
		own, asked := forms[form]
		if !asked {
			own = isOwnWordForm(ctx, res, form, budget)
			forms[form] = own
		}
		if own {
			out = append(out, w)
		}
	}
	return out
}

// ownWordFormOps is every operator pdfcpu writes into a text stamp's form, and `BMC`/`EMC` for ADR-097's bracket.
// None of them paints: a form holding any other operator draws something, and is not a word of a text layer.
var ownWordFormOps = map[string]bool{
	"q": true, "Q": true, "cm": true, "BT": true, "ET": true, "Tf": true, "Tw": true, "Td": true, "Tr": true,
	"RG": true, "rg": true, "Tj": true, "BMC": true, "EMC": true,
}

// isOwnWordForm is whether the form a page draws as name is one OCR word as Nib stamps it: one string, set
// invisibly (render mode 3) in a face Nib stamps a layer in, and nothing else.
func isOwnWordForm(ctx *model.Context, res types.Dict, name string, budget *formWalkBudget) bool {
	xobjs, err := ctx.DereferenceDict(res["XObject"])
	if err != nil || xobjs == nil {
		return false
	}
	ref, ok := xobjs[name].(types.IndirectRef)
	if !ok {
		return false
	}
	sd, _, err := ctx.DereferenceStreamDict(ref)
	if err != nil || sd == nil {
		return false
	}
	if st := sd.Dict.NameEntry("Subtype"); st == nil || *st != "Form" {
		return false
	}
	body := budget.formContent(sd, ref)
	if body == nil {
		return false
	}
	shows, mode, font := 0, "", ""
	var operands []string
	for _, tk := range contentstream.Tokenize(body) {
		switch tk.Kind {
		case contentstream.Whitespace:
			continue
		case contentstream.Operator:
			o := string(tk.Bytes(body))
			if !ownWordFormOps[o] {
				return false
			}
			last := ""
			if len(operands) > 0 {
				last = operands[len(operands)-1]
			}
			switch o {
			case "Tr":
				mode = last
			case "Tf":
				if len(operands) != 2 {
					return false
				}
				font = strings.TrimPrefix(operands[0], "/")
			case "Tj":
				if mode != "3" {
					return false
				}
				shows++
			case "BMC":
				if last != "/"+reversedCharsTag {
					return false
				}
			}
			operands = operands[:0]
		case contentstream.InlineImage:
			return false
		default:
			operands = append(operands, string(tk.Bytes(body)))
		}
	}
	if shows != 1 || font == "" {
		return false
	}
	fres, err := ctx.DereferenceDict(sd.Dict["Resources"])
	if err != nil || fres == nil {
		return false
	}
	fonts, err := ctx.DereferenceDict(fres["Font"])
	if err != nil || fonts == nil {
		return false
	}
	fd, err := ctx.DereferenceDict(fonts[font])
	if err != nil || fd == nil {
		return false
	}
	base := fd.NameEntry("BaseFont")
	if base == nil {
		return false
	}
	_, isFace := ocrFaceName(*base)
	return isFace
}

// layerRemoval takes Nib's own text layer out of the pages of one context, with whatever structure describes it.
// The same code answers "can this page's layer be replaced" (`TextPages`, on a context that is thrown away) and
// does the replacing (`removeOwnTextLayers`), so what the window is told and what the route does cannot differ.
type layerRemoval struct {
	ctx    *model.Context
	budget *formWalkBudget
	// tree is the document's structure, read at the first tagged word; noTree says there is none to read.
	tree    *structTree
	noTree  bool
	treeErr error
	owners  map[[2]int][]*structElem // (page object, MCID) -> the elements whose /K names it
	touched bool                     // the tree lost something
}

func newLayerRemoval(ctx *model.Context) *layerRemoval {
	return &layerRemoval{ctx: ctx, budget: newFormWalkBudget(ctx.PageCount)}
}

// structure reads the tree once, and indexes which element owns which marked content of a page's own stream.
func (rm *layerRemoval) structure() {
	if rm.tree != nil || rm.noTree || rm.treeErr != nil {
		return
	}
	tree, err := readStructTree(rm.ctx, livePageObjects(rm.ctx))
	if err == errNoStructTree {
		rm.noTree = true
		return
	}
	if err != nil {
		rm.treeErr = err
		return
	}
	rm.tree, rm.owners = tree, map[[2]int][]*structElem{}
	for _, e := range tree.elems {
		for _, k := range e.kids {
			if (k.kind == kidMCID || k.kind == kidMCR) && k.stm == 0 && !k.stmMalformed {
				at := [2]int{k.pgObj, k.mcid}
				rm.owners[at] = append(rm.owners[at], e)
			}
		}
	}
}

// kidOnPage is the marked-content id a raw `/K` entry of e names in the own stream of the page whose object is
// pageObj — a bare integer, or a marked-content reference with no `/Stm`.
func (rm *layerRemoval) kidOnPage(e *structElem, en types.Object, pageObj int) (int, bool) {
	if n, ok := en.(types.Integer); ok {
		return n.Value(), e.pgObj == pageObj
	}
	d, err := rm.ctx.DereferenceDict(en)
	if err != nil || d == nil {
		return 0, false
	}
	if ty := d.NameEntry("Type"); ty == nil || *ty != "MCR" {
		return 0, false
	}
	if _, inStream := d["Stm"]; inStream {
		return 0, false
	}
	n, ok := d["MCID"].(types.Integer)
	if !ok {
		return 0, false
	}
	pg := e.pgObj
	if ir, isRef := d["Pg"].(types.IndirectRef); isRef {
		pg = ir.ObjectNumber.Value()
	}
	return n.Value(), pg == pageObj
}

// elementEdit is one element's `/K` with the layer's words taken out, worked out before anything is written.
type elementEdit struct {
	e    *structElem
	kept types.Array
	set  func(types.Array)
}

// planStructure works out what leaves the tree with the tagged words of one page, and refuses — writing nothing —
// when the tree cannot be made to say exactly that.
func (rm *layerRemoval) planStructure(pg pdfread.Page, words []ownWord) ([]elementEdit, bool) {
	mine := map[int]bool{}
	for _, w := range words {
		if w.mcid >= 0 {
			mine[w.mcid] = true
		}
	}
	if len(mine) == 0 {
		return nil, true
	}
	rm.structure()
	if rm.noTree {
		return nil, true // marked content nothing describes: there is nothing to take out of a tree
	}
	if rm.treeErr != nil || pg.Ref == nil {
		return nil, false
	}
	pageObj := pg.Ref.ObjectNumber.Value()
	want := map[*structElem]int{}
	var order []*structElem
	for mcid := range mine {
		owners := rm.owners[[2]int{pageObj, mcid}]
		if len(owners) > 1 || (len(owners) == 1 && owners[0].objNr == 0) {
			return nil, false // owned twice, or by an element no reference can name
		}
		if len(owners) == 1 {
			if want[owners[0]] == 0 {
				order = append(order, owners[0])
			}
			want[owners[0]]++
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i].objNr < order[j].objNr })
	var edits []elementEdit
	for _, e := range order {
		kids, set := kidsArray(rm.ctx, e.dict)
		kept, gone := kids[:0:0], 0
		for _, en := range kids {
			if m, here := rm.kidOnPage(e, en, pageObj); here && mine[m] {
				gone++
				continue
			}
			kept = append(kept, en)
		}
		// The tree's own reading said this element owns that many of the words; its /K must give them all up.
		if gone != want[e] {
			return nil, false
		}
		edits = append(edits, elementEdit{e: e, kept: kept, set: set})
	}
	return edits, true
}

// removeFrom takes Nib's own layer off one page, or leaves the page exactly as it was and says what it holds.
func (rm *layerRemoval) removeFrom(pg pdfread.Page) (TextLayerKind, error) {
	if pg.Err != nil || pg.Dict == nil {
		return LayerOther, nil
	}
	src, err := pdfread.PageContent(rm.ctx, pg.Dict, pg.Nr)
	if err != nil {
		return LayerOther, nil
	}
	var res types.Dict
	if pg.Attrs != nil {
		res = pg.Attrs.Resources
	}
	words := ownOCRWords(rm.ctx, res, src, rm.budget)
	if len(words) == 0 {
		return LayerOther, nil
	}
	edits, ok := rm.planStructure(pg, words)
	if !ok {
		return LayerStructure, nil
	}
	edit := contentstream.NewEdit(src)
	for _, w := range words {
		edit.Replace(w.start, w.end, nil)
	}
	edited, err := edit.Apply()
	if err != nil {
		return LayerOther, nil
	}
	edited = peelStampWrappers(edited, len(words))
	// The content first, and read back by the rule that found the layer: if invisible text is still there, the
	// page holds words Nib did not stamp, and it is put back as it was.
	was, had := pg.Dict["Contents"] //pagecontent:key the entry itself, kept to put the page back; the content is read above
	if err := setPageContent(rm.ctx, pg.Dict, edited); err != nil {
		return "", err
	}
	if still, herr := hiddenTextOn(rm.ctx, pg); herr != nil || still {
		if had {
			pg.Dict["Contents"] = was
		} else {
			delete(pg.Dict, "Contents")
		}
		return LayerOther, nil
	}
	if err := rm.applyStructure(pg, words, edits); err != nil {
		return "", err
	}
	rm.dropResources(pg, res, edited, words)
	return LayerOwn, nil
}

// peelStampWrappers takes off the `q … Q` pdfcpu put round the whole of a page's content each time it stamped a
// word onto it: every stamp wraps what was there, so a page of n words is n levels deep, and a layer replaced
// without this would leave them and add n more. At most n pairs, and only a `q` that opens the content and is
// closed by the `Q` that ends it — a pair like that changes nothing the page draws. Never more than n: a pair
// beyond the stamp's own may be the page's, and what it restores matters to whatever is drawn after it.
func peelStampWrappers(src []byte, n int) []byte {
	var toks []contentstream.Token
	for _, tk := range contentstream.Tokenize(src) {
		if tk.Kind != contentstream.Whitespace {
			toks = append(toks, tk)
		}
	}
	closes := map[int]int{} // a `q`'s index -> its `Q`'s
	var open []int
	for i, tk := range toks {
		if tk.Kind != contentstream.Operator {
			continue
		}
		switch string(tk.Bytes(src)) {
		case "q":
			open = append(open, i)
		case "Q":
			if len(open) == 0 {
				return src // more closed than opened: not content to reason about
			}
			closes[open[len(open)-1]] = i
			open = open[:len(open)-1]
		}
	}
	edit := contentstream.NewEdit(src)
	for lo, hi := 0, len(toks)-1; n > 0 && lo < hi; lo, hi, n = lo+1, hi-1, n-1 {
		if at, ok := closes[lo]; !ok || at != hi {
			break
		}
		edit.Replace(toks[lo].Start, toks[lo].End, nil)
		edit.Replace(toks[hi].Start, toks[hi].End, nil)
	}
	out, err := edit.Apply()
	if err != nil {
		return src
	}
	return out
}

// applyStructure writes a plan: the words leave their elements, an element left with nothing leaves its parent —
// and so on up — and the page's parent-tree slots for those words are emptied.
func (rm *layerRemoval) applyStructure(pg pdfread.Page, words []ownWord, edits []elementEdit) error {
	if rm.tree == nil {
		return nil
	}
	for _, ed := range edits {
		ed.set(ed.kept)
		rm.touched = true
		for e := ed.e; e != nil && e.objNr != 0; e = e.parent {
			if kids, _ := kidsArray(rm.ctx, e.dict); len(kids) > 0 {
				break
			}
			if _, err := removeFromParent(rm.ctx, rm.tree, e); err != nil {
				return err
			}
		}
	}
	key, has, _ := structParentsOf(rm.ctx.XRefTable, pg.Dict)
	if !has {
		return nil
	}
	for _, w := range words {
		if w.mcid < 0 {
			continue
		}
		if err := clearParentTreeSlot(rm.ctx, rm.tree, key, w.mcid); err != nil {
			return err
		}
	}
	return nil
}

// dropResources takes the words' forms and graphics states out of the page's resources, where what is left of the
// page's content no longer names them. The page gets a dictionary of its own to take them out of: resources are
// shared between pages and inherited from the tree, and another page may draw by the same name.
func (rm *layerRemoval) dropResources(pg pdfread.Page, res types.Dict, edited []byte, words []ownWord) {
	if res == nil {
		return
	}
	used := map[string]bool{}
	for _, tk := range contentstream.Tokenize(edited) {
		if tk.Kind == contentstream.Operand {
			if s := string(tk.Bytes(edited)); len(s) > 1 && s[0] == '/' {
				used[s[1:]] = true
			}
		}
	}
	forms, states := map[string]bool{}, map[string]bool{}
	for _, w := range words {
		if !used[w.form] {
			forms[w.form] = true
		}
		for _, g := range w.gs {
			if !used[g] {
				states[g] = true
			}
		}
	}
	own := maps.Clone(res)
	for key, gone := range map[string]map[string]bool{"XObject": forms, "ExtGState": states} {
		d, err := rm.ctx.DereferenceDict(res[key])
		if err != nil || d == nil || len(gone) == 0 {
			continue
		}
		d = maps.Clone(d)
		for n := range gone {
			delete(d, n)
		}
		if len(d) == 0 {
			delete(own, key)
		} else {
			own[key] = d
		}
	}
	pg.Dict["Resources"] = own
}

// finish settles the tree once every page is done. A tree the layer's words were the whole of is taken off the
// catalog with its claim: a document that says it is tagged over an empty tree is the claim ADR-031 forbids, and
// before the layer was added the document made none.
func (rm *layerRemoval) finish() error {
	if rm.tree == nil || !rm.touched {
		return nil
	}
	if kids, _ := kidsArray(rm.ctx, rm.tree.root); len(kids) > 0 {
		return nil
	}
	cat, err := rm.ctx.XRefTable.Catalog()
	if err != nil {
		return err
	}
	delete(cat, "StructTreeRoot")
	delete(cat, "MarkInfo")
	for _, pg := range pdfread.Pages(rm.ctx) {
		if pg.Dict != nil {
			delete(pg.Dict, "StructParents")
		}
	}
	return nil
}

// TextPages is whose text layer each page of pdf carries — a page with none is not in layers — and which pages
// nothing has read. layers is `PagesWithTextLayer` with the second question answered — can this page's layer be
// replaced — by taking the layer out of a copy and seeing what is left, which is the answer the replacing itself
// will reach. (It was `TextLayers` until ADR-106 added the second answer.)
//
// unread is, from the same read of the document, the pages that set no text and are not blank (`textOn`), in page
// order — the pages a command that needs text has read for it (ADR-106). The page map's `NoText` is the same
// reader's answer for one page; this is every page for one parse of the file, where asking the map page by page
// parses it once a page. A page that will not read is in neither.
func TextPages(pdf []byte) (layers map[int]TextLayerKind, unread []int, err error) {
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil, nil, err
	}
	layers = map[int]TextLayerKind{}
	unread = []int{}
	rm := newLayerRemoval(ctx)
	for _, pg := range pdfread.Pages(ctx) {
		has, bare, herr := textOn(ctx, pg)
		if herr != nil {
			continue
		}
		if bare {
			unread = append(unread, pg.Nr)
		}
		if !has {
			continue
		}
		kind, rerr := rm.removeFrom(pg)
		if rerr != nil {
			kind = LayerStructure
		}
		layers[pg.Nr] = kind
	}
	return layers, unread, nil
}

// removeOwnTextLayers takes Nib's own text layer off the named pages. removed is the pages it came off; left is the
// named pages that have a layer and keep it, with why. With nothing removed pdf comes back as it was given.
func removeOwnTextLayers(pdf []byte, pages map[int]bool) (out []byte, removed []int, left map[int]TextLayerKind, err error) {
	left = map[int]TextLayerKind{}
	out, err = writeMutated(pdf, func(ctx *model.Context) error {
		rm := newLayerRemoval(ctx)
		for _, pg := range pdfread.Pages(ctx) {
			if !pages[pg.Nr] {
				continue
			}
			if has, herr := hiddenTextOn(ctx, pg); herr != nil || !has {
				continue
			}
			kind, rerr := rm.removeFrom(pg)
			if rerr != nil {
				return fmt.Errorf("pdfops: page %d: its text layer could not be taken out: %w", pg.Nr, rerr)
			}
			if kind == LayerOwn {
				removed = append(removed, pg.Nr)
			} else {
				left[pg.Nr] = kind
			}
		}
		return rm.finish()
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if len(removed) == 0 {
		return pdf, nil, left, nil
	}
	return out, removed, left, nil
}

// ReplaceOCRLayer stamps words as `TagOCRLayer` does, first taking Nib's own earlier layer off each page the words
// are for. It is what a user ASKS for — "read these pages again" — and never what a second OCR does: ADR-094's rule
// stands, and a page whose layer is not Nib's own is left as it is whatever is asked (ADR-101).
//
// replaced is the pages whose layer was taken out and stamped again; left is the pages that have a layer, keep it
// and got none of these words, with why. A page is only ever replaced by words: one the request carries no word
// for keeps the layer it has. out is nil when nothing was changed.
func ReplaceOCRLayer(pdf []byte, words []Word, lang string) (out []byte, tagged bool, replaced []int, left map[int]TextLayerKind, err error) {
	pages := map[int]bool{}
	for _, w := range words {
		if strings.TrimSpace(w.Text) != "" {
			pages[w.Page] = true
		}
	}
	stripped, replaced, left, err := removeOwnTextLayers(pdf, pages)
	if err != nil {
		return nil, false, nil, nil, err
	}
	kept := make([]Word, 0, len(words))
	for _, w := range words {
		if _, stays := left[w.Page]; !stays {
			kept = append(kept, w)
		}
	}
	if len(kept) == 0 {
		return nil, false, nil, left, nil
	}
	out, tagged, err = TagOCRLayer(stripped, kept, lang)
	if err != nil {
		return nil, false, nil, nil, err
	}
	return out, tagged, replaced, left, nil
}
