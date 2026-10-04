package pdfops

import (
	"bytes"
	"fmt"

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
// # What it does not repair
//
// Annotations. pdfcpu moves no annotation when it internalizes a rotation, so a widget, link or note on a
// turned page keeps a `/Rect` written for the old user space — at any box corner, the origin included. That
// is a separate defect, filed rather than built here, because an annotation's `/Rect`, its appearance
// matrix and its rotation flag are a larger change than the content stream's one matrix.
func stampInPlace(ctx *model.Context, stamp func() error) error {
	turned := turnedOffsetPages(ctx)
	if err := stamp(); err != nil {
		return err
	}
	return turnAboutTheCorner(ctx, turned)
}

// turnedOffset is a page pdfcpu will turn about the wrong point: its rotation, and the lower-left corner of
// the box pdfcpu reads as the visible one (`stamp.go` `viewPort`: the crop box where there is one).
type turnedOffset struct {
	nr       int
	rot      int
	llx, lly float64
}

func turnedOffsetPages(ctx *model.Context) []turnedOffset {
	var out []turnedOffset
	for _, p := range pdfread.Pages(ctx) {
		if p.Err != nil || p.Attrs == nil || p.Attrs.Rotate%360 == 0 {
			continue
		}
		vp := p.Attrs.MediaBox
		if p.Attrs.CropBox != nil {
			vp = p.Attrs.CropBox
		}
		if vp == nil || (vp.LL.X == 0 && vp.LL.Y == 0) {
			continue
		}
		out = append(out, turnedOffset{nr: p.Nr, rot: p.Attrs.Rotate, llx: vp.LL.X, lly: vp.LL.Y})
	}
	return out
}

// errStampMovedPage is a stamp that moved a turned page's drawing and could not put it back.
var errStampMovedPage = fmt.Errorf("pdfops: stamping would move a turned page's content, and it could not be put back")

func turnAboutTheCorner(ctx *model.Context, turned []turnedOffset) error {
	if len(turned) == 0 {
		return nil
	}
	pages := pdfread.Pages(ctx)
	done := map[int]bool{} // a content stream two pages share is patched once, as pdfcpu patched it once
	for _, t := range turned {
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
