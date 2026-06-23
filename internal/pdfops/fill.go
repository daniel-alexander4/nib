package pdfops

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// FillFormJSON fills pdf's AcroForm fields from a pdfcpu form-data JSON document
// (the same shape ExportFormJSON emits — fill is its exact inverse) and returns
// the filled PDF. Fields are matched by id or name. Any existing signature is
// removed by the fill, since the content changes (pdfcpu does this unconditionally).
func FillFormJSON(pdf, data []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := api.FillForm(bytes.NewReader(pdf), bytes.NewReader(data), &out, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// FillFormCSV mail-merges pdf against a wide CSV — row 0 holds field names (or
// ids), each later row is one record producing one filled PDF. A column whose
// header matches no field is ignored, so a label column can ride along. When
// nameCol names a header, that column's value names each output file (sanitised,
// de-duplicated); otherwise outputs are row-001, row-002, … Returns one SplitPart
// per record. Filling removes any existing signature (the content changes).
func FillFormCSV(pdf, data []byte, nameCol string) ([]SplitPart, error) {
	rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("CSV needs a header row of field names and at least one data row")
	}
	header := rows[0]
	colOf := map[string]int{}
	for i, h := range header {
		colOf[h] = i
	}

	nameIdx := -1
	if nameCol != "" {
		i, ok := colOf[nameCol]
		if !ok {
			return nil, fmt.Errorf("--name-col %q is not a CSV column", nameCol)
		}
		nameIdx = i
	}

	// The blank form's typed skeleton (fields with their real options) is the
	// template each record fills, so combobox/radio option-membership checks pass.
	skeleton, err := api.ExportForm(bytes.NewReader(pdf), "nib", model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	if skeleton == nil || len(skeleton.Forms) == 0 {
		return nil, fmt.Errorf("the PDF has no form fields to fill")
	}
	skelJSON, err := json.Marshal(skeleton)
	if err != nil {
		return nil, err
	}

	seen := map[string]int{}
	parts := make([]SplitPart, 0, len(rows)-1)
	for r, row := range rows[1:] {
		var fg form.FormGroup
		if err := json.Unmarshal(skelJSON, &fg); err != nil {
			return nil, err
		}
		f := &fg.Forms[0]
		for _, x := range f.TextFields {
			if v, ok := cell(x.Name, x.ID, colOf, row); ok {
				x.Value = v
			}
		}
		for _, x := range f.DateFields {
			if v, ok := cell(x.Name, x.ID, colOf, row); ok {
				x.Value = v
			}
		}
		for _, x := range f.CheckBoxes {
			if v, ok := cell(x.Name, x.ID, colOf, row); ok {
				x.Value = truthy(v)
			}
		}
		for _, x := range f.ComboBoxes {
			if v, ok := cell(x.Name, x.ID, colOf, row); ok {
				x.Value = v
			}
		}
		for _, x := range f.RadioButtonGroups {
			if v, ok := cell(x.Name, x.ID, colOf, row); ok {
				x.Value = v
			}
		}
		for _, x := range f.ListBoxes {
			if v, ok := cell(x.Name, x.ID, colOf, row); ok {
				x.Values = splitMulti(v)
			}
		}

		b, err := json.Marshal(fg)
		if err != nil {
			return nil, err
		}
		var out bytes.Buffer
		if err := api.FillForm(bytes.NewReader(pdf), bytes.NewReader(b), &out, model.NewDefaultConfiguration()); err != nil {
			return nil, fmt.Errorf("row %d: %w", r+1, err)
		}
		base := fmt.Sprintf("row-%03d", r+1)
		if nameIdx >= 0 && nameIdx < len(row) {
			if s := SanitizeFilename(row[nameIdx]); s != "" {
				base = s
			}
		}
		parts = append(parts, SplitPart{Name: UniqueName(base, r+1, seen), Data: out.Bytes()})
	}
	return parts, nil
}

// cell returns the row value under whichever of the field's name or id is a CSV
// column header (pdfcpu fills match on either).
func cell(name, id string, colOf map[string]int, row []string) (string, bool) {
	for _, key := range []string{name, id} {
		if key == "" {
			continue
		}
		if c, ok := colOf[key]; ok && c < len(row) {
			return row[c], true
		}
	}
	return "", false
}

// truthy reads a CSV checkbox cell — true for t/true/1/yes/y/x/checked/on (any case).
func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "t", "true", "1", "yes", "y", "x", "checked", "on":
		return true
	}
	return false
}

// splitMulti splits a multi-select list-box cell on commas (trimmed, blanks dropped).
func splitMulti(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
