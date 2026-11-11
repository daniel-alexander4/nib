package pdfops

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"

	"nib/internal/pdfread"
)

// The objects a page draws, with an address an edit can name (PLAN-image-edit P01.S01, D1 and D2).
//
// `ReadPageObjects` is the ONE reader that lists `pageRuns.images`: `ReadUntagged` derives its image rows from it
// (`untaggedPieces`), and `TestEveryImageListingRoutesThroughReadPageObjects` fails a second listing. One function,
// `objectEditability`, owns `Editable` and every `Why`, so the reasons a listing gives are the causes an edit will
// refuse with.
//
// **What is not listed**: a picture in an annotation's appearance, a pattern, a Type 3 glyph or a soft-mask group is
// never walked (the walker reads the page's content and the forms it draws, nothing else), so it is in no list —
// declared, not "not editable". A form the walk met and could not read is counted (`FormsNotRead`), so the answer
// never claims every picture.

// maxPageImages bounds how many images one page's walk keeps (`drawnImage`), for every reader that keeps them — the
// page map, the export (and its visibility filter), the table and Figure proposer, the OCR pages' text read
// (`textOn`), the region edit, the untagged listing and this reader. Past it the read refuses with the count and the cap
// (`ErrTooManyImages`), never a list cut short.
//
// Measured 2026-11-09 (`TestTenThousandDistinctImagesAreReadWithinTheCeiling`, which logs it): one page drawing
// 10,000 DISTINCT image XObjects reads through `ReadPageObjects` in 1.07-1.16 s with 214 MB allocated, nearly all of it
// pdfcpu's parse of 10,000 objects, which this cap does not bound; the walk alone takes 0.14-0.20 s and allocates
// 42 MB (~4.2 KB an image, mostly the image-dictionary lookups), and keeps 184 bytes an image (`drawnImage`). The cap
// is ten times that page: at 100,000 the walk is ~2 s and ~420 MB allocated, ~18 MB kept. The corpora were not
// counted for their largest page (the instrument does not report it); a real page near the cap would be refused by
// the map and the export as well, by name.
const maxPageImages = 100_000

// ErrTooManyImages is the refusal past `maxPageImages`; its text names the page's count and the cap.
var ErrTooManyImages = errors.New("the page draws more images than nib reads on one page")

func tooManyImages(n int) error {
	return fmt.Errorf("%w: it draws %d, and nib reads at most %d", ErrTooManyImages, n, maxPageImages)
}

// Why an object is not editable — stable strings, the same an edit refuses with.
const (
	// WhyFormDrawn: a form XObject's stream draws it, and editing the form changes every page that draws it (L5).
	WhyFormDrawn = "form-drawn"
	// WhyTagged: a marked-content sequence with an MCID is open around it and Nib cannot take its structure apart
	// safely — two elements own it, its owner is written inline, the tree does not read, or it was not asked
	// (D4; the other tagged causes are `WhyTaggedShared` and `WhyTaggedLast`).
	WhyTagged = "tagged"
	// WhyNotIsolated: no `q` is open around it, so nothing brackets the state it is drawn in (D3's hostile pin).
	WhyNotIsolated = "not-isolated"
	// WhyUnreadableBox: its box is not a number (a CTM that is not finite), so it cannot be shown or hit.
	WhyUnreadableBox = "unreadable-box"
	// WhyOffPage: it is drawn wholly outside the displayed page.
	WhyOffPage = "off-page"

	// Why a picture cannot be moved (`objectMovability`, D8), beside the causes above that apply to a move too.
	// WhyNotAlone: its `q` holds more than one `cm` and the picture — another `cm`, something drawn, a clip.
	WhyNotAlone = "not-alone"
	// WhyNotUpright: it is turned, mirrored or slanted on the page, so a box does not say where it is.
	WhyNotUpright = "not-upright"
	// WhyPartlyOffPage: part of it is outside the displayed page, so the box Nib shows is not all of it.
	WhyPartlyOffPage = "partly-off-page"
	// WhyClipped: a clip in force hides part of the page, so a move could hide part of the picture.
	WhyClipped = "clipped"

	// Why a drawing cannot be removed or moved, beside the causes above (PLAN-image-edit P03.S03).
	// WhyClips: a path of it is a clip as well as painted (`W` before its paint), so taking it changes what is
	// drawn after it.
	WhyClips = "clips"
	// WhyPatterned: a path of it is painted with a pattern, which is laid in the pattern's own space: moving the path
	// slides it over a pattern that stays where it is.
	WhyPatterned = "patterned"
	// WhyNotAPicture: a replace names a drawing.
	WhyNotAPicture = "not-a-picture"
	// WhyNotPathOnly: a path of it holds an operator a path object may not (a colour, a `cm`, a `q`) between its first
	// point and its paint (`pathOnly`), so taking or moving its bytes would change what is drawn after it.
	WhyNotPathOnly = "not-path-only"
)

