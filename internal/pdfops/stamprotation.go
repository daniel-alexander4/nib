package pdfops

import (
	"bytes"
	"fmt"
	"slices"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// stampInPlace runs a pdfcpu stamp (`pdfcpu.AddWatermarks`, `pdfcpu.AddWatermarksSliceMap`) and puts back what
// that stamp moves on a turned page — /pending 457, found by its geometry census. It is the one door every
// pdfcpu stamp in this package goes through (ADR-009): the bake's fields and images, the watermark, the page
// numbers and the OCR text layer. `TestEveryPdfcpuStampGoesThroughStampInPlace` holds the routing.
//
// # What pdfcpu does to a turned page
//
// To stamp a page with a `/Rotate`, pdfcpu (v0.13.0, `stamp.go` `addPageWatermark`) "internalizes" the
// rotation: it deletes `/Rotate`, sets `/MediaBox` and `/CropBox` to the visible box with its sides swapped,
// and prefixes the page's first content stream with ` q ` and the matrix `model.ContentBytesForPageRotation`
// writes. That matrix turns the page about the ORIGIN and shifts it by the box's width or height — right
// for a box whose lower-left corner is (0, 0), and wrong by exactly that corner for any other. A scan
// cropped anywhere but its top-right, or any page whose producer did not start its box at the origin, had
// its whole drawing moved across the page, most of it out of the box; measured on a turned page cropped
// at (29.75, 84.2), the drawn text left the page. Every flag on it then pointed at what the move left
// behind, which is how the census found it — but the flag was the smallest part of the damage.
//
// # The repair
//
// The rotation is turned about the box's corner instead: `T(ll) · R · T(-ll)`, written as
// `1 0 0 1 llx lly cm  R  1 0 0 1 -llx -lly cm`, so the content is carried to the origin, turned as pdfcpu
// turns it, and carried back to the box pdfcpu has just written. The stamp itself is not touched: pdfcpu
// places it in the box it wrote, which was already right.
//
// It rewrites pdfcpu's matrix only where it finds it byte for byte, and refuses the stamp where a turned,
// offset page does not carry it: a content stream pdfcpu could not decode is left unpatched by pdfcpu
// itself (`patchFirstContentStreamForWatermark` returns nil on an unsupported filter) with its `/Rotate`
// already deleted, so that page would be drawn unturned in a turned box. A refused stamp is a refused
// save; a silently displaced page is a document the user signs without seeing it.
//
// # Annotations
//
// pdfcpu moves no annotation when it internalizes a rotation, so a widget, link or note on a turned page
// kept a `/Rect` written for the old user space — at any box corner, the origin included (/pending 829).
// turnAnnotations carries them with the same matrix, after the content is where it belongs.
func stampInPlace(ctx *model.Context, stamp func() error) error {
	turned := turnedPages(ctx)
	if err := stamp(); err != nil {
		return err
	}
	if err := turnAboutTheCorner(ctx, turned); err != nil {
		return err
	}
	return turnAnnotations(ctx, turned)
}

// turnedPage is a page pdfcpu will turn when it stamps it: its rotation, the lower-left corner of the box
// pdfcpu reads as the visible one (`stamp.go` `viewPort`: the crop box where there is one) — the point the
// turn belongs about, and pdfcpu's own only where it is the origin — and how many annotations it holds now,
// before the stamp can add one.
type turnedPage struct {
	nr       int
	rot      int
	llx, lly float64
	annots   int
}

// offset is a page pdfcpu turns about the wrong point.
func (t turnedPage) offset() bool { return t.llx != 0 || t.lly != 0 }

func turnedPages(ctx *model.Context) []turnedPage {
	var out []turnedPage
	for _, p := range pdfread.Pages(ctx) {
		if p.Err != nil || p.Attrs == nil || p.Attrs.Rotate%360 == 0 {
			continue
		}
		vp := p.Attrs.MediaBox
		if p.Attrs.CropBox != nil {
			vp = p.Attrs.CropBox
		}
		if vp == nil {
			continue
		}
		annots, _ := ctx.DereferenceArray(p.Dict["Annots"])
		out = append(out, turnedPage{nr: p.Nr, rot: p.Attrs.Rotate, llx: vp.LL.X, lly: vp.LL.Y, annots: len(annots)})
	}
	return out
}

// errStampMovedPage is a stamp that moved a turned page's drawing and could not put it back.
var errStampMovedPage = fmt.Errorf("pdfops: stamping would move a turned page's content, and it could not be put back")

func turnAboutTheCorner(ctx *model.Context, turned []turnedPage) error {
	if !slices.ContainsFunc(turned, turnedPage.offset) {
		return nil
	}
	pages := pdfread.Pages(ctx)
	done := map[int]bool{} // a content stream two pages share is patched once, as pdfcpu patched it once
	for _, t := range turned {
		if !t.offset() {
			continue // pdfcpu's turn about the origin is this page's own
		}
		if t.nr > len(pages) || pages[t.nr-1].Err != nil {
			return errStampMovedPage
		}
		p := pages[t.nr-1]
		if p.Attrs.Rotate%360 != 0 {
			continue // this page was not stamped, so pdfcpu turned nothing
		}
		box := p.Attrs.MediaBox
		if box == nil {
			return errStampMovedPage
		}
		ref, sd, err := firstContentStream(ctx, p.Dict)
		if err != nil || sd == nil {
			return errStampMovedPage
		}
		if done[ref.ObjectNumber.Value()] {
			continue
		}
		// decode:exempt — the document's own content stream, and uncapped ON PURPOSE (`/pending 795`): pdfcpu's stamp
		// has just inflated this same stream whole to write the prefix read below (`stamp.go`
		// `patchFirstContentStreamForWatermark`), and it is rewritten whole here. A cap would refuse a stamp pdfcpu
		// had already paid for, and save nothing.
		if err := sd.Decode(); err != nil {
			return errStampMovedPage
		}
		// pdfcpu's first stamp on the page writes ` q R `; each later one sees no `/Rotate` any more and
		// writes only ` q ` in front of it — so R follows a run of ` q `.
		turn := model.ContentBytesForPageRotation(t.rot, box.Width(), box.Height())
		at := -1
		for i := 0; at < 0 && bytes.HasPrefix(sd.Content[i:], []byte(" q ")); i += len(" q ") {
			if bytes.HasPrefix(sd.Content[i+len(" q "):], turn) {
				at = i + len(" q ")
			}
		}
		if at < 0 {
			return errStampMovedPage
		}
		var fixed bytes.Buffer
		fixed.Write(sd.Content[:at])
		fmt.Fprintf(&fixed, "1 0 0 1 %.5f %.5f cm ", t.llx, t.lly)
		fixed.Write(turn)
		fmt.Fprintf(&fixed, "1 0 0 1 %.5f %.5f cm ", -t.llx, -t.lly)
		fixed.Write(sd.Content[at+len(turn):])
		sd.Content = fixed.Bytes()
		if err := sd.Encode(); err != nil {
			return errStampMovedPage
		}
		entry, ok := ctx.FindTableEntryForIndRef(&ref)
		if !ok || entry == nil {
			return errStampMovedPage
		}
		entry.Object = *sd
		done[ref.ObjectNumber.Value()] = true
	}
	return nil
}

// holdContentWhileStamping is off only in the test that compares the hold with pdfcpu's own stamp — the definition
// of what a stamp writes (`TestStampingWithTheContentHeldWritesWhatPdfcpusOwnStampWrites`).
var holdContentWhileStamping = true

// contentHeldUnfiltered runs a pdfcpu stamp of several watermarks a page with the stream pdfcpu patches held
// unfiltered, and puts that stream's own filters back once, afterwards (/pending 881).
//
// pdfcpu stamps a page one watermark at a time (`stamp.go` `AddWatermarksSliceMap` → `addPageWatermark`), and for
// each one rewrites the page's first content stream WHOLE through its filters (`patchFirstContentStreamForWatermark`
// ends in `sd.Encode()`). The stream grows by a hundred bytes or so a watermark, so an OCR layer's words deflated
// the page's content once a word: measured at 3,000 words on one page, 76% of the stamp (15.4 s of 20.3 s), and 52%
// at 600. With no filter to apply, pdfcpu's `Encode` is an assignment; the content pdfcpu builds is the same bytes
// in the same order, and they go through the same `Encode` with the same filters — once.
//
// Only what pdfcpu itself would patch is held, and only where this can read it: the first content stream
// (`firstContentStream`) of a page given two watermarks or more, behind filters that decode. A stream that does not
// decode is left exactly as it is, so pdfcpu meets it and refuses the stamp as it always did, and so is a stream
// with no filter, which was never deflated. The filters put back are the stream's OWN, in their order, with their
// parameters: nothing is added, upgraded or dropped — so a stream behind a filter pdfcpu cannot write (JPX, JBIG2:
// its decode hands those bytes back as they are) is refused here, by the same `Encode` that refused it per watermark.
// It holds no more memory than the stamp did: pdfcpu kept the decoded content on the stream from its first patch on
// (measured, peak heap at 3,000 words on one page: 60 MB either way).
func contentHeldUnfiltered(ctx *model.Context, wms map[int][]*model.Watermark, stamp func() error) error {
	if !holdContentWhileStamping {
		return stamp()
	}
	type hold struct {
		entry   *model.XRefTableEntry
		filters []types.PDFFilter
	}
	var held []hold
	pages := pdfread.Pages(ctx)
	for nr, page := range wms {
		if len(page) < 2 || nr < 1 || nr > len(pages) || pages[nr-1].Err != nil || pages[nr-1].Dict == nil {
			continue
		}
		ref, sd, err := firstContentStream(ctx, pages[nr-1].Dict)
		// A stream two pages share is held once: the second page finds it with no filters, as it finds one that
		// never had any.
		if err != nil || sd == nil || sd.FilterPipeline == nil {
			continue
		}
		entry, ok := ctx.FindTableEntryForIndRef(&ref)
		if !ok || entry == nil {
			continue
		}
		// decode:exempt — the document's own content stream, and no inflation of this function's: pdfcpu's stamp
		// inflates this same stream whole at its first patch, and finds it already done.
		if err := sd.Decode(); err != nil {
			continue
		}
		if sd.Content == nil {
			// pdfcpu reads a nil Content as "not decoded yet", and with no filter would take the raw bytes for it.
			sd.Content = []byte{}
		}
		held = append(held, hold{entry, sd.FilterPipeline})
		sd.FilterPipeline = nil
		entry.Object = *sd
	}
	err := stamp()
	for _, h := range held {
		sd, ok := h.entry.Object.(types.StreamDict)
		if !ok {
			if err == nil {
				err = fmt.Errorf("pdfops: a page's content stream was replaced while it was being stamped")
			}
			continue
		}
		sd.FilterPipeline = h.filters
		if eerr := sd.Encode(); eerr != nil && err == nil {
			err = eerr
		}
		h.entry.Object = sd
	}
	return err
}

// firstContentStream is the stream pdfcpu prefixes: the page's `/Contents` when it is one stream, its first
// element when it is an array (`stamp.go` `updatePageContentsForWM`).
func firstContentStream(ctx *model.Context, page types.Dict) (types.IndirectRef, *types.StreamDict, error) {
	o, _ := page.Find("Contents") //pagecontent:exempt stamp-prefix
	if arr, ok := o.(types.Array); ok {
		if len(arr) == 0 {
			return types.IndirectRef{}, nil, nil
		}
		o = arr[0]
	} else if ir, ok := o.(types.IndirectRef); ok {
		if a, err := ctx.Dereference(ir); err == nil {
			if arr, ok := a.(types.Array); ok && len(arr) > 0 {
				o = arr[0]
			}
		}
	}
	ir, ok := o.(types.IndirectRef)
	if !ok {
		return types.IndirectRef{}, nil, fmt.Errorf("a direct content stream")
	}
	sd, _, err := ctx.DereferenceStreamDict(ir)
	return ir, sd, err
}
