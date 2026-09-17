package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
)

// Carrying a sticky note through an n-up — ADR-045, `/pending 562`.
//
// # What `api.NUp` does to an annotation
//
// It composes the sheets as NEW page objects holding each source page's content inside a Form
// XObject, and it never looks at `/Annots`. The source page dictionaries then leave the page tree,
// so pdfcpu — which writes by reachability — drops every annotation with them. **Measured on a
// three-page document carrying one `AddNotes` sticky note per page: 3 annotations in, 0 out.**
//
// A sticky note is the user's own comment, so that is silent loss of authored content on an
// operation nothing warns about. The consequence that made it visible is `/pending 488`'s golden:
// `ContentDigest` of `notes-then-nup` was byte-identical to that of `nup2`, because there was
// nothing left of the notes to hash.
//
// **And it cost more than the notes.** `describeNotes` nests a note of a TAGGED document in an
// `/Annot` structure element with its own `/ParentTree` key, claimed by the annotation's
// `/StructParent`. With the annotation gone that key is claimed by nobody, which is
// `structureCarriedCompletely`'s `unowned-key` — so `completeOrHonest` abandoned the whole carry and
// `honest` deleted `/StructTreeRoot`. Measured on `collidingMCIDFixture`: the same document n-ups
// `carried` without notes and `dropped` with them, `unowned-key key=2` and `key=3`. Carrying the
// annotation restores the claim, so this door is what lets a noted tagged document keep its tree.
//
// # The transform, and where the matrix lives
//
// pdfcpu draws each source page as `q a b c d e f cm /FmN Do Q` on its sheet
// (`pdfcpu/model/nup.go:280`), so **the matrix is in the sheet's own content stream** and is read
// out of the document rather than re-derived from pdfcpu's best-fit arithmetic. Two further
// matrices compose with it, and neither is optional:
//
//   - the form's `/Matrix`, which pdfcpu writes as a translation by `-cropBox.LL`
//     (`createNUpFormForPDF`). It is the identity only for a page whose crop box starts at the
//     origin; a cropped page's notes would land by the crop offset if it were assumed away.
//   - a page-rotation prefix. A source page with `/Rotate` has
//     `ContentBytesForPageRotation`'s `cm` prepended INSIDE the form, so form space is not page
//     space. It is read back as the form content's prefix, never recomputed.
//
// So a point of page space reaches sheet space through `R × FormMatrix × CM`, in that order.
// pdfcpu's best fit rotates by a multiple of 90° only, so a rect's corner-bounding-box transform is
// **exact** rather than an over-approximation.
//
// # Which annotations, and the boundary is declared
//
// `/Text` only. A sticky note is drawn by the viewer as a fixed-size icon at its `/Rect` — nib's
// own notes carry no appearance stream at all (`notes.go`) — so a rotating placement moves it
// correctly. **An annotation WITH an appearance stream would not survive one**: §12.5.5 maps
// `/AP`'s bounding box onto `/Rect` axis-aligned, so a highlight, an ink drawing or a widget would
// be drawn upright inside a rotated box. Carrying those needs the appearance transformed too, which
// is a different job; `carryNoteAnnots` counts what it left behind so the residue is a figure
// rather than a silence.
//
// **`/Widget` is excluded on a second ground and it is not cosmetic.** A signature widget reaching
// a `/V` blob would come back onto a document whose byte ranges the composition has destroyed, and
// four gates key on an edited document having no signature at all — `pageselect.go`'s
// `dropSignature` enumerates them. `api.NUp` leaves `sign.Verify` reading `unsigned` (measured);
// nothing here changes that.
//
// # It is attempted and abandoned, never half-done
//
// Any disagreement — a placement count that is not the source page count, a form whose content is
// not its source page's, a prefix that is not exactly one `cm` — abandons the whole carry and
// returns the composed document untouched. A document that gains some of its notes and loses others
// silently is worse than one that loses them all, because only the second is a fact a user can
// state.