// PageObject is one picture or drawing a page draws, as an edit addresses it: (Page, Object, Ordinal, Digest), never
// position alone (L2).
type PageObject struct {
	Page int `json:"page"`
	// Object is ObjectPicture or ObjectDrawing: which of PageObjects' two lists it is in, and what Ordinal counts.
	Object string `json:"object"`
	// Ordinal is the picture's index among the pictures the page's walk met, in draw order, from 0 — never the shared
	// draw-order count (`drawnImage.seq`), which numbers text and shapes too.
	Ordinal int `json:"ordinal"`
	// Kind is, for a picture, "image" (an image XObject's `Do`) or "inline" (`BI … EI`); for a drawing, "drawing",
	// "rule" (one thin straight path) or "box" (one plain rectangle) — `drawingKind`.
	Kind string `json:"kind"`
	// Digest is the address's digest, lowercase hex (`addressDigest`): SHA-256 over the operator's digest
	// (`objectDigest`: its bytes, the `cm` that places it, the `q` depth) and how many pictures on the page share that
	// operator digest. The count is what makes an address go stale when its twin is removed: two identical `Do`s share
	// a digest and differ by ordinal, and once one goes the other takes its ordinal — with a count of one, so a replay
	// of the removed one's address names nothing (code-review v2.56.0 C1).
	Digest string `json:"digest"`
	// Box is where it is, as fractions of the displayed page (the page map's space, ADR-088); zero when OnPage is false.
	Box    MapRect `json:"box"`
	OnPage bool    `json:"onPage"`
	// Editable says an edit may act on it; Why is the cause when it may not ("" when it may).
	Editable bool   `json:"editable"`
	Why      string `json:"why"`
	// Movable says a move may act on it, MoveWhy the cause when it may not (`objectMovability`). A move writes no
	// structure, so a tagged picture's tag causes do not stop it (P02.S01 grill pin 3).
	Movable bool   `json:"movable"`
	MoveWhy string `json:"moveWhy"`
	// Replaceable says a replace may act on it, ReplaceWhy the cause when it may not: `objectEditability` without the
	// tag causes, because a replace keeps the element (PLAN-image-edit P03.S01). The door adds what only the
	// image's own object can say (a stencil, a structure reference) when it is asked.
	Replaceable bool   `json:"replaceable"`
	ReplaceWhy  string `json:"replaceWhy"`
	// PlaceSpan is the byte span of the `cm` that last set the matrix at its `q` level, or null.
	PlaceSpan *[2]int `json:"placeSpan"`
	// QDepth is how many `q` are open around it.
	QDepth int `json:"qDepth"`
	// InForm, Tagged and Decoration: a form draws it, a structure element owns it, an `/Artifact` sequence holds it.
	InForm     bool `json:"inForm"`
	Tagged     bool `json:"tagged"`
	Decoration bool `json:"decoration"`
	// MCID is the MCID in force, or -1; Stream the stream it is in (0 the page's, else a form's object number).
	MCID   int `json:"mcid"`
	Stream int `json:"stream"`
	// Alt is the description (`/Alt`) of the structure element that owns it and would go with it, or "" (P01.S04:
	// the window names a picture by its place and its description, and warns that the description goes too).
	Alt string `json:"alt"`

	// span is the operator in its stream; never on the wire — an edit takes it from its own re-read (D2). objNr is
	// the image XObject's object number, 0 for an inline image. operator is `objectDigest` in hex, without the count:
	// what the after-check compares across a write, since removing a twin moves the survivor's Digest and not its
	// operator.
	span     opSpan
	objNr    int
	operator string
	// userBox is the picture's box in user space, unclamped, plain whether it is drawn straight and painter the
	// digest of its own operator bytes: what a move's after-check compares (`rewriteAgrees`).
	userBox [4]float64
	plain   bool
	painter [32]byte
	// members are a drawing's paths, in the order the page draws them, and memberAt where each is in the walk's list
	// (`pageRuns.paths`); nil for a picture. A drawing has no one span: span is {-1, -1}.
	members  []drawnPath
	memberAt []int
}

