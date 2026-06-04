// Package pdfops wraps the pdfcpu operations Nib performs server-side:
// assembling rasterized pages into a PDF (the guaranteed flatten), protecting
// output with a password, and exporting form data.
package pdfops

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders for image.DecodeConfig
	_ "image/png"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ImagesToPDF builds a PDF with one image per page. It is the rasterize path for
// flatten and image export: the client renders each page to a PNG, the server
// assembles them into a guaranteed-flat PDF.
func ImagesToPDF(images [][]byte) ([]byte, error) {
	imp, err := api.Import("", types.POINTS)
	if err != nil {
		return nil, err
	}
	readers := make([]io.Reader, len(images))
	for i, img := range images {
		readers[i] = bytes.NewReader(img)
	}
	var out bytes.Buffer
	if err := api.ImportImages(nil, &out, readers, imp, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Rotate rotates the given pages (e.g. []string{"1","3-5"}, or nil for all) by
// deg degrees (90, 180, 270, or negatives).
func Rotate(pdf []byte, pages []string, deg int) ([]byte, error) {
	var out bytes.Buffer
	if err := api.Rotate(bytes.NewReader(pdf), &out, deg, pages, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// RemovePages drops the given pages from the PDF.
func RemovePages(pdf []byte, pages []string) ([]byte, error) {
	var out bytes.Buffer
	if err := api.RemovePages(bytes.NewReader(pdf), &out, pages, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Reorder keeps the pages in the exact order given (e.g. []string{"3","1","2"}),
// which also covers deletion by omission.
func Reorder(pdf []byte, order []string) ([]byte, error) {
	var out bytes.Buffer
	if err := api.Collect(bytes.NewReader(pdf), &out, order, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Append concatenates other after pdf (merge).
func Append(pdf, other []byte) ([]byte, error) {
	var out bytes.Buffer
	rs := []io.ReadSeeker{bytes.NewReader(pdf), bytes.NewReader(other)}
	if err := api.MergeRaw(rs, &out, false, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// RedactPages rebuilds a PDF so that each page given in raster (1-based page
// number -> a PNG of that page with the redaction boxes already painted in) is
// replaced by that flat image, while every other page is kept as-is. Because a
// redacted page becomes a pure image, the underlying text/content is genuinely
// gone — not merely covered. This is the guaranteed-removal redaction.
func RedactPages(original []byte, raster map[int][]byte) ([]byte, error) {
	n, err := PageCount(original)
	if err != nil {
		return nil, err
	}
	segments := make([]io.ReadSeeker, 0, n)
	for i := 1; i <= n; i++ {
		var seg []byte
		if png, ok := raster[i]; ok {
			seg, err = ImagesToPDF([][]byte{png})
		} else {
			seg, err = Reorder(original, []string{strconv.Itoa(i)}) // extract page i, vector intact
		}
		if err != nil {
			return nil, err
		}
		segments = append(segments, bytes.NewReader(seg))
	}
	var out bytes.Buffer
	if err := api.MergeRaw(segments, &out, false, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// PageCount returns the number of pages in the PDF. (pdfcpu's PageCount path
// dereferences the configuration, so it must be non-nil.)
func PageCount(pdf []byte) (int, error) {
	return api.PageCount(bytes.NewReader(pdf), model.NewDefaultConfiguration())
}

// Field is a filled overlay field to stamp onto the page: the text (already "X"
// for a checked box) and its rectangle in PDF points (bottom-left origin).
type Field struct {
	Page int        `json:"page"`
	Rect [4]float64 `json:"rect"` // x0, y0, x1, y1
	Text string     `json:"text"`
}

// StampFields bakes the given overlay-field values onto the PDF as text, sized to
// each field's rectangle. All fields are applied in a single pass.
func StampFields(pdf []byte, fields []Field) ([]byte, error) {
	wms := map[int][]*model.Watermark{}
	for _, f := range fields {
		if strings.TrimSpace(f.Text) == "" {
			continue
		}
		h := f.Rect[3] - f.Rect[1]
		pts := int(h * 0.72)
		if pts < 6 {
			pts = 8
		}
		if pts > 48 {
			pts = 48
		}
		// Anchor the text near the field's bottom-left (small inset), in points.
		desc := fmt.Sprintf("fontname:Helvetica, points:%d, scalefactor:1 abs, position:bl, offset:%.1f %.1f, fillcolor:#000000, rotation:0",
			pts, f.Rect[0]+2, f.Rect[1]+2)
		wm, err := api.TextWatermark(f.Text, desc, true, false, types.POINTS)
		if err != nil {
			return nil, err
		}
		page := f.Page
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

// Stamp is an image to bake onto the page at a given rectangle (PDF points,
// bottom-left origin): a chosen signature image, or a drawn Y/N circle. The
// image is scaled to fill the rectangle's width; callers render the PNG at the
// rectangle's aspect ratio so it sits correctly on the line/letter.
type Stamp struct {
	Page int        `json:"page"`
	Rect [4]float64 `json:"rect"` // x0, y0, x1, y1
	PNG  []byte     `json:"-"`    // resolved image bytes (PNG or JPEG)
}

// StampImages bakes the given image stamps onto the PDF as image watermarks,
// each scaled and positioned to its rectangle. Applied in a single pass on top
// of any existing content.
func StampImages(pdf []byte, stamps []Stamp) ([]byte, error) {
	wms := map[int][]*model.Watermark{}
	for _, s := range stamps {
		if len(s.PNG) == 0 {
			continue
		}
		boxW := s.Rect[2] - s.Rect[0]
		boxH := s.Rect[3] - s.Rect[1]
		if boxW <= 0 || boxH <= 0 {
			continue
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(s.PNG))
		if err != nil || cfg.Width == 0 || cfg.Height == 0 {
			return nil, fmt.Errorf("decode stamp image: %w", err)
		}
		// pdfcpu sizes an image watermark as its pixel dimensions (treated as
		// points) times the absolute scale factor. Fit the image inside the box
		// preserving aspect (so a wide signature sits at line height rather than
		// ballooning to the full underline width); position:bl anchors the
		// image's bottom-left corner at the box's bottom-left.
		scale := boxW / float64(cfg.Width)
		if sy := boxH / float64(cfg.Height); sy < scale {
			scale = sy
		}
		desc := fmt.Sprintf("position:bl, offset:%.2f %.2f, scalefactor:%.5f abs, rotation:0", s.Rect[0], s.Rect[1], scale)
		wm, err := api.ImageWatermarkForReader(bytes.NewReader(s.PNG), desc, true, false, types.POINTS)
		if err != nil {
			return nil, err
		}
		page := s.Page
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

// WatermarkStyle controls how StampWatermark renders the label.
type WatermarkStyle struct {
	Color   string  `json:"color"`   // #RRGGBB
	Opacity float64 `json:"opacity"` // 0..1
	OnTop   bool    `json:"onTop"`   // over the content vs behind it
	Scale   float64 `json:"scale"`   // page-relative size, 0..1
	Angle   int     `json:"angle"`   // degrees
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// sanitize clamps the style to safe ranges and a valid hex colour. The colour
// and numbers are interpolated into the pdfcpu watermark description string, so
// this is what stops a crafted value from injecting extra description keys.
func (s WatermarkStyle) sanitize() WatermarkStyle {
	if !hexColor.MatchString(s.Color) {
		s.Color = "#8a8a8a"
	}
	s.Opacity = clampUnit(s.Opacity, 0.02, 1)
	s.Scale = clampUnit(s.Scale, 0.1, 1)
	switch {
	case s.Angle < -90:
		s.Angle = -90
	case s.Angle > 90:
		s.Angle = 90
	}
	return s
}

func clampUnit(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// StampWatermark draws text as a large diagonal watermark across every page,
// styled by st. Finalize applies it before signing so the certification covers
// the watermark.
func StampWatermark(pdf []byte, text string, st WatermarkStyle) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return pdf, nil
	}
	st = st.sanitize()
	desc := fmt.Sprintf("fontname:Helvetica, scalefactor:%.3f rel, fillcolor:%s, opacity:%.3f, rotation:%d, position:c",
		st.Scale, st.Color, st.Opacity, st.Angle)
	wm, err := api.TextWatermark(text, desc, st.OnTop, false, types.POINTS)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.AddWatermarks(bytes.NewReader(pdf), &out, nil, wm, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Encrypt password-protects a PDF with AES (user password = owner password).
func Encrypt(pdf []byte, password string) ([]byte, error) {
	conf := model.NewDefaultConfiguration()
	conf.UserPW = password
	conf.OwnerPW = password
	conf.EncryptUsingAES = true
	var out bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(pdf), &out, conf); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ExportFormJSON returns the form field data of pdf as pdfcpu's JSON.
func ExportFormJSON(pdf []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := api.ExportFormJSON(bytes.NewReader(pdf), &out, "nib", nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ExportFormCSV returns the form field data as two columns: field name and value.
// It reuses pdfcpu's own JSON export (the single source of truth) and reshapes it.
func ExportFormCSV(pdf []byte) ([]byte, error) {
	jsonData, err := ExportFormJSON(pdf)
	if err != nil {
		return nil, err
	}
	var fg form.FormGroup
	if err := json.Unmarshal(jsonData, &fg); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{"field", "value"})
	for _, f := range fg.Forms {
		for _, x := range f.TextFields {
			_ = cw.Write([]string{x.Name, x.Value})
		}
		for _, x := range f.DateFields {
			_ = cw.Write([]string{x.Name, x.Value})
		}
		for _, x := range f.CheckBoxes {
			_ = cw.Write([]string{x.Name, strconv.FormatBool(x.Value)})
		}
		for _, x := range f.RadioButtonGroups {
			_ = cw.Write([]string{x.Name, x.Value})
		}
		for _, x := range f.ComboBoxes {
			_ = cw.Write([]string{x.Name, x.Value})
		}
		for _, x := range f.ListBoxes {
			_ = cw.Write([]string{x.Name, joinValues(x.Values)})
		}
	}
	cw.Flush()
	return buf.Bytes(), cw.Error()
}

func joinValues(vs []string) string { return strings.Join(vs, "; ") }