// noteCarry is what a carry did: the notes placed, and the annotations it did not attempt.
type noteCarry struct {
	carried int // `/Text` annotations placed on a sheet
	left    int // annotations of any other subtype, which an n-up drops
}

// matrix is a PDF transformation matrix [a b c d e f], applied to a row vector:
// x' = a·x + c·y + e, y' = b·x + d·y + f.
type matrix [6]float64

var identityMatrix = matrix{1, 0, 0, 1, 0, 0}

// mul returns the matrix that applies m first and then n — `m × n` in ISO 32000-1's row-vector
// convention, which is also the order a content stream composes `cm` operators in.
func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[1]*n[2],
		m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2],
		m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4],
		m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

func (m matrix) apply(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

// rect transforms an axis-aligned rectangle and returns the bounding box of the image. Under a
// rotation by a multiple of 90° — the only rotations pdfcpu's n-up produces — the image IS an
// axis-aligned rectangle, so this is exact.
func (m matrix) rect(llx, lly, urx, ury float64) (float64, float64, float64, float64) {
	xs := make([]float64, 0, 4)
	ys := make([]float64, 0, 4)
	for _, p := range [4][2]float64{{llx, lly}, {urx, lly}, {urx, ury}, {llx, ury}} {
		x, y := m.apply(p[0], p[1])
		xs = append(xs, x)
		ys = append(ys, y)
	}
	minX, maxX, minY, maxY := xs[0], xs[0], ys[0], ys[0]
	for i := 1; i < 4; i++ {
		if xs[i] < minX {
			minX = xs[i]
		}
		if xs[i] > maxX {
			maxX = xs[i]
		}
		if ys[i] < minY {
			minY = ys[i]
		}
		if ys[i] > maxY {
			maxY = ys[i]
		}
	}
	return minX, minY, maxX, maxY
}

// capturedNote is one `/Text` annotation lifted out of the source document.
//
// `objNr` is its object number THERE, and it is kept for one reason: a tagged document's `/Annot`
// structure element reaches the annotation through an `OBJR` whose `/Obj` names exactly that
// number. `api.NUp` copies the structure tree wholesale, so the OBJR survives the composition and
// keeps the source annotation object ALIVE — reachable from the tree and on no page at all.
// Measured before this was carried: two OBJRs naming objects 15 and 16, both resolving, both on
// page 0, while the sheet held two fresh copies the tree did not describe. So the carry repoints
// the OBJR at the copy, and the orphan then has no reference left and is not written.
type capturedNote struct {
	objNr int
	dict  types.Dict
}

// noteSource is one source page's annotations, captured BEFORE the n-up runs — the composed
// document no longer holds them.
type noteSource struct {
	content []byte         // the decoded page content stream, the key a placement is verified against
	notes   []capturedNote // `/Text` annotations, deep-copied out of the source context
	left    int            // annotations of another subtype on this page
}

// captureNoteAnnots records what an n-up is about to discard, per 1-based source page.
//
// **The dictionaries are deep-copied here rather than referenced.** They live in the SOURCE
// context, whose object numbers mean nothing in the composed one, so an indirect reference inside
// an annotation would name whatever object happened to take that number. `migrateObject` resolves
// them against the source and rebuilds them; an annotation it cannot rebuild is counted as left
// behind rather than carried half-resolved.
func captureNoteAnnots(pdf []byte) (map[int]*noteSource, int, bool) {
	// **`ReadAndValidate`, because that is the read `api.NUp` itself performs** (`api/nup.go:116`),
	// and this function's whole job is to hold the bytes that read produced up against the forms it
	// built out of them. Matching it is correctness before it is cost: the verification below is
	// byte equality on a decoded content stream, so a read path that normalised the stream
	// differently would abandon every carry.
	//
	// It is also the cheapest read that can be used at all. Measured on a 40-page document,
	// 30 calls each, interleaved in three rounds: `ReadContext` 1.6 ms, `ReadAndValidate` 9.6 ms,
	// `ReadValidateAndOptimize` 15.7 ms. **`ReadContext` is not an option** — pdfcpu fills
	// `ctx.PageCount` during validation, so a plain read reports zero pages and this walk visits
	// none; it was tried, and the carry abandoned silently until the suffix check turned it red.
	ctx, err := api.ReadAndValidate(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, 0, false
	}
	out := map[int]*noteSource{}
	total := 0
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, perr := ctx.PageDict(p, false)
		if perr != nil || d == nil {
			return nil, 0, false
		}
		annots, aerr := ctx.DereferenceArray(d["Annots"])
		if aerr != nil {
			return nil, 0, false
		}
		src := &noteSource{}
		for _, a := range annots {
			ad, derr := ctx.DereferenceDict(a)
			if derr != nil || ad == nil {
				src.left++
				continue
			}
			if sub := ad.NameEntry("Subtype"); sub == nil || *sub != "Text" {
				src.left++
				continue
			}
			copied, ok := copyAnnot(ctx, ad)
			if !ok {
				src.left++
				continue
			}
			nr := 0
			if ir, isRef := a.(types.IndirectRef); isRef {
				nr = ir.ObjectNumber.Value()
			}
			src.notes = append(src.notes, capturedNote{objNr: nr, dict: copied})
			total++
		}
		if len(src.notes) == 0 && src.left == 0 {
			continue
		}
		if len(src.notes) > 0 {
			// **Read only where something is being carried, and this is not a saving.** The content
			// is the key `placementMatrix` verifies a placement against, and a page with no note has
			// no placement to verify — so a page whose content will not read (an empty stream, which
			// pdfcpu answers with an error) would otherwise abandon the carry for every OTHER page's
			// notes over a page this door was never going to touch. The page is still recorded, for
			// its residue count.
			b, cerr := ctx.PageContent(d, p)
			if cerr != nil {
				return nil, 0, false
			}
			src.content = b
		}
		out[p] = src
	}
	return out, total, true
}

