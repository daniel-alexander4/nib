package pdfops

import (
	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// FieldWidget is one place a field is drawn. Page is 1-based; Rect is the widget's /Rect as [llx lly urx ury] in
// that page's own user space — raw, so /Rotate and the boxes are the caller's to apply (`nib fields`).
type FieldWidget struct {
	Page int        `json:"page"`
	Rect [4]float64 `json:"rect"`
}

// FormFieldInfo is one terminal field of a form as `nib fields` lists it. It carries NO value: what a field holds is
// the export's (a listing run over a filled form must not copy what was typed into one more place).
type FormFieldInfo struct {
	// Name is the fully qualified name — the one `nib fill` and the export use, so a listing can be filled against.
	Name string `json:"name"`
	// Type is text, checkbox, radio, pushbutton, combobox, listbox, signature or unknown.
	Type string `json:"type"`
	// Fillable says a fill can set it: not a signature, not a push button.
	Fillable bool `json:"fillable"`
	// Flags is /Ff as the file gives it (own or the nearest ancestor's), for read-only, required and the rest.
	Flags int `json:"flags"`
	// Tooltip is the field's own /TU — what an author wrote to say what the field is for ("Line 1a"), or "".
	Tooltip string `json:"tooltip,omitempty"`
	// Label is the text an XFA form's template gives the field (`<assist><speak>`, else `<toolTip>`) — the printed line
	// label of an IRS form, which carries no /TU. "" when the document has no XFA template, the template names no
	// label, or two fields of one XFA name path disagree (pdfread.XFALabels).
	Label string `json:"label,omitempty"`
	// OnPage is false for a field in the form's tree that no page draws (ADR-166); it then has no widgets, and a
	// fill does nothing to it.
	OnPage  bool          `json:"onPage"`
	Widgets []FieldWidget `json:"widgets"`
}

// fieldType is the one word for what a field is, from its /FT and /Ff alone (ISO 32000-1 Tables 226 and 230).
func fieldType(f formField) string {
	switch f.kind {
	case "Tx":
		return "text"
	case "Btn":
		switch buttonKind(f.flags) {
		case "button":
			return "pushbutton"
		case "radio":
			return "radio"
		}
		return "checkbox"
	case "Ch":
		if f.flags&(1<<17) != 0 {
			return "combobox"
		}
		return "listbox"
	case "Sig":
		return "signature"
	}
	return "unknown"
}

// ListFormFields lists the terminal fields of pdf's form, in the breadth-first order of the form's tree (`eachFormField`: the top level first, then each level below it) — over
// `formFields`, the one answer to what a form's fields are and whether a page draws them (ADR-009, ADR-166). A
// document with no form lists none and is not an error.
func ListFormFields(pdf []byte) ([]FormFieldInfo, error) {
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.EXPORTFORMFIELDS // the reading the export, and so the fill's names, are made over
	ctx, err := pdfread.ReadOptimized(pdf, conf)
	if err != nil {
		return nil, err
	}
	fields, err := formFields(ctx.XRefTable)
	if err != nil {
		return nil, err
	}
	labels, err := pdfread.XFALabels(pdf)
	if err != nil {
		labels = nil // a label is a convenience over the listing, never a condition of it
	}
	out := make([]FormFieldInfo, 0, len(fields))
	for _, f := range fields {
		info := FormFieldInfo{
			Name:     f.name,
			Type:     fieldType(f),
			Fillable: f.fillable(),
			Flags:    f.flags,
			Tooltip:  f.tooltip,
			Label:    labels[f.name],
			OnPage:   f.onPage,
			Widgets:  make([]FieldWidget, 0, len(f.widgets)),
		}
		for _, w := range f.widgets {
			info.Widgets = append(info.Widgets, FieldWidget{Page: w.page, Rect: w.rect})
		}
		out = append(out, info)
	}
	return out, nil
}
