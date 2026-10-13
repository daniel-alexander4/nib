package pdfread_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// xfaDocument is a one-page document whose catalog carries catalogExtra and, where form is not empty, an
// `/AcroForm` written as form; object 5 is an XFA packet and object 6 a text field with its widget on the page.
func xfaDocument(catalogExtra, form string) []byte {
	packet := `<xdp:xdp xmlns:xdp="http://ns.adobe.com/xdp/"><template/></xdp:xdp>`
	acro := ""
	if form != "" {
		acro = " /AcroForm " + form
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R" + acro + " " + catalogExtra + " >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Annots [6 0 R] >>",
		"<< /Length 0 >>\nstream\n\nendstream",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(packet), packet),
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /DA (/Helv 10 Tf 0 g) /Rect [10 10 100 30] /P 3 0 R >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

// The four shapes: an XFA-only form (no fields) and a hybrid one (fields and a packet), each static and dynamic.
const (
	xfaOnlyForm = "<< /XFA 5 0 R >>"
	hybridForm  = "<< /Fields [6 0 R] /XFA 5 0 R >>"
	plainForm   = "<< /Fields [6 0 R] >>"
)

// catalogOf reads pdf WITHOUT validation and returns its catalog and form as written.
func catalogOf(t *testing.T, pdf []byte) (root, form types.Dict) {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err = ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if o, _ := ctx.Dereference(root["AcroForm"]); o != nil {
		form, _ = o.(types.Dict)
	}
	return root, form
}

// TestTheValidatorDeletesAnXFAOnlyFormAndTheReadStillKnowsOfIt is the premise of ADR-145, measured: after a
// validated read the context of an XFA-only document has no `/AcroForm` at all — pdfcpu's validator deleted it —
// while `/NeedsRendering` is still on the catalog; and what the document said is still known, because the door
// read it first.
func TestTheValidatorDeletesAnXFAOnlyFormAndTheReadStillKnowsOfIt(t *testing.T) {
	for _, tc := range []struct {
		name, extra, form string
		formKept          bool
		want              string
	}{
		{"XFA-only, dynamic", "/NeedsRendering true", xfaOnlyForm, false, pdfread.XFADynamic},
		{"XFA-only, static", "", xfaOnlyForm, false, pdfread.XFAStatic},
		{"XFA-only, NeedsRendering false", "/NeedsRendering false", xfaOnlyForm, false, pdfread.XFAStatic},
		{"hybrid, static", "", hybridForm, true, pdfread.XFAStatic},
		{"hybrid, dynamic", "/NeedsRendering true", hybridForm, true, pdfread.XFADynamic},
		{"NeedsRendering true and no packet", "/NeedsRendering true", plainForm, true, pdfread.XFADynamic},
		{"a form with no XFA", "", plainForm, true, ""},
		{"no form", "", "", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pdf := xfaDocument(tc.extra, tc.form)
			if _, form := catalogOf(t, pdf); (form != nil) != (tc.form != "") {
				t.Fatalf("fixture: the file's form is %v", form)
			}
			ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			root, _ := ctx.XRefTable.Catalog()
			if _, kept := root["AcroForm"]; kept != tc.formKept {
				t.Errorf("after validation the catalog holds /AcroForm: %v, want %v", kept, tc.formKept)
			}
			if _, kept := root["NeedsRendering"]; kept != strings.Contains(tc.extra, "NeedsRendering") {
				t.Errorf("after validation the catalog holds /NeedsRendering: %v", kept)
			}
			if got := pdfread.XFAOf(ctx); got != tc.want {
				t.Errorf("XFAOf = %q, want %q", got, tc.want)
			}
			if got, err := pdfread.ReadXFA(pdf); err != nil || got != tc.want {
				t.Errorf("ReadXFA = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}

// TestNoWriteKeepsAnXFAPacketOrNeedsRendering — ADR-145: whatever a document said about XFA, what `Write` produces
// has neither `/XFA` nor `/NeedsRendering`, and a form's fields are still there.
func TestNoWriteKeepsAnXFAPacketOrNeedsRendering(t *testing.T) {
	for _, tc := range []struct {
		name, extra, form string
		fields            bool
	}{
		{"XFA-only, static", "", xfaOnlyForm, false},
		{"XFA-only, NeedsRendering false", "/NeedsRendering false", xfaOnlyForm, false},
		{"hybrid, static", "", hybridForm, true},
		{"hybrid, NeedsRendering false", "/NeedsRendering false", hybridForm, true},
		{"a form with no XFA", "", plainForm, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := pdfread.ReadOptimized(xfaDocument(tc.extra, tc.form), model.NewDefaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			out, err := pdfread.Write(ctx)
			if err != nil {
				t.Fatal(err)
			}
			root, form := catalogOf(t, out)
			if _, has := root["NeedsRendering"]; has {
				t.Errorf("the written catalog still holds /NeedsRendering: %v", root)
			}
			if _, has := form["XFA"]; has {
				t.Errorf("the written form still holds /XFA: %v", form)
			}
			if _, has := form["Fields"]; has != tc.fields {
				t.Errorf("the written form holds /Fields: %v, want %v", has, tc.fields)
			}
			if bytes.Contains(out, []byte("xdp:xdp")) {
				t.Error("the packet's stream was written though nothing names it")
			}
			if got, err := pdfread.ReadXFA(out); err != nil || got != "" {
				t.Errorf("the written document reads as XFA %q, %v", got, err)
			}
		})
	}
}

// TestADocumentWhosePagesAreAPlaceholderIsNotWritten — ADR-145: `/NeedsRendering true` refuses the write with its
// cause, and still does after an operation has taken the key off the catalog (a page selection rebuilds the
// catalog from an allow-list) or the validator has taken the form.
func TestADocumentWhosePagesAreAPlaceholderIsNotWritten(t *testing.T) {
	for _, form := range []string{xfaOnlyForm, hybridForm, plainForm} {
		ctx, err := pdfread.ReadOptimized(xfaDocument("/NeedsRendering true", form), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pdfread.Write(ctx); !errors.Is(err, pdfread.ErrDynamicXFA) {
			t.Errorf("form %s: Write = %v, want ErrDynamicXFA", form, err)
		}
		root, _ := ctx.XRefTable.Catalog()
		delete(root, "NeedsRendering")
		delete(root, "AcroForm")
		if _, err := pdfread.Write(ctx); !errors.Is(err, pdfread.ErrDynamicXFA) {
			t.Errorf("form %s, the key taken off the catalog: Write = %v, want ErrDynamicXFA", form, err)
		}
		if err := pdfread.RefuseDynamicXFA(ctx); !errors.Is(err, pdfread.ErrDynamicXFA) {
			t.Errorf("form %s: RefuseDynamicXFA = %v", form, err)
		}
	}
	// A context nothing was remembered for is judged by its catalog as it stands.
	ctx, err := pdfread.ReadOptimized(xfaDocument("", plainForm), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, _ := ctx.XRefTable.Catalog()
	root["NeedsRendering"] = types.Boolean(true)
	if _, err := pdfread.Write(ctx); !errors.Is(err, pdfread.ErrDynamicXFA) {
		t.Errorf("a catalog given /NeedsRendering true: Write = %v, want ErrDynamicXFA", err)
	}
	// A merge of a placeholder into another document is refused at the source.
	if _, err := pdfread.MergeRaw([][]byte{xfaDocument("", plainForm), xfaDocument("/NeedsRendering true", xfaOnlyForm)}); !errors.Is(err, pdfread.ErrDynamicXFA) {
		t.Errorf("MergeRaw with a dynamic source = %v, want ErrDynamicXFA", err)
	}
}