// maxAnnotCopyObjects bounds a single annotation's object graph. An annotation that reaches more
// than this is refused rather than walked: the copy is a convenience for the notes a user wrote,
// not a general document merger, and an unbounded walk from an annotation can reach the page it
// sits on and through it the whole file.
const maxAnnotCopyObjects = 64

// copyAnnot rebuilds one annotation dictionary as self-contained objects.
//
// `/P` and `/Popup` are dropped rather than copied. `/P` names the source page, which the caller
// overwrites with the sheet; `/Popup` names a SECOND annotation that would have to be carried and
// re-parented alongside, and every viewer nib targets synthesizes a popup from `/Contents` (see
// `AddNotes`). What is lost with it is the popup's own geometry and open state, never the comment.
func copyAnnot(ctx *model.Context, ad types.Dict) (types.Dict, bool) {
	budget := maxAnnotCopyObjects
	out := types.Dict{}
	for k, v := range ad {
		if k == "P" || k == "Popup" {
			continue
		}
		c, ok := copyObject(ctx, v, 0, &budget)
		if !ok {
			return nil, false
		}
		out[k] = c
	}
	return out, true
}

// copyObject resolves an object out of the source context into a value that stands alone.
//
// A stream cannot be flattened into a value, so an annotation that reaches one is refused here and
// counted as left behind — which is the same boundary the header states from the other side: an
// annotation with an appearance stream would not survive the rotation anyway.
func copyObject(ctx *model.Context, o types.Object, depth int, budget *int) (types.Object, bool) {
	if depth > 8 || *budget <= 0 {
		return nil, false
	}
	*budget--
	switch v := o.(type) {
	case types.IndirectRef:
		r, err := ctx.Dereference(v)
		if err != nil || r == nil {
			return nil, false
		}
		return copyObject(ctx, r, depth+1, budget)
	case types.StreamDict, *types.StreamDict:
		return nil, false
	case types.Dict:
		d := types.Dict{}
		for k, e := range v {
			c, ok := copyObject(ctx, e, depth+1, budget)
			if !ok {
				return nil, false
			}
			d[k] = c
		}
		return d, true
	case types.Array:
		a := make(types.Array, 0, len(v))
		for _, e := range v {
			c, ok := copyObject(ctx, e, depth+1, budget)
			if !ok {
				return nil, false
			}
			a = append(a, c)
		}
		return a, true
	case nil:
		return nil, false
	}
	return o, true
}

