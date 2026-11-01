package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"nib/internal/pdfops"
)

// stampEntry is one stamp of `nib stamp --stamps` (RelaxTax B3): where it goes (page, rect in points from the lower
// left), and either a text line (with optional font, size, colour) or a picture file. It is the window's overlay
// field (pdfops.Field) and image stamp (pdfops.Stamp) addressed from a file.
type stampEntry struct {
	Page  int        `json:"page"`
	Rect  [4]float64 `json:"rect"`
	Text  string     `json:"text,omitempty"`
	Font  string     `json:"font,omitempty"`
	Size  float64    `json:"size,omitempty"`
	Color string     `json:"color,omitempty"`
	Image string     `json:"image,omitempty"` // a PNG or JPEG file
}

// stampsReport is the stderr summary, JSON with --json: counts and how each text fitted, never the text.
type stampsReport struct {
	Text    int `json:"text"`
	Images  int `json:"images"`
	Shrunk  int `json:"shrunk"`
	Wrapped int `json:"wrapped"`
	Overran int `json:"overran"`
}

// applyStamps reads the stamp list and bakes every entry into pdf: text through StampFields (which fits it to its
// rectangle), pictures through StampImages. Refusals name the entry's number, never its text.
func applyStamps(pdf, list []byte) ([]byte, stampsReport, error) {
	var rep stampsReport
	var entries []stampEntry
	if err := json.Unmarshal(list, &entries); err != nil {
		return nil, rep, fmt.Errorf("--stamps is not a JSON array of stamps: %v", err)
	}
	if len(entries) == 0 {
		return nil, rep, errors.New("--stamps lists no stamp")
	}
	pages, err := pdfops.PageCount(pdf)
	if err != nil {
		return nil, rep, err
	}
	var fields []pdfops.Field
	var images []pdfops.Stamp
	for i, e := range entries {
		n := i + 1
		if e.Page < 1 || e.Page > pages {
			return nil, rep, fmt.Errorf("stamp %d is on page %d, and the document has pages 1-%d", n, e.Page, pages)
		}
		if e.Rect[2] <= e.Rect[0] || e.Rect[3] <= e.Rect[1] {
			return nil, rep, fmt.Errorf("stamp %d: rect is [x0,y0,x1,y1] with x1 > x0 and y1 > y0, in points from the lower left", n)
		}
		switch hasText, hasImage := strings.TrimSpace(e.Text) != "", e.Image != ""; {
		case hasText == hasImage:
			return nil, rep, fmt.Errorf("stamp %d: give exactly one of text and image", n)
		case hasText:
			fields = append(fields, pdfops.Field{Page: e.Page, Rect: e.Rect, Text: e.Text, Font: e.Font, Size: e.Size, Color: e.Color})
		default:
			b, err := os.ReadFile(e.Image)
			if err != nil {
				return nil, rep, fmt.Errorf("stamp %d: %v", n, err)
			}
			images = append(images, pdfops.Stamp{Page: e.Page, Rect: e.Rect, PNG: b})
		}
	}
	out := pdf
	if len(fields) > 0 {
		var fits []pdfops.Fit
		if out, fits, err = pdfops.StampFields(out, fields); err != nil {
			return nil, rep, err
		}
		rep.Text = len(fits)
		for _, f := range fits {
			switch f.Outcome {
			case pdfops.FitShrunk:
				rep.Shrunk++
			case pdfops.FitWrapped:
				rep.Wrapped++
			case pdfops.FitOverran:
				rep.Overran++
			}
		}
	}
	if len(images) > 0 {
		if out, err = pdfops.StampImages(out, images); err != nil {
			return nil, rep, err
		}
		rep.Images = len(images)
	}
	return out, rep, nil
}
