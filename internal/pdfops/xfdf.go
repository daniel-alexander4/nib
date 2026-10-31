package pdfops

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"nib/internal/pdfread"
	"sort"
	"strings"

	"golang.org/x/text/encoding/ianaindex"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"
)

// XFDF (XML Forms Data Format, ISO 19444-1) is the form-data interchange format
// Acrobat and Foxit read and write. pdfcpu has no FDF/XFDF support, so this is a
// hand-rolled (de)serializer over encoding/xml — which gives us the lexer,
// escaping, Unicode, and (crucially) XXE/entity-expansion safety for free. We
// implement XFDF only, not legacy FDF (which is PDF/COS syntax and would need a
// whole object lexer).

const (
	xfdfNS = "http://ns.adobe.com/xfdf/"
	// maxXFDFBytes caps a parsed XFDF document; form data is tiny, so a generous
	// limit is plenty and bounds memory on untrusted input.
	maxXFDFBytes = 10 << 20 // 10 MiB
)

// xfdfDoc is the root <xfdf> element. XMLName carries no namespace so import is
// liberal (matches by local name, accepting documents from any writer); on export
// the namespace is emitted via the Xmlns attribute to stay spec-correct.
type xfdfDoc struct {
	XMLName xml.Name    `xml:"xfdf"`
	Xmlns   string      `xml:"xmlns,attr,omitempty"`
	Fields  []xfdfField `xml:"fields>field"`
}

// xfdfField is one <field>: a leaf carries <value> element(s); an intermediate
// node carries nested <field> children. name is the partial (single-level) field
// name — the fully-qualified name is the ancestor names joined with ".".
type xfdfField struct {
	Name   string      `xml:"name,attr"`
	Values []string    `xml:"value"`
	Fields []xfdfField `xml:"field"`
}

// ExportFormXFDF returns pdf's form field data as an XFDF document. It reuses
// pdfcpu's JSON export (the single source of truth) and reshapes it; a
// hierarchical field name (a.b.c) becomes nested <field> elements, as the spec
// prescribes for portability across readers.
func ExportFormXFDF(pdf []byte) ([]byte, error) {
	jsonData, err := ExportFormJSON(pdf)
	if err != nil {
		return nil, err
	}
	var fg form.FormGroup
	if err := json.Unmarshal(jsonData, &fg); err != nil {
		return nil, err
	}

	states, err := CheckStates(pdf) // which on-state a ticked box holds (checkstate.go)
	if err != nil {
		return nil, err
	}
	var root xfdfField // virtual root; its children are the top-level fields
	add := func(name string, vals []string) {
		if name != "" {
			insertField(&root, strings.Split(name, "."), vals)
		}
	}
	for _, f := range fg.Forms {
		for _, x := range f.TextFields {
			add(x.Name, []string{x.Value})
		}
		for _, x := range f.DateFields {
			add(x.Name, []string{x.Value})
		}
		for _, x := range f.CheckBoxes {
			add(x.Name, []string{xfdfCheckboxValue(x.Value, answerOf(states, x.Name))})
		}
		for _, x := range f.RadioButtonGroups {
			add(x.Name, []string{x.Value})
		}
		for _, x := range f.ComboBoxes {
			add(x.Name, []string{x.Value})
		}
		for _, x := range f.ListBoxes {
			add(x.Name, x.Values)
		}
	}

	body, err := xml.MarshalIndent(xfdfDoc{Xmlns: xfdfNS, Fields: root.Fields}, "", "  ")
	if err != nil {
		return nil, err
	}
	out := append([]byte(xml.Header), body...)
	return append(out, '\n'), nil
}

// FillFormXFDF fills pdf's AcroForm from an XFDF document and returns the filled
// PDF. Values are matched by fully-qualified field name (nested and flat dotted
// names are both accepted). Filling removes any existing signature, since the
// content changes.
func FillFormXFDF(pdf, data []byte) ([]byte, error) {
	values, err := parseXFDF(data)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("the XFDF has no field values")
	}
	skeleton, err := pdfread.ExportForm(pdf, "nib", setPushButtonsAside)
	if err != nil {
		return nil, noFormFields(err)
	}
	if skeleton == nil || len(skeleton.Forms) == 0 {
		return nil, ErrNoFormFields
	}
	skelJSON, err := json.Marshal(skeleton)
	if err != nil {
		return nil, err
	}
	return fillFromValues(pdf, skelJSON, values, xfdfChecked)
}

