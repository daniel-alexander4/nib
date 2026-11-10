package pdfops

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"

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
)

// PageObject is one picture a page draws, as an edit addresses it: (Page, Ordinal, Digest), never position alone (L2).
type PageObject struct {
	Page int `json:"page"`
	// Ordinal is the picture's index among the pictures the page's walk met, in draw order, from 0 — never the shared
	// draw-order count (`drawnImage.seq`), which numbers text and shapes too.
	Ordinal int `json:"ordinal"`
	// Kind is "image" (an image XObject's `Do`) or "inline" (`BI … EI`).
	Kind string `json:"kind"`
	// Digest is lowercase hex SHA-256 over the operator's bytes, the `cm` that places it and the `q` depth
	// (`objectDigest`): two identical `Do`s at different depths or under different matrices differ.
	Digest string `json:"digest"`
	// Box is where it is, as fractions of the displayed page (the page map's space, ADR-088); zero when OnPage is false.
	Box    MapRect `json:"box"`
	OnPage bool    `json:"onPage"`
	// Editable says an edit may act on it; Why is the cause when it may not ("" when it may).
	Editable bool   `json:"editable"`
	Why      string `json:"why"`
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
	// the image XObject's object number, 0 for an inline image.
	span  opSpan
	objNr int
}

// PageObjects is one page's pictures.
type PageObjects struct {
	Page    int          `json:"page"`
	Objects []PageObject `json:"objects"`
	// FormsNotRead counts form XObjects the walk met and could not read; what they draw is not listed.
	FormsNotRead int `json:"formsNotRead"`
}

// ReadPageObjects reads the pictures page of pdf draws, each with its address and whether an edit may act on it.
// It writes nothing.
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

// pageObjectsOf lists pr's images as page objects — the one place `pageRuns.images` becomes a listing. pr must come
// from the region reader (`readPageDrawings`), the only one that sets an image's digest. tagging is what the
// structure tree says of each marked picture (`taggingOf`); nil means it was not asked, and every marked picture is
// then `tagged`.
func pageObjectsOf(nr int, sp displaySpace, pr pageRuns, tagging map[int]objectTagging) PageObjects {
	out := PageObjects{Page: nr, Objects: make([]PageObject, 0, len(pr.images)), FormsNotRead: pr.formsUnread}
	for i, im := range pr.images {
		o := PageObject{Page: nr, Ordinal: i, Kind: "image", Digest: hex.EncodeToString(im.digest[:]), QDepth: im.qDepth,
			InForm: im.inForm, Tagged: im.marked, Decoration: im.artifact, MCID: im.mcid, Stream: im.stm, span: im.span, objNr: im.objNr}
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
		out.Objects = append(out.Objects, o)
	}
	return out
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
