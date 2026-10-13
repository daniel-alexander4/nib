package pdfread_test

import (
	"bytes"
	"compress/zlib"
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

// xfaFileOf assembles a one-page document from the objects after the page (object 4 onward), with catalog as
// object 1's body.
func xfaFileOf(catalog string, rest ...string) []byte {
	objs := append([]string{
		catalog,
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
	}, rest...)
	var b bytes.Buffer
	b.WriteString("%PDF-1.6\n")
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

func plainStream(body string) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(body), body)
}

func flateStream(body string) string {
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write([]byte(body))
	w.Close()
	return fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", z.Len(), z.String())
}

// TestAFormThatSaysRenderMeDynamicallyInItsPacketIsDynamic — `/pending 876`. `/NeedsRendering` came with PDF 1.7;
// a dynamic form saved for an older reader says it is one only inside the XFA configuration
// (`<dynamicRender>required</dynamicRender>`). Read as static it was shown as its placeholder page, could be
// changed, and was written back without the form. It is read as dynamic, and refused at the write, in every
// shape the packet takes; a form that says `forbidden`, or nothing, is static as before.
func TestAFormThatSaysRenderMeDynamicallyInItsPacketIsDynamic(t *testing.T) {
	config := func(v string) string {
		return `<config xmlns="http://www.xfa.org/schema/xci/2.6/"><acrobat><acrobat7><dynamicRender>` + v + `</dynamicRender></acrobat7></acrobat></config>`
	}
	template := `<template xmlns="http://www.xfa.org/schema/xfa-template/2.5/"/>`
	whole := func(v string) string {
		return `<xdp:xdp xmlns:xdp="http://ns.adobe.com/xdp/">` + config(v) + template + `</xdp:xdp>`
	}
	const oneStream = "<< /Type /Catalog /Pages 2 0 R /AcroForm << /XFA 4 0 R >> >>"
	const packets = "<< /Type /Catalog /Pages 2 0 R /AcroForm << /XFA [(preamble) 4 0 R (config) 5 0 R (template) 6 0 R (postamble) 7 0 R] >> >>"
	for _, c := range []struct {
		name string
		pdf  []byte
		want string
	}{
		{"one plain stream, required", xfaFileOf(oneStream, plainStream(whole("required"))), pdfread.XFADynamic},
		{"one compressed stream, required", xfaFileOf(oneStream, flateStream(whole("required"))), pdfread.XFADynamic},
		{"packets, the configuration compressed, required", xfaFileOf(packets, plainStream(`<xdp:xdp xmlns:xdp="http://ns.adobe.com/xdp/">`),
			flateStream(config("required")), flateStream(template), plainStream(`</xdp:xdp>`)), pdfread.XFADynamic},
		{"required, spaced as a formatter leaves it", xfaFileOf(oneStream, plainStream(whole("\n      required\n    "))), pdfread.XFADynamic},
		// As six real forms on the build machine write it (five USPTO, one of veraPDF's corpus): a line break
		// inside each tag.
		{"required, as Adobe's serializer writes it", xfaFileOf(oneStream, flateStream(
			strings.Replace(whole("required"), "<dynamicRender>required</dynamicRender>", "<dynamicRender\n>required</dynamicRender\n>", 1))), pdfread.XFADynamic},
		{"forbidden", xfaFileOf(oneStream, flateStream(whole("forbidden"))), pdfread.XFAStatic},
		{"packets, forbidden", xfaFileOf(packets, plainStream(`<xdp:xdp xmlns:xdp="http://ns.adobe.com/xdp/">`),
			flateStream(config("forbidden")), flateStream(template), plainStream(`</xdp:xdp>`)), pdfread.XFAStatic},
		{"no configuration", xfaFileOf(oneStream, plainStream(`<xdp:xdp xmlns:xdp="http://ns.adobe.com/xdp/">`+template+`</xdp:xdp>`)), pdfread.XFAStatic},
		{"a stream that does not decode says nothing", xfaFileOf(oneStream, "<< /Length 9 /Filter /FlateDecode >>\nstream\nnot zlib!\nendstream"), pdfread.XFAStatic},
	} {
		got, err := pdfread.ReadXFA(c.pdf)
		if err != nil || got != c.want {
			t.Errorf("%s: read as %q (%v), want %q", c.name, got, err, c.want)
			continue
		}
		ctx, err := pdfread.ReadOptimized(c.pdf, model.NewDefaultConfiguration())
		if err != nil {
			if c.want == pdfread.XFAStatic {
				continue // the undecodable stream is the validator's to refuse; the reading above is the subject
			}
			t.Errorf("%s: the document does not read: %v", c.name, err)
			continue
		}
		if pdfread.XFAOf(ctx) != c.want {
			t.Errorf("%s: the validated read remembers %q, want %q", c.name, pdfread.XFAOf(ctx), c.want)
		}
		_, werr := pdfread.Write(ctx)
		if refused := errors.Is(werr, pdfread.ErrDynamicXFA); refused != (c.want == pdfread.XFADynamic) {
			t.Errorf("%s: Write = %v", c.name, werr)
		}
	}
}