// xfdfCharset decodes an XFDF that declares an encoding other than UTF-8.
//
// **Without one, `encoding/xml` refuses the document outright** — "xml: encoding declared but
// Decoder.CharsetReader is nil" — which Nib then wrapped as "invalid XFDF": the same sentence a
// corrupted or non-XFDF file gets. The user was told their file is not an XFDF when the truth was
// that Nib did not read that encoding (/pending 266). XFDF is an interchange format whose whole
// job is arriving from another vendor's product, so the encoding it arrives in is not Nib's to
// dictate.
//
// An encoding nothing can decode still fails, but by NAME. "this XFDF declares an encoding Nib
// cannot read (x-mac-farsi)" tells the user what to do; "invalid XFDF" tells them to give up.
func xfdfCharset(label string, input io.Reader) (io.Reader, error) {
	enc, err := ianaindex.IANA.Encoding(label)
	if err != nil || enc == nil {
		return nil, fmt.Errorf("this XFDF declares an encoding Nib cannot read (%s)", label)
	}
	return enc.NewDecoder().Reader(input), nil
}

// XFDFFieldNames is the fully-qualified field names an XFDF document gives a value to, sorted — what a fill
// from it was asked to set. An unreadable document names none.
func XFDFFieldNames(data []byte) []string {
	values, err := parseXFDF(data)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// parseXFDF reads an XFDF document into a flat map of fully-qualified field name →
// value(s). encoding/xml resolves no external entities and does no recursive
// entity expansion, so XFDF is safe to parse; we additionally cap the input size.
func parseXFDF(data []byte) (map[string][]string, error) {
	if len(data) > maxXFDFBytes {
		return nil, fmt.Errorf("XFDF too large")
	}
	var doc xfdfDoc
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = xfdfCharset
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("invalid XFDF: %w", err)
	}
	values := map[string][]string{}
	budget := maxXFDFBytes
	for i := range doc.Fields {
		if !flattenField(&doc.Fields[i], "", values, &budget) {
			return nil, fmt.Errorf("this XFDF's field names are too long once each nested name is joined to the " +
				"names above it — more than 10 MiB in all, where Nib stops")
		}
	}
	return values, nil
}

// insertField places vals at the dotted path under parent, creating intermediate
// <field> nodes as needed. Single-level (flat) names create one node, so a form
// with no hierarchy round-trips unchanged.
func insertField(parent *xfdfField, path []string, vals []string) {
	name := path[0]
	var node *xfdfField
	for i := range parent.Fields {
		if parent.Fields[i].Name == name {
			node = &parent.Fields[i]
			break
		}
	}
	if node == nil {
		parent.Fields = append(parent.Fields, xfdfField{Name: name})
		node = &parent.Fields[len(parent.Fields)-1]
	}
	if len(path) == 1 {
		node.Values = vals
		return
	}
	insertField(node, path[1:], vals)
}

// flattenField walks a <field> subtree, joining ancestor names with "." to rebuild
// fully-qualified names. A name that is itself dotted needs no splitting here —
// joining yields the same key either way, so flat-dotted documents are accepted.
//
// **Every joined name is charged to `budget`, and false is the budget spent** (/pending 769). A field's key repeats
// every name above it, so what the keys weigh is the names times everything beneath them, not the size of the
// file. Measured inside the 10 MiB input cap: 300 nested fields held 1.5 GiB of keys and 1,000 held 5 GiB while
// the walk stood at its deepest, and it needs no depth at all — one field with a 1 MiB name over 3,000 children
// held 3 GiB from a 1.1 MiB file. The budget is the input cap itself: a document with no nesting spends less than
// its own length, so none that was read before is refused, and a nested one may weigh what a flat one may.
func flattenField(f *xfdfField, prefix string, out map[string][]string, budget *int) bool {
	need := len(f.Name)
	if prefix != "" {
		need += len(prefix) + 1
	}
	if *budget -= need; *budget < 0 {
		return false
	}
	name := f.Name
	if prefix != "" {
		name = prefix + "." + f.Name
	}
	// A field's OWN values are emitted whether or not it also has children. The early
	// return this replaces dropped them silently: <field name="a"><value>x</value>
	// <field name="b">…</field></field> lost "x" entirely, and XFDF permits exactly that
	// — a parent field with a value of its own and qualified children beneath it.
	if len(f.Values) > 0 {
		out[name] = f.Values
	}
	// Nested a→b builds the same key as a flat "a.b", so a document containing both is
	// ambiguous. LAST occurrence wins, which is what this has always done and what most
	// parsers do with a repeated key; stated here so it is a decision rather than an
	// accident of document order.
	for i := range f.Fields {
		if !flattenField(&f.Fields[i], name, out, budget) {
			return false
		}
	}
	return true
}

// xfdfCheckboxValue maps a checkbox's boolean state to its XFDF value — the export-value name of the on-state it
// holds (answer), as Acrobat writes it, or "Off". On import xfdfChecked reads any name but Off as checked, and a
// field with several on-states reads the name itself, so the round-trip is exact.
func xfdfCheckboxValue(on bool, answer string) string {
	if on {
		return answer
	}
	return "Off"
}