// PageObjects is one page's pictures and drawings.
type PageObjects struct {
	Page    int          `json:"page"`
	Objects []PageObject `json:"objects"`
	// Drawings are the page's painted paths as the ONE grouping (`groupDrawnPaths`, ADR-125) makes them drawings, in
	// the order the page draws each one's first path, numbered apart from the pictures (PLAN-image-edit P03.S03).
	Drawings []PageObject `json:"drawings"`
	// DrawingsUngrouped says the page draws more overlapping paths than the grouping compares (`clusterBudget`): a
	// drawing could be listed in parts, so none is listed.
	DrawingsUngrouped bool `json:"drawingsUngrouped"`
	// FormsNotRead counts form XObjects the walk met and could not read; what they draw is not listed.
	FormsNotRead int `json:"formsNotRead"`
}

// ReadPageObjects reads the pictures and drawings page of pdf draws, each with its address and whether an edit may act
// on it. It writes nothing.
func ReadPageObjects(pdf []byte, page int) (PageObjects, error) {
	ctx, err := inspectionRead(pdf)
	if err != nil {
		return PageObjects{}, err
	}
	walked := pdfread.Pages(ctx)
	if err := pageInDocument(page, len(walked)); err != nil {
		return PageObjects{}, err
	}
	pg := pageAt(ctx, walked, page)
	pr, err := readPageDrawings(ctx, pg)
	if err != nil {
		return PageObjects{}, err
	}
	var tagging map[int]objectTagging
	if src, serr := pdfread.PageContent(ctx, pg.Dict, page); pg.Ref != nil && serr == nil {
		tree, treeErr := readTreeForEdit(ctx, livePageObjects(ctx))
		tagging = taggingOf(ctx, tree, treeErr, pg.Ref.ObjectNumber.Value(), pr, src)
	}
	return pageObjectsOf(page, newDisplaySpace(pg), pr, tagging), nil
}

// pageObjectsOf lists pr's images and drawings as page objects — the one place `pageRuns.images` becomes a listing,
// and with `untaggedPieces` one of the two callers of the path grouping (`groupDrawnPaths`). pr must come
// from the region reader (`readPageDrawings`), the only one that sets an image's digest. tagging is what the
// structure tree says of each marked picture (`taggingOf`); nil means it was not asked, and every marked picture is
// then `tagged`.
func pageObjectsOf(nr int, sp displaySpace, pr pageRuns, tagging map[int]objectTagging) PageObjects {
	out := PageObjects{Page: nr, Objects: make([]PageObject, 0, len(pr.images)), FormsNotRead: pr.formsUnread}
	twins := map[[32]byte]int{}
	for _, im := range pr.images {
		twins[im.digest]++
	}
	for i, im := range pr.images {
		o := PageObject{Page: nr, Object: ObjectPicture, Ordinal: i, Kind: "image", Digest: addressDigest(im.digest, twins[im.digest]), QDepth: im.qDepth,
			InForm: im.inForm, Tagged: im.marked, Decoration: im.artifact, MCID: im.mcid, Stream: im.stm, span: im.span, objNr: im.objNr,
			operator: hex.EncodeToString(im.digest[:]), userBox: im.box, plain: im.plain, painter: im.painter}
		if im.objNr == 0 {
			o.Kind = "inline"
		}
		if im.place.start >= 0 {
			o.PlaceSpan = &[2]int{im.place.start, im.place.end}
		}
		o.Box, o.OnPage = pieceRect(sp, im.box)
		tagWhy := WhyTagged
		if t, ok := tagging[i]; ok {
			tagWhy = t.why
			o.Alt = t.alt
		}
		o.Editable, o.Why = objectEditability(im, o.OnPage, tagWhy)
		o.Movable, o.MoveWhy = objectMovability(im, sp)
		o.Replaceable, o.ReplaceWhy = objectEditability(im, o.OnPage, "")
		out.Objects = append(out.Objects, o)
	}
	out.Drawings, out.DrawingsUngrouped = drawingsOf(nr, sp, pr.paths)
	return out
}