// sheetPlacement is one `… cm /Fm Do` on a sheet: where it was drawn and under what matrix.
type sheetPlacement struct {
	sheet int    // 1-based page number of the sheet
	name  string // the XObject resource name, as the content stream spelled it
	cm    matrix
}

// readPlacements returns every form-XObject draw in the composed document, in the order the
// content streams perform them.
//
// **That order IS source page order, and it is arithmetic rather than a hope.** `nupPages`
// (`pdfcpu/pkg/pdfcpu/nup.go`) walks the selected pages ascending, appends one tile per page to the
// sheet's buffer and wraps a sheet every `len(rr)` tiles; a padding tile at the end emits nothing at
// all, so no source page is ever skipped and none is dropped. The caller still verifies each
// placement against its page's content bytes, so a pdfcpu whose emission order changed abandons the
// carry instead of moving a note to the wrong page.
func readPlacements(ctx *model.Context) ([]sheetPlacement, error) {
	var out []sheetPlacement
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil || d == nil {
			return nil, fmt.Errorf("pdfops: sheet %d does not resolve: %w", p, err)
		}
		b, cerr := ctx.PageContent(d, p)
		if cerr != nil {
			// A sheet with no content stream draws nothing, which is the all-padding case.
			continue
		}
		out = append(out, drawsIn(b, p)...)
	}
	return out, nil
}

// drawsIn scans one content stream for `a b c d e f cm /Name Do`.
//
// It reads the six operands that immediately precede the `cm` a `Do` follows, and it requires them
// to be adjacent: anything between the `cm` and the `Do` means this is not the shape pdfcpu emits
// and the draw is not reported, which abandons the carry upstream.
func drawsIn(src []byte, sheet int) []sheetPlacement {
	toks := contentstream.Tokenize(src)
	// operands holds the operand tokens seen since the last operator, so `cm`'s arguments are the
	// last six of them.
	var operands []contentstream.Token
	var pending *matrix
	var out []sheetPlacement
	for _, tk := range toks {
		switch tk.Kind {
		case contentstream.Whitespace:
			continue
		case contentstream.Operand:
			operands = append(operands, tk)
			continue
		case contentstream.Operator:
		default:
			operands = nil
			pending = nil
			continue
		}
		op := string(tk.Bytes(src))
		switch op {
		case "cm":
			if len(operands) >= 6 {
				var m matrix
				ok := true
				for i, t := range operands[len(operands)-6:] {
					f, err := strconv.ParseFloat(string(t.Bytes(src)), 64)
					if err != nil {
						ok = false
						break
					}
					m[i] = f
				}
				if ok {
					pending = &m
				} else {
					pending = nil
				}
			} else {
				pending = nil
			}
		case "Do":
			if pending != nil && len(operands) >= 1 {
				n := string(operands[len(operands)-1].Bytes(src))
				if len(n) > 1 && n[0] == '/' {
					out = append(out, sheetPlacement{sheet: sheet, name: n[1:], cm: *pending})
				}
			}
			pending = nil
		default:
			// `q` and `Q` neither establish nor destroy the pending matrix for this purpose:
			// pdfcpu writes `q <cm> /Fm Do Q`, so the `q` precedes the `cm`. Any OTHER operator
			// between the two is a stream this code does not recognise, and it clears the pending
			// matrix so the draw is not reported.
			if op != "q" {
				pending = nil
			}
		}
		operands = nil
	}
	return out
}

