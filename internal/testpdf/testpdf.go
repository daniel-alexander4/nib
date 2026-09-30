// Package testpdf generates small AcroForm PDFs for tests, and for
// `build/genpdf.go`'s fixture generator. No product package imports it — the nib
// binary reaches pdfcpu through `internal/pdfops`, not through here — so what it
// builds is a fixture, never a document nib ships.
package testpdf

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// formJSON describes a one-page form with a text field and a checkbox, using
// pdfcpu's create schema. No external resources, so it builds anywhere.
const formJSON = `{
  "paper": "A4P",
  "origin": "LowerLeft",
  "fonts": {
    "input": {"name": "Courier", "size": 12},
    "label": {"name": "Courier", "size": 12}
  },
  "pages": {
    "1": {
      "content": {
        "textfield": [
          {"id": "fullName", "value": "", "pos": [120, 700], "width": 200,
           "label": {"value": "Name:", "pos": "left", "width": 60, "gap": 8}}
        ],
        "checkbox": [
          {"id": "agree", "value": false, "pos": [120, 660], "width": 12,
           "label": {"value": "I agree", "pos": "right", "width": 60, "gap": 8}}
        ]
      }
    }
  }
}`

// Form returns the bytes of a freshly generated AcroForm PDF with a text field
// ("fullName") and a checkbox ("agree").
func Form() ([]byte, error) {
	var out bytes.Buffer
	if err := api.Create(nil, bytes.NewReader([]byte(formJSON)), &out, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Text returns a PDF with one page per string, each drawing that string as body
// text (Courier, so it lands in the page content stream as extractable text).
// Tests that need known, recoverable content — e.g. proving redaction truly
// removes it — use this.
func Text(pages ...string) ([]byte, error) {
	pageMap := map[string]any{}
	for i, s := range pages {
		pageMap[strconv.Itoa(i+1)] = map[string]any{
			"content": map[string]any{
				"text": []any{map[string]any{
					"Value": s,
					"pos":   []float64{100, 700},
					"Font":  map[string]any{"name": "Courier", "size": 14},
				}},
			},
		}
	}
	doc := map[string]any{"paper": "A4P", "origin": "LowerLeft", "pages": pageMap}
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.Create(nil, bytes.NewReader(b), &out, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
