package uacheck

import (
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfops"
)

// **A form field's locator names the field** (`/pending 653`). `Result.Where` is specified to be specific, and
// every field used to answer "form field" (7.2 t25) or "a form field" (7.18.6.2) — in a form of forty fields,
// no help at all. It is the field's fully qualified name: its ancestors' `/T`s and its own, joined by periods.
func TestAFormFieldsLocatorNamesTheField(t *testing.T) {
	base, err := pdfops.AuthorForm(plainDoc(t), []pdfops.FormField{{
		Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "surname", Label: "Surname"}})
	if err != nil {
		t.Fatal(err)
	}
	wheres := func(d *Document) []string {
		var out []string
		for _, s := range d.scanAnnotsAndFields().subjects {
			if s.kind == kindFormField {
				out = append(out, s.where)
			}
		}
		return out
	}
	field := func(d *Document) types.Dict {
		arr, _ := d.Ctx.DereferenceArray(d.dict(d.Catalog["AcroForm"])["Fields"])
		if len(arr) != 1 || d.dict(arr[0]) == nil {
			t.Fatalf("setup: the form has %d top-level field(s), want one", len(arr))
		}
		return d.dict(arr[0])
	}
	plain := openMutated(t, base, func(*Document, types.Dict) {})
	if got := wheres(plain); len(got) != 1 || got[0] != `form field "surname"` {
		t.Errorf("a field named surname is located as %q, want [form field \"surname\"]", got)
	}
	nested := openMutated(t, base, func(d *Document, _ types.Dict) {
		field(d)["Kids"] = types.Array{
			types.Dict{"T": types.StringLiteral("first")},
			types.Dict{"Subtype": types.Name("Widget")}, // a widget: no name of its own
		}
	})
	if got := wheres(nested); len(got) != 3 || got[1] != `form field "surname.first"` || got[2] != `form field "surname"` {
		t.Errorf("a field's kids are located as %q, want the named kid as surname.first and the widget as surname", got)
	}
	unnamed := openMutated(t, base, func(d *Document, _ types.Dict) { delete(field(d), "T") })
	if got := wheres(unnamed); len(got) != 1 || got[0] != "a form field with no name" {
		t.Errorf("a field with no /T is located as %q, want it said to have no name", got)
	}
	// And the same locator reaches the other reader of this walk: a clip found on a field's action.
	clip := openMutated(t, base, func(d *Document, _ types.Dict) {
		field(d)["AA"] = types.Dict{"K": types.Dict{"S": types.Name("Rendition"),
			"R": types.Dict{"C": types.Dict{"S": types.Name("MCD")}}}}
	})
	clips, _ := clip.mediaClips()
	if len(clips) != 1 || clips[0].where != `form field "surname"'s /AA /K` {
		t.Errorf("a clip on the field's /AA /K is located as %+v, want it under form field \"surname\"", clips)
	}
}