// errNoteCarry is the abandonment signal. It never reaches a caller of `NUp`: `carryNoteAnnots`
// returns the composed document unchanged on any error.
var errNoteCarry = errors.New("pdfops: the n-up's annotations cannot be carried")

// carryNoteAnnots puts each source page's sticky notes back onto the sheet its content landed on,
// with the rect transformed by the matrix that placed the content.
//
// It returns the composed document unchanged, and a zero report, whenever anything disagrees.
func carryNoteAnnots(src, composed []byte) ([]byte, noteCarry) {
	sources, total, ok := captureNoteAnnots(src)
	if !ok || len(sources) == 0 {
		return composed, noteCarry{}
	}
	left := 0
	for _, s := range sources {
		left += s.left
	}
	if total == 0 {
		// Nothing to carry: the document's annotations are all of kinds this door leaves behind.
		return composed, noteCarry{left: left}
	}

	placed := 0
	out, err := writeMutated(composed, func(ctx *model.Context) error {
		places, perr := readPlacements(ctx)
		if perr != nil {
			return perr
		}
		moved := map[int]types.IndirectRef{} // source annotation object number → the copy on its sheet
		// One placement per source page, in source page order — `readPlacements`' header says why
		// that is arithmetic and not a hope, and `placementMatrix` verifies it per page anyway.
		//
		// **Only a page with something to carry is verified**, because a verification failure
		// abandons the WHOLE carry: a page holding nothing but a widget has no note to misplace, so
		// letting it fail the identity check would lose another page's notes over a page whose
		// annotations this door was never going to touch.
		for i, pl := range places {
			srcPage := i + 1
			s, has := sources[srcPage]
			if !has || len(s.notes) == 0 {
				continue
			}
			m, merr := placementMatrix(ctx, pl, s.content)
			if merr != nil {
				return merr
			}
			if aerr := attachNotes(ctx, pl.sheet, s.notes, m, moved, &placed); aerr != nil {
				return aerr
			}
		}
		if placed != total {
			return fmt.Errorf("%w: %d of %d notes found a sheet", errNoteCarry, placed, total)
		}
		return repointOBJRs(ctx, moved)
	})
	if err != nil {
		return composed, noteCarry{left: left + total}
	}
	return out, noteCarry{carried: placed, left: left}
}

// placementMatrix returns the transform from one source page's own coordinate space to the sheet,
// having first verified that this placement really is that page's content.
func placementMatrix(ctx *model.Context, pl sheetPlacement, pageContent []byte) (matrix, error) {
	d, _, _, err := ctx.PageDict(pl.sheet, false)
	if err != nil || d == nil {
		return identityMatrix, fmt.Errorf("%w: sheet %d does not resolve", errNoteCarry, pl.sheet)
	}
	res, rerr := ctx.DereferenceDict(d["Resources"])
	if rerr != nil || res == nil {
		return identityMatrix, fmt.Errorf("%w: sheet %d has no resources", errNoteCarry, pl.sheet)
	}
	xod, xerr := ctx.DereferenceDict(res["XObject"])
	if xerr != nil || xod == nil {
		return identityMatrix, fmt.Errorf("%w: sheet %d has no XObject resources", errNoteCarry, pl.sheet)
	}
	ir, isRef := xod[pl.name].(types.IndirectRef)
	if !isRef {
		return identityMatrix, fmt.Errorf("%w: /%s on sheet %d is not an indirect reference", errNoteCarry, pl.name, pl.sheet)
	}
	sd, _, serr := ctx.DereferenceStreamDict(ir)
	if serr != nil || sd == nil {
		return identityMatrix, fmt.Errorf("%w: /%s on sheet %d does not resolve to a stream", errNoteCarry, pl.name, pl.sheet)
	}
	if derr := sd.Decode(); derr != nil {
		return identityMatrix, fmt.Errorf("%w: /%s on sheet %d does not decode", errNoteCarry, pl.name, pl.sheet)
	}
	// **The identity check.** The form's content is the source page's content, possibly behind a
	// page-rotation `cm`. Anything else means this placement is not the page the ordinal says it is.
	if !bytes.HasSuffix(sd.Content, pageContent) {
		return identityMatrix, fmt.Errorf("%w: /%s on sheet %d does not hold its source page's content",
			errNoteCarry, pl.name, pl.sheet)
	}
	rot := identityMatrix
	if prefix := sd.Content[:len(sd.Content)-len(pageContent)]; len(bytes.TrimSpace(prefix)) > 0 {
		m, ok := singleCM(prefix)
		if !ok {
			return identityMatrix, fmt.Errorf("%w: /%s on sheet %d carries a prefix that is not one cm",
				errNoteCarry, pl.name, pl.sheet)
		}
		rot = m
	}
	form := identityMatrix
	if fm, ok := matrixFrom(ctx, sd.Dict["Matrix"]); ok {
		form = fm
	}
	return rot.mul(form).mul(pl.cm), nil
}

