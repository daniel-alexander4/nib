package pdfops

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Authored form fields carry `/TU` and `/Tabs` — `PLAN-accessibility.md` P06.S05.

// fieldTUs returns, per field name (`/T`), the accessible name (`/TU`) the document
// gives it — and the second result says whether the key was present at all, which is
// the distinction this slice turns on: an ABSENT /TU and an empty one are different
// documents, and only one of them is what the code intends.
func fieldTUs(t *testing.T, pdf []byte) map[string]string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	out := map[string]string{}
	var walk func(d types.Dict, inheritedT string)
	walk = func(d types.Dict, inheritedT string) {
		name := inheritedT
		if s, ok := d["T"].(types.StringLiteral); ok {
			if dec, derr := types.StringLiteralToString(s); derr == nil {
				name = dec
			}
		}
		if _, has := d["TU"]; has {
			tu := ""
			if s, ok := d["TU"].(types.StringLiteral); ok {
				tu, _ = types.StringLiteralToString(s)
			}
			out[name] = tu
		} else if name != "" {
			if _, isField := d["FT"]; isField {
				if _, already := out[name]; !already {
					out[name] = "\x00absent"
				}
			}
		}
		kids, _ := ctx.DereferenceArray(d["Kids"])
		for _, k := range kids {
			kd, kerr := ctx.DereferenceDict(k)
			if kerr == nil && kd != nil {
				walk(kd, name)
			}
		}
	}
	root, rerr := ctx.XRefTable.Catalog()
	if rerr != nil {
		t.Fatal(rerr)
	}
	af, aerr := ctx.DereferenceDict(root["AcroForm"])
	if aerr != nil || af == nil {
		t.Fatal("the authored document has no /AcroForm, so there are no fields to ask about")
	}
	fields, ferr := ctx.DereferenceArray(af["Fields"])
	if ferr != nil {
		t.Fatal(ferr)
	}
	for _, f := range fields {
		fd, derr := ctx.DereferenceDict(f)
		if derr == nil && fd != nil {
			walk(fd, "")
		}
	}
	return out
}

// pageTabs returns each page's /Tabs value, "" where the key is absent.
func pageTabs(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	out := make([]string, 0, ctx.PageCount)
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, derr := ctx.PageDict(p, false)
		if derr != nil || d == nil {
			out = append(out, "?")
			continue
		}
		n, _ := d["Tabs"].(types.Name)
		out = append(out, string(n))
	}
	return out
}

// TestEveryNamedFieldAnnouncesTheNameTheUserGave — S05's first acceptance clause, across
// all four kinds, because `withTip` is one door and a door is only one if every site uses it.
func TestEveryNamedFieldAnnouncesTheNameTheUserGave(t *testing.T) {
	base, err := testpdf.Text("a form")
	if err != nil {
		t.Fatal(err)
	}
	out, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "full_name", Label: "Your full name"},
		{Page: 1, Rect: [4]float64{100, 660, 112, 672}, Kind: "check", Name: "agree", Label: "I agree to the terms"},
		{Page: 1, Rect: [4]float64{100, 600, 300, 620}, Kind: "dropdown", Name: "title", Label: "Preferred title", Options: []string{"Dr", "Mx"}},
		{Page: 1, Rect: [4]float64{100, 520, 300, 540}, Kind: "radio", Name: "post", Label: "How to reach you", Options: []string{"Email", "Post"}},
	})
	if err != nil {
		t.Fatalf("AuthorForm: %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("the authored form does not validate: %v", err)
	}
	got := fieldTUs(t, out)
	want := map[string]string{
		"full_name": "Your full name",
		"agree":     "I agree to the terms",
		"title":     "Preferred title",
		"post":      "How to reach you",
	}
	for name, w := range want {
		g, seen := got[name]
		if !seen {
			t.Errorf("field %q is not in the authored document at all — the fixture changed under "+
				"this test and the /TU assertion below would pass by never running", name)
			continue
		}
		if g != w {
			t.Errorf("field %q announces %q, want %q — a screen reader reads /TU, and with none it "+
				"falls back to /T, which is the de-duped identifier and not what anyone typed",
				name, strings.TrimPrefix(g, "\x00"), w)
		}
	}
}

// TestAFieldTheUserDidNotNameAnnouncesNothing — the other half of the same clause, and it is
// asserted rather than left to happen.
//
// `field_3` in a /TU is identical to the /T a reader already falls back to when there is none, so
// writing it announces nothing new while claiming to be a name. That is ADR-031's shape one key
// over, and `TitleFromName` refuses the same thing for the same reason.
func TestAFieldTheUserDidNotNameAnnouncesNothing(t *testing.T) {
	base, err := testpdf.Text("a form")
	if err != nil {
		t.Fatal(err)
	}
	out, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "named", Label: "Your full name"},
		{Page: 1, Rect: [4]float64{100, 660, 300, 680}, Kind: "text", Name: "field_2"},
	})
	if err != nil {
		t.Fatalf("AuthorForm: %v", err)
	}
	got := fieldTUs(t, out)
	// Control: the NAMED field really did get one in this same document, or "absent" below is
	// measuring a build where nothing writes /TU at all.
	if got["named"] != "Your full name" {
		t.Fatalf("control: the named field announces %q — /TU is not being written in this "+
			"document, so the assertion below cannot tell a deliberate omission from a dead feature",
			got["named"])
	}
	if v := got["field_2"]; v != "\x00absent" {
		t.Errorf("the unnamed field announces %q; want no /TU at all. Writing the internal "+
			"identifier as an accessible name is a label that says nothing while claiming to be one",
			v)
	}
}

