package uacheck

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"nib/internal/pdfops"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// What a rule needs from BEFORE pdfcpu's validator — `/pending 656`.
//
// Two rules each re-read the whole file unvalidated, each for one thing the validator removes, and each kept its own
// parse: a document needing both was parsed three times, and the form's re-read was paid on every document with no
// validated AcroForm. The form is now noted by `open` from the parse it already makes (`Document.deletedForm`), which
// leaves one re-read in the package.

// TestTheFileIsReReadUnvalidatedInOnePlace holds that count (ADR-009): a second `api.ReadContext` in this package is a
// second door, and the place to ask is the one that exists — or `open`'s hook, which sees the context before the
// validator does and costs no parse at all.
func TestTheFileIsReReadUnvalidatedInOnePlace(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var sites []string
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, line := range strings.Split(string(src), "\n") {
			code, _, _ := strings.Cut(line, "//")
			if strings.Contains(code, "api.ReadContext(") {
				sites = append(sites, name)
			}
		}
	}
	if checked < 10 {
		t.Fatalf("the guard read %d source file(s) — it is not looking at the package", checked)
	}
	if len(sites) != 1 || sites[0] != "rules_language.go" {
		t.Errorf("the file is re-read unvalidated in %v, want exactly once, in rules_language.go (`scanInlineType3`) — "+
			"a second re-read is a second parse of the whole file for something `open` could have noted", sites)
	}
}

// TestAFormTheValidatorDeletedIsKeptByOpen — the form half. The verdicts are the ones the re-read gave (and veraPDF's
// own `7.15-t01-fail-a.pdf` holds the dynamic one in `TestAnAcroFormPDFCPUDroppedIsStillFound`); what is new is where
// the form comes from.
func TestAFormTheValidatorDeletedIsKeptByOpen(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name    string
		pdf     []byte
		deleted bool
		want    Verdict
	}{
		{"no form at all", base, false, NotApplicable},
		{"an XFA-only form, dynamic", withDynamicXFA(t, base), true, Fail},
		{"an XFA-only form, static", withStaticXFA(t, base), true, Pass},
	} {
		d, err := open(c.pdf)
		if err != nil {
			t.Fatalf("%s: open: %v", c.name, err)
		}
		// The stimulus: the validator still deletes the key, or the row measures the validated path.
		if _, kept := d.Catalog["AcroForm"]; kept {
			t.Fatalf("setup: %s: the validated catalog keeps an /AcroForm", c.name)
		}
		if got := d.deletedForm != nil; got != c.deleted {
			t.Errorf("%s: open kept a deleted form: %v, want %v", c.name, got, c.deleted)
		}
		if got := registry["7.15 t1"].Check(d); got.Verdict != c.want {
			t.Errorf("%s: 7.15 t1 reports %v (%s), want %v", c.name, got.Verdict, got.Why, c.want)
		}
	}
	// A form the validator KEEPS is read from the catalog, and nothing is recorded beside it.
	kept, err := pdfops.AuthorForm(plainDoc(t), []pdfops.FormField{{
		Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "n", Label: "Name"}})
	if err != nil {
		t.Fatal(err)
	}
	d, err := open(kept)
	if err != nil {
		t.Fatal(err)
	}
	if _, has := d.Catalog["AcroForm"]; !has || d.deletedForm != nil {
		t.Errorf("a form with fields: in the catalog %v, recorded as deleted %v — want true, false", has, d.deletedForm != nil)
	}
}

// TestAFlagsPastInt64IsZeroInEveryParse — the entry's second half, and why it is not fixed: it expected an
// unvalidated parse to carry the number the validator had rewritten. Nothing rewrites it; pdfcpu's parser never held
// it. So the divergence `isSymbolic` declares cannot be closed from any pdfcpu context, validated or not.
func TestAFlagsPastInt64IsZeroInEveryParse(t *testing.T) {
	flagsIn := func(ctx *model.Context) types.Object {
		for nr := range ctx.XRefTable.Table {
			o, _ := ctx.Dereference(types.IndirectRef{ObjectNumber: types.Integer(nr)})
			if d, ok := o.(types.Dict); ok {
				if f, has := d["Flags"]; has {
					return f
				}
			}
		}
		return nil
	}
	var docs [][]byte
	for _, flags := range []string{"/Flags 100000000000000000000.0", "/Flags 9223372036854775808"} {
		pdf := ttDoc(flags, "/Encoding /WinAnsiEncoding", "(ABC) Tj", ttProgram(sub31))
		docs = append(docs, pdf)
		raw, err := api.ReadContext(bytes.NewReader(pdf), checkerConfig())
		if err != nil {
			t.Fatal(err)
		}
		d, err := open(pdf)
		if err != nil {
			t.Fatal(err)
		}
		for what, got := range map[string]types.Object{"the unvalidated parse": flagsIn(raw), "the validated one": flagsIn(d.Ctx)} {
			if got != types.Integer(0) {
				t.Errorf("%s: %s holds %T %v, want Integer 0 — pdfcpu now carries the number, so the divergence "+
					"`isSymbolic` declares can be closed", flags, what, got, got)
			}
		}
		// The divergence itself, pinned as declared: veraPDF saturates to a symbolic font and fails the clause.
		if got := verdictOf(t, pdf, "7.21.6 t3"); got.Verdict != Pass {
			t.Errorf("%s: 7.21.6 t3 reports %v (%s) — the declared divergence has moved; re-measure it", flags, got.Verdict, got.Why)
		}
	}
	for i, v := range veraAsk(t, docs) {
		if v["7.21.6 t3"] != "failed" {
			t.Errorf("document %d: veraPDF now says %q for 7.21.6 t3 where \"failed\" was measured — the row is stale", i, v["7.21.6 t3"])
		}
	}
}