// singleCM reads a content-stream fragment that must be exactly one `cm` and nothing else.
func singleCM(b []byte) (matrix, bool) {
	var m matrix
	var operands []contentstream.Token
	seen := false
	for _, tk := range contentstream.Tokenize(b) {
		switch tk.Kind {
		case contentstream.Whitespace:
			continue
		case contentstream.Operand:
			operands = append(operands, tk)
		case contentstream.Operator:
			if seen || string(tk.Bytes(b)) != "cm" || len(operands) != 6 {
				return m, false
			}
			for i, t := range operands {
				f, err := strconv.ParseFloat(string(t.Bytes(b)), 64)
				if err != nil {
					return m, false
				}
				m[i] = f
			}
			seen = true
			operands = nil
		default:
			return m, false
		}
	}
	return m, seen && len(operands) == 0
}

// matrixFrom reads a six-number array.
func matrixFrom(ctx *model.Context, o types.Object) (matrix, bool) {
	var m matrix
	arr, err := ctx.DereferenceArray(o)
	if err != nil || len(arr) != 6 {
		return m, false
	}
	for i, e := range arr {
		f, ok := pdfNumber(ctx.XRefTable, e)
		if !ok {
			return m, false
		}
		m[i] = f
	}
	return m, true
}

// attachNotes writes one source page's notes onto its sheet, recording where each one went.
func attachNotes(ctx *model.Context, sheet int, notes []capturedNote, m matrix, moved map[int]types.IndirectRef, placed *int) error {
	if len(notes) == 0 {
		return nil
	}
	d, _, _, err := ctx.PageDict(sheet, false)
	if err != nil || d == nil {
		return fmt.Errorf("%w: sheet %d does not resolve", errNoteCarry, sheet)
	}
	pageRef, perr := ctx.PageDictIndRef(sheet)
	if perr != nil || pageRef == nil {
		return fmt.Errorf("%w: sheet %d has no indirect reference", errNoteCarry, sheet)
	}
	annots, aerr := ctx.DereferenceArray(d["Annots"])
	if aerr != nil {
		return fmt.Errorf("%w: sheet %d has an unreadable /Annots", errNoteCarry, sheet)
	}
	for _, n := range notes {
		llx, lly, urx, ury, ok := rectOf(ctx, n.dict["Rect"])
		if !ok {
			return fmt.Errorf("%w: a note on sheet %d has no readable /Rect", errNoteCarry, sheet)
		}
		a, b, c, e := m.rect(llx, lly, urx, ury)
		// A fresh dictionary per placement, because a repeated source page draws the same note
		// twice and one dictionary cannot hold two rects. Nothing repeats a page today — `api.NUp`
		// places each selected page once — but sharing the map would make that a silent misplacement
		// rather than a second note.
		nd, isDict := n.dict.Clone().(types.Dict)
		if !isDict {
			return fmt.Errorf("%w: a note on sheet %d does not clone to a dictionary", errNoteCarry, sheet)
		}
		nd["Rect"] = types.NewNumberArray(a, b, c, e)
		nd["P"] = *pageRef
		ref, ierr := ctx.XRefTable.InsertObject(nd)
		if ierr != nil {
			return fmt.Errorf("%w: %v", errNoteCarry, ierr)
		}
		annots = append(annots, *types.NewIndirectRef(ref, 0))
		if n.objNr != 0 {
			moved[n.objNr] = *types.NewIndirectRef(ref, 0)
		}
		*placed++
	}
	d["Annots"] = annots
	return nil
}

