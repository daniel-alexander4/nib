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
	multi := listBoxKeys(skeleton) // which headers feed multi-select list boxes

	seen := map[string]int{}
	parts := make([]SplitPart, 0, len(rows)-1)
	for r, row := range rows[1:] {
		out, err := fillFromValues(pdf, skelJSON, rowValues(header, row, multi), truthy)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", r+1, err)
		}
		base := fmt.Sprintf("row-%03d", r+1)
		if nameIdx >= 0 && nameIdx < len(row) {
			if s := SanitizeFilename(row[nameIdx]); s != "" {
				base = s
			}
		}
		parts = append(parts, SplitPart{Name: UniqueName(base, r+1, seen), Data: out})
	}
	return parts, nil
}

// fillFromValues fills the marshalled skeleton form group with values keyed by
// field name or id, runs pdfcpu's fill over pdf, and returns the filled PDF. Both
// FillFormCSV (per row) and FillFormXFDF feed this one applier, so the
// name→typed-field mapping lives in a single place. List boxes take every value;
// every other field takes the first.
//
// **`checked` is the one rule that is genuinely per-format, and it is a parameter rather than
// a widened shared predicate.** A CSV cell is a human's "yes"/"no"; an XFDF `<value>` is the
// checkbox's EXPORT-VALUE NAME, which is any name the form chose and is "on" unless it is
// `Off`. Reading XFDF with the CSV rule silently left a box unchecked whenever the form's
// on-state was not one of eight English words, and reported the fill a success (/pending 9).
// Widening `truthy` instead would have made a CSV "no" mean checked, so the two callers pass
// their own rule and the mapping above still lives in one place.
func fillFromValues(pdf, skelJSON []byte, values map[string][]string, checked func(string) bool) ([]byte, error) {
	var fg form.FormGroup
	if err := json.Unmarshal(skelJSON, &fg); err != nil {
		return nil, err
	}
	f := &fg.Forms[0]
	for _, x := range f.TextFields {
		if v, ok := pick(values, x.Name, x.ID); ok {
			x.Value = first(v)
		}
	}
	for _, x := range f.DateFields {
		if v, ok := pick(values, x.Name, x.ID); ok {
			x.Value = first(v)
		}
	}
	for _, x := range f.CheckBoxes {
		if v, ok := pick(values, x.Name, x.ID); ok {
			x.Value = checked(first(v))
		}
	}
	for _, x := range f.ComboBoxes {
		if v, ok := pick(values, x.Name, x.ID); ok {
			x.Value = first(v)
		}
	}
	for _, x := range f.RadioButtonGroups {
		if v, ok := pick(values, x.Name, x.ID); ok {
			x.Value = first(v)
		}
	}
	for _, x := range f.ListBoxes {
		if v, ok := pick(values, x.Name, x.ID); ok {
			x.Values = v
		}
	}
	b, err := json.Marshal(fg)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.FillForm(bytes.NewReader(pdf), bytes.NewReader(b), &out, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// rowValues turns a CSV record into the name→value(s) map fillFromValues consumes,
// keyed by each column header (a field name or id). A header feeding a list box is
// split into multiple values; every other column carries a single value.
func rowValues(header, row []string, multi map[string]bool) map[string][]string {
	values := make(map[string][]string, len(header))
	for i, h := range header {
		if i >= len(row) {
			continue
		}
		if multi[h] {
			values[h] = splitMulti(row[i])
		} else {
			values[h] = []string{row[i]}
		}
	}
	return values
}

// listBoxKeys returns the set of names and ids that identify list-box fields, so
// the CSV path knows which cells to split into multiple selections.
func listBoxKeys(fg *form.FormGroup) map[string]bool {
	keys := map[string]bool{}
	for _, f := range fg.Forms {
		for _, x := range f.ListBoxes {
			keys[x.Name] = true
			keys[x.ID] = true
		}
	}
	return keys
}

// pick returns the values stored under whichever of the field's name or id is a
// key (pdfcpu fills match on either).
func pick(values map[string][]string, name, id string) ([]string, bool) {
	for _, key := range []string{name, id} {
		if key == "" {
			continue
		}
		if v, ok := values[key]; ok {
			return v, true
		}
	}
	return nil, false
}

// first returns the leading value, or "" for an empty slice.
func first(v []string) string {
	if len(v) > 0 {
		return v[0]
	}
	return ""
}

// xfdfChecked reads an XFDF checkbox value, which is an export-value NAME rather than a
// boolean word: `Off` is the one name PDF reserves for the unchecked state, so anything else
// — `Yes`, `Ja`, `1`, `On`, a form's own label — means checked. Empty is unchecked, because an
// empty `<value/>` is how a clearing export writes "not set".
func xfdfChecked(s string) bool {
	t := strings.TrimSpace(s)
	return t != "" && !strings.EqualFold(t, "off")
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