// drawingsOf lists a page's painted paths as drawings: the grouping door's groups, each a PageObject numbered in the
// order the page draws its first path. Past the grouping's budget it lists none and says so.
func drawingsOf(nr int, sp displaySpace, painted []drawnPath) ([]PageObject, bool) {
	groups, spent := groupDrawnPaths(painted)
	if spent {
		return []PageObject{}, true
	}
	type drawing struct {
		at      []int // members, as indexes into painted, in draw order
		members []drawnPath
		box     [4]float64
	}
	ds := make([]drawing, 0, len(groups))
	for _, g := range groups {
		d := drawing{at: g.members, box: g.box}
		for _, i := range g.members {
			d.members = append(d.members, painted[i])
		}
		ds = append(ds, d)
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].at[0] < ds[j].at[0] })
	out := make([]PageObject, 0, len(ds))
	twins := map[[32]byte]int{}
	digests := make([][32]byte, len(ds))
	for i, d := range ds {
		digests[i] = drawingDigest(d.members)
		twins[digests[i]]++
	}
	for i, d := range ds {
		first := d.members[0]
		o := PageObject{Page: nr, Object: ObjectDrawing, Ordinal: i, Kind: drawingKind(first, len(d.members)),
			Digest: addressDigest(digests[i], twins[digests[i]]), QDepth: first.qDepth, InForm: first.inForm, Tagged: first.marked,
			Decoration: first.artifact, MCID: first.mcid, Stream: first.stm, operator: hex.EncodeToString(digests[i][:]),
			userBox: d.box, members: d.members, memberAt: d.at, span: opSpan{-1, -1}}
		o.Box, o.OnPage = pieceRect(sp, d.box)
		o.Editable, o.Why = drawingEditability(d.members, d.box, o.OnPage)
		o.Movable, o.MoveWhy = drawingMovability(d.members, d.box, sp)
		o.Replaceable, o.ReplaceWhy = false, WhyNotAPicture
		out = append(out, o)
	}
	return out, false
}

// drawingDigest names a drawing's operators for an address: SHA-256 under its own label over each member path's
// stream, matrix and bytes, in draw order, length-prefixed — so a drawing's digest is never a picture's, and two
// drawings drawn by the same bytes under the same matrix are twins (ADR-220's rule, restated for a drawing:
// `addressDigest` counts them). The matrix stands where a picture's digest has its `cm`.
func drawingDigest(members []drawnPath) [32]byte {
	h := sha256.New()
	h.Write([]byte("nib-drawing-v1\x00"))
	var n [8]byte
	for _, p := range members {
		binary.BigEndian.PutUint64(n[:], uint64(p.stm))
		h.Write(n[:])
		for _, v := range p.ctm {
			binary.BigEndian.PutUint64(n[:], math.Float64bits(v))
			h.Write(n[:])
		}
		binary.BigEndian.PutUint64(n[:], uint64(len(p.bytes)))
		h.Write(n[:])
		h.Write(p.bytes)
	}
	var out [32]byte
	h.Sum(out[:0])
	return out
}

// drawingEditability is the ONE answer to whether a removal may act on a drawing, and why not. A path changes no
// graphics state, so taking one needs no `q` around it (a picture's `not-isolated` does not apply) — unless it clips.
func drawingEditability(members []drawnPath, box [4]float64, onPage bool) (bool, string) {
	first := members[0]
	switch {
	case first.inForm:
		return false, WhyFormDrawn
	case !finiteBox(box):
		return false, WhyUnreadableBox
	case !onPage:
		return false, WhyOffPage
	case first.marked:
		// A D4 analogue for a drawing's element is not decided (plan-review pin): listed, and refused.
		return false, WhyTagged
	}
	for _, p := range members {
		switch {
		case p.clips:
			return false, WhyClips
		case !pathOnly(p):
			return false, WhyNotPathOnly
		}
	}
	return true, ""
}