// TestEveryAnnotatedPageOrdersItsTabsByStructure — S05's second acceptance clause.
//
// The population is every page carrying an annotation, not every page this call placed a field on:
// ua1 7.18.3 is about a PAGE with annotations and does not ask who put them there.
func TestEveryAnnotatedPageOrdersItsTabsByStructure(t *testing.T) {
	base, err := testpdf.Text("one", "two", "three")
	if err != nil {
		t.Fatal(err)
	}
	out, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "a", Label: "A"},
		{Page: 3, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "c", Label: "C"},
	})
	if err != nil {
		t.Fatalf("AuthorForm: %v", err)
	}
	tabs := pageTabs(t, out)
	if len(tabs) != 3 {
		t.Fatalf("the authored document has %d page(s), want 3", len(tabs))
	}
	for _, p := range []int{1, 3} {
		if tabs[p-1] != "S" {
			t.Errorf("page %d carries a widget and its /Tabs is %q, want \"S\" — tab order would "+
				"follow the order widgets happen to sit in /Annots instead of the reading order",
				p, tabs[p-1])
		}
	}
	// Page 2 has no annotation, so the key has no subject there. Asserted because a blanket write
	// would make the loop above pass without ever reading /Annots.
	if tabs[1] != "" {
		t.Errorf("page 2 carries no annotation and got /Tabs %q — the rule is scoped to annotated "+
			"pages, and a key written everywhere is not evidence the scope was read", tabs[1])
	}
}

// TestAnAccessibleNameCostsALanguageClauseWhereTheDocumentDeclaresNone — S05's fourth acceptance
// clause, and the cost of `/pending 471` measured rather than described.
//
// `/TU` is text, and ua1 **7.2 t25** requires that text's language be determinable. Measured: only
// the CATALOG /Lang clears it — a /Lang on the field dictionary itself does nothing. `AuthorForm`
// works on the USER'S document, so writing that catalog key is the guess P03.S02 measured
// LibreOffice making and `/pending 471` parks.
//
// The slice writes /TU anyway. Withholding an accessible name to keep a clause table clean scores
// honesty as a regression, which P01 wrote down when it struck its own acceptance for the same
// reason. This test is what keeps the price visible until 471 is answered.
func TestAnAccessibleNameCostsALanguageClauseWhereTheDocumentDeclaresNone(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so P06.S05's language cost is UNMEASURED.")
	}
	base, err := testpdf.Text("a form")
	if err != nil {
		t.Fatal(err)
	}
	noLang, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "n", Label: "Your full name"},
	})
	if err != nil {
		t.Fatal(err)
	}
	withLang, err := SetLang(noLang, "en")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"nolang.pdf": noLang, "withlang.pdf": withLang} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	cl := ua1FailedClauses(t, vp, files)
	if cl["nolang.pdf"] == nil || cl["withlang.pdf"] == nil {
		t.Fatal("veraPDF could not validate one of the pair, so there is no differential")
	}
	if !cl["nolang.pdf"]["7.2 t25"] {
		t.Errorf("a document with no catalog /Lang does NOT fail 7.2 t25. Either /TU stopped being "+
			"written — which the tests above would catch — or veraPDF changed what it asks. "+
			"It fails %v", sortedClauses(cl["nolang.pdf"]))
	}
	// And the clause is the ONLY cost: a catalog /Lang clears it and takes nothing else with it.
	var stillFailing []string
	for c := range cl["withlang.pdf"] {
		if !cl["nolang.pdf"][c] {
			stillFailing = append(stillFailing, c)
		}
	}
	sort.Strings(stillFailing)
	if cl["withlang.pdf"]["7.2 t25"] {
		t.Errorf("a catalog /Lang did not clear 7.2 t25, so the gate named on `/pending 471` is not "+
			"the thing that would pay this price: %v", sortedClauses(cl["withlang.pdf"]))
	}
	if len(stillFailing) > 0 {
		t.Errorf("declaring a language ADDED %v — the remedy this test names costs more than the "+
			"clause it clears", strings.Join(stillFailing, ", "))
	}
	t.Logf("no /Lang: %v\nwith /Lang: %v", sortedClauses(cl["nolang.pdf"]), sortedClauses(cl["withlang.pdf"]))
}
