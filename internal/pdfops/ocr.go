package pdfops

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/draw"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ocrFont is the Unicode TrueType font the OCR text layer is written in. It must
// NOT be a core (Base-14) font: pdfcpu's TextWatermark truncates any rune > U+00FF
// to a space on the core-font path, which would silently drop the smart quotes,
// dashes, ellipses and ligatures real OCR emits. Roboto is a Type0/CID font (with
// a /ToUnicode CMap, so the layer extracts correctly) that pdfcpu bundles and
// installs into its user-font dir by default; it covers Latin + common punctuation.
const ocrFont = "Roboto-Regular"

// Word is one OCR'd word: its text and its rectangle in PDF points (bottom-left
// origin), already mapped from the rendered raster by the caller.
type Word struct {
	Page int        `json:"page"`
	Rect [4]float64 `json:"rect"` // x0,y0,x1,y1 — PDF points, bottom-left origin
	Text string     `json:"text"`
}

// StampTextLayer bakes an invisible, selectable text layer onto pdf — one text run
// per OCR'd word, positioned at its rect and drawn in render mode 3 (invisible).
// The page still looks exactly like the scan, but the text is searchable and
// copy-pasteable: this is the standard "searchable PDF" / OCR layer. The text is a
// real (...) Tj run (not glyph outlines), so pdftotext and pdf.js Find recover it.
func StampTextLayer(pdf []byte, words []Word) ([]byte, error) {
	// api.TextWatermark below validates the Roboto user font against pdfcpu's
	// in-memory font registry, which is only populated the first time a default
	// config is built (it installs + loads the bundled Roboto). Force that now,
	// before the per-word TextWatermark calls — otherwise a process whose first
	// font op is OCR rejects Roboto with "unsupported".
	model.NewDefaultConfiguration()
	wms := map[int][]*model.Watermark{}
	for _, w := range words {
		if strings.TrimSpace(w.Text) == "" {
			continue
		}
		// Size the glyphs to the word-box height (in points); a small floor keeps a
		// degenerate box's text legible to extraction.
		pts := int(w.Rect[3] - w.Rect[1])
		if pts < 4 {
			pts = 4
		}
		// Anchor at the box's bottom-left, in points. No fillcolor/mode in the
		// description string (the parser rejects mode 3) — RenderMode is set below.
		desc := fmt.Sprintf("fontname:%s, points:%d, scalefactor:1 abs, position:bl, offset:%.2f %.2f, rotation:0",
			ocrFont, pts, w.Rect[0], w.Rect[1])
		wm, err := api.TextWatermark(w.Text, desc, true, false, types.POINTS)
		if err != nil {
			return nil, err
		}
		wm.RenderMode = draw.RenderMode(3) // invisible: a real Tj run that paints nothing
		page := w.Page
		if page < 1 {
			page = 1
		}
		wms[page] = append(wms[page], wm)
	}
	if len(wms) == 0 {
		return pdf, nil
	}
	var out bytes.Buffer
	if err := api.AddWatermarksSliceMap(bytes.NewReader(pdf), &out, wms, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
