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
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// RasterPage is a rasterized page image plus the point size its PDF page should
// take. The client rasterizes at 2× for crispness, so the image's pixel
// dimensions are double the page; carrying the target size explicitly keeps the
// physical page at its true size instead of letting pdfcpu treat 1px as 1pt.
type RasterPage struct {
	Image []byte
	W, H  float64 // target page size in PDF points
}

// ImagesToPDF builds a PDF with one image per page, each page sized to its
// RasterPage's point dimensions. It is the rasterize path for flatten and image
// export: the client renders each page to a PNG, the server assembles them into a
// guaranteed-flat PDF at the original physical size.
func ImagesToPDF(pages []RasterPage) ([]byte, error) {
	segs := make([][]byte, len(pages))
	for i, p := range pages {
		b, err := imageToPage(p)
		if err != nil {
			return nil, err
		}
		segs[i] = b
	}
	if len(segs) == 1 {
		return segs[0], nil
	}
	readers := make([]io.ReadSeeker, len(segs))
	for i, b := range segs {
		readers[i] = bytes.NewReader(b)
	}
	var out bytes.Buffer
	if err := api.MergeRaw(readers, &out, false, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// imageToPage builds a one-page PDF whose page is exactly p.W×p.H points, with the
// image scaled to fill it. pdfcpu's default Full anchor sizes the page to the
// image's pixel dimensions (the 2× bug); a non-Full anchor with relative scale 1
// instead sizes the page to PageDim and fills it (the image's aspect matches the
// page, since it was rasterized from it).
func imageToPage(p RasterPage) ([]byte, error) {
	imp := &pdfcpu.Import{
		PageDim: &types.Dim{Width: p.W, Height: p.H},
		Pos:     types.Center,
		Scale:   1.0,
		InpUnit: types.POINTS,
	}
	var out bytes.Buffer
	if err := api.ImportImages(nil, &out, []io.Reader{bytes.NewReader(p.Image)}, imp, nil); err != nil {
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
// number -> a RasterPage: a PNG of that page with the redaction boxes already
// painted in, plus the page's true point size) is replaced by that flat image,
// while every other page is kept as-is. Because a
// redacted page becomes a pure image, the underlying text/content is genuinely
// gone — not merely covered. This is the guaranteed-removal redaction.
func RedactPages(original []byte, raster map[int]RasterPage) ([]byte, error) {
	n, err := PageCount(original)
	if err != nil {
		return nil, err
	}
	segments := make([]io.ReadSeeker, 0, n)
	for i := 1; i <= n; i++ {
		var seg []byte
		if page, ok := raster[i]; ok {
			seg, err = ImagesToPDF([]RasterPage{page})
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
// for a checked box) and its rectangle in PDF points (bottom-left origin). Font,
// Size and Color are optional cover-and-replace overrides; left zero they keep
// the historical auto-detected-field behaviour (Helvetica, size from the rect
// height, black), so detected form fields are unaffected.
type Field struct {
	Page  int        `json:"page"`
	Rect  [4]float64 `json:"rect"` // x0, y0, x1, y1
	Text  string     `json:"text"`
	Font  string     `json:"font,omitempty"`  // a Base-14 core font name; defaults to Helvetica
	Size  float64    `json:"size,omitempty"`  // explicit point size; 0 = derive from rect height
	Color string     `json:"color,omitempty"` // #RRGGBB; "" = black
}

// coreFonts is the Base-14 text-font allowlist StampFields will honour. The name
// is interpolated into the pdfcpu watermark description, so an unlisted value
// falls back to Helvetica rather than risk injecting extra description keys.
var coreFonts = map[string]bool{
	"Helvetica": true, "Helvetica-Bold": true, "Helvetica-Oblique": true, "Helvetica-BoldOblique": true,
	"Times-Roman": true, "Times-Bold": true, "Times-Italic": true, "Times-BoldItalic": true,
	"Courier": true, "Courier-Bold": true, "Courier-Oblique": true, "Courier-BoldOblique": true,
}

func coreFont(name string) string {
	if coreFonts[name] {
		return name
	}
	return "Helvetica"
}

// StampFields bakes the given overlay-field values onto the PDF as text, sized to
// each field's rectangle. All fields are applied in a single pass.
func StampFields(pdf []byte, fields []Field) ([]byte, error) {
	wms := map[int][]*model.Watermark{}
	for _, f := range fields {
		if strings.TrimSpace(f.Text) == "" {
			continue
		}
		// An explicit Size (cover-and-replace edit) is honoured up to 144pt; with
		// none, size is derived from the rect height as detected fields always have,
		// capped at 48 so a tall detected box doesn't balloon the text.
		pts, max := int(f.Size), 144
		if pts <= 0 {
			pts, max = int((f.Rect[3]-f.Rect[1])*0.72), 48
		}
		if pts < 6 {
			pts = 8
		}
		if pts > max {
			pts = max
		}
		color := "#000000"
		if hexColor.MatchString(f.Color) {
			color = f.Color
		}
		// Anchor the text near the field's bottom-left (small inset), in points.
		desc := fmt.Sprintf("fontname:%s, points:%d, scalefactor:1 abs, position:bl, offset:%.1f %.1f, fillcolor:%s, rotation:0",
			coreFont(f.Font), pts, f.Rect[0]+2, f.Rect[1]+2, color)
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
	// Always drawn on top: a "behind" watermark is hidden by any page with an
	// opaque background, inconsistently per page — opacity gives the subtle look.
	wm, err := api.TextWatermark(text, desc, true, false, types.POINTS)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.AddWatermarks(bytes.NewReader(pdf), &out, nil, wm, model.NewDefaultConfiguration()); err != nil {
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
