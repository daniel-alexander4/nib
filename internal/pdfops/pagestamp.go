package pdfops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// A mark on every page — `/pending 907`.
//
// # One operation, three kinds
//
// A running header or footer, a diagonal text watermark and a logo are the same request: put this on
// each of these pages. `StampPages` is the one door for all three, over doors that already existed —
// `stampWatermarkOn` (the diagonal watermark Finalize bakes), the text stamp `StampPageNumbers` uses,
// and pdfcpu's image watermark `StampImages` uses — so no new PDF machinery was written, and a mark
// made here is the same layer (`HasStampLayer`) and the same optional-content correction as theirs.
//
// # Pages
//
// A selection in pdfcpu's own words (`1-3,5`, `even`), empty for every page. A term that names no page
// of the document is refused with `PageRangeError` (`selectionInDocument`, the package's one door for
// it) rather than stamping nothing under a success.
//
// # An image is sized per page
//
// Its width is a fraction of EACH page's width, so a document of mixed page sizes carries the logo at one
// proportion. Absolute scale is computed here, per page, because pdfcpu's own relative scale is read
// from the library and not measured: the number written is the number meant.

// The kinds of mark StampPages draws.
const (
	StampKindText      = "text"      // a line of text at a position — a footer, a header, a corner
	StampKindWatermark = "watermark" // the large diagonal text watermark
	StampKindImage     = "image"     // a logo or picture at a position
)

// PageStamp describes one mark for StampPages.
type PageStamp struct {
	Kind     string
	Text     string  // text and watermark
	Image    []byte  // image: PNG or JPEG
	Position string  // text and image: tl tc tr cl c cr bl bc br ("" = bc for text, br for an image)
	Size     int     // text: point size (6-72, default 11)
	Color    string  // text and watermark: #RRGGBB ("" = black for text)
	Opacity  float64 // all three: 0.02-1 (0 = 1 for text and image, the watermark's own default)
	Scale    float64 // image: width as a fraction of the page's width, 0.02-1 (default 0.2); watermark: its WatermarkStyle.Scale
	Angle    int     // watermark: degrees, -90..90
	Margin   float64 // text and image: points in from the edge (default 36)
	Pages    []string
}

// pageStampPositions maps Nib's position codes to pdfcpu's anchors; an unlisted code never reaches the
// description string (same guard rationale as pageNumPositions).
var pageStampPositions = map[string]string{
	"tl": "tl", "tc": "tc", "tr": "tr",
	"cl": "l", "c": "c", "cr": "r",
	"bl": "bl", "bc": "bc", "br": "br",
}

// pageStampOffset insets a mark from the edges its position names; the centre gets no offset.
func pageStampOffset(pos string, margin float64) (x, y float64) {
	switch pos[1] {
	case 'l':
		x = margin
	case 'r':
		x = -margin
	}
	switch pos[0] {
	case 't':
		y = -margin
	case 'b':
		y = margin
	}
	return x, y
}

