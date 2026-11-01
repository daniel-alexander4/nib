package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"nib/internal/pdfops"
)

// authorField is one entry of `nib author --fields`: the app's own FormField (page, rect in points from the lower
// left, kind text|check|dropdown|radio, name, options, label, orientation) plus a default value (RelaxTax B2). The
// value is the CLI's alone — the window authors blank forms — so it lives here and not in pdfops.FormField.
type authorField struct {
	pdfops.FormField
	// Value is a text field's, dropdown's or radio group's text, or a check box's boolean (or "true"/"yes"/"on"/"1").
	Value any `json:"value,omitempty"`
}

// cmdAuthor adds fillable fields to the pages of IN (ADR-183, RelaxTax B2): the window's authoring door, with a
// default value per field so a template can carry one.
func cmdAuthor(args []string) int {
	fs := flag.NewFlagSet("nib author", flag.ContinueOnError)
	var out, fieldsPath string
	outFlag(fs, &out)
	fs.StringVar(&fieldsPath, "fields", "", "the fields to add, a JSON array: [{\"page\":1,\"rect\":[x0,y0,x1,y1],\"kind\":\"text|check|dropdown|radio\",\"name\":\"id\",\"options\":[…],\"label\":\"…\",\"value\":…}]; - reads it from stdin (required)")
	fs.Usage = usageFunc(fs, "nib author IN --fields F.json -o OUT",
		"Add real fillable form fields to the pages of a PDF, as the window's Add fields does (a tagged form where the document allows it). rect is in points from the page's lower left.\n"+
			"An optional value per field is the default a template carries: text, a dropdown or radio option, or true/false for a check box. A value that is not one of the options is refused, naming the field and never the value.\n"+
			"IN may be - (stdin) when --fields is a file; -o - writes the PDF to stdout.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fieldsPath == "" {
		errf("missing --fields (the JSON list of fields to add)")
		return 1
	}
	in, code := singleInput(fs, out)
	if code != 0 {
		return code
	}
	if in == "-" && fieldsPath == "-" {
		errf("the PDF and --fields cannot both be - : stdin is one stream")
		return 1
	}
	raw, err := readInput(fieldsPath)
	if err != nil {
		errf("%v", err)
		return 1
	}
	var fields []authorField
	if err := json.Unmarshal(raw, &fields); err != nil {
		// A decoder's own message names a type or a position, never the text (ADR-183).
		errf("--fields is not a JSON array of fields: %v", err)
		return 1
	}
	plain := make([]pdfops.FormField, len(fields))
	for i, f := range fields {
		plain[i] = f.FormField
	}
	data, n, err := authorDefaults(fields)
	if err != nil {
		errf("%v", err)
		return 1
	}
	var tagged bool
	rc := transform(in, out, func(b []byte) ([]byte, error) {
		authored, isTagged, err := pdfops.AuthorTaggedForm(b, plain)
		if err != nil {
			return nil, err
		}
		tagged = isTagged
		if n == 0 {
			return authored, nil
		}
		return pdfops.FillFormJSON(authored, data)
	})
	if rc != 0 {
		return rc
	}
	kind := "untagged"
	if tagged {
		kind = "tagged"
	}
	errf("added %d field(s), %d with a default value; the form is %s", len(fields), n, kind)
	return 0
}

// authorDefaults is the fill that gives the authored fields their default values, as pdfcpu's form JSON, and how many
// carry one. Refusals name the field, never the value (ADR-183).
func authorDefaults(fields []authorField) (data []byte, n int, err error) {
	type entry struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	}
	form := map[string][]entry{}
	for i, f := range fields {
		if f.Value == nil {
			continue
		}
		name := f.Name
		if name == "" {
			return nil, 0, fmt.Errorf("field %d has a value and no name", i+1)
		}
		switch f.Kind {
		case "check":
			b, ok := truthyValue(f.Value)
			if !ok {
				return nil, 0, fmt.Errorf("field %q: a check box's value is true or false", name)
			}
			form["checkbox"] = append(form["checkbox"], entry{name, b})
		case "dropdown", "radio":
			s, isStr := f.Value.(string)
			if !isStr || !containsString(f.Options, s) {
				return nil, 0, fmt.Errorf("field %q: its value is not one of its options", name)
			}
			key := "combobox"
			if f.Kind == "radio" {
				key = "radiobuttongroup"
			}
			form[key] = append(form[key], entry{name, s})
		default: // text
			s, isStr := f.Value.(string)
			if !isStr {
				return nil, 0, fmt.Errorf("field %q: a text field's value is a string", name)
			}
			if s == "" {
				continue
			}
			form["textfield"] = append(form["textfield"], entry{name, s})
		}
		n++
	}
	if n == 0 {
		return nil, 0, nil
	}
	data, err = json.Marshal(map[string]any{"forms": []any{form}})
	return data, n, err
}

func truthyValue(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		switch strings.ToLower(x) {
		case "true", "yes", "on", "1":
			return true, true
		case "false", "no", "off", "0", "":
			return false, true
		}
	}
	return false, false
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
