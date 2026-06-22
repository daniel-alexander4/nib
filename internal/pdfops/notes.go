package pdfops

import (
	"bytes"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// noteIconSize is the side of the sticky-note icon box in PDF points. pdf.js (and
// most viewers) draw the note icon at a small fixed size, so this only sets where
// the icon sits, not how big it renders.
const noteIconSize = 20.0

// Note is a comment to drop on a page: its text and the page-space point (PDF
// points, bottom-left origin) where the note icon's top-left corner sits.
type Note struct {
	Page int     `json:"page"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Text string  `json:"text"`
}

// AddNotes adds each note as a native /Text (sticky-note) annotation: a clickable
// icon whose popup shows the comment. pdf.js draws the icon from /Name and
// synthesizes the popup from /Contents, so no appearance stream or explicit
// /Popup is needed. Empty input is a no-op.
func AddNotes(pdf []byte, notes []Note) ([]byte, error) {
	if len(notes) == 0 {
		return pdf, nil
	}
	m := map[int][]model.AnnotationRenderer{}
	for _, nt := range notes {
		rect := types.NewRectangle(nt.X, nt.Y-noteIconSize, nt.X+noteIconSize, nt.Y)
		ann := model.NewTextAnnotation(*rect, 0, nt.Text, "", "", 0, nil, "", nil, nil, "", "", 0, 0, 0, false, "Note")
		m[nt.Page] = append(m[nt.Page], &ann)
	}
	var out bytes.Buffer
	if err := api.AddAnnotationsMap(bytes.NewReader(pdf), &out, m, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