// stampedPages resolves a selection to the pages it names, ascending; nil is every page. A term that names no
// page of the document is refused (`selectionInDocument`).
func stampedPages(sel []string, n int) ([]int, error) {
	var out []int
	if len(sel) == 0 {
		for p := 1; p <= n; p++ {
			out = append(out, p)
		}
		return out, nil
	}
	if err := selectionInDocument(sel, n); err != nil {
		return nil, err
	}
	set, err := api.PagesForPageSelection(n, sel, false, false)
	if err != nil {
		return nil, selectionError(err)
	}
	for p, on := range set {
		if on && p >= 1 && p <= n {
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out, nil
}

// ErrStampNeedsContent is the refusal of a mark with nothing to draw.
var ErrStampNeedsContent = errors.New("pdfops: there is nothing to stamp — give the text, or a picture")

// StampPages bakes one mark onto each page a selection names. The result is a rewrite, so it drops a PDF/UA
// claim like every other stamp (ADR-032); a signed document is the caller's to refuse.
func StampPages(pdf []byte, st PageStamp) ([]byte, error) {
	n, err := PageCount(pdf)
	if err != nil {
		return nil, err
	}
	pages, err := stampedPages(st.Pages, n)
	if err != nil {
		return nil, err
	}
	switch st.Kind {
	case StampKindWatermark:
		if st.Text == "" {
			return nil, ErrStampNeedsContent
		}
		return stampWatermarkOn(pdf, st.Text, WatermarkStyle{
			Scale: st.Scale, Opacity: st.Opacity, Angle: st.Angle, Color: st.Color,
		}, st.Pages)
	case StampKindText:
		return stampLine(pdf, st, pages)
	case StampKindImage:
		return stampPicture(pdf, st, pages)
	}
	return nil, fmt.Errorf("pdfops: unknown stamp kind %q", st.Kind)
}

func stampLine(pdf []byte, st PageStamp, pages []int) ([]byte, error) {
	text, err := stampText(st.Text)
	if err != nil {
		return nil, err
	}
	if text == "" {
		return nil, ErrStampNeedsContent
	}
	pos := st.Position
	if pageStampPositions[pos] == "" {
		pos = "bc"
	}
	size := st.Size
	if size < 6 {
		size = 11
	}
	if size > 72 {
		size = 72
	}
	color := "#000000"
	if hexColor.MatchString(st.Color) {
		color = st.Color
	}
	opacity := 1.0
	if st.Opacity > 0 {
		opacity = clampUnit(st.Opacity, 0.02, 1)
	}
	margin := st.Margin
	if margin <= 0 {
		margin = 36
	}
	ox, oy := pageStampOffset(pos, margin)
	face, _, embedded := AuthoredTextFaces()
	return stampTextWatermarks(pdf, embedded, []string{face}, func(ctx *model.Context, embedded bool) error {
		name := "Helvetica"
		if embedded {
			name = face
		}
		desc := fmt.Sprintf("fontname:%s, points:%d, scalefactor:1 abs, position:%s, offset:%.1f %.1f, fillcolor:%s, opacity:%.3f, rotation:0",
			name, size, pageStampPositions[pos], ox, oy, color, opacity)
		wms := map[int][]*model.Watermark{}
		for _, p := range pages {
			wm, err := api.TextWatermark(context.Background(), text, desc, true, false, types.POINTS, nil)
			if err != nil {
				return err
			}
			wms[p] = append(wms[p], wm)
		}
		return contentHeldUnfiltered(ctx, wms, func() error { return pdfcpu.AddWatermarksSliceMap(context.Background(), ctx, wms) })
	})
}

func stampPicture(pdf []byte, st PageStamp, pages []int) ([]byte, error) {
	if len(st.Image) == 0 {
		return nil, ErrStampNeedsContent
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(st.Image))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("pdfops: this picture could not be read: %v", err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil, fmt.Errorf("pdfops: this picture is %d×%d, and nib stamps pictures up to %d megapixels", cfg.Width, cfg.Height, maxImagePixels/1_000_000)
	}
	pos := st.Position
	if pageStampPositions[pos] == "" {
		pos = "br"
	}
	frac := st.Scale
	if frac <= 0 {
		frac = 0.2
	}
	frac = clampUnit(frac, 0.02, 1)
	opacity := 1.0
	if st.Opacity > 0 {
		opacity = clampUnit(st.Opacity, 0.02, 1)
	}
	margin := st.Margin
	if margin <= 0 {
		margin = 36
	}
	ox, oy := pageStampOffset(pos, margin)
	dims, err := pdfread.PageDims(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	wms := map[int][]*model.Watermark{}
	for _, p := range pages {
		if p > len(dims) {
			return nil, &PageRangeError{Page: p, Pages: len(dims)}
		}
		scale := frac * dims[p-1].Width / float64(cfg.Width)
		desc := fmt.Sprintf("position:%s, offset:%.2f %.2f, scalefactor:%.5f abs, opacity:%.3f, rotation:0",
			pageStampPositions[pos], ox, oy, scale, opacity)
		wm, err := api.ImageWatermarkForReader(context.Background(), bytes.NewReader(st.Image), desc, true, false, types.POINTS)
		if err != nil {
			return nil, err
		}
		wms[p] = append(wms[p], wm)
	}
	out, err := addWatermarks(pdf, wms)
	if err != nil {
		return nil, err
	}
	return honestOptionalContent(out), nil
}