// drawingMovability is the ONE answer to whether a move may act on a drawing, and why not. A move translates every
// path by its own operands (`translatedPath`), so each path's matrix must be invertible; the drawing must be wholly on
// the page and under a clip that leaves the page open, as a picture must; and a pattern fill or a clip refuses. A move
// writes no structure, so a tagged drawing may move (as a tagged picture may).
func drawingMovability(members []drawnPath, box [4]float64, sp displaySpace) (bool, string) {
	first := members[0]
	raw := sp.rect(box)
	switch {
	case first.inForm:
		return false, WhyFormDrawn
	case !finiteBox(box) || sp.w <= 0 || sp.h <= 0:
		return false, WhyUnreadableBox
	case raw[2] <= 0 || raw[0] >= 1 || raw[3] <= 0 || raw[1] >= 1:
		return false, WhyOffPage
	case raw[0] < -displayEpsilon || raw[1] < -displayEpsilon || raw[2] > 1+displayEpsilon || raw[3] > 1+displayEpsilon:
		return false, WhyPartlyOffPage
	}
	for _, p := range members {
		switch {
		case p.clips:
			return false, WhyClips
		case !pathOnly(p):
			return false, WhyNotPathOnly
		case p.patterned:
			return false, WhyPatterned
		case p.clipRough || !clipLeavesPageOpen(sp.box, p.clipBox):
			return false, WhyClipped
		}
		if _, ok := linearPart(p.ctm).inverse(); !ok {
			return false, WhyUnreadableBox
		}
	}
	return true, ""
}

// linearPart is m without its translation.
func linearPart(m runMatrix) runMatrix { return runMatrix{m[0], m[1], m[2], m[3], 0, 0} }

// DrawingAt is the drawing objs lists at ordinal, only when its digest is digest too (D2, as `ObjectAt`).
func DrawingAt(objs PageObjects, ordinal int, digest string) (PageObject, bool) {
	if ordinal < 0 || ordinal >= len(objs.Drawings) {
		return PageObject{}, false
	}
	if o := objs.Drawings[ordinal]; o.Digest == digest {
		return o, true
	}
	return PageObject{}, false
}

// objectEditability is the ONE answer to whether an edit may act on a picture, and why not. onPage is whether its
// box places on the displayed page (`pieceRect`); tagWhy is the structure tree's verdict on a marked picture ("" when
// it may go with its element, `tagVerdict`). Order is the order of the causes' weight: a form-drawn picture is not
// the page's to edit whatever else is true of it.
func objectEditability(im drawnImage, onPage bool, tagWhy string) (bool, string) {
	switch {
	case im.inForm:
		return false, WhyFormDrawn
	case !finiteBox(im.box):
		return false, WhyUnreadableBox
	case !onPage:
		return false, WhyOffPage
	case im.marked && tagWhy != "":
		return false, tagWhy
	case im.qDepth < 1:
		return false, WhyNotIsolated
	}
	return true, ""
}

// objectMovability is the ONE answer to whether a move may act on a picture, and why not (D8): drawn by the page,
// with a box that is a number and wholly on the displayed page, under a `q` holding exactly one `cm` and the picture
// (`aloneInBracket`), drawn straight (`plain`), under a clip that leaves the whole displayed page open, and with an
// outer matrix that can be undone. So every target inside the page (the door's `moveBoxProblem`) is seen whole.
func objectMovability(im drawnImage, sp displaySpace) (bool, string) {
	raw := sp.rect(im.box)
	switch {
	case im.inForm:
		return false, WhyFormDrawn
	case !finiteBox(im.box) || sp.w <= 0 || sp.h <= 0:
		return false, WhyUnreadableBox
	case raw[2] <= 0 || raw[0] >= 1 || raw[3] <= 0 || raw[1] >= 1:
		return false, WhyOffPage
	case raw[0] < -displayEpsilon || raw[1] < -displayEpsilon || raw[2] > 1+displayEpsilon || raw[3] > 1+displayEpsilon:
		return false, WhyPartlyOffPage
	case im.qDepth < 1:
		return false, WhyNotIsolated
	case !im.alone:
		return false, WhyNotAlone
	case !im.plain:
		return false, WhyNotUpright
	case !im.outerOK:
		return false, WhyUnreadableBox
	case im.clipRough || !clipLeavesPageOpen(sp.box, im.clipBox):
		// Only a clip that leaves the whole displayed page open: under a tighter one nearly every move is cut, so a
		// picture listed movable would refuse almost every box it was dragged to (measured: 20 of 394 corpus moves).
		return false, WhyClipped
	}
	return true, ""
}

