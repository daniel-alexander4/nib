package pdfops

import (
	"bytes"
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// NormalizePageSizes resizes every page to one common size — the document's modal
// (most common) page size — so a document with mixed page sizes / form factors
// becomes uniform. Each page's content is scaled to fit the target box, preserving
// aspect ratio and centred (letterboxed where the aspect differs); pdfcpu's resize
// keeps text/vector content live (no rasterization) and flattens any /Rotate.
//
// Orientation is respected, not forced: with EnforceOrient off, pdfcpu gives a
// portrait page the target box and a landscape page that box with width/height
// swapped — so a mixed-orientation document ends up uniform *within* each
// orientation rather than rotating landscape content to fit one literal box.
func NormalizePageSizes(pdf []byte) ([]byte, error) {
	conf := model.NewDefaultConfiguration()
	dims, err := api.PageDims(bytes.NewReader(pdf), conf)
	if err != nil {
		return nil, err
	}
	target := modalPageDim(dims)
	res := &model.Resize{PageDim: &target} // EnforceOrient false: respect each page's orientation
	var out bytes.Buffer
	if err := api.Resize(bytes.NewReader(pdf), &out, nil /* all pages */, res, conf); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// modalPageDim picks the standardization target: the most frequent page size in
// the document, compared in a canonical portrait orientation (so a portrait and a
// landscape page of the same paper count as one size) and bucketed by rounded
// points (so sub-point scan jitter doesn't split a size). Ties — including an
// all-distinct document — break toward the larger area; the target is returned in
// portrait form (pdfcpu re-orients it per page). PageDims yields one entry per
// page, so dims is non-empty for any valid PDF.
func modalPageDim(dims []types.Dim) types.Dim {
	type key struct{ w, h float64 } // rounded points, portrait (w <= h)
	count := map[key]int{}
	for _, d := range dims {
		w, h := math.Round(d.Width), math.Round(d.Height)
		if w > h {
			w, h = h, w
		}
		count[key{w, h}]++
	}
	var best key
	var bestN int
	for k, n := range count {
		if n > bestN || (n == bestN && k.w*k.h > best.w*best.h) {
			best, bestN = k, n
		}
	}
	return types.Dim{Width: best.w, Height: best.h}
}
