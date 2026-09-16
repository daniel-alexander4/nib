package pdfops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
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
	// Label is the name the USER typed for this field, and it becomes the field's
	// `/TU` — the accessible name a screen reader announces. It is deliberately not
	// Name: `Name` is the internal `/T`, which the client trims, defaults to
	// `field_N` and de-dupes with a numeric suffix, so by the time it arrives here
	// it is an identifier and no longer what anyone typed.
	//
	// **Empty means the user named nothing, and then no `/TU` is written.** A `/TU`
	// of "field_3" is identical to the `/T` a reader already falls back to when
	// there is none, so it announces nothing new while claiming to be a name —
	// which is what `TitleFromName` refuses for the same reason, one key over.
	Label string `json:"label,omitempty"`
	// Orientation lays out a radio group: "vert" stacks the buttons downward from
	// the box's top edge, anything else (the default) marches them right from the
	// lower-left. Ignored for non-radio kinds.
	Orientation string `json:"orientation,omitempty"`
}

// withTip attaches the user's own name for a field as pdfcpu's `Tip`, which becomes
// the field dictionary's `/TU` — the accessible name a screen reader announces.
//
// **One door, because the rule is one rule and there are four kinds.** ADR-009: a rule
// holding at more than one call site is written once and every site calls it, and the
// guard checks the door rather than agreement between copies.
//
// An empty label attaches nothing. That is not a gap to fill later: see FormField.Label
// for why "field_3" is worse than silence here.
func withTip(spec map[string]any, label string) map[string]any {
	if label != "" {
		spec["tip"] = label
	}
	return spec
}

// AuthorForm adds real fillable AcroForm widgets (text fields, checkboxes) to the
// existing pages of pdf at the given rects, producing a blank fillable form — the
// document's existing content (e.g. a scan) is preserved; pdfcpu's api.Create with
// an existing reader appends the widgets onto the existing pages' /Annots and
// writes a proper catalog /AcroForm with generated appearance streams. Fields are
// authored blank (a template to distribute), not pre-filled.
func AuthorForm(pdf []byte, fields []FormField) ([]byte, error) {
	// **Base-14 Helvetica on purpose, and it costs PDF/UA 7.21.4.1** (`/pending 479`). The obvious
	// fix — the embedded face `AuthoredTextFaces` gives every other authoring door — breaks FILLING:
	// pdfcpu v0.13.0 writes a filled value as one-byte text against the Identity-H font
	// (`model.PrepBytes` skips the glyph encoding for a form's own fill font), so "Zoë Ñúñez"
	// renders as a missing-glyph box, and v0.15.0 refuses the fill outright. Both server fill doors
	// (CSV, XFDF) would ship that on every form nib authors, and their tests read the value back
	// rather than the appearance, so they stay green over it.
	// `TestPdfcpuStillCannotFillAFieldSetInAnEmbeddedFace` goes red when that changes.
	return authorFormIn(pdf, fields, "Helvetica")
}

// authorFormIn is AuthorForm with the face its field text is set in named, so the test that
// watches `/pending 479`'s gate exercises the exact spec AuthorForm ships.
func authorFormIn(pdf []byte, fields []FormField, face string) ([]byte, error) {
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
			content["checkbox"] = append(arr, withTip(map[string]any{
				"id": f.Name, "value": false,
				"pos": []float64{x0, y0}, "width": side,
			}, f.Label))
		case "dropdown":
			if len(f.Options) == 0 {
				return nil, fmt.Errorf("dropdown %q needs at least one option", f.Name)
			}
			arr, _ := content["combobox"].([]any)
			content["combobox"] = append(arr, withTip(map[string]any{
				"id": f.Name, "options": f.Options,
				"pos": []float64{x0, y0}, "width": x1 - x0,
			}, f.Label))
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
			content["radiobuttongroup"] = append(arr, withTip(map[string]any{
				"id": f.Name, "orientation": orientation,
				"pos": pos, "width": width,
				"buttons": map[string]any{
					"values": f.Options,
					"label":  map[string]any{"value": "dummy", "width": 60, "gap": 4, "pos": "right"},
				},
			}, f.Label))
		default: // text
			arr, _ := content["textfield"].([]any)
			content["textfield"] = append(arr, withTip(map[string]any{
				"id": f.Name, "value": "",
				"pos": []float64{x0, y0}, "width": x1 - x0, "height": y1 - y0,
			}, f.Label))
		}
	}
	doc := map[string]any{
		"origin": "LowerLeft",
		"fonts": map[string]any{
			"input": map[string]any{"name": face, "size": 10},
			"label": map[string]any{"name": face, "size": 9}, // radio button value labels
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
	return setWidgetTabOrder(out.Bytes())
}

// setWidgetTabOrder writes `/Tabs /S` on every page that carries an annotation, so
// tab order follows the document's structure instead of the order the widgets happen
// to sit in `/Annots`.
//
// # Why it is a post-pass and not part of the JSON
//
// pdfcpu's create schema has no page-level `Tabs`, so there is nowhere to say it going
// in. Measured on veraPDF ua1: without this an authored form fails **7.18.3 t1**, and
// with it that clause goes — `/TU` does not move it and this does not move `/TU`'s.
//
// # Why every annotated page and not just the pages this call placed fields on
//
// The clause is about a PAGE with annotations, not about who put them there. A document
// that already had a widget on page 3 keeps failing on page 3 if the rule is scoped to
// this call's fields, and the user cannot tell the difference from a bug. `S` is the
// only value that means structure order; the alternatives (`R`, `C`) are geometric and
// say nothing about reading order.
//
// **It never fails the author.** A form that could not be given a tab order is still a
// form, and refusing the whole operation over an ordering key would cost the user the
// fields to gain a clause.
func setWidgetTabOrder(pdf []byte) ([]byte, error) {
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		setStructureTabOrder(ctx)
		return nil
	})
	if err != nil {
		return pdf, nil
	}
	return out, nil
}

// setStructureTabOrder is the rule itself, on a parsed document, so an operation that already holds
// one — `AddNotes` — applies it inside its own rewrite instead of paying for a second (ADR-009).
func setStructureTabOrder(ctx *model.Context) {
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, derr := ctx.PageDict(p, false)
		if derr != nil || d == nil {
			continue
		}
		annots, aerr := ctx.DereferenceArray(d["Annots"])
		if aerr != nil || len(annots) == 0 {
			continue
		}
		d["Tabs"] = types.Name("S")
	}
}