// clipLeavesPageOpen says the clip's box holds the whole displayed page, to displayEpsilon of the page's size: a
// producer's page-sized clip reached through a scaled `cm` can land a rounding error inside the page, and refusing
// every picture under it `clipped` would be a refusal of nothing (P02 phase close, W12).
func clipLeavesPageOpen(page, clip [4]float64) bool {
	tol := displayEpsilon * math.Max(page[2]-page[0], page[3]-page[1])
	return clip[0] <= page[0]+tol && clip[1] <= page[1]+tol && clip[2] >= page[2]-tol && clip[3] >= page[3]-tol
}

// displayEpsilon is how far past the page's edge, as a fraction of it, a box may reach and still be wholly on it:
// a producer placing a picture at the page's edge writes a number that rounds either way.
const displayEpsilon = 1e-6

// objectDigest names an operator for an address: SHA-256 over its bytes in src, the bytes of the `cm` that places
// it (place, or none), and the q depth, each length-prefixed so no two different triples hash alike.
func objectDigest(src []byte, span, place opSpan, qDepth int) [32]byte {
	h := sha256.New()
	var n [8]byte
	part := func(s opSpan) {
		var b []byte
		if s.start >= 0 && s.start <= s.end && s.end <= len(src) {
			b = src[s.start:s.end]
		}
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		h.Write(n[:])
		h.Write(b)
	}
	part(span)
	part(place)
	binary.BigEndian.PutUint64(n[:], uint64(qDepth))
	h.Write(n[:])
	var out [32]byte
	h.Sum(out[:0])
	return out
}

// addressDigest is an address's digest: the operator's digest and the number of pictures on its page that share it,
// length-prefixed under its own label so it can never equal an operator digest.
func addressDigest(operator [32]byte, twins int) string {
	h := sha256.New()
	h.Write([]byte("nib-object-address-v1\x00"))
	h.Write(operator[:])
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(twins))
	h.Write(n[:])
	return hex.EncodeToString(h.Sum(nil))
}

// ObjectAt is the object objs lists at ordinal, only when its digest is digest too: an address is both or nothing
// (D2), so a right digest at the wrong ordinal does not name the other of two identical pictures.
func ObjectAt(objs PageObjects, ordinal int, digest string) (PageObject, bool) {
	if ordinal < 0 || ordinal >= len(objs.Objects) {
		return PageObject{}, false
	}
	o := objs.Objects[ordinal]
	if o.Ordinal != ordinal || o.Digest != digest {
		return PageObject{}, false
	}
	return o, true
}

// finiteBox says every edge of box is a number.
func finiteBox(box [4]float64) bool {
	for _, v := range box {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// aloneInBracket says the bytes around an image are exactly `q a b c d e f cm <image> Q`, white space apart: place
// (the last `cm` since its level's `q`, `runGState.place`) follows the `q` straight away and the image follows the
// `cm`, and a `Q` follows the image. So the `cm` is the only one at that level, nothing else is drawn or clipped in
// the bracket, and rewriting it moves this picture and nothing else (D8). Read from the bytes because the walk keeps
// only the LAST `cm` of a level (deepdive 2026-11-09). A comment between the `cm` and the image, or between the image
// and the `Q`, makes it not alone. The `q` test is a byte look, not a token read (a comment ending in `q` passes it);
// that is safe, because rewriting a `cm` changes nothing drawn before it and the outer matrix carries every earlier
// `cm` — the `q` asks only that the bracket is the plain shape D8 names.
func aloneInBracket(src []byte, span, place opSpan) bool {
	if place.start < 0 || place.end > span.start || span.end > len(src) || span.start < 0 {
		return false
	}
	white := func(b []byte) bool {
		for _, c := range b {
			switch c {
			case 0, '\t', '\n', '\f', '\r', ' ':
			default:
				return false
			}
		}
		return true
	}
	if !white(src[place.end:span.start]) {
		return false
	}
	i := place.start
	for i > 0 && white(src[i-1:i]) {
		i--
	}
	if i < 1 || src[i-1] != 'q' || (i >= 2 && !white(src[i-2:i-1]) && !isDelimiter(src[i-2])) {
		return false
	}
	j := span.end
	for j < len(src) && white(src[j:j+1]) {
		j++
	}
	if j >= len(src) || src[j] != 'Q' {
		return false
	}
	return j+1 == len(src) || white(src[j+1:j+2]) || isDelimiter(src[j+1])
}

// isDelimiter is one of PDF's delimiter characters (ISO 32000-1 §7.2.2).
func isDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}
