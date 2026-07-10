// SPDX-License-Identifier: AGPL-3.0-or-later

package mdpdf

// This file provides generic PDF-assembly primitives for building a single
// paginated "packet" out of a cover PDF plus heterogeneous exhibit files
// (PDFs and raster images). Like the rest of mdpdf it depends only on pdfcpu
// and stays importable standalone.

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// AssemblePacket concatenates a cover PDF with exhibit files into one
// paginated PDF (the packet). Each exhibit is auto-detected: a PDF is merged
// as-is (after a validating round-trip); a raster image (JPEG, PNG, or TIFF)
// is rendered as an A4 page via ImageToPDF. An exhibit that can't be read or
// converted is SKIPPED — its index is returned in skipped — so one corrupt
// file never fails the whole packet. Order is preserved: cover first, then
// the surviving exhibits in input order. Fully in-memory; nothing touches
// disk. cover must be a valid PDF; a bad cover or a write failure returns
// err (exhibits never cause err, only skips).
func AssemblePacket(cover []byte, exhibits [][]byte) (packet []byte, skipped []int, err error) {
	defer catchPanic(&err)

	if _, err := api.ReadAndValidate(bytes.NewReader(cover), model.NewDefaultConfiguration()); err != nil {
		return nil, nil, fmt.Errorf("mdpdf: invalid cover: %w", err)
	}

	// Normalize every exhibit in isolation first: api.MergeRaw aborts the
	// entire merge on its first unreadable input, so a corrupt exhibit must
	// never reach it.
	type survivor struct {
		idx int
		pdf []byte
	}
	var survivors []survivor
	for i, ex := range exhibits {
		if pdf, ok := exhibitToPDF(ex); ok {
			survivors = append(survivors, survivor{i, pdf})
		} else {
			skipped = append(skipped, i)
		}
	}

	parts := make([][]byte, 0, len(survivors)+1)
	parts = append(parts, cover)
	for _, s := range survivors {
		parts = append(parts, s.pdf)
	}
	if packet, err = mergePDFs(parts); err == nil {
		return packet, skipped, nil
	}

	// Rare: every part validated alone but the combined merge failed (e.g. a
	// PDF 2.0 exhibit appended to an older cover). Retry pairwise so the
	// offending exhibit is skipped instead of sinking the packet.
	packet, err = mergePDFs([][]byte{cover})
	if err != nil {
		return nil, nil, fmt.Errorf("mdpdf: write packet: %w", err)
	}
	for _, s := range survivors {
		next, err := mergePDFs([][]byte{packet, s.pdf})
		if err != nil {
			skipped = append(skipped, s.idx)
			continue
		}
		packet = next
	}
	slices.Sort(skipped)
	return packet, skipped, nil
}

// ImageToPDF renders a single raster image (JPEG, PNG, or TIFF) as one A4
// page, centered and scaled to fit, returning a one-page PDF. It errors on
// an unsupported or corrupt image. Known limitation: a multi-page TIFF
// (common for faxed documents) imports only its first page.
func ImageToPDF(img []byte) (pdf []byte, err error) {
	defer catchPanic(&err)

	imp, err := api.Import("formsize:A4, position:c, scalefactor:1.0 rel", types.POINTS)
	if err != nil {
		return nil, fmt.Errorf("mdpdf: import config: %w", err)
	}
	var out bytes.Buffer
	// rs == nil starts a fresh single-image document.
	if err := api.ImportImages(nil, &out, []io.Reader{bytes.NewReader(img)}, imp, model.NewDefaultConfiguration()); err != nil {
		return nil, fmt.Errorf("mdpdf: import image: %w", err)
	}
	return out.Bytes(), nil
}

// mergePDFs concatenates already-validated PDFs, first as the base.
func mergePDFs(parts [][]byte) ([]byte, error) {
	rsc := make([]io.ReadSeeker, len(parts))
	for i, p := range parts {
		rsc[i] = bytes.NewReader(p)
	}
	var out bytes.Buffer
	if err := api.MergeRaw(rsc, &out, false, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// exhibitToPDF converts one exhibit into a standalone validated PDF, or
// reports that it can't be (→ skip). A panic anywhere below is a skip, not
// an abort: no exhibit may sink the packet.
func exhibitToPDF(b []byte) (pdf []byte, ok bool) {
	defer func() {
		if recover() != nil {
			pdf, ok = nil, false
		}
	}()

	switch http.DetectContentType(b) {
	case "application/pdf":
		pdf, err := normalizePDF(b)
		return pdf, err == nil
	case "image/jpeg", "image/png", "image/tiff":
		pdf, err := ImageToPDF(b)
		return pdf, err == nil
	default:
		// Ambiguous sniff (e.g. application/octet-stream, or a PDF with
		// junk before its header): try both readers before giving up.
		if pdf, err := normalizePDF(b); err == nil {
			return pdf, true
		}
		if pdf, err := ImageToPDF(b); err == nil {
			return pdf, true
		}
		return nil, false
	}
}

// normalizePDF round-trips a PDF through pdfcpu (read, validate, optimize,
// rewrite), proving in isolation that it can survive the merge.
func normalizePDF(pdf []byte) ([]byte, error) {
	conf := model.NewDefaultConfiguration()
	ctx, err := api.ReadAndValidate(bytes.NewReader(pdf), conf)
	if err != nil {
		return nil, err
	}
	if err := api.OptimizeContext(ctx); err != nil {
		return nil, err
	}
	ctx.EnsureVersionForWriting()
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// catchPanic converts a panic into an error. pdfcpu wraps its entry points
// in fault.Catch already; this is a defensive second net at the library
// boundary.
func catchPanic(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("mdpdf: internal panic: %v", r)
	}
}