// repointOBJRs sends every `OBJR` that named a source annotation at the copy now on a sheet.
//
// **Without it the carry makes the tree describe the wrong object and keeps the orphan alive.** The
// OBJR is reachable from `/StructTreeRoot`, and pdfcpu writes by reachability, so the source
// annotation ships — off every page, claiming the same `/ParentTree` key as the copy, and invisible
// to `parentTreeOwners`, which walks the annotations of PAGES. Measured on `collidingMCIDFixture`
// with two notes: `OBJR -> obj 15 onPage=0` and `OBJR -> obj 16 onPage=0`.
//
// It walks `/K` the way `carryTagsThroughNUp` does and keys the visited set on the OBJECT NUMBER,
// for the reason recorded there: two byte-identical elements are one entry in a content-keyed set
// and only one of them gets repaired.
func repointOBJRs(ctx *model.Context, moved map[int]types.IndirectRef) error {
	if len(moved) == 0 {
		return nil
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		return fmt.Errorf("%w: %v", errNoteCarry, err)
	}
	root, rerr := ctx.DereferenceDict(cat["StructTreeRoot"])
	if rerr != nil || root == nil {
		return nil // an untagged composition has no OBJR to repoint
	}
	seen := map[int]bool{}
	var walk func(o types.Object, depth int)
	walk = func(o types.Object, depth int) {
		if depth > 64 {
			return
		}
		if arr, e := ctx.DereferenceArray(o); e == nil && arr != nil {
			for _, x := range arr {
				walk(x, depth+1)
			}
			return
		}
		if ir, isRef := o.(types.IndirectRef); isRef {
			nr := ir.ObjectNumber.Value()
			if seen[nr] {
				return
			}
			seen[nr] = true
		}
		d, e := ctx.DereferenceDict(o)
		if e != nil || d == nil {
			return
		}
		if t := d.NameEntry("Type"); t != nil && *t == "OBJR" {
			if ir, isRef := d["Obj"].(types.IndirectRef); isRef {
				if to, ok := moved[ir.ObjectNumber.Value()]; ok {
					d["Obj"] = to
				}
			}
		}
		if k, ok := d["K"]; ok {
			walk(k, depth+1)
		}
	}
	walk(root["K"], 0)
	return nil
}

// rectOf reads a `/Rect`, normalised so the lower-left corner really is the lower left.
func rectOf(ctx *model.Context, o types.Object) (llx, lly, urx, ury float64, ok bool) {
	arr, err := ctx.DereferenceArray(o)
	if err != nil || len(arr) != 4 {
		return 0, 0, 0, 0, false
	}
	v := make([]float64, 4)
	for i, e := range arr {
		f, good := pdfNumber(ctx.XRefTable, e)
		if !good {
			return 0, 0, 0, 0, false
		}
		v[i] = f
	}
	llx, urx = v[0], v[2]
	if llx > urx {
		llx, urx = urx, llx
	}
	lly, ury = v[1], v[3]
	if lly > ury {
		lly, ury = ury, lly
	}
	return llx, lly, urx, ury, true
}
