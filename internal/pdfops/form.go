package pdfops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// FormField is one interactive AcroForm field to author onto an existing page.
// Rect is in PDF points, bottom-left origin (x0,y0,x1,y1) — the same space the
// client's rectPoints produces. Kind is "text", "check", "dropdown", or "radio".
// Options is the choice list for dropdown (≥1) and radio (≥2); ignored otherwise.
type FormField struct {
	Page    int        `json:"page"`
	Rect    [4]float64 `json:"rect"`
	Kind    string     `json:"kind"`
	Name    string     `json:"name"`
	Options []string   `json:"options,omitempty"`
	// Orientation lays out a radio group: "vert" stacks the buttons downward from
	// the box's top edge, anything else (the default) marches them right from the
	// lower-left. Ignored for non-radio kinds.
	Orientation string `json:"orientation,omitempty"`
}

// AuthorForm adds real fillable AcroForm widgets (text fields, checkboxes) to the
// existing pages of pdf at the given rects, producing a blank fillable form — the
// document's existing content (e.g. a scan) is preserved; pdfcpu's api.Create with
// an existing reader appends the widgets onto the existing pages' /Annots and
// writes a proper catalog /AcroForm with generated appearance streams. Fields are
// authored blank (a template to distribute), not pre-filled.
func AuthorForm(pdf []byte, fields []FormField) ([]byte, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("no fields to author")
	}
	pages := map[string]any{}
	for _, f := range fields {
		if f.Name == "" {
			return nil, fmt.Errorf("every field needs a name")
		}
		key := strconv.Itoa(f.Page)
		pg, ok := pages[key].(map[string]any)
		if !ok {
			pg = map[string]any{"content": map[string]any{}}
			pages[key] = pg
		}
		content := pg["content"].(map[string]any)
		x0, y0, x1, y1 := f.Rect[0], f.Rect[1], f.Rect[2], f.Rect[3]
		switch f.Kind {
		case "check":
			side := x1 - x0
			if h := y1 - y0; h < side {
				side = h // a checkbox is square; use the smaller side
			}
			arr, _ := content["checkbox"].([]any)
			content["checkbox"] = append(arr, map[string]any{
				"id": f.Name, "value": false,
				"pos": []float64{x0, y0}, "width": side,
			})
		case "dropdown":
			if len(f.Options) == 0 {
				return nil, fmt.Errorf("dropdown %q needs at least one option", f.Name)
			}
			arr, _ := content["combobox"].([]any)
			content["combobox"] = append(arr, map[string]any{
				"id": f.Name, "options": f.Options,
				"pos": []float64{x0, y0}, "width": x1 - x0,
			})
		case "radio":
			// pdfcpu's radiobuttongroup auto-lays-out the buttons from a single
			// anchor, so the drawn box just picks the start point and a button size;
			// each button is labelled with its value. A horizontal group anchors at
			// the lower-left and marches right, its button size taken from the box
			// height; a vertical group anchors at the top-left and stacks downward,
			// its button size taken from the box width. pdfcpu requires ≥2 values.
			if len(f.Options) < 2 {
				return nil, fmt.Errorf("radio %q needs at least two options", f.Name)
			}
			orientation, pos, width := "hor", []float64{x0, y0}, y1-y0
			if f.Orientation == "vert" {
				orientation, pos, width = "vert", []float64{x0, y1}, x1-x0
			}
			arr, _ := content["radiobuttongroup"].([]any)
			content["radiobuttongroup"] = append(arr, map[string]any{
				"id": f.Name, "orientation": orientation,
				"pos": pos, "width": width,
				"buttons": map[string]any{
					"values": f.Options,
					"label":  map[string]any{"value": "dummy", "width": 60, "gap": 4, "pos": "right"},
				},
			})
		default: // text
			arr, _ := content["textfield"].([]any)
			content["textfield"] = append(arr, map[string]any{
				"id": f.Name, "value": "",
				"pos": []float64{x0, y0}, "width": x1 - x0, "height": y1 - y0,
			})
		}
	}
	doc := map[string]any{
		"origin": "LowerLeft",
		"fonts": map[string]any{
			"input": map[string]any{"name": "Helvetica", "size": 10},
			"label": map[string]any{"name": "Helvetica", "size": 9}, // radio button value labels
		},
		"pages": pages,
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.Create(bytes.NewReader(pdf), bytes.NewReader(b), &out, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
